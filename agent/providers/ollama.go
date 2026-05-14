package providers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// OllamaClient handles communication with an Ollama server.
// It translates between Claude-canonical types (used by the agent) and
// Ollama's /api/chat wire format.
type OllamaClient struct {
	baseURL string // e.g. "http://localhost:11434"
	modelID string // e.g. "qwen3.5:35b"
	think   bool   // enable thinking mode
}

// NewOllamaClient creates a new Ollama API client.
func NewOllamaClient(baseURL, modelID string, think bool) *OllamaClient {
	return &OllamaClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		modelID: modelID,
		think:   think,
	}
}

// --- Ollama wire-format types (private) ---

type ollamaRequest struct {
	Model    string           `json:"model"`
	Messages []ollamaMessage  `json:"messages"`
	Tools    []ollamaTool     `json:"tools,omitempty"`
	Stream   bool             `json:"stream"`
	Think    bool             `json:"think,omitempty"`
}

type ollamaMessage struct {
	Role      string           `json:"role"`
	Content   string           `json:"content"`
	ToolCalls []ollamaToolCall `json:"tool_calls,omitempty"`
	// Ollama uses "images" for multimodal (not implemented in MVP).
}

type ollamaToolCall struct {
	Function ollamaFunctionCall `json:"function"`
}

type ollamaFunctionCall struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

type ollamaTool struct {
	Type     string         `json:"type"` // "function"
	Function ollamaFunction `json:"function"`
}

type ollamaFunction struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Parameters  interface{} `json:"parameters"`
}

type ollamaResponse struct {
	Model           string        `json:"model"`
	Message         ollamaRespMsg `json:"message"`
	Done            bool          `json:"done"`
	DoneReason      string        `json:"done_reason"`
	PromptEvalCount int           `json:"prompt_eval_count"`
	EvalCount       int           `json:"eval_count"`
}

type ollamaRespMsg struct {
	Role      string           `json:"role"`
	Content   string           `json:"content"`
	Thinking  string           `json:"thinking,omitempty"`
	ToolCalls []ollamaToolCall `json:"tool_calls,omitempty"`
}

// Call sends a request to the Ollama API, translating between Claude-canonical
// types and Ollama wire format. It:
//  1. Converts messages    → Ollama message format
//  2. Converts tools       → Ollama tool format
//  3. POSTs to /api/chat
//  4. Converts response    → providers.Response (Claude-canonical)
func (c *OllamaClient) Call(
	systemPrompt string,
	messages []Message,
	tools []Tool,
) (*Response, error) {

	// 1. Convert messages
	ollamaMsgs := c.convertMessages(systemPrompt, messages)

	// 2. Convert tools
	ollamaTools := convertTools(tools)

	// 3. Build request
	reqBody := ollamaRequest{
		Model:    c.modelID,
		Messages: ollamaMsgs,
		Tools:    ollamaTools,
		Stream:   false,
		Think:    c.think,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal Ollama request: %w", err)
	}

	url := c.baseURL + "/api/chat"
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create Ollama request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request to Ollama: %w\n\n"+
			"Check that Ollama is running:\n"+
			"  - Run: ollama serve\n"+
			"  - Or check: curl %s/api/tags", err, c.baseURL)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read Ollama response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Ollama API error (status %d): %s\n\n"+
			"Suggestions:\n"+
			"  - Ensure the model is pulled: ollama pull %s\n"+
			"  - Check Ollama logs for details",
			resp.StatusCode, string(body), c.modelID)
	}

	// 4. Parse and convert response
	var ollamaResp ollamaResponse
	if err := json.Unmarshal(body, &ollamaResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal Ollama response: %w\nResponse body: %s", err, string(body))
	}

	return c.convertResponse(&ollamaResp), nil
}

// convertMessages translates Claude-canonical messages to Ollama format.
// System prompt becomes a system message prepended to the array.
func (c *OllamaClient) convertMessages(systemPrompt string, messages []Message) []ollamaMessage {
	var result []ollamaMessage

	// System prompt → system message
	if systemPrompt != "" {
		result = append(result, ollamaMessage{
			Role:    "system",
			Content: systemPrompt,
		})
	}

	for i, msg := range messages {
		switch msg.Role {
		case "user":
			result = append(result, c.convertUserMessage(msg, messages, i)...)
		case "assistant":
			result = append(result, c.convertAssistantMessage(msg))
		}
	}

	return result
}

// convertUserMessage translates a Claude user message to Ollama format.
// Plain text stays as-is. Tool results are split into separate "tool" messages.
func (c *OllamaClient) convertUserMessage(msg Message, allMessages []Message, msgIdx int) []ollamaMessage {
	switch content := msg.Content.(type) {
	case string:
		return []ollamaMessage{{Role: "user", Content: content}}

	case []ContentBlock:
		var result []ollamaMessage
		var textParts []string

		for _, block := range content {
			switch block.Type {
			case "text":
				textParts = append(textParts, block.Text)

			case "tool_result":
				// Ollama needs role="tool" with tool_name.
				// Look up tool name from the preceding assistant message's tool_use block.
				toolName := c.findToolName(block.ToolUseID, allMessages, msgIdx)
				resultContent := ""
				if s, ok := block.Content.(string); ok {
					resultContent = s
				}
				result = append(result, ollamaMessage{
					Role:    "tool",
					Content: resultContent,
				})
				// Ollama doesn't have a tool_name field on messages — the tool
				// result is correlated positionally with the preceding tool_calls.
				// We set the content and let Ollama handle correlation.
				_ = toolName

			case "image":
				// MVP: skip images for Ollama
				textParts = append(textParts, "[Image content not supported with Ollama provider]")
			}
		}

		// If there were text parts outside of tool results, emit a user message
		if len(textParts) > 0 {
			// Prepend text before tool results
			result = append([]ollamaMessage{{Role: "user", Content: strings.Join(textParts, "\n")}}, result...)
		}

		return result

	case []interface{}:
		// Handle generic interface slice (from JSON deserialization)
		var textParts []string
		var toolResults []ollamaMessage

		for _, item := range content {
			if m, ok := item.(map[string]interface{}); ok {
				blockType, _ := m["type"].(string)
				switch blockType {
				case "text":
					if text, ok := m["text"].(string); ok {
						textParts = append(textParts, text)
					}
				case "tool_result":
					resultContent := ""
					if c, ok := m["content"].(string); ok {
						resultContent = c
					}
					toolResults = append(toolResults, ollamaMessage{
						Role:    "tool",
						Content: resultContent,
					})
				}
			}
		}

		var result []ollamaMessage
		if len(textParts) > 0 {
			result = append(result, ollamaMessage{Role: "user", Content: strings.Join(textParts, "\n")})
		}
		result = append(result, toolResults...)
		return result
	}

	// Fallback
	return []ollamaMessage{{Role: "user", Content: fmt.Sprintf("%v", msg.Content)}}
}

// convertAssistantMessage translates a Claude assistant message to Ollama format.
// Flattens content blocks into a single Ollama message with content + tool_calls.
func (c *OllamaClient) convertAssistantMessage(msg Message) ollamaMessage {
	om := ollamaMessage{Role: "assistant"}

	switch content := msg.Content.(type) {
	case string:
		om.Content = content

	case []ContentBlock:
		var textParts []string
		var toolCalls []ollamaToolCall

		for _, block := range content {
			switch block.Type {
			case "text":
				if block.Text != "" {
					textParts = append(textParts, block.Text)
				}
			case "tool_use":
				args := block.Input
				if args == nil {
					args = map[string]interface{}{}
				}
				toolCalls = append(toolCalls, ollamaToolCall{
					Function: ollamaFunctionCall{
						Name:      block.Name,
						Arguments: args,
					},
				})
			case "thinking", "redacted_thinking":
				// Strip thinking blocks — Ollama doesn't accept them in input
			}
		}

		om.Content = strings.Join(textParts, "\n")
		if len(toolCalls) > 0 {
			om.ToolCalls = toolCalls
		}

	case []interface{}:
		// Handle generic interface slice
		var textParts []string
		var toolCalls []ollamaToolCall

		for _, item := range content {
			if m, ok := item.(map[string]interface{}); ok {
				blockType, _ := m["type"].(string)
				switch blockType {
				case "text":
					if text, ok := m["text"].(string); ok && text != "" {
						textParts = append(textParts, text)
					}
				case "tool_use":
					name, _ := m["name"].(string)
					args := map[string]interface{}{}
					if input, ok := m["input"].(map[string]interface{}); ok {
						args = input
					}
					toolCalls = append(toolCalls, ollamaToolCall{
						Function: ollamaFunctionCall{
							Name:      name,
							Arguments: args,
						},
					})
				}
			}
		}

		om.Content = strings.Join(textParts, "\n")
		if len(toolCalls) > 0 {
			om.ToolCalls = toolCalls
		}
	}

	return om
}

// findToolName looks backwards from msgIdx to find the tool name for a given tool_use_id.
func (c *OllamaClient) findToolName(toolUseID string, allMessages []Message, msgIdx int) string {
	// Search backwards for the most recent assistant message with tool_use blocks
	for i := msgIdx - 1; i >= 0; i-- {
		msg := allMessages[i]
		if msg.Role != "assistant" {
			continue
		}
		if blocks, ok := msg.Content.([]ContentBlock); ok {
			for _, block := range blocks {
				if block.Type == "tool_use" && block.ID == toolUseID {
					return block.Name
				}
			}
		}
	}
	return "unknown"
}

// convertTools translates Claude tool definitions to Ollama format.
// Claude: {name, description, input_schema}
// Ollama: {type: "function", function: {name, description, parameters}}
func convertTools(tools []Tool) []ollamaTool {
	if len(tools) == 0 {
		return nil
	}

	result := make([]ollamaTool, len(tools))
	for i, tool := range tools {
		result[i] = ollamaTool{
			Type: "function",
			Function: ollamaFunction{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  tool.InputSchema,
			},
		}
	}
	return result
}

// convertResponse translates an Ollama response to Claude-canonical format.
func (c *OllamaClient) convertResponse(resp *ollamaResponse) *Response {
	var content []ContentBlock

	// Thinking → thinking content block
	if resp.Message.Thinking != "" {
		content = append(content, ContentBlock{
			Type:      "thinking",
			Thinking:  resp.Message.Thinking,
			Signature: "", // Ollama has no signatures
		})
	}

	// Text content
	if resp.Message.Content != "" {
		content = append(content, ContentBlock{
			Type: "text",
			Text: resp.Message.Content,
		})
	}

	// Tool calls → tool_use content blocks with synthetic IDs
	for i, tc := range resp.Message.ToolCalls {
		args := tc.Function.Arguments
		if args == nil {
			args = map[string]interface{}{}
		}
		content = append(content, ContentBlock{
			Type:  "tool_use",
			ID:    fmt.Sprintf("ollama_%d", i),
			Name:  tc.Function.Name,
			Input: args,
		})
	}

	// Determine stop reason
	stopReason := "end_turn"
	if len(resp.Message.ToolCalls) > 0 {
		stopReason = "tool_use"
	}

	return &Response{
		ID:         fmt.Sprintf("ollama-%s", resp.Model),
		Type:       "message",
		Role:       "assistant",
		Content:    content,
		Model:      resp.Model,
		StopReason: stopReason,
		Usage: Usage{
			InputTokens:  resp.PromptEvalCount,
			OutputTokens: resp.EvalCount,
		},
	}
}
