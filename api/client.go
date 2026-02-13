package api

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
	apiKey    string
	apiURL    string
	modelID   string
	maxTokens int
}

// NewClient creates a new Claude API client
func NewClient(apiKey, apiURL, modelID string, maxTokens int) *Client {
	return &Client{
		apiKey:    apiKey,
		apiURL:    apiURL,
		modelID:   modelID,
		maxTokens: maxTokens,
	}
}

// Call sends a request to the Claude API with the given messages and tools
func (c *Client) Call(systemPrompt string, messages []Message, tools []Tool) (*Response, error) {
	reqBody := Request{
		Model:     c.modelID,
		MaxTokens: c.maxTokens,
		System:    systemPrompt,
		Messages:  messages,
		Tools:     tools,
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
