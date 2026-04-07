package api

import "encoding/json"

// Message represents a single message in the conversation
type Message struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"`
}

// Tool represents a Claude API tool definition
type Tool struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema interface{} `json:"input_schema"`
}

// CacheControl represents prompt caching control
type CacheControl struct {
	Type string `json:"type"` // "ephemeral"
}

// ThinkingConfig configures extended/adaptive thinking for the Claude API.
//
// For Claude Opus 4.6 and Sonnet 4.6, use adaptive thinking:
//
//	{Type: "adaptive"}
//
// For older models, use manual thinking with a budget:
//
//	{Type: "enabled", BudgetTokens: 8192}
type ThinkingConfig struct {
	Type         string `json:"type"`                    // "enabled" or "adaptive"
	BudgetTokens int    `json:"budget_tokens,omitempty"` // Required for type="enabled", ignored for "adaptive"
}

// Request represents a Claude API request
type Request struct {
	Model        string         `json:"model"`
	MaxTokens    int            `json:"max_tokens"`
	CacheControl *CacheControl  `json:"cache_control,omitempty"`
	System       string         `json:"system"`
	Messages     []Message      `json:"messages"`
	Tools        []Tool         `json:"tools,omitempty"`
	Thinking     *ThinkingConfig `json:"thinking,omitempty"`
}

// ImageSource represents the source of an image in a content block
type ImageSource struct {
	Type      string `json:"type"`                // "base64" or "url"
	MediaType string `json:"media_type"`          // "image/jpeg", "image/png", "image/webp", "image/gif"
	Data      string `json:"data,omitempty"`      // Base64 data (for type="base64")
	URL       string `json:"url,omitempty"`       // URL (for type="url")
}

// ContentBlock represents a block of content in a Claude response.
//
// Block types:
//   - "text":              Text content (Text field populated)
//   - "thinking":          Thinking trace (Thinking + Signature fields populated)
//   - "redacted_thinking": Redacted thinking (Data field populated)
//   - "tool_use":          Tool call (ID, Name, Input fields populated)
//   - "tool_result":       Tool result (ToolUseID, Content fields populated)
//   - "image":             Image content (Source field populated)
type ContentBlock struct {
	Type      string                 `json:"type"`
	Text      string                 `json:"text,omitempty"`
	ID        string                 `json:"id,omitempty"`
	Name      string                 `json:"name,omitempty"`
	Input     map[string]interface{} `json:"input,omitempty"`
	Content   interface{}            `json:"content,omitempty"`
	ToolUseID string                 `json:"tool_use_id,omitempty"`
	IsError   bool                   `json:"is_error,omitempty"`
	Source    *ImageSource           `json:"source,omitempty"`  // For type="image"

	// Thinking block fields
	Thinking  string `json:"thinking,omitempty"`  // Thinking trace text (type="thinking")
	Signature string `json:"signature,omitempty"` // Signature for verification (type="thinking")
	Data      string `json:"data,omitempty"`      // Encrypted data (type="redacted_thinking")
}

// MarshalJSON implements custom JSON marshaling for ContentBlock.
// For tool_use blocks, the "input" field is always included (even when empty),
// because the Claude API requires it. For other block types, input is omitted
// when nil/empty (standard omitempty behavior).
func (b ContentBlock) MarshalJSON() ([]byte, error) {
	// Use an alias to avoid infinite recursion
	type Alias ContentBlock
	if b.Type == "tool_use" {
		// For tool_use: ensure input is always present
		inputVal := b.Input
		if inputVal == nil {
			inputVal = map[string]interface{}{}
		}
		return json.Marshal(&struct {
			Alias
			Input map[string]interface{} `json:"input"` // no omitempty
		}{
			Alias: Alias(b),
			Input: inputVal,
		})
	}
	// For all other types: use default serialization (with omitempty on input)
	return json.Marshal(&struct {
		Alias
	}{
		Alias: Alias(b),
	})
}

// Usage represents token usage information in a response
type Usage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
}

// Response represents a Claude API response
type Response struct {
	ID         string         `json:"id"`
	Type       string         `json:"type"`
	Role       string         `json:"role"`
	Content    []ContentBlock `json:"content"`
	Model      string         `json:"model"`
	StopReason string         `json:"stop_reason"`
	Usage      Usage          `json:"usage"`
}
