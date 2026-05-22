package providers

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

// OllamaClient handles communication with an Ollama server.
// It translates between Claude-canonical types (used by the agent) and
// Ollama's /api/chat wire format.
type OllamaClient struct {
	baseURL          string        // e.g. "http://localhost:11434"
	modelID          string        // e.g. "qwen3.6:27b"
	think            bool          // enable thinking mode
	preflightTimeout time.Duration // timeout for Preflight health checks (default 15s)
}

// NewOllamaClient creates a new Ollama API client.
func NewOllamaClient(baseURL, modelID string, think bool) *OllamaClient {
	return &OllamaClient{
		baseURL:          strings.TrimRight(baseURL, "/"),
		modelID:          modelID,
		think:            think,
		preflightTimeout: 15 * time.Second,
	}
}

// WithPreflightTimeout sets the timeout for Preflight health checks.
// The timeout governs how long Preflight waits for Ollama to start
// after launching 'ollama serve'. Default is 15 seconds.
func (c *OllamaClient) WithPreflightTimeout(d time.Duration) *OllamaClient {
	c.preflightTimeout = d
	return c
}

// --- Preflight: auto-start Ollama and verify model ---

// ollamaTagsResponse is the JSON shape of GET /api/tags.
type ollamaTagsResponse struct {
	Models []ollamaModelInfo `json:"models"`
}

// ollamaModelInfo is a single entry in the /api/tags response.
type ollamaModelInfo struct {
	Name string `json:"name"`
}

// Preflight performs startup checks for the Ollama provider:
//  1. Checks if the Ollama server is reachable.
//  2. If not, attempts to start 'ollama serve' and waits for readiness.
//  3. Queries /api/tags to verify the configured model is available.
//
// Returns nil on success, or a descriptive error with actionable instructions.
// This is a one-time check intended to be called before the first API call.
//
// When Ollama is already running with the model pulled, Preflight completes
// in well under 500ms (two HTTP round-trips to localhost).
func (c *OllamaClient) Preflight() error {
	// 1. Check connectivity
	if err := c.checkConnectivity(); err != nil {
		// 2. Ollama is not reachable — try to auto-start it
		if startErr := c.startOllamaServe(); startErr != nil {
			return startErr
		}
	}

	// 3. Check that the configured model is available
	return c.checkModel()
}

// checkConnectivity performs a quick GET to the Ollama root endpoint.
// Ollama returns "Ollama is running" with status 200 when healthy.
func (c *OllamaClient) checkConnectivity() error {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(c.baseURL + "/")
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d from Ollama", resp.StatusCode)
	}
	return nil
}

// startOllamaServe attempts to start 'ollama serve' as a background process
// and polls for readiness. Returns nil once the server is responding, or an
// error if the binary is not found or the server fails to start within the
// preflight timeout.
func (c *OllamaClient) startOllamaServe() error {
	// Verify the ollama binary exists
	ollamaPath, err := exec.LookPath("ollama")
	if err != nil {
		return fmt.Errorf("Ollama is not running and the 'ollama' binary was not found in PATH.\n\n"+
			"Install Ollama: https://ollama.com/download\n"+
			"Then start it:  ollama serve")
	}

	// Start 'ollama serve' as a detached background process.
	// Setpgid puts it in its own process group so Ctrl+C on our terminal
	// doesn't kill it. Stdout/Stderr go to /dev/null to avoid cluttering
	// the terminal.
	cmd := exec.Command(ollamaPath, "serve")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err == nil {
		cmd.Stdout = devNull
		cmd.Stderr = devNull
		defer devNull.Close()
	}
	cmd.Stdin = nil

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start 'ollama serve': %w\n\n"+
			"Start it manually:\n  ollama serve", err)
	}

	// Reap the child asynchronously so it doesn't become a zombie
	// if it exits while we're still running.
	go cmd.Wait()

	// Poll for readiness
	deadline := time.Now().Add(c.preflightTimeout)
	pollInterval := 250 * time.Millisecond
	for time.Now().Before(deadline) {
		time.Sleep(pollInterval)
		if err := c.checkConnectivity(); err == nil {
			return nil
		}
	}

	return fmt.Errorf("started 'ollama serve' but it did not become ready within %s.\n\n"+
		"Check Ollama logs or try starting it manually:\n  ollama serve",
		c.preflightTimeout)
}

// checkModel queries /api/tags and verifies the configured model is available.
// If the model is not found, returns an error with the exact 'ollama pull'
// command and a list of available models.
func (c *OllamaClient) checkModel() error {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(c.baseURL + "/api/tags")
	if err != nil {
		return fmt.Errorf("failed to query Ollama models: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read Ollama models response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to query Ollama models (status %d): %s", resp.StatusCode, string(body))
	}

	var tags ollamaTagsResponse
	if err := json.Unmarshal(body, &tags); err != nil {
		return fmt.Errorf("failed to parse Ollama models response: %w", err)
	}

	// Try exact match first
	for _, model := range tags.Models {
		if model.Name == c.modelID {
			return nil
		}
	}

	// If the configured model has no tag (no ':'), also try matching with ":latest"
	if !strings.Contains(c.modelID, ":") {
		for _, model := range tags.Models {
			if model.Name == c.modelID+":latest" {
				return nil
			}
		}
	}

	// Model not found — build a helpful error message
	var available []string
	for _, model := range tags.Models {
		available = append(available, model.Name)
	}

	msg := fmt.Sprintf("Model %q not found in Ollama.\n\nPull it with:\n  ollama pull %s",
		c.modelID, c.modelID)
	if len(available) > 0 {
		msg += "\n\nAvailable models:\n"
		for _, name := range available {
			msg += fmt.Sprintf("  - %s\n", name)
		}
	} else {
		msg += "\n\nNo models are currently pulled. Pull one with:\n  ollama pull " + c.modelID
	}

	return fmt.Errorf("%s", msg)
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
				// MVP: skip images for Ollama — the text tool_result
				// "Image loaded successfully" is sufficient context.
				// Silently omit rather than inserting confusing placeholder text.
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

// StreamCall implements StreamingProvider for the Ollama client.
// It sends a streaming request (stream: true) and calls onText/onThinking
// callbacks for each token as it arrives from the NDJSON stream.
//
// The returned Response is identical to what Call() would return —
// all content is assembled into the same Claude-canonical format.
// Downstream code (history, session persistence, compaction) is unaffected
// by whether streaming or non-streaming was used.
func (c *OllamaClient) StreamCall(
	systemPrompt string,
	messages []Message,
	tools []Tool,
	onText func(text string),
	onThinking func(text string),
) (*Response, error) {
	// 1. Convert messages and tools (same as Call)
	ollamaMsgs := c.convertMessages(systemPrompt, messages)
	ollamaTools := convertTools(tools)

	// 2. Build request with stream=true
	reqBody := ollamaRequest{
		Model:    c.modelID,
		Messages: ollamaMsgs,
		Tools:    ollamaTools,
		Stream:   true,
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

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Ollama API error (status %d): %s\n\n"+
			"Suggestions:\n"+
			"  - Ensure the model is pulled: ollama pull %s\n"+
			"  - Check Ollama logs for details",
			resp.StatusCode, string(body), c.modelID)
	}

	// 3. Read NDJSON stream, accumulate content, and fire callbacks
	var thinkingBuf strings.Builder
	var contentBuf strings.Builder
	var toolCalls []ollamaToolCall
	var lastChunk ollamaResponse

	scanner := bufio.NewScanner(resp.Body)
	// Set a large buffer for potentially long lines (tool call arguments)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var chunk ollamaResponse
		if err := json.Unmarshal(line, &chunk); err != nil {
			return nil, fmt.Errorf("failed to parse streaming chunk: %w\nChunk: %s", err, string(line))
		}

		// Accumulate and emit thinking tokens
		if chunk.Message.Thinking != "" {
			thinkingBuf.WriteString(chunk.Message.Thinking)
			if onThinking != nil {
				onThinking(chunk.Message.Thinking)
			}
		}

		// Accumulate and emit text tokens
		if chunk.Message.Content != "" {
			contentBuf.WriteString(chunk.Message.Content)
			if onText != nil {
				onText(chunk.Message.Content)
			}
		}

		// Collect tool calls (typically appear in the final chunk)
		if len(chunk.Message.ToolCalls) > 0 {
			toolCalls = append(toolCalls, chunk.Message.ToolCalls...)
		}

		if chunk.Done {
			lastChunk = chunk
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading Ollama stream: %w", err)
	}

	// 4. Assemble the final response using the same convertResponse path
	model := lastChunk.Model
	if model == "" {
		model = c.modelID
	}

	assembledResp := &ollamaResponse{
		Model: model,
		Message: ollamaRespMsg{
			Role:      "assistant",
			Content:   contentBuf.String(),
			Thinking:  thinkingBuf.String(),
			ToolCalls: toolCalls,
		},
		Done:            true,
		DoneReason:      lastChunk.DoneReason,
		PromptEvalCount: lastChunk.PromptEvalCount,
		EvalCount:       lastChunk.EvalCount,
	}

	return c.convertResponse(assembledResp), nil
}
