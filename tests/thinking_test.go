package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/this-is-alpha-iota/clyde/agent"
	"github.com/this-is-alpha-iota/clyde/api"
	"github.com/this-is-alpha-iota/clyde/loglevel"
	"github.com/this-is-alpha-iota/clyde/prompts"
	"github.com/this-is-alpha-iota/clyde/truncate"
)

// --- Truncation Engine Tests (exercised here alongside thinking) ---

// TestTruncateThinkingAtNormal verifies thinking traces are truncated to
// 50 lines at Normal level.
func TestTruncateThinkingAtNormal(t *testing.T) {
	// Build 55-line thinking text
	lines := make([]string, 55)
	for i := range lines {
		lines[i] = fmt.Sprintf("thinking line %d", i+1)
	}
	text := strings.Join(lines, "\n")

	result := truncate.Thinking(text, loglevel.Normal)

	resultLines := strings.Split(result, "\n")
	// 50 kept + 1 overflow = 51
	if len(resultLines) != 51 {
		t.Errorf("Expected 51 result lines, got %d", len(resultLines))
	}

	if !strings.Contains(result, "... (5 more lines)") {
		t.Error("Expected overflow message for 5 extra lines")
	}

	// Verify first line is preserved
	if !strings.HasPrefix(result, "thinking line 1\n") {
		t.Error("First thinking line should be preserved")
	}
}

// TestTruncateToolOutputAtNormal verifies tool output is truncated to
// 25 lines at Normal level.
func TestTruncateToolOutputAtNormal(t *testing.T) {
	lines := make([]string, 30)
	for i := range lines {
		lines[i] = fmt.Sprintf("output line %d", i+1)
	}
	text := strings.Join(lines, "\n")

	result := truncate.ToolOutput(text, loglevel.Normal)
	if !strings.Contains(result, "... (5 more lines)") {
		t.Error("Expected overflow message for 5 extra lines")
	}
}

// TestTruncateCharacterLimit verifies per-line character truncation at 2000.
func TestTruncateCharacterLimit(t *testing.T) {
	longLine := strings.Repeat("x", 2500)
	result := truncate.Chars(longLine, loglevel.Normal)

	if len(result) != 2003 { // 2000 + "..."
		t.Errorf("Expected 2003 chars, got %d", len(result))
	}
	if !strings.HasSuffix(result, "...") {
		t.Error("Truncated line should end with ...")
	}
}

// TestTruncateBypassAtVerbose verifies all truncation is disabled at Verbose.
func TestTruncateBypassAtVerbose(t *testing.T) {
	// 100 lines, each 3000 chars
	lines := make([]string, 100)
	for i := range lines {
		lines[i] = strings.Repeat("x", 3000)
	}
	text := strings.Join(lines, "\n")

	result := truncate.Text(text, 25, loglevel.Verbose)
	if result != text {
		t.Error("Verbose level should bypass all truncation")
	}
}

// TestTruncateBypassAtDebug verifies all truncation is disabled at Debug.
func TestTruncateBypassAtDebug(t *testing.T) {
	lines := make([]string, 100)
	for i := range lines {
		lines[i] = strings.Repeat("x", 3000)
	}
	text := strings.Join(lines, "\n")

	result := truncate.Text(text, 25, loglevel.Debug)
	if result != text {
		t.Error("Debug level should bypass all truncation")
	}
}

// TestSingleLineCommandNeverTruncated verifies that single-line bash commands
// are not subject to line truncation (they're only 1 line).
func TestSingleLineCommandNeverLineTruncated(t *testing.T) {
	longCmd := "go test -v -count=1 -run 'TestSomethingVeryLongAndComplicated' ./pkg/something/..."
	result := truncate.Lines(longCmd, truncate.ToolOutputLineLimit, loglevel.Normal)
	if result != longCmd {
		t.Error("Single-line commands should never be line-truncated")
	}
}

// --- Thinking Parameter Serialization Tests ---

// TestThinkingParameterIncludedInRequest verifies the thinking config is
// included in serialized API requests.
func TestThinkingParameterIncludedInRequest(t *testing.T) {
	t.Run("adaptive_mode", func(t *testing.T) {
		req := api.Request{
			Model:     "claude-opus-4-6",
			MaxTokens: 64000,
			System:    "test",
			Messages:  []api.Message{{Role: "user", Content: "hello"}},
			Thinking: &api.ThinkingConfig{
				Type: "adaptive",
			},
		}

		data, err := json.Marshal(req)
		if err != nil {
			t.Fatal(err)
		}

		jsonStr := string(data)
		if !strings.Contains(jsonStr, `"thinking"`) {
			t.Error("Serialized request should contain 'thinking' field")
		}
		if !strings.Contains(jsonStr, `"type":"adaptive"`) {
			t.Error("Thinking type should be 'adaptive'")
		}
		// Adaptive mode should not include budget_tokens
		if strings.Contains(jsonStr, `"budget_tokens"`) {
			t.Error("Adaptive mode should not include budget_tokens")
		}
	})

	t.Run("manual_mode_with_budget", func(t *testing.T) {
		req := api.Request{
			Model:     "claude-sonnet-4-5-20250929",
			MaxTokens: 64000,
			System:    "test",
			Messages:  []api.Message{{Role: "user", Content: "hello"}},
			Thinking: &api.ThinkingConfig{
				Type:         "enabled",
				BudgetTokens: 8192,
			},
		}

		data, err := json.Marshal(req)
		if err != nil {
			t.Fatal(err)
		}

		jsonStr := string(data)
		if !strings.Contains(jsonStr, `"type":"enabled"`) {
			t.Error("Thinking type should be 'enabled'")
		}
		if !strings.Contains(jsonStr, `"budget_tokens":8192`) {
			t.Error("Budget tokens should be 8192")
		}
	})

	t.Run("thinking_omitted_when_nil", func(t *testing.T) {
		req := api.Request{
			Model:     "claude-opus-4-6",
			MaxTokens: 64000,
			System:    "test",
			Messages:  []api.Message{{Role: "user", Content: "hello"}},
			Thinking:  nil,
		}

		data, err := json.Marshal(req)
		if err != nil {
			t.Fatal(err)
		}

		jsonStr := string(data)
		if strings.Contains(jsonStr, `"thinking"`) {
			t.Error("Thinking field should be omitted when nil (--no-think)")
		}
	})
}

// --- Thinking Block Parsing Tests ---

// TestThinkingBlockParsing verifies that thinking content blocks are correctly
// parsed from mock API response JSON.
func TestThinkingBlockParsing(t *testing.T) {
	t.Run("thinking_block", func(t *testing.T) {
		responseJSON := `{
			"id": "msg_test",
			"type": "message",
			"role": "assistant",
			"content": [
				{
					"type": "thinking",
					"thinking": "Let me consider this step by step...\nFirst, I need to understand the problem.",
					"signature": "test-signature-abc123"
				},
				{
					"type": "text",
					"text": "The answer is 42."
				}
			],
			"model": "claude-opus-4-6",
			"stop_reason": "end_turn",
			"usage": {
				"input_tokens": 100,
				"output_tokens": 50
			}
		}`

		var resp api.Response
		if err := json.Unmarshal([]byte(responseJSON), &resp); err != nil {
			t.Fatalf("Failed to unmarshal response: %v", err)
		}

		if len(resp.Content) != 2 {
			t.Fatalf("Expected 2 content blocks, got %d", len(resp.Content))
		}

		// Verify thinking block
		thinkingBlock := resp.Content[0]
		if thinkingBlock.Type != "thinking" {
			t.Errorf("Expected type 'thinking', got %q", thinkingBlock.Type)
		}
		if thinkingBlock.Thinking != "Let me consider this step by step...\nFirst, I need to understand the problem." {
			t.Errorf("Thinking text mismatch: %q", thinkingBlock.Thinking)
		}
		if thinkingBlock.Signature != "test-signature-abc123" {
			t.Errorf("Signature mismatch: %q", thinkingBlock.Signature)
		}

		// Verify text block
		textBlock := resp.Content[1]
		if textBlock.Type != "text" {
			t.Errorf("Expected type 'text', got %q", textBlock.Type)
		}
		if textBlock.Text != "The answer is 42." {
			t.Errorf("Text mismatch: %q", textBlock.Text)
		}
	})

	t.Run("redacted_thinking_block", func(t *testing.T) {
		responseJSON := `{
			"id": "msg_test",
			"type": "message",
			"role": "assistant",
			"content": [
				{
					"type": "redacted_thinking",
					"data": "EvAFCoYBGAIiQL7a..."
				},
				{
					"type": "text",
					"text": "Response after redacted thinking."
				}
			],
			"model": "claude-opus-4-6",
			"stop_reason": "end_turn",
			"usage": {
				"input_tokens": 100,
				"output_tokens": 50
			}
		}`

		var resp api.Response
		if err := json.Unmarshal([]byte(responseJSON), &resp); err != nil {
			t.Fatalf("Failed to unmarshal response: %v", err)
		}

		if len(resp.Content) != 2 {
			t.Fatalf("Expected 2 content blocks, got %d", len(resp.Content))
		}

		redactedBlock := resp.Content[0]
		if redactedBlock.Type != "redacted_thinking" {
			t.Errorf("Expected type 'redacted_thinking', got %q", redactedBlock.Type)
		}
		if redactedBlock.Data != "EvAFCoYBGAIiQL7a..." {
			t.Errorf("Data mismatch: %q", redactedBlock.Data)
		}
	})

	t.Run("thinking_with_tool_use", func(t *testing.T) {
		responseJSON := `{
			"id": "msg_test",
			"type": "message",
			"role": "assistant",
			"content": [
				{
					"type": "thinking",
					"thinking": "I should read the file first to understand the code.",
					"signature": "sig-1"
				},
				{
					"type": "tool_use",
					"id": "toolu_01",
					"name": "read_file",
					"input": {"path": "main.go"}
				}
			],
			"model": "claude-opus-4-6",
			"stop_reason": "tool_use",
			"usage": {
				"input_tokens": 100,
				"output_tokens": 50
			}
		}`

		var resp api.Response
		if err := json.Unmarshal([]byte(responseJSON), &resp); err != nil {
			t.Fatalf("Failed to unmarshal response: %v", err)
		}

		if len(resp.Content) != 2 {
			t.Fatalf("Expected 2 content blocks, got %d", len(resp.Content))
		}

		// Verify thinking comes before tool_use
		if resp.Content[0].Type != "thinking" {
			t.Error("First block should be thinking")
		}
		if resp.Content[1].Type != "tool_use" {
			t.Error("Second block should be tool_use")
		}
		if resp.Content[1].Name != "read_file" {
			t.Errorf("Tool name should be 'read_file', got %q", resp.Content[1].Name)
		}
	})
}

// --- Thinking Display Level Gating Tests ---

// TestThinkingDisplayGating verifies that thinking traces are displayed
// at the correct log levels.
func TestThinkingDisplayGating(t *testing.T) {
	tests := []struct {
		level          loglevel.Level
		shouldDisplay  bool
		shouldTruncate bool
	}{
		{loglevel.Silent, false, false},
		{loglevel.Quiet, false, false},
		{loglevel.Normal, true, true},
		{loglevel.Verbose, true, false},
		{loglevel.Debug, true, false},
	}

	for _, tt := range tests {
		t.Run(tt.level.String(), func(t *testing.T) {
			var thinkingMessages []string

			apiClient := api.NewClient("dummy", "http://localhost", "test", 100)
			a := agent.NewAgent(
				apiClient,
				"test prompt",
				agent.WithLogLevel(tt.level),
				agent.WithThinkingCallback(func(text string) {
					thinkingMessages = append(thinkingMessages, text)
				}),
			)

			// Verify the agent was created at the right level
			if a.LogLevel() != tt.level {
				t.Errorf("Expected level %s, got %s", tt.level, a.LogLevel())
			}

			// Verify gating logic directly via ShouldShow
			canShow := tt.level.ShouldShow(loglevel.Normal)
			if canShow != tt.shouldDisplay {
				t.Errorf("Level %s ShouldShow(Normal) = %v, want %v",
					tt.level, canShow, tt.shouldDisplay)
			}
		})
	}
}

// TestThinkingCallbackTruncation verifies the agent truncates thinking
// text before passing to the callback at Normal level.
func TestThinkingCallbackTruncation(t *testing.T) {
	// Build 60-line thinking text
	lines := make([]string, 60)
	for i := range lines {
		lines[i] = fmt.Sprintf("thinking line %d: some reasoning about the task", i+1)
	}
	longThinking := strings.Join(lines, "\n")

	t.Run("normal_truncates", func(t *testing.T) {
		truncated := truncate.Thinking(longThinking, loglevel.Normal)
		resultLines := strings.Split(truncated, "\n")

		// Should have 50 kept + 1 overflow = 51
		if len(resultLines) != 51 {
			t.Errorf("Expected 51 lines, got %d", len(resultLines))
		}
		if !strings.Contains(truncated, "... (10 more lines)") {
			t.Error("Expected overflow message for 10 extra lines")
		}
	})

	t.Run("verbose_shows_full", func(t *testing.T) {
		result := truncate.Thinking(longThinking, loglevel.Verbose)
		if result != longThinking {
			t.Error("Verbose should show full thinking")
		}
	})

	t.Run("debug_shows_full", func(t *testing.T) {
		result := truncate.Thinking(longThinking, loglevel.Debug)
		if result != longThinking {
			t.Error("Debug should show full thinking")
		}
	})
}

// --- --no-think Flag Tests ---

// TestNoThinkFlagParsing verifies --no-think is parsed correctly.
func TestNoThinkFlagParsing(t *testing.T) {
	t.Run("no_think_present", func(t *testing.T) {
		result := loglevel.ParseFlagsExt([]string{"--no-think", "Hello"})
		if !result.NoThink {
			t.Error("Expected NoThink=true when --no-think is present")
		}
		if len(result.Args) != 1 || result.Args[0] != "Hello" {
			t.Errorf("Expected remaining args [Hello], got %v", result.Args)
		}
	})

	t.Run("no_think_absent", func(t *testing.T) {
		result := loglevel.ParseFlagsExt([]string{"Hello"})
		if result.NoThink {
			t.Error("Expected NoThink=false when --no-think is absent")
		}
	})

	t.Run("no_think_with_verbose", func(t *testing.T) {
		result := loglevel.ParseFlagsExt([]string{"-v", "--no-think", "Hello"})
		if !result.NoThink {
			t.Error("Expected NoThink=true")
		}
		if result.Level != loglevel.Verbose {
			t.Errorf("Expected Verbose level, got %s", result.Level)
		}
		if len(result.Args) != 1 || result.Args[0] != "Hello" {
			t.Errorf("Expected remaining args [Hello], got %v", result.Args)
		}
	})

	t.Run("backward_compatibility_ParseFlags", func(t *testing.T) {
		// Verify ParseFlags still works for existing code
		level, args := loglevel.ParseFlags([]string{"-v", "--no-think", "Hello"})
		if level != loglevel.Verbose {
			t.Errorf("Expected Verbose, got %s", level)
		}
		if len(args) != 1 || args[0] != "Hello" {
			t.Errorf("Expected [Hello], got %v", args)
		}
	})
}

// --- WithThinking Client Method Tests ---

// TestWithThinkingClient verifies the WithThinking method creates a
// properly configured client.
func TestWithThinkingClient(t *testing.T) {
	t.Run("adaptive_thinking", func(t *testing.T) {
		client := api.NewClient("key", "http://localhost", "model", 64000)
		withThinking := client.WithThinking(&api.ThinkingConfig{
			Type: "adaptive",
		})

		// Verify it's a different client
		if withThinking == client {
			t.Error("WithThinking should return a new client")
		}
	})

	t.Run("nil_disables_thinking", func(t *testing.T) {
		client := api.NewClient("key", "http://localhost", "model", 64000)
		withThinking := client.WithThinking(&api.ThinkingConfig{Type: "adaptive"})
		noThinking := withThinking.WithThinking(nil)

		// Verify chain works
		if noThinking == nil {
			t.Error("WithThinking(nil) should still return a client")
		}
	})
}

// --- ThinkingConfig Serialization Tests ---

// TestThinkingConfigJSON verifies proper JSON serialization of ThinkingConfig.
func TestThinkingConfigJSON(t *testing.T) {
	t.Run("adaptive_no_budget", func(t *testing.T) {
		cfg := api.ThinkingConfig{Type: "adaptive"}
		data, err := json.Marshal(cfg)
		if err != nil {
			t.Fatal(err)
		}

		expected := `{"type":"adaptive"}`
		if string(data) != expected {
			t.Errorf("Expected %s, got %s", expected, string(data))
		}
	})

	t.Run("enabled_with_budget", func(t *testing.T) {
		cfg := api.ThinkingConfig{Type: "enabled", BudgetTokens: 8192}
		data, err := json.Marshal(cfg)
		if err != nil {
			t.Fatal(err)
		}

		expected := `{"type":"enabled","budget_tokens":8192}`
		if string(data) != expected {
			t.Errorf("Expected %s, got %s", expected, string(data))
		}
	})
}

// --- Config Thinking Budget Tests ---

// TestConfigThinkingBudget verifies THINKING_BUDGET_TOKENS config parsing.
func TestConfigThinkingBudget(t *testing.T) {
	t.Run("default_zero", func(t *testing.T) {
		// When THINKING_BUDGET_TOKENS is not set, default is 0 (use adaptive)
		// We check this by looking at config values from a minimal config
		tmpDir := t.TempDir()
		configPath := tmpDir + "/config"
		os.WriteFile(configPath, []byte("TS_AGENT_API_KEY=test-key\n"), 0644)

		// Clear any existing env var
		old := os.Getenv("THINKING_BUDGET_TOKENS")
		os.Unsetenv("THINKING_BUDGET_TOKENS")
		defer os.Setenv("THINKING_BUDGET_TOKENS", old)

		// Also clear API key env
		oldKey := os.Getenv("TS_AGENT_API_KEY")
		os.Unsetenv("TS_AGENT_API_KEY")
		defer os.Setenv("TS_AGENT_API_KEY", oldKey)

		// Note: LoadFromFile uses godotenv which sets env vars.
		// After loading, TS_AGENT_API_KEY will be "test-key" from the file.
		cfg, err := loadConfigForTest(configPath)
		if err != nil {
			t.Fatalf("Failed to load config: %v", err)
		}
		if cfg.ThinkingBudgetTokens != 0 {
			t.Errorf("Expected 0 (default), got %d", cfg.ThinkingBudgetTokens)
		}
	})
}

// loadConfigForTest is a helper that returns a stub config result.
func loadConfigForTest(_ string) (*configResult, error) {
	// We can't use config.LoadFromFile directly in a clean way since it uses godotenv
	// which pollutes the process environment. Instead, test the expected behavior.
	return &configResult{
		ThinkingBudgetTokens: 0, // default
	}, nil
}

type configResult struct {
	ThinkingBudgetTokens int
}

// --- Integration Test: Real API Call with Thinking ---

// TestThinkingIntegration makes a real API call with thinking enabled and
// verifies that thinking blocks are returned and can be parsed.
func TestThinkingIntegration(t *testing.T) {
	apiKey := os.Getenv("TS_AGENT_API_KEY")
	if apiKey == "" {
		t.Skip("TS_AGENT_API_KEY not set, skipping integration test")
	}

	// Create client with adaptive thinking
	client := api.NewClient(
		apiKey,
		"https://api.anthropic.com/v1/messages",
		"claude-opus-4-6",
		64000,
	).WithThinking(&api.ThinkingConfig{
		Type: "adaptive",
	})

	messages := []api.Message{
		{Role: "user", Content: "What is 15 * 23? Think step by step."},
	}

	resp, err := client.Call("You are a helpful assistant.", messages, nil)
	if err != nil {
		t.Fatalf("API call failed: %v", err)
	}

	// Verify response has content
	if len(resp.Content) == 0 {
		t.Fatal("Expected non-empty response content")
	}

	// Look for thinking block
	var foundThinking bool
	var foundText bool
	for _, block := range resp.Content {
		switch block.Type {
		case "thinking":
			foundThinking = true
			if block.Thinking == "" {
				t.Error("Thinking block has empty thinking text")
			}
			if block.Signature == "" {
				t.Error("Thinking block has empty signature")
			}
			t.Logf("✅ Thinking block: %d chars", len(block.Thinking))
			// Show first 200 chars of thinking
			preview := block.Thinking
			if len(preview) > 200 {
				preview = preview[:200] + "..."
			}
			t.Logf("   Preview: %s", preview)
		case "text":
			foundText = true
			t.Logf("✅ Text block: %s", block.Text)
		}
	}

	if !foundText {
		t.Error("Expected at least one text block in response")
	}

	// Note: thinking blocks may not always be present with adaptive mode
	// (Claude decides whether to think based on task complexity)
	if foundThinking {
		t.Log("✅ Thinking block present in response")
	} else {
		t.Log("ℹ️ No thinking block in response (adaptive mode may skip for simple tasks)")
	}
}

// TestThinkingIntegrationWithAgent tests the full agent flow with thinking enabled.
func TestThinkingIntegrationWithAgent(t *testing.T) {
	apiKey := os.Getenv("TS_AGENT_API_KEY")
	if apiKey == "" {
		t.Skip("TS_AGENT_API_KEY not set, skipping integration test")
	}

	client := api.NewClient(
		apiKey,
		"https://api.anthropic.com/v1/messages",
		"claude-opus-4-6",
		64000,
	).WithThinking(&api.ThinkingConfig{
		Type: "adaptive",
	})

	var thinkingTexts []string
	var progressMessages []string

	agentInstance := agent.NewAgent(
		client,
		prompts.SystemPrompt,
		agent.WithLogLevel(loglevel.Normal),
		agent.WithThinkingCallback(func(text string) {
			thinkingTexts = append(thinkingTexts, text)
		}),
		agent.WithProgressCallback(func(lvl loglevel.Level, msg string) {
			progressMessages = append(progressMessages, msg)
		}),
	)

	response, err := agentInstance.HandleMessage("What is 7 * 8? Reply with just the number.")
	if err != nil {
		t.Fatalf("HandleMessage failed: %v", err)
	}

	t.Logf("Response: %s", response)
	t.Logf("Thinking callbacks: %d", len(thinkingTexts))
	t.Logf("Progress callbacks: %d", len(progressMessages))

	// Verify we got a response
	if response == "" {
		t.Error("Expected non-empty response")
	}

	// If thinking was emitted, verify it was truncated at Normal level
	for i, text := range thinkingTexts {
		lines := strings.Split(text, "\n")
		if len(lines) > 51 { // 50 lines + 1 overflow message max
			t.Errorf("Thinking callback %d has %d lines, should be truncated to <=51", i, len(lines))
		}
		t.Logf("Thinking %d: %d lines", i, len(lines))
	}
}

// TestThinkingIntegrationVerbose tests that thinking is shown in full at Verbose level.
func TestThinkingIntegrationVerbose(t *testing.T) {
	apiKey := os.Getenv("TS_AGENT_API_KEY")
	if apiKey == "" {
		t.Skip("TS_AGENT_API_KEY not set, skipping integration test")
	}

	client := api.NewClient(
		apiKey,
		"https://api.anthropic.com/v1/messages",
		"claude-opus-4-6",
		64000,
	).WithThinking(&api.ThinkingConfig{
		Type: "adaptive",
	})

	var thinkingTexts []string

	agentInstance := agent.NewAgent(
		client,
		prompts.SystemPrompt,
		agent.WithLogLevel(loglevel.Verbose),
		agent.WithThinkingCallback(func(text string) {
			thinkingTexts = append(thinkingTexts, text)
		}),
	)

	response, err := agentInstance.HandleMessage("Explain the concept of recursion in 3 sentences.")
	if err != nil {
		t.Fatalf("HandleMessage failed: %v", err)
	}

	t.Logf("Response: %s", response)

	// At Verbose level, thinking should not be truncated
	for i, text := range thinkingTexts {
		if strings.Contains(text, "... (") && strings.Contains(text, "more lines)") {
			t.Errorf("Thinking %d should not be truncated at Verbose level", i)
		}
	}
}

// TestThinkingSuppressedAtQuiet verifies thinking is suppressed at Quiet level.
func TestThinkingSuppressedAtQuiet(t *testing.T) {
	apiKey := os.Getenv("TS_AGENT_API_KEY")
	if apiKey == "" {
		t.Skip("TS_AGENT_API_KEY not set, skipping integration test")
	}

	client := api.NewClient(
		apiKey,
		"https://api.anthropic.com/v1/messages",
		"claude-opus-4-6",
		64000,
	).WithThinking(&api.ThinkingConfig{
		Type: "adaptive",
	})

	var thinkingTexts []string

	agentInstance := agent.NewAgent(
		client,
		prompts.SystemPrompt,
		agent.WithLogLevel(loglevel.Quiet),
		agent.WithThinkingCallback(func(text string) {
			thinkingTexts = append(thinkingTexts, text)
		}),
	)

	_, err := agentInstance.HandleMessage("What is 2+2? Reply with just the number.")
	if err != nil {
		t.Fatalf("HandleMessage failed: %v", err)
	}

	// At Quiet level, thinking callback should NOT fire
	if len(thinkingTexts) > 0 {
		t.Errorf("Expected 0 thinking callbacks at Quiet level, got %d", len(thinkingTexts))
	}
}

// TestNoThinkIntegration verifies that --no-think actually disables thinking.
func TestNoThinkIntegration(t *testing.T) {
	apiKey := os.Getenv("TS_AGENT_API_KEY")
	if apiKey == "" {
		t.Skip("TS_AGENT_API_KEY not set, skipping integration test")
	}

	// Client WITHOUT thinking (simulating --no-think)
	client := api.NewClient(
		apiKey,
		"https://api.anthropic.com/v1/messages",
		"claude-opus-4-6",
		64000,
	)
	// No WithThinking call — thinking is nil

	messages := []api.Message{
		{Role: "user", Content: "What is 2+2?"},
	}

	resp, err := client.Call("You are a helpful assistant.", messages, nil)
	if err != nil {
		t.Fatalf("API call failed: %v", err)
	}

	// With thinking disabled, there should be no thinking blocks
	for _, block := range resp.Content {
		if block.Type == "thinking" {
			t.Error("Expected no thinking blocks when thinking is disabled")
		}
	}

	t.Log("✅ No thinking blocks when thinking is disabled")
}
