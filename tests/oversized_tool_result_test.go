package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/superbuilders/clyde/agent"
	"github.com/superbuilders/clyde/agent/providers"
	"github.com/superbuilders/clyde/agent/session"
)

// --- Fix: Tool calls that flood the context ---

// TestGuardOversizedToolResults_SmallResult verifies that small tool results
// pass through unchanged when the context window has ample room.
func TestGuardOversizedToolResults_SmallResult(t *testing.T) {
	client := providers.NewClient("fake", "http://localhost", "m", 1000)
	a := agent.NewAgent(client, "test",
		agent.WithContextWindowSize(200000),
		agent.WithReserveTokens(16000),
	)

	// Simulate small tool output — should NOT be replaced
	smallContent := "total 5 files\ndrwxr-xr-x  2 user group  4096 main.go"
	result := a.GuardOversizedToolResults([]providers.ContentBlock{
		{Type: "tool_result", ToolUseID: "toolu_1", Content: smallContent},
	})

	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	content, ok := result[0].Content.(string)
	if !ok {
		t.Fatal("content should be a string")
	}
	if content != smallContent {
		t.Errorf("small result should pass through unchanged, got: %q", content)
	}
	if result[0].IsError {
		t.Error("small result should not be marked as error")
	}
}

// TestGuardOversizedToolResults_HugeResult verifies that a tool result
// larger than the context window is replaced with an error message.
func TestGuardOversizedToolResults_HugeResult(t *testing.T) {
	client := providers.NewClient("fake", "http://localhost", "m", 1000)
	a := agent.NewAgent(client, "test",
		agent.WithContextWindowSize(10000), // small window for testing
		agent.WithReserveTokens(2000),
	)

	// Create a tool result that's way too large (~50k tokens at 3.5 chars/token)
	hugeContent := strings.Repeat("x", 200000)
	result := a.GuardOversizedToolResults([]providers.ContentBlock{
		{Type: "tool_result", ToolUseID: "toolu_1", Content: hugeContent},
	})

	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	content, ok := result[0].Content.(string)
	if !ok {
		t.Fatal("content should be a string")
	}
	if !strings.Contains(content, "ERROR: Tool result too large") {
		t.Errorf("oversized result should be replaced with error, got: %q", content[:100])
	}
	if !result[0].IsError {
		t.Error("oversized result should be marked as error")
	}
	// Should contain helpful suggestions
	if !strings.Contains(content, "head/tail/grep") {
		t.Error("error message should suggest head/tail/grep")
	}
	if !strings.Contains(content, "redirect output to a file") {
		t.Error("error message should suggest redirecting to file")
	}
}

// TestGuardOversizedToolResults_ExceedsHalfContextWindow verifies the hard
// ceiling: a single result cannot exceed 50% of the context window even if
// there's technically "room" (because the context is currently empty).
func TestGuardOversizedToolResults_ExceedsHalfContextWindow(t *testing.T) {
	client := providers.NewClient("fake", "http://localhost", "m", 1000)
	a := agent.NewAgent(client, "test",
		agent.WithContextWindowSize(100000), // 100k tokens
		agent.WithReserveTokens(16000),
	)
	// lastUsage is zero, so remaining = 100000 - 0 - 16000 = 84000
	// But hard ceiling = 50000 (50% of 100k)
	// Create content that's ~60k tokens (210k chars): exceeds 50k ceiling
	bigContent := strings.Repeat("x", 210000) // ~60k tokens
	result := a.GuardOversizedToolResults([]providers.ContentBlock{
		{Type: "tool_result", ToolUseID: "toolu_1", Content: bigContent},
	})

	content, _ := result[0].Content.(string)
	if !strings.Contains(content, "ERROR: Tool result too large") {
		t.Error("result exceeding 50% of context window should be replaced")
	}
}

// TestGuardOversizedToolResults_NoContextWindow verifies that the guard
// is a no-op when contextWindowSize is not configured.
func TestGuardOversizedToolResults_NoContextWindow(t *testing.T) {
	client := providers.NewClient("fake", "http://localhost", "m", 1000)
	a := agent.NewAgent(client, "test") // no context window size

	hugeContent := strings.Repeat("x", 1000000)
	result := a.GuardOversizedToolResults([]providers.ContentBlock{
		{Type: "tool_result", ToolUseID: "toolu_1", Content: hugeContent},
	})

	content, _ := result[0].Content.(string)
	if content != hugeContent {
		t.Error("without context window config, guard should be a no-op")
	}
}

// TestGuardOversizedToolResults_NonToolResultBlocksIgnored verifies that
// non-tool_result blocks (like images) are never modified.
func TestGuardOversizedToolResults_NonToolResultBlocksIgnored(t *testing.T) {
	client := providers.NewClient("fake", "http://localhost", "m", 1000)
	a := agent.NewAgent(client, "test",
		agent.WithContextWindowSize(10000),
		agent.WithReserveTokens(2000),
	)

	result := a.GuardOversizedToolResults([]providers.ContentBlock{
		{Type: "image", Source: &providers.ImageSource{Type: "base64", MediaType: "image/png", Data: strings.Repeat("x", 100000)}},
	})

	if result[0].Type != "image" {
		t.Error("image blocks should pass through unchanged")
	}
}

// TestGuardOversizedToolResults_DiagnosticEmitted verifies that a diagnostic
// message is emitted when a tool result is discarded.
func TestGuardOversizedToolResults_DiagnosticEmitted(t *testing.T) {
	client := providers.NewClient("fake", "http://localhost", "m", 1000)

	var diagnostics []string
	a := agent.NewAgent(client, "test",
		agent.WithContextWindowSize(10000),
		agent.WithReserveTokens(2000),
		agent.WithDiagnosticCallback(func(msg string) {
			diagnostics = append(diagnostics, msg)
		}),
	)

	hugeContent := strings.Repeat("x", 200000)
	a.GuardOversizedToolResults([]providers.ContentBlock{
		{Type: "tool_result", ToolUseID: "toolu_1", Content: hugeContent},
	})

	found := false
	for _, d := range diagnostics {
		if strings.Contains(d, "⚠️ Tool result discarded") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected diagnostic about discarded tool result, got: %v", diagnostics)
	}
}

// TestGuardOversizedToolResults_MultipleResults verifies that only the
// oversized results are replaced; others pass through.
func TestGuardOversizedToolResults_MultipleResults(t *testing.T) {
	client := providers.NewClient("fake", "http://localhost", "m", 1000)
	a := agent.NewAgent(client, "test",
		agent.WithContextWindowSize(10000),
		agent.WithReserveTokens(2000),
	)

	smallContent := "small output"
	hugeContent := strings.Repeat("x", 200000)

	results := a.GuardOversizedToolResults([]providers.ContentBlock{
		{Type: "tool_result", ToolUseID: "toolu_1", Content: smallContent},
		{Type: "tool_result", ToolUseID: "toolu_2", Content: hugeContent},
	})

	// First should be unchanged
	c1, _ := results[0].Content.(string)
	if c1 != smallContent {
		t.Error("small result should pass through unchanged")
	}
	if results[0].IsError {
		t.Error("small result should not be marked as error")
	}

	// Second should be replaced
	c2, _ := results[1].Content.(string)
	if !strings.Contains(c2, "ERROR: Tool result too large") {
		t.Error("huge result should be replaced with error")
	}
	if !results[1].IsError {
		t.Error("huge result should be marked as error")
	}
}

// TestGuardOversizedToolResults_SessionFileGetsErrorNotRawContent verifies the
// critical invariant: it must be impossible for clyde to produce a tool result
// that is impossible for clyde to load.
//
// When a tool result is too large and the guard replaces it with an error
// message, the outputCallback (which writes session files) must receive the
// error message — NOT the raw oversized content. Otherwise, resuming the
// session with -r would reload the oversized content into the context window.
//
// This is a session-safety test, not just a guard test.
func TestGuardOversizedToolResults_SessionFileGetsErrorNotRawContent(t *testing.T) {
	// Track what the outputCallback receives — this is what gets written to
	// the *_tool-result.md session file.
	var callbackOutputs []struct {
		content   string
		toolUseID string
	}

	client := providers.NewClient("fake", "http://localhost", "m", 1000)
	a := agent.NewAgent(client, "test",
		agent.WithContextWindowSize(10000), // small window
		agent.WithReserveTokens(2000),
		agent.WithOutputCallback(func(output string, toolUseID string) {
			callbackOutputs = append(callbackOutputs, struct {
				content   string
				toolUseID string
			}{output, toolUseID})
		}),
	)

	// Simulate the exact flow: tool produces huge output → guard replaces →
	// outputCallback receives the guarded content.
	hugeContent := strings.Repeat("x", 200000) // ~57k tokens, way over 5k limit
	smallContent := "total 3 files"

	toolResults := []providers.ContentBlock{
		{Type: "tool_result", ToolUseID: "toolu_small", Content: smallContent},
		{Type: "tool_result", ToolUseID: "toolu_huge", Content: hugeContent},
	}

	// Run the guard (this is what agent.go does before emitting callbacks)
	toolResults = a.GuardOversizedToolResults(toolResults)

	// Simulate the post-guard outputCallback loop (mirrors agent.go)
	for _, block := range toolResults {
		if block.Type != "tool_result" {
			continue
		}
		content, ok := block.Content.(string)
		if !ok || content == "" {
			continue
		}
		a.EmitOutputCallback(content, block.ToolUseID)
	}

	// Verify: outputCallback must have received the error message, not raw data
	if len(callbackOutputs) != 2 {
		t.Fatalf("expected 2 callback outputs, got %d", len(callbackOutputs))
	}

	// Small result passes through unchanged
	if callbackOutputs[0].content != smallContent {
		t.Errorf("small result callback should get original content")
	}
	if callbackOutputs[0].toolUseID != "toolu_small" {
		t.Errorf("small result callback should get correct toolUseID")
	}

	// Huge result: callback must get the ERROR message, not the 200k raw content
	if len(callbackOutputs[1].content) > 1000 {
		t.Fatalf("CRITICAL: outputCallback received %d chars — raw oversized content leaked to session file! "+
			"On resume, this would reload into the context window and defeat the guard.",
			len(callbackOutputs[1].content))
	}
	if !strings.Contains(callbackOutputs[1].content, "ERROR: Tool result too large") {
		t.Errorf("outputCallback should receive the error message, got: %q", callbackOutputs[1].content[:100])
	}
	if callbackOutputs[1].toolUseID != "toolu_huge" {
		t.Errorf("huge result callback should get correct toolUseID")
	}
}

// TestGuardOversizedToolResults_ResumedSessionSurvives verifies that a session
// file containing an oversized-guard error message can be loaded back without
// issues. This is the end-to-end resume guarantee: write error → load error.
func TestGuardOversizedToolResults_ResumedSessionSurvives(t *testing.T) {
	sessionDir := t.TempDir()

	// Simulate a session where the guard fired: tool-result file contains
	// the error message, not the raw content.
	errorMsg := "ERROR: Tool result too large for context window (estimated 57142 tokens, limit 5000 tokens, context window 10000 tokens). " +
		"The output was 200000 characters long and has been discarded to prevent a crash.\n\n" +
		"Suggestions:\n  - For bash commands: redirect output to a file"

	writeGuardTestFile(t, sessionDir, "2026-07-14T09-32-00.000_user.md", "**User:**\n\nRead /tmp/huge-file.txt\n")
	writeGuardTestFile(t, sessionDir, "2026-07-14T09-32-02.000_tool-use.md",
		"[toolu_guard_test]\n**read_file** — Reading file: /tmp/huge-file.txt\n```json\n{\"path\":\"/tmp/huge-file.txt\"}\n```\n")
	writeGuardTestFile(t, sessionDir, "2026-07-14T09-32-02.100_tool-result.md",
		"[toolu_guard_test]\n```\n"+errorMsg+"\n```\n")
	writeGuardTestFile(t, sessionDir, "2026-07-14T09-32-04.000_assistant.md",
		"**Claude:**\n\nThe file was too large to read directly.\n")

	// Reconstruct the history — this is what -r does
	messages, warnings, err := session.ReconstructHistory(sessionDir)
	if err != nil {
		t.Fatalf("ReconstructHistory failed: %v", err)
	}
	for _, w := range warnings {
		t.Logf("Warning: %s", w)
	}

	// Find the tool_result message and verify it contains the error, not raw data
	foundToolResult := false
	for _, msg := range messages {
		blocks, ok := msg.Content.([]providers.ContentBlock)
		if !ok {
			continue
		}
		for _, block := range blocks {
			if block.Type == "tool_result" && block.ToolUseID == "toolu_guard_test" {
				foundToolResult = true
				content, ok := block.Content.(string)
				if !ok {
					t.Fatal("tool_result content should be a string")
				}
				if !strings.Contains(content, "ERROR: Tool result too large") {
					t.Errorf("resumed tool_result should contain error message, got: %q", content)
				}
				// Critical: must be small enough to fit in context window
				tokens := agent.EstimateTokens(content)
				if tokens > 1000 {
					t.Errorf("resumed tool_result is %d tokens — too large for any context window", tokens)
				}
				t.Logf("Resumed tool_result: %d chars, ~%d tokens — safe for context window", len(content), tokens)
			}
		}
	}
	if !foundToolResult {
		t.Error("should find the tool_result with guard error in reconstructed history")
	}
	t.Logf("Reconstructed %d messages, %d warnings — session is resumable", len(messages), len(warnings))
}

// writeGuardTestFile is a helper that writes a file to the session directory.
func writeGuardTestFile(t *testing.T, dir, name, content string) {
	t.Helper()
	err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644)
	if err != nil {
		t.Fatalf("failed to write %s: %v", name, err)
	}
}

// TestEstimateTokens verifies the token estimation helper.
func TestEstimateTokens(t *testing.T) {
	// 3500 chars / 3.5 chars_per_token = 1000 tokens
	tokens := agent.EstimateTokens(strings.Repeat("x", 3500))
	if tokens != 1000 {
		t.Errorf("expected 1000 tokens, got %d", tokens)
	}

	// Empty string
	if agent.EstimateTokens("") != 0 {
		t.Error("empty string should be 0 tokens")
	}
}
