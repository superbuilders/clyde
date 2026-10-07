package providers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Retry defaults for transient upstream failures (502 and friends).
//
// A 502/503/504/529 from the Anthropic edge is an upstream hiccup, not a
// request defect: the same payload almost always succeeds moments later.
// Failing the whole turn on one of those throws away the entire conversation
// state, so the client retries a bounded number of times with a fixed delay.
const (
	// DefaultMaxAttempts is the total number of attempts (1 initial + retries).
	DefaultMaxAttempts = 5
	// DefaultRetryDelay is the fixed wait between attempts.
	DefaultRetryDelay = 5 * time.Second
	// MaxRetryAfterDelay caps how long a server-provided Retry-After can stall
	// the turn. Without a cap a hostile or buggy header could hang the CLI.
	MaxRetryAfterDelay = 60 * time.Second
)

// RetryNotifier is invoked before each retry sleep so the caller can surface a
// visible notice explaining why the turn is stalled.
//
// attempt is the 1-based number of the attempt that just failed, maxAttempts
// is the total budget, status is the HTTP status (0 for transport errors), and
// delay is how long the client is about to wait.
type RetryNotifier func(attempt, maxAttempts, status int, delay time.Duration)

// Client handles communication with the Claude API
type Client struct {
	apiKey        string
	apiURL        string
	modelID       string
	maxTokens     int
	thinking      *ThinkingConfig
	outputConfig  *OutputConfig
	toolChoice    *ToolChoice
	maxAttempts   int
	retryDelay    time.Duration
	retryNotifier RetryNotifier
	sleep         func(time.Duration)
}

// IsRetryableStatus reports whether an HTTP status from the API is a transient
// failure that is safe to retry with the identical payload.
//
// Only 408/429 and the transient 5xx family qualify. Every other 4xx is a
// defect in the request itself — retrying it just burns the budget and delays
// the real error.
func IsRetryableStatus(status int) bool {
	switch status {
	case http.StatusRequestTimeout, // 408
		http.StatusTooManyRequests,     // 429
		http.StatusInternalServerError, // 500
		http.StatusBadGateway,          // 502
		http.StatusServiceUnavailable,  // 503
		http.StatusGatewayTimeout,      // 504
		529:                            // Anthropic "overloaded"
		return true
	}
	return false
}

// parseRetryAfter interprets a Retry-After header in either of its legal forms
// (delta-seconds or an HTTP-date) and returns the delay plus whether it parsed.
func parseRetryAfter(value string, now time.Time) (time.Duration, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	if secs, err := strconv.ParseFloat(value, 64); err == nil {
		if secs < 0 {
			return 0, false
		}
		return time.Duration(secs * float64(time.Second)), true
	}
	if t, err := http.ParseTime(value); err == nil {
		d := t.Sub(now)
		if d < 0 {
			d = 0
		}
		return d, true
	}
	return 0, false
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
		maxAttempts:  DefaultMaxAttempts,
		retryDelay:   DefaultRetryDelay,
		sleep:        time.Sleep,
	}
}

// WithRetryPolicy returns a new client with the given retry budget and fixed
// delay. Non-positive values leave the corresponding default in place.
func (c *Client) WithRetryPolicy(maxAttempts int, delay time.Duration) *Client {
	cp := c.clone()
	if maxAttempts > 0 {
		cp.maxAttempts = maxAttempts
	}
	if delay > 0 {
		cp.retryDelay = delay
	}
	return cp
}

// WithRetryNotifier returns a new client that reports each retry to cb.
func (c *Client) WithRetryNotifier(cb RetryNotifier) *Client {
	cp := c.clone()
	cp.retryNotifier = cb
	return cp
}

// MaxAttempts exposes the retry budget (for diagnostics/tests).
func (c *Client) MaxAttempts() int { return c.maxAttempts }

// RetryDelay exposes the fixed retry delay (for diagnostics/tests).
func (c *Client) RetryDelay() time.Duration { return c.retryDelay }

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

	attempts := c.maxAttempts
	if attempts < 1 {
		attempts = 1
	}
	sleep := c.sleep
	if sleep == nil {
		sleep = time.Sleep
	}

	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		resp, status, retryAfter, err := c.doAttempt(jsonData)
		if err == nil {
			return resp, nil
		}
		lastErr = err

		// Only the transient status family is worth repeating: a transport
		// failure (status 0) usually means a dead endpoint or no network, and
		// every other status is a defect in the request itself.
		if !IsRetryableStatus(status) || attempt == attempts {
			break
		}

		delay := c.retryDelay
		if retryAfter > 0 {
			delay = retryAfter
			if delay > MaxRetryAfterDelay {
				delay = MaxRetryAfterDelay
			}
		}
		if c.retryNotifier != nil {
			c.retryNotifier(attempt, attempts, status, delay)
		}
		sleep(delay)
	}

	return nil, lastErr
}

// doAttempt performs exactly one HTTP round trip. It returns the parsed
// response on success, or the HTTP status (0 for transport failures), any
// server-requested Retry-After delay, and the error.
func (c *Client) doAttempt(jsonData []byte) (*Response, int, time.Duration, error) {
	req, err := http.NewRequest("POST", c.apiURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, 0, 0, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("failed to send request to Claude API: %w\nCheck your internet connection", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, 0, fmt.Errorf("failed to read response: %w", err)
	}

	retryAfter, _ := parseRetryAfter(resp.Header.Get("Retry-After"), time.Now())

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

		return nil, resp.StatusCode, retryAfter, fmt.Errorf("%s", strings.Join(suggestions, "\n"))
	}

	var apiResp Response
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, resp.StatusCode, 0, fmt.Errorf("failed to unmarshal response: %w\nResponse body: %s", err, string(body))
	}

	return &apiResp, resp.StatusCode, 0, nil
}
