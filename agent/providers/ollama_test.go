package providers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestConvertToolsToOllama(t *testing.T) {
	tools := []Tool{
		{
			Name:        "read_file",
			Description: "Read a file",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"path": map[string]interface{}{
						"type":        "string",
						"description": "File path",
					},
				},
				"required": []string{"path"},
			},
		},
	}

	result := convertTools(tools)

	if len(result) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(result))
	}
	if result[0].Type != "function" {
		t.Errorf("expected type 'function', got %q", result[0].Type)
	}
	if result[0].Function.Name != "read_file" {
		t.Errorf("expected name 'read_file', got %q", result[0].Function.Name)
	}
	if result[0].Function.Description != "Read a file" {
		t.Errorf("expected description 'Read a file', got %q", result[0].Function.Description)
	}
}

func TestConvertToolsEmpty(t *testing.T) {
	result := convertTools(nil)
	if result != nil {
		t.Errorf("expected nil for empty tools, got %v", result)
	}
}

func TestConvertAssistantTextMessage(t *testing.T) {
	c := NewOllamaClient("http://localhost:11434", "test", false)
	msg := Message{Role: "assistant", Content: "hello world"}
	result := c.convertAssistantMessage(msg)

	if result.Role != "assistant" {
		t.Errorf("expected role 'assistant', got %q", result.Role)
	}
	if result.Content != "hello world" {
		t.Errorf("expected content 'hello world', got %q", result.Content)
	}
	if len(result.ToolCalls) != 0 {
		t.Errorf("expected no tool calls, got %d", len(result.ToolCalls))
	}
}

func TestConvertAssistantToolUseMessage(t *testing.T) {
	c := NewOllamaClient("http://localhost:11434", "test", false)
	msg := Message{
		Role: "assistant",
		Content: []ContentBlock{
			{Type: "text", Text: "Let me read that file."},
			{Type: "tool_use", ID: "toolu_123", Name: "read_file", Input: map[string]interface{}{"path": "main.go"}},
		},
	}

	result := c.convertAssistantMessage(msg)

	if result.Content != "Let me read that file." {
		t.Errorf("expected text content, got %q", result.Content)
	}
	if len(result.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(result.ToolCalls))
	}
	if result.ToolCalls[0].Function.Name != "read_file" {
		t.Errorf("expected tool name 'read_file', got %q", result.ToolCalls[0].Function.Name)
	}
	if result.ToolCalls[0].Function.Arguments["path"] != "main.go" {
		t.Errorf("expected path 'main.go', got %v", result.ToolCalls[0].Function.Arguments["path"])
	}
}

func TestConvertAssistantStripsThinking(t *testing.T) {
	c := NewOllamaClient("http://localhost:11434", "test", false)
	msg := Message{
		Role: "assistant",
		Content: []ContentBlock{
			{Type: "thinking", Thinking: "Let me think about this..."},
			{Type: "text", Text: "Here's my answer."},
			{Type: "redacted_thinking", Data: "encrypted"},
		},
	}

	result := c.convertAssistantMessage(msg)

	if result.Content != "Here's my answer." {
		t.Errorf("expected only text content, got %q", result.Content)
	}
	if len(result.ToolCalls) != 0 {
		t.Errorf("expected no tool calls after stripping thinking, got %d", len(result.ToolCalls))
	}
}

func TestConvertUserTextMessage(t *testing.T) {
	c := NewOllamaClient("http://localhost:11434", "test", false)
	msg := Message{Role: "user", Content: "hello"}
	result := c.convertUserMessage(msg, nil, 0)

	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}
	if result[0].Role != "user" {
		t.Errorf("expected role 'user', got %q", result[0].Role)
	}
	if result[0].Content != "hello" {
		t.Errorf("expected content 'hello', got %q", result[0].Content)
	}
}

func TestConvertUserToolResultMessage(t *testing.T) {
	c := NewOllamaClient("http://localhost:11434", "test", false)

	// The preceding assistant message with tool_use
	assistantMsg := Message{
		Role: "assistant",
		Content: []ContentBlock{
			{Type: "tool_use", ID: "toolu_123", Name: "read_file", Input: map[string]interface{}{"path": "main.go"}},
		},
	}

	// Tool result message
	userMsg := Message{
		Role: "user",
		Content: []ContentBlock{
			{Type: "tool_result", ToolUseID: "toolu_123", Content: "package main\n\nfunc main() {}"},
		},
	}

	allMessages := []Message{assistantMsg, userMsg}
	result := c.convertUserMessage(userMsg, allMessages, 1)

	if len(result) != 1 {
		t.Fatalf("expected 1 message (tool result), got %d", len(result))
	}
	if result[0].Role != "tool" {
		t.Errorf("expected role 'tool', got %q", result[0].Role)
	}
	if result[0].Content != "package main\n\nfunc main() {}" {
		t.Errorf("unexpected content: %q", result[0].Content)
	}
}

func TestConvertMessages_SystemPromptPrepended(t *testing.T) {
	c := NewOllamaClient("http://localhost:11434", "test", false)
	messages := []Message{
		{Role: "user", Content: "hello"},
	}

	result := c.convertMessages("You are a helpful assistant.", messages)

	if len(result) != 2 {
		t.Fatalf("expected 2 messages (system + user), got %d", len(result))
	}
	if result[0].Role != "system" {
		t.Errorf("expected first message role 'system', got %q", result[0].Role)
	}
	if result[0].Content != "You are a helpful assistant." {
		t.Errorf("unexpected system content: %q", result[0].Content)
	}
	if result[1].Role != "user" {
		t.Errorf("expected second message role 'user', got %q", result[1].Role)
	}
}

func TestConvertMessages_EmptySystemPrompt(t *testing.T) {
	c := NewOllamaClient("http://localhost:11434", "test", false)
	messages := []Message{
		{Role: "user", Content: "hello"},
	}

	result := c.convertMessages("", messages)

	if len(result) != 1 {
		t.Fatalf("expected 1 message (no system), got %d", len(result))
	}
	if result[0].Role != "user" {
		t.Errorf("expected role 'user', got %q", result[0].Role)
	}
}

func TestConvertResponse_TextOnly(t *testing.T) {
	c := NewOllamaClient("http://localhost:11434", "test", false)
	resp := &ollamaResponse{
		Model: "qwen3.5:35b",
		Message: ollamaRespMsg{
			Role:    "assistant",
			Content: "Hello! How can I help?",
		},
		Done:            true,
		DoneReason:      "stop",
		PromptEvalCount: 100,
		EvalCount:       50,
	}

	result := c.convertResponse(resp)

	if result.Role != "assistant" {
		t.Errorf("expected role 'assistant', got %q", result.Role)
	}
	if result.StopReason != "end_turn" {
		t.Errorf("expected stop_reason 'end_turn', got %q", result.StopReason)
	}
	if result.Usage.InputTokens != 100 {
		t.Errorf("expected input tokens 100, got %d", result.Usage.InputTokens)
	}
	if result.Usage.OutputTokens != 50 {
		t.Errorf("expected output tokens 50, got %d", result.Usage.OutputTokens)
	}
	if len(result.Content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(result.Content))
	}
	if result.Content[0].Type != "text" {
		t.Errorf("expected type 'text', got %q", result.Content[0].Type)
	}
	if result.Content[0].Text != "Hello! How can I help?" {
		t.Errorf("unexpected text: %q", result.Content[0].Text)
	}
}

func TestConvertResponse_WithThinking(t *testing.T) {
	c := NewOllamaClient("http://localhost:11434", "test", true)
	resp := &ollamaResponse{
		Model: "qwen3.5:35b",
		Message: ollamaRespMsg{
			Role:     "assistant",
			Content:  "The answer is 42.",
			Thinking: "Let me think about this question...",
		},
		Done:       true,
		DoneReason: "stop",
	}

	result := c.convertResponse(resp)

	if len(result.Content) != 2 {
		t.Fatalf("expected 2 content blocks (thinking + text), got %d", len(result.Content))
	}
	if result.Content[0].Type != "thinking" {
		t.Errorf("expected first block type 'thinking', got %q", result.Content[0].Type)
	}
	if result.Content[0].Thinking != "Let me think about this question..." {
		t.Errorf("unexpected thinking: %q", result.Content[0].Thinking)
	}
	if result.Content[0].Signature != "" {
		t.Errorf("expected empty signature, got %q", result.Content[0].Signature)
	}
	if result.Content[1].Type != "text" {
		t.Errorf("expected second block type 'text', got %q", result.Content[1].Type)
	}
}

func TestConvertResponse_WithToolCalls(t *testing.T) {
	c := NewOllamaClient("http://localhost:11434", "test", false)
	resp := &ollamaResponse{
		Model: "qwen3.5:35b",
		Message: ollamaRespMsg{
			Role:    "assistant",
			Content: "",
			ToolCalls: []ollamaToolCall{
				{Function: ollamaFunctionCall{
					Name:      "read_file",
					Arguments: map[string]interface{}{"path": "main.go"},
				}},
				{Function: ollamaFunctionCall{
					Name:      "list_files",
					Arguments: map[string]interface{}{"path": "."},
				}},
			},
		},
		Done:       true,
		DoneReason: "stop",
	}

	result := c.convertResponse(resp)

	if result.StopReason != "tool_use" {
		t.Errorf("expected stop_reason 'tool_use', got %q", result.StopReason)
	}
	if len(result.Content) != 2 {
		t.Fatalf("expected 2 content blocks (tool_use x2), got %d", len(result.Content))
	}
	if result.Content[0].Type != "tool_use" {
		t.Errorf("expected type 'tool_use', got %q", result.Content[0].Type)
	}
	if result.Content[0].ID != "ollama_0" {
		t.Errorf("expected synthetic ID 'ollama_0', got %q", result.Content[0].ID)
	}
	if result.Content[0].Name != "read_file" {
		t.Errorf("expected name 'read_file', got %q", result.Content[0].Name)
	}
	if result.Content[1].ID != "ollama_1" {
		t.Errorf("expected synthetic ID 'ollama_1', got %q", result.Content[1].ID)
	}
}

func TestSyntheticIDRoundTrip(t *testing.T) {
	// Verify that synthetic IDs from a tool_use response survive through
	// tool_result messages and get properly handled on the next call.
	c := NewOllamaClient("http://localhost:11434", "test", false)

	// Simulate: Ollama returned a tool call, agent executed it, now building next request.
	messages := []Message{
		{Role: "user", Content: "Read main.go"},
		{
			Role: "assistant",
			Content: []ContentBlock{
				{Type: "tool_use", ID: "ollama_0", Name: "read_file", Input: map[string]interface{}{"path": "main.go"}},
			},
		},
		{
			Role: "user",
			Content: []ContentBlock{
				{Type: "tool_result", ToolUseID: "ollama_0", Content: "package main"},
			},
		},
	}

	result := c.convertMessages("system", messages)

	// system + user + assistant + tool = 4
	if len(result) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(result))
	}

	// The assistant message should have tool_calls
	if len(result[2].ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call on assistant message, got %d", len(result[2].ToolCalls))
	}
	if result[2].ToolCalls[0].Function.Name != "read_file" {
		t.Errorf("expected tool name 'read_file', got %q", result[2].ToolCalls[0].Function.Name)
	}

	// The tool result message
	if result[3].Role != "tool" {
		t.Errorf("expected role 'tool', got %q", result[3].Role)
	}
	if result[3].Content != "package main" {
		t.Errorf("expected content 'package main', got %q", result[3].Content)
	}
}

func TestOllamaClient_Call_MockServer(t *testing.T) {
	// Set up a mock Ollama server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}

		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Parse request to verify it's well-formed
		var req ollamaRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("failed to decode request: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		if req.Model != "qwen3.5:35b" {
			t.Errorf("expected model 'qwen3.5:35b', got %q", req.Model)
		}
		if req.Stream {
			t.Errorf("expected stream=false")
		}

		// Return a simple text response
		resp := ollamaResponse{
			Model: "qwen3.5:35b",
			Message: ollamaRespMsg{
				Role:    "assistant",
				Content: "Hello from Ollama!",
			},
			Done:            true,
			DoneReason:      "stop",
			PromptEvalCount: 42,
			EvalCount:       10,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Create client pointing at mock server
	client := NewOllamaClient(server.URL, "qwen3.5:35b", false)

	messages := []Message{
		{Role: "user", Content: "Hello!"},
	}

	result, err := client.Call("You are helpful.", messages, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Role != "assistant" {
		t.Errorf("expected role 'assistant', got %q", result.Role)
	}
	if result.StopReason != "end_turn" {
		t.Errorf("expected stop_reason 'end_turn', got %q", result.StopReason)
	}
	if len(result.Content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(result.Content))
	}
	if result.Content[0].Text != "Hello from Ollama!" {
		t.Errorf("unexpected text: %q", result.Content[0].Text)
	}
	if result.Usage.InputTokens != 42 {
		t.Errorf("expected input tokens 42, got %d", result.Usage.InputTokens)
	}
}

func TestOllamaClient_Call_ToolUseLoop(t *testing.T) {
	// Mock server that returns a tool call on first request,
	// then a text response on the second.
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++

		var resp ollamaResponse
		if callCount == 1 {
			// First call: return tool_use
			resp = ollamaResponse{
				Model: "qwen3.5:35b",
				Message: ollamaRespMsg{
					Role: "assistant",
					ToolCalls: []ollamaToolCall{
						{Function: ollamaFunctionCall{
							Name:      "read_file",
							Arguments: map[string]interface{}{"path": "main.go"},
						}},
					},
				},
				Done:       true,
				DoneReason: "stop",
			}
		} else {
			// Second call: return text
			resp = ollamaResponse{
				Model: "qwen3.5:35b",
				Message: ollamaRespMsg{
					Role:    "assistant",
					Content: "The file contains a main function.",
				},
				Done:       true,
				DoneReason: "stop",
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewOllamaClient(server.URL, "qwen3.5:35b", false)

	// First call
	messages := []Message{{Role: "user", Content: "Read main.go"}}
	result1, err := client.Call("system", messages, []Tool{{Name: "read_file", Description: "Read a file"}})
	if err != nil {
		t.Fatalf("unexpected error on first call: %v", err)
	}

	if result1.StopReason != "tool_use" {
		t.Errorf("expected stop_reason 'tool_use', got %q", result1.StopReason)
	}
	if len(result1.Content) != 1 || result1.Content[0].Type != "tool_use" {
		t.Fatalf("expected tool_use content block")
	}
	if result1.Content[0].ID != "ollama_0" {
		t.Errorf("expected synthetic ID 'ollama_0', got %q", result1.Content[0].ID)
	}

	// Add tool_use and tool_result, then call again
	messages = append(messages, Message{
		Role: "assistant",
		Content: []ContentBlock{
			{Type: "tool_use", ID: "ollama_0", Name: "read_file", Input: map[string]interface{}{"path": "main.go"}},
		},
	})
	messages = append(messages, Message{
		Role: "user",
		Content: []ContentBlock{
			{Type: "tool_result", ToolUseID: "ollama_0", Content: "package main\n\nfunc main() {}"},
		},
	})

	result2, err := client.Call("system", messages, []Tool{{Name: "read_file", Description: "Read a file"}})
	if err != nil {
		t.Fatalf("unexpected error on second call: %v", err)
	}

	if result2.StopReason != "end_turn" {
		t.Errorf("expected stop_reason 'end_turn', got %q", result2.StopReason)
	}
	if len(result2.Content) != 1 || result2.Content[0].Text != "The file contains a main function." {
		t.Errorf("unexpected response content: %+v", result2.Content)
	}
}

func TestOllamaClient_Call_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error": "model not found"}`, http.StatusNotFound)
	}))
	defer server.Close()

	client := NewOllamaClient(server.URL, "nonexistent", false)
	_, err := client.Call("system", []Message{{Role: "user", Content: "hi"}}, nil)

	if err == nil {
		t.Fatal("expected error for 404 response")
	}
	if !contains(err.Error(), "Ollama API error") {
		t.Errorf("expected 'Ollama API error' in error, got: %q", err.Error())
	}
}

func TestOllamaClient_Call_ConnectionRefused(t *testing.T) {
	// Point at a port that's not listening
	client := NewOllamaClient("http://localhost:1", "test", false)
	_, err := client.Call("system", []Message{{Role: "user", Content: "hi"}}, nil)

	if err == nil {
		t.Fatal("expected error for connection refused")
	}
	if !contains(err.Error(), "failed to send request to Ollama") {
		t.Errorf("expected connection error message, got: %q", err.Error())
	}
}

func TestProviderInterfaceSatisfied(t *testing.T) {
	// Compile-time check that both clients satisfy Provider
	var _ Provider = (*Client)(nil)
	var _ Provider = (*OllamaClient)(nil)
}

// helper
func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
