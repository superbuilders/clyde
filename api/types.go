package api

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

// Request represents a Claude API request
type Request struct {
	Model        string        `json:"model"`
	MaxTokens    int           `json:"max_tokens"`
	CacheControl *CacheControl `json:"cache_control,omitempty"`
	System       string        `json:"system"`
	Messages     []Message     `json:"messages"`
	Tools        []Tool        `json:"tools,omitempty"`
}

// ImageSource represents the source of an image in a content block
type ImageSource struct {
	Type      string `json:"type"`                // "base64" or "url"
	MediaType string `json:"media_type"`          // "image/jpeg", "image/png", "image/webp", "image/gif"
	Data      string `json:"data,omitempty"`      // Base64 data (for type="base64")
	URL       string `json:"url,omitempty"`       // URL (for type="url")
}

// ContentBlock represents a block of content in a Claude response
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
