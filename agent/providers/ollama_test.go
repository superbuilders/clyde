package providers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
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
		Model: "qwen3.6:27b",
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
		Model: "qwen3.6:27b",
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
		Model: "qwen3.6:27b",
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

		if req.Model != "qwen3.6:27b" {
			t.Errorf("expected model 'qwen3.6:27b', got %q", req.Model)
		}
		if req.Stream {
			t.Errorf("expected stream=false")
		}

		// Return a simple text response
		resp := ollamaResponse{
			Model: "qwen3.6:27b",
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
	client := NewOllamaClient(server.URL, "qwen3.6:27b", false)

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
				Model: "qwen3.6:27b",
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
				Model: "qwen3.6:27b",
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

	client := NewOllamaClient(server.URL, "qwen3.6:27b", false)

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

func TestStreamingProviderInterfaceSatisfied(t *testing.T) {
	// Compile-time check that OllamaClient satisfies StreamingProvider
	var _ StreamingProvider = (*OllamaClient)(nil)
}

func TestClaudeNotStreamingProvider(t *testing.T) {
	// Claude client should NOT implement StreamingProvider
	var c Provider = &Client{}
	_, ok := c.(StreamingProvider)
	if ok {
		t.Error("Claude client should not implement StreamingProvider")
	}
}

// --- Streaming tests ---

// writeStreamChunks writes NDJSON chunks to a ResponseWriter, flushing between each.
func writeStreamChunks(w http.ResponseWriter, chunks []ollamaResponse) {
	flusher, _ := w.(http.Flusher)
	for _, chunk := range chunks {
		data, _ := json.Marshal(chunk)
		w.Write(data)
		w.Write([]byte("\n"))
		if flusher != nil {
			flusher.Flush()
		}
	}
}

func TestStreamCall_TextOnly(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request has stream=true
		var req ollamaRequest
		json.NewDecoder(r.Body).Decode(&req)
		if !req.Stream {
			t.Errorf("expected stream=true, got stream=false")
		}

		w.Header().Set("Content-Type", "application/x-ndjson")
		writeStreamChunks(w, []ollamaResponse{
			{Model: "test", Message: ollamaRespMsg{Role: "assistant", Content: "Hello"}, Done: false},
			{Model: "test", Message: ollamaRespMsg{Role: "assistant", Content: " world"}, Done: false},
			{Model: "test", Message: ollamaRespMsg{Role: "assistant", Content: "!"}, Done: false},
			{Model: "test", Message: ollamaRespMsg{Role: "assistant", Content: ""}, Done: true, DoneReason: "stop", PromptEvalCount: 100, EvalCount: 3},
		})
	}))
	defer server.Close()

	client := NewOllamaClient(server.URL, "test", false)

	var tokens []string
	resp, err := client.StreamCall(
		"system", []Message{{Role: "user", Content: "hi"}}, nil,
		func(text string) { tokens = append(tokens, text) },
		nil,
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify tokens were emitted incrementally
	expected := []string{"Hello", " world", "!"}
	if len(tokens) != len(expected) {
		t.Fatalf("expected %d text tokens, got %d: %v", len(expected), len(tokens), tokens)
	}
	for i, tok := range tokens {
		if tok != expected[i] {
			t.Errorf("token %d: expected %q, got %q", i, expected[i], tok)
		}
	}

	// Verify assembled response matches what Call() would return
	if len(resp.Content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(resp.Content))
	}
	if resp.Content[0].Type != "text" {
		t.Errorf("expected type 'text', got %q", resp.Content[0].Type)
	}
	if resp.Content[0].Text != "Hello world!" {
		t.Errorf("expected assembled text 'Hello world!', got %q", resp.Content[0].Text)
	}
	if resp.StopReason != "end_turn" {
		t.Errorf("expected stop_reason 'end_turn', got %q", resp.StopReason)
	}
	if resp.Usage.InputTokens != 100 {
		t.Errorf("expected input tokens 100, got %d", resp.Usage.InputTokens)
	}
	if resp.Usage.OutputTokens != 3 {
		t.Errorf("expected output tokens 3, got %d", resp.Usage.OutputTokens)
	}
	if resp.Role != "assistant" {
		t.Errorf("expected role 'assistant', got %q", resp.Role)
	}
}

func TestStreamCall_WithThinking(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		writeStreamChunks(w, []ollamaResponse{
			{Model: "test", Message: ollamaRespMsg{Role: "assistant", Thinking: "Let me"}, Done: false},
			{Model: "test", Message: ollamaRespMsg{Role: "assistant", Thinking: " think..."}, Done: false},
			{Model: "test", Message: ollamaRespMsg{Role: "assistant", Content: "The answer"}, Done: false},
			{Model: "test", Message: ollamaRespMsg{Role: "assistant", Content: " is 42."}, Done: false},
			{Model: "test", Message: ollamaRespMsg{Role: "assistant"}, Done: true, DoneReason: "stop", PromptEvalCount: 50, EvalCount: 10},
		})
	}))
	defer server.Close()

	client := NewOllamaClient(server.URL, "test", true)

	var thinkTokens []string
	var textTokens []string
	resp, err := client.StreamCall(
		"system", []Message{{Role: "user", Content: "what is the meaning?"}}, nil,
		func(text string) { textTokens = append(textTokens, text) },
		func(thinking string) { thinkTokens = append(thinkTokens, thinking) },
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify thinking tokens
	if len(thinkTokens) != 2 {
		t.Fatalf("expected 2 thinking tokens, got %d: %v", len(thinkTokens), thinkTokens)
	}
	if thinkTokens[0] != "Let me" || thinkTokens[1] != " think..." {
		t.Errorf("unexpected thinking tokens: %v", thinkTokens)
	}

	// Verify text tokens
	if len(textTokens) != 2 {
		t.Fatalf("expected 2 text tokens, got %d: %v", len(textTokens), textTokens)
	}
	if textTokens[0] != "The answer" || textTokens[1] != " is 42." {
		t.Errorf("unexpected text tokens: %v", textTokens)
	}

	// Verify assembled response has both thinking and text blocks
	if len(resp.Content) != 2 {
		t.Fatalf("expected 2 content blocks (thinking + text), got %d", len(resp.Content))
	}
	if resp.Content[0].Type != "thinking" {
		t.Errorf("expected first block type 'thinking', got %q", resp.Content[0].Type)
	}
	if resp.Content[0].Thinking != "Let me think..." {
		t.Errorf("expected thinking 'Let me think...', got %q", resp.Content[0].Thinking)
	}
	if resp.Content[0].Signature != "" {
		t.Errorf("expected empty signature, got %q", resp.Content[0].Signature)
	}
	if resp.Content[1].Type != "text" {
		t.Errorf("expected second block type 'text', got %q", resp.Content[1].Type)
	}
	if resp.Content[1].Text != "The answer is 42." {
		t.Errorf("expected text 'The answer is 42.', got %q", resp.Content[1].Text)
	}
}

func TestStreamCall_WithToolCalls(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		writeStreamChunks(w, []ollamaResponse{
			{Model: "test", Message: ollamaRespMsg{Role: "assistant", Content: "Let me read that."}, Done: false},
			{
				Model: "test",
				Message: ollamaRespMsg{
					Role: "assistant",
					ToolCalls: []ollamaToolCall{
						{Function: ollamaFunctionCall{
							Name:      "read_file",
							Arguments: map[string]interface{}{"path": "main.go"},
						}},
					},
				},
				Done:            true,
				DoneReason:      "stop",
				PromptEvalCount: 50,
				EvalCount:       10,
			},
		})
	}))
	defer server.Close()

	client := NewOllamaClient(server.URL, "test", false)

	var textTokens []string
	resp, err := client.StreamCall(
		"system", []Message{{Role: "user", Content: "read main.go"}},
		[]Tool{{Name: "read_file", Description: "Read a file"}},
		func(text string) { textTokens = append(textTokens, text) },
		nil,
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify text was streamed
	if len(textTokens) != 1 || textTokens[0] != "Let me read that." {
		t.Errorf("expected text token 'Let me read that.', got %v", textTokens)
	}

	// Verify response has text + tool_use
	if resp.StopReason != "tool_use" {
		t.Errorf("expected stop_reason 'tool_use', got %q", resp.StopReason)
	}
	if len(resp.Content) != 2 {
		t.Fatalf("expected 2 content blocks (text + tool_use), got %d", len(resp.Content))
	}
	if resp.Content[0].Type != "text" {
		t.Errorf("expected first block type 'text', got %q", resp.Content[0].Type)
	}
	if resp.Content[0].Text != "Let me read that." {
		t.Errorf("expected text 'Let me read that.', got %q", resp.Content[0].Text)
	}
	if resp.Content[1].Type != "tool_use" {
		t.Errorf("expected second block type 'tool_use', got %q", resp.Content[1].Type)
	}
	if resp.Content[1].ID != "ollama_0" {
		t.Errorf("expected synthetic ID 'ollama_0', got %q", resp.Content[1].ID)
	}
	if resp.Content[1].Name != "read_file" {
		t.Errorf("expected tool name 'read_file', got %q", resp.Content[1].Name)
	}
}

func TestStreamCall_ToolCallsOnly(t *testing.T) {
	// Test: model produces only tool calls, no text content
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		writeStreamChunks(w, []ollamaResponse{
			{
				Model: "test",
				Message: ollamaRespMsg{
					Role: "assistant",
					ToolCalls: []ollamaToolCall{
						{Function: ollamaFunctionCall{
							Name:      "list_files",
							Arguments: map[string]interface{}{"path": "."},
						}},
					},
				},
				Done:            true,
				DoneReason:      "stop",
				PromptEvalCount: 30,
				EvalCount:       5,
			},
		})
	}))
	defer server.Close()

	client := NewOllamaClient(server.URL, "test", false)

	textCalled := false
	resp, err := client.StreamCall(
		"system", []Message{{Role: "user", Content: "list files"}},
		[]Tool{{Name: "list_files", Description: "List files"}},
		func(text string) { textCalled = true },
		nil,
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if textCalled {
		t.Error("onText should not have been called for tool-only response")
	}

	if resp.StopReason != "tool_use" {
		t.Errorf("expected stop_reason 'tool_use', got %q", resp.StopReason)
	}
	if len(resp.Content) != 1 {
		t.Fatalf("expected 1 content block (tool_use), got %d", len(resp.Content))
	}
	if resp.Content[0].Type != "tool_use" {
		t.Errorf("expected type 'tool_use', got %q", resp.Content[0].Type)
	}
}

func TestStreamCall_NilCallbacks(t *testing.T) {
	// Test: nil callbacks don't cause panics
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		writeStreamChunks(w, []ollamaResponse{
			{Model: "test", Message: ollamaRespMsg{Role: "assistant", Thinking: "hmm"}, Done: false},
			{Model: "test", Message: ollamaRespMsg{Role: "assistant", Content: "ok"}, Done: false},
			{Model: "test", Message: ollamaRespMsg{Role: "assistant"}, Done: true, DoneReason: "stop", PromptEvalCount: 10, EvalCount: 2},
		})
	}))
	defer server.Close()

	client := NewOllamaClient(server.URL, "test", true)

	// Both callbacks nil — should not panic
	resp, err := client.StreamCall(
		"system", []Message{{Role: "user", Content: "hi"}}, nil,
		nil, nil,
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Content) != 2 {
		t.Fatalf("expected 2 content blocks, got %d", len(resp.Content))
	}
	if resp.Content[0].Type != "thinking" || resp.Content[1].Type != "text" {
		t.Errorf("unexpected block types: %q, %q", resp.Content[0].Type, resp.Content[1].Type)
	}
}

func TestStreamCall_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error": "model not found"}`, http.StatusNotFound)
	}))
	defer server.Close()

	client := NewOllamaClient(server.URL, "nonexistent", false)
	_, err := client.StreamCall(
		"system", []Message{{Role: "user", Content: "hi"}}, nil,
		nil, nil,
	)

	if err == nil {
		t.Fatal("expected error for 404 response")
	}
	if !contains(err.Error(), "Ollama API error") {
		t.Errorf("expected 'Ollama API error' in error, got: %q", err.Error())
	}
}

func TestStreamCall_EmptyStream(t *testing.T) {
	// Test: only a done=true chunk with no content
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		writeStreamChunks(w, []ollamaResponse{
			{Model: "test", Message: ollamaRespMsg{Role: "assistant"}, Done: true, DoneReason: "stop", PromptEvalCount: 5, EvalCount: 0},
		})
	}))
	defer server.Close()

	client := NewOllamaClient(server.URL, "test", false)

	textCalled := false
	resp, err := client.StreamCall(
		"system", []Message{{Role: "user", Content: "hi"}}, nil,
		func(text string) { textCalled = true },
		nil,
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if textCalled {
		t.Error("onText should not have been called for empty response")
	}
	if len(resp.Content) != 0 {
		t.Errorf("expected 0 content blocks, got %d", len(resp.Content))
	}
	if resp.StopReason != "end_turn" {
		t.Errorf("expected stop_reason 'end_turn', got %q", resp.StopReason)
	}
}

func TestStreamCall_ModelFallback(t *testing.T) {
	// Test: if done chunk has no model, falls back to configured modelID
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		writeStreamChunks(w, []ollamaResponse{
			{Message: ollamaRespMsg{Role: "assistant", Content: "hi"}, Done: false},
			{Message: ollamaRespMsg{Role: "assistant"}, Done: true, DoneReason: "stop"},
		})
	}))
	defer server.Close()

	client := NewOllamaClient(server.URL, "my-model", false)
	resp, err := client.StreamCall(
		"system", []Message{{Role: "user", Content: "hi"}}, nil,
		nil, nil,
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Model != "my-model" {
		t.Errorf("expected model 'my-model', got %q", resp.Model)
	}
}

func TestStreamCall_MultipleToolCalls(t *testing.T) {
	// Test: multiple tool calls in a single response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		writeStreamChunks(w, []ollamaResponse{
			{
				Model: "test",
				Message: ollamaRespMsg{
					Role: "assistant",
					ToolCalls: []ollamaToolCall{
						{Function: ollamaFunctionCall{Name: "read_file", Arguments: map[string]interface{}{"path": "a.go"}}},
						{Function: ollamaFunctionCall{Name: "read_file", Arguments: map[string]interface{}{"path": "b.go"}}},
					},
				},
				Done:       true,
				DoneReason: "stop",
			},
		})
	}))
	defer server.Close()

	client := NewOllamaClient(server.URL, "test", false)
	resp, err := client.StreamCall(
		"system", []Message{{Role: "user", Content: "read both"}}, nil,
		nil, nil,
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Content) != 2 {
		t.Fatalf("expected 2 tool_use blocks, got %d", len(resp.Content))
	}
	if resp.Content[0].ID != "ollama_0" || resp.Content[1].ID != "ollama_1" {
		t.Errorf("expected synthetic IDs ollama_0/ollama_1, got %q/%q",
			resp.Content[0].ID, resp.Content[1].ID)
	}
	if resp.Content[0].Name != "read_file" || resp.Content[1].Name != "read_file" {
		t.Errorf("unexpected tool names")
	}
}

func TestStreamCall_ConnectionRefused(t *testing.T) {
	client := NewOllamaClient("http://localhost:1", "test", false)
	_, err := client.StreamCall(
		"system", []Message{{Role: "user", Content: "hi"}}, nil,
		nil, nil,
	)

	if err == nil {
		t.Fatal("expected error for connection refused")
	}
	if !contains(err.Error(), "failed to send request to Ollama") {
		t.Errorf("expected connection error, got: %q", err.Error())
	}
}

func TestStreamCall_RequestFormat(t *testing.T) {
	// Verify the streaming request sends stream=true and correct format
	var capturedReq ollamaRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&capturedReq)
		w.Header().Set("Content-Type", "application/x-ndjson")
		writeStreamChunks(w, []ollamaResponse{
			{Model: "test", Message: ollamaRespMsg{Role: "assistant", Content: "ok"}, Done: true, DoneReason: "stop"},
		})
	}))
	defer server.Close()

	client := NewOllamaClient(server.URL, "qwen3.6:27b", true)
	client.StreamCall(
		"You are helpful.",
		[]Message{{Role: "user", Content: "hello"}},
		[]Tool{{Name: "read_file", Description: "Read", InputSchema: map[string]interface{}{"type": "object"}}},
		nil, nil,
	)

	if !capturedReq.Stream {
		t.Error("expected stream=true in request")
	}
	if capturedReq.Model != "qwen3.6:27b" {
		t.Errorf("expected model 'qwen3.6:27b', got %q", capturedReq.Model)
	}
	if !capturedReq.Think {
		t.Error("expected think=true in request")
	}
	if len(capturedReq.Messages) != 2 { // system + user
		t.Errorf("expected 2 messages (system + user), got %d", len(capturedReq.Messages))
	}
	if len(capturedReq.Tools) != 1 {
		t.Errorf("expected 1 tool, got %d", len(capturedReq.Tools))
	}
}

// --- Preflight tests ---

func TestPreflight_ServerReachable_ModelFound(t *testing.T) {
	// Happy path: server is running, model is available.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("Ollama is running"))
		case "/api/tags":
			json.NewEncoder(w).Encode(ollamaTagsResponse{
				Models: []ollamaModelInfo{
					{Name: "llama3:8b"},
					{Name: "qwen3.6:27b"},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewOllamaClient(server.URL, "qwen3.6:27b", false)
	err := client.Preflight()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestPreflight_ServerReachable_ModelNotFound(t *testing.T) {
	// Server is running but the configured model is not pulled.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("Ollama is running"))
		case "/api/tags":
			json.NewEncoder(w).Encode(ollamaTagsResponse{
				Models: []ollamaModelInfo{
					{Name: "llama3:8b"},
					{Name: "codellama:13b"},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewOllamaClient(server.URL, "qwen3.6:27b", false)
	err := client.Preflight()
	if err == nil {
		t.Fatal("expected error for model not found")
	}

	// Error should contain the exact pull command
	errMsg := err.Error()
	if !contains(errMsg, "ollama pull qwen3.6:27b") {
		t.Errorf("expected 'ollama pull qwen3.6:27b' in error, got: %s", errMsg)
	}
	// Error should list available models
	if !contains(errMsg, "llama3:8b") {
		t.Errorf("expected available model 'llama3:8b' in error, got: %s", errMsg)
	}
	if !contains(errMsg, "codellama:13b") {
		t.Errorf("expected available model 'codellama:13b' in error, got: %s", errMsg)
	}
}

func TestPreflight_ServerReachable_NoModels(t *testing.T) {
	// Server is running but no models are pulled at all.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.WriteHeader(http.StatusOK)
		case "/api/tags":
			json.NewEncoder(w).Encode(ollamaTagsResponse{Models: nil})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewOllamaClient(server.URL, "qwen3.6:27b", false)
	err := client.Preflight()
	if err == nil {
		t.Fatal("expected error for no models available")
	}
	if !contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %s", err.Error())
	}
}

func TestPreflight_ServerUnreachable(t *testing.T) {
	// Server is not running. Auto-start will fail because we're using
	// a non-standard port. Verifies the error message is helpful.
	client := NewOllamaClient("http://localhost:1", "test", false)
	// Use a very short timeout so the test doesn't wait long
	client.WithPreflightTimeout(100 * time.Millisecond)

	err := client.Preflight()
	if err == nil {
		t.Fatal("expected error for unreachable server")
	}
	// The error should mention either "not found in PATH" or "not become ready"
	// or "ollama serve" — it depends on whether the ollama binary exists
	errMsg := err.Error()
	if !contains(errMsg, "ollama") {
		t.Errorf("expected error to mention 'ollama', got: %s", errMsg)
	}
}

func TestCheckConnectivity_Reachable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Ollama is running"))
	}))
	defer server.Close()

	client := NewOllamaClient(server.URL, "test", false)
	err := client.checkConnectivity()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestCheckConnectivity_Unreachable(t *testing.T) {
	client := NewOllamaClient("http://localhost:1", "test", false)
	err := client.checkConnectivity()
	if err == nil {
		t.Fatal("expected error for unreachable server")
	}
}

func TestCheckConnectivity_NonOKStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewOllamaClient(server.URL, "test", false)
	err := client.checkConnectivity()
	if err == nil {
		t.Fatal("expected error for non-OK status")
	}
	if !contains(err.Error(), "unexpected status") {
		t.Errorf("expected 'unexpected status' in error, got: %s", err.Error())
	}
}

func TestCheckModel_ExactMatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(ollamaTagsResponse{
			Models: []ollamaModelInfo{
				{Name: "qwen3.6:27b"},
				{Name: "llama3:8b"},
			},
		})
	}))
	defer server.Close()

	client := NewOllamaClient(server.URL, "qwen3.6:27b", false)
	err := client.checkModel()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestCheckModel_MatchesLatestTag(t *testing.T) {
	// When user configures "qwen3.6" (no tag), it should match "qwen3.6:latest".
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(ollamaTagsResponse{
			Models: []ollamaModelInfo{
				{Name: "qwen3.6:latest"},
				{Name: "llama3:8b"},
			},
		})
	}))
	defer server.Close()

	client := NewOllamaClient(server.URL, "qwen3.6", false)
	err := client.checkModel()
	if err != nil {
		t.Fatalf("expected 'qwen3.6' to match 'qwen3.6:latest', got error: %v", err)
	}
}

func TestCheckModel_NoLatestFallbackWhenTagPresent(t *testing.T) {
	// When user configures "qwen3.6:27b" (has tag), don't try ":latest" fallback.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(ollamaTagsResponse{
			Models: []ollamaModelInfo{
				{Name: "qwen3.6:latest"},
			},
		})
	}))
	defer server.Close()

	client := NewOllamaClient(server.URL, "qwen3.6:27b", false)
	err := client.checkModel()
	if err == nil {
		t.Fatal("expected error — 'qwen3.6:27b' should NOT match 'qwen3.6:latest'")
	}
}

func TestCheckModel_NotFound_ShowsAvailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(ollamaTagsResponse{
			Models: []ollamaModelInfo{
				{Name: "llama3:8b"},
				{Name: "codellama:13b"},
				{Name: "mistral:7b"},
			},
		})
	}))
	defer server.Close()

	client := NewOllamaClient(server.URL, "qwen3.6:27b", false)
	err := client.checkModel()
	if err == nil {
		t.Fatal("expected error for model not found")
	}

	errMsg := err.Error()
	// Should contain the pull command
	if !contains(errMsg, "ollama pull qwen3.6:27b") {
		t.Errorf("missing pull command in error: %s", errMsg)
	}
	// Should list all available models
	for _, model := range []string{"llama3:8b", "codellama:13b", "mistral:7b"} {
		if !contains(errMsg, model) {
			t.Errorf("missing available model %q in error: %s", model, errMsg)
		}
	}
}

func TestCheckModel_EmptyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(ollamaTagsResponse{Models: nil})
	}))
	defer server.Close()

	client := NewOllamaClient(server.URL, "qwen3.6:27b", false)
	err := client.checkModel()
	if err == nil {
		t.Fatal("expected error for empty model list")
	}
	if !contains(err.Error(), "No models are currently pulled") {
		t.Errorf("expected 'No models are currently pulled' in error, got: %s", err.Error())
	}
}

func TestCheckModel_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewOllamaClient(server.URL, "test", false)
	err := client.checkModel()
	if err == nil {
		t.Fatal("expected error for server error response")
	}
	if !contains(err.Error(), "status 500") {
		t.Errorf("expected status 500 in error, got: %s", err.Error())
	}
}

func TestCheckModel_MalformedJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not json"))
	}))
	defer server.Close()

	client := NewOllamaClient(server.URL, "test", false)
	err := client.checkModel()
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
	if !contains(err.Error(), "failed to parse") {
		t.Errorf("expected 'failed to parse' in error, got: %s", err.Error())
	}
}

func TestWithPreflightTimeout(t *testing.T) {
	client := NewOllamaClient("http://localhost:11434", "test", false)

	// Default should be 15 seconds
	if client.preflightTimeout != 15*time.Second {
		t.Errorf("expected default timeout 15s, got %s", client.preflightTimeout)
	}

	// WithPreflightTimeout should override
	client.WithPreflightTimeout(30 * time.Second)
	if client.preflightTimeout != 30*time.Second {
		t.Errorf("expected timeout 30s, got %s", client.preflightTimeout)
	}
}

func TestPreflight_FastWhenAlreadyRunning(t *testing.T) {
	// When Ollama is already running, preflight should complete quickly.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.WriteHeader(http.StatusOK)
		case "/api/tags":
			json.NewEncoder(w).Encode(ollamaTagsResponse{
				Models: []ollamaModelInfo{{Name: "qwen3.6:27b"}},
			})
		}
	}))
	defer server.Close()

	client := NewOllamaClient(server.URL, "qwen3.6:27b", false)

	start := time.Now()
	err := client.Preflight()
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should complete well under 500ms (typically <10ms with local mock server)
	if elapsed > 500*time.Millisecond {
		t.Errorf("preflight took %s, expected <500ms when server is already running", elapsed)
	}
}

func TestPreflight_FullIntegration_ToolCallAfterPreflight(t *testing.T) {
	// Full integration test: preflight succeeds, then a normal API call works.
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.WriteHeader(http.StatusOK)
		case "/api/tags":
			json.NewEncoder(w).Encode(ollamaTagsResponse{
				Models: []ollamaModelInfo{{Name: "qwen3.6:27b"}},
			})
		case "/api/chat":
			callCount++
			resp := ollamaResponse{
				Model: "qwen3.6:27b",
				Message: ollamaRespMsg{
					Role:    "assistant",
					Content: "Hello after preflight!",
				},
				Done:            true,
				DoneReason:      "stop",
				PromptEvalCount: 42,
				EvalCount:       10,
			}
			json.NewEncoder(w).Encode(resp)
		}
	}))
	defer server.Close()

	client := NewOllamaClient(server.URL, "qwen3.6:27b", false)

	// Preflight
	if err := client.Preflight(); err != nil {
		t.Fatalf("preflight failed: %v", err)
	}

	// Normal API call
	resp, err := client.Call("system", []Message{{Role: "user", Content: "hi"}}, nil)
	if err != nil {
		t.Fatalf("call failed after preflight: %v", err)
	}
	if len(resp.Content) != 1 || resp.Content[0].Text != "Hello after preflight!" {
		t.Errorf("unexpected response: %+v", resp.Content)
	}
	if callCount != 1 {
		t.Errorf("expected 1 API call, got %d", callCount)
	}
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
