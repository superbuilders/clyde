package providers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Client handles communication with the Claude API
type Client struct {
	apiKey       string
	apiURL       string
	modelID      string
	maxTokens    int
	thinking     *ThinkingConfig
	outputConfig *OutputConfig
	toolChoice   *ToolChoice
}

// NewClient creates a new Claude API client with all model-behavior knobs
// pinned to the harness defaults (see the "Pinned request defaults" block in
// types.go). Callers may override individual pins with the With* methods.
func NewClient(apiKey, apiURL, modelID string, maxTokens int) *Client {
	return &Client{
		apiKey:    apiKey,
		apiURL:    apiURL,
		modelID:   modelID,
		maxTokens: maxTokens,
		// Pin thinking on by default, with visible reasoning — matches Opus 4.6.
		thinking: &ThinkingConfig{
			Type:    DefaultThinkingType,
			Display: DefaultThinkingDisplay,
		},
		outputConfig: &OutputConfig{Effort: DefaultEffort},
		toolChoice:   &ToolChoice{Type: DefaultToolChoice},
	}
}

// clone returns a shallow copy so the With* methods stay immutable.
func (c *Client) clone() *Client {
	cp := *c
	return &cp
}

// WithThinking returns a new client with the given thinking configuration.
//
// Passing nil disables thinking by explicitly sending {type: "disabled"},
// rather than omitting the field. Omitting it is NOT equivalent: Opus 5 and
// Sonnet 5 think by default, so an absent thinking field leaves thinking on.
func (c *Client) WithThinking(thinking *ThinkingConfig) *Client {
	cp := c.clone()
	if thinking == nil {
		cp.thinking = &ThinkingConfig{Type: ThinkingTypeDisabled}
		return cp
	}
	// Default the display mode if the caller did not pin one, so thinking text
	// is never silently dropped.
	t := *thinking
	if t.Display == "" && t.Type != ThinkingTypeDisabled {
		t.Display = DefaultThinkingDisplay
	}
	cp.thinking = &t
	return cp
}

// WithEffort returns a new client using the given reasoning effort level.
// An empty or invalid value leaves the pinned default in place.
func (c *Client) WithEffort(effort string) *Client {
	cp := c.clone()
	if IsValidEffort(effort) {
		cp.outputConfig = &OutputConfig{Effort: effort}
	}
	return cp
}

// WithToolChoice returns a new client with the given tool choice policy.
func (c *Client) WithToolChoice(tc *ToolChoice) *Client {
	cp := c.clone()
	cp.toolChoice = tc
	return cp
}

// Thinking exposes the pinned thinking configuration (for diagnostics/tests).
func (c *Client) Thinking() *ThinkingConfig { return c.thinking }

// Effort exposes the pinned reasoning effort (for diagnostics/tests).
func (c *Client) Effort() string {
	if c.outputConfig == nil {
		return ""
	}
	return c.outputConfig.Effort
}

// Call sends a request to the Claude API with the given messages and tools
func (c *Client) Call(systemPrompt string, messages []Message, tools []Tool) (*Response, error) {
	reqBody := Request{
		Model:        c.modelID,
		MaxTokens:    c.maxTokens,
		CacheControl: &CacheControl{Type: "ephemeral"}, // Enable automatic prompt caching
		System:       systemPrompt,
		Messages:     messages,
		Tools:        tools,
		Thinking:     c.thinking,
		OutputConfig: c.outputConfig,
	}

	// tool_choice is only meaningful when tools are present; sending it with an
	// empty tool list is rejected by some routes.
	if len(tools) > 0 {
		reqBody.ToolChoice = c.toolChoice
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", c.apiURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request to Claude API: %w\nCheck your internet connection", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		// Try to parse error response for better messages
		var errorResp struct {
			Error struct {
				Type    string `json:"type"`
				Message string `json:"message"`
			} `json:"error"`
		}

		suggestions := []string{
			fmt.Sprintf("API error (status %d)", resp.StatusCode),
		}

		if json.Unmarshal(body, &errorResp) == nil && errorResp.Error.Message != "" {
			suggestions = append(suggestions, fmt.Sprintf("Error: %s", errorResp.Error.Message))
		} else {
			suggestions = append(suggestions, fmt.Sprintf("Response: %s", string(body)))
		}

		// Add context-specific help
		switch resp.StatusCode {
		case 401:
			suggestions = append(suggestions,
				"",
				"Authentication failed. Check your API key:",
				"  - Verify TS_AGENT_API_KEY in .env file",
				"  - Ensure the key starts with 'sk-ant-'",
				"  - Try generating a new key at https://console.anthropic.com/",
			)
		case 429:
			suggestions = append(suggestions,
				"",
				"Rate limit exceeded. Suggestions:",
				"  - Wait a moment and try again",
				"  - You may have hit your usage limit",
				"  - Check your plan limits at https://console.anthropic.com/",
			)
		case 400:
			suggestions = append(suggestions,
				"",
				"Bad request. This may indicate:",
				"  - Invalid tool parameters",
				"  - Message format issues",
				"  - Try a simpler request to test",
			)
			// The tool_use/tool_result pairing family is recoverable: the
			// on-disk session replays cleanly, only the in-memory history is
			// malformed. Say so, or the user assumes the session is dead.
			if containsToolPairingMarker(string(body)) {
				suggestions = append(suggestions, ToolPairingRecoveryHint)
			}
		case 500, 502, 503, 504:
			suggestions = append(suggestions,
				"",
				"Claude API server error. Suggestions:",
				"  - This is temporary, try again in a moment",
				"  - Check https://status.anthropic.com/ for service status",
			)
		}

		return nil, fmt.Errorf("%s", strings.Join(suggestions, "\n"))
	}

	var apiResp Response
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w\nResponse body: %s", err, string(body))
	}

	return &apiResp, nil
}
