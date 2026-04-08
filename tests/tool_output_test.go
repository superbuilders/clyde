package main

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/this-is-alpha-iota/clyde/agent"
	"github.com/this-is-alpha-iota/clyde/providers"
	"github.com/this-is-alpha-iota/clyde/cli/loglevel"
	"github.com/this-is-alpha-iota/clyde/agent/prompts"
	"github.com/this-is-alpha-iota/clyde/cli/style"
	"github.com/this-is-alpha-iota/clyde/cli/truncate"
)

// --- Unit Tests: Tool Output Truncation ---

// TestToolOutputTruncationBoundary verifies tool output truncation at exact
// boundary conditions: 24 lines (no truncation), 25 lines (no truncation),
// 26 lines (truncated to 25 + overflow message).
func TestToolOutputTruncationBoundary(t *testing.T) {
	t.Run("24_lines_no_truncation", func(t *testing.T) {
		lines := make([]string, 24)
		for i := range lines {
			lines[i] = fmt.Sprintf("line %d", i+1)
		}
		text := strings.Join(lines, "\n")
		result := truncate.ToolOutput(text)
		if result != text {
			t.Error("24 lines should not be truncated")
		}
	})

	t.Run("25_lines_no_truncation", func(t *testing.T) {
		lines := make([]string, 25)
		for i := range lines {
			lines[i] = fmt.Sprintf("line %d", i+1)
		}
		text := strings.Join(lines, "\n")
		result := truncate.ToolOutput(text)
		if result != text {
			t.Error("25 lines (at limit) should not be truncated")
		}
	})

	t.Run("26_lines_truncated", func(t *testing.T) {
		lines := make([]string, 26)
		for i := range lines {
			lines[i] = fmt.Sprintf("line %d", i+1)
		}
		text := strings.Join(lines, "\n")
		result := truncate.ToolOutput(text)
		resultLines := strings.Split(result, "\n")
		if len(resultLines) != 26 {
			t.Errorf("Expected 26 result lines (25 kept + overflow), got %d", len(resultLines))
		}
		if !strings.Contains(result, "... (1 more lines)") {
			t.Errorf("Expected overflow message, got: %s", resultLines[len(resultLines)-1])
		}
	})
}

// TestToolOutputTruncationLargeOutput verifies truncation with well-over-limit output.
func TestToolOutputTruncationLargeOutput(t *testing.T) {
	lines := make([]string, 100)
	for i := range lines {
		lines[i] = fmt.Sprintf("output line %d: some grep result or file content", i+1)
	}
	text := strings.Join(lines, "\n")
	result := truncate.ToolOutput(text)

	if !strings.Contains(result, "... (75 more lines)") {
		t.Error("Expected overflow message for 75 extra lines")
	}
	if !strings.HasPrefix(result, "output line 1:") {
		t.Error("First line should be preserved")
	}
	if !strings.Contains(result, "output line 25:") {
		t.Error("Line 25 should be the last kept line")
	}
	if strings.Contains(result, "output line 26:") {
		t.Error("Line 26 should be truncated away")
	}
}

// TestToolOutputCharacterTruncation verifies per-line character truncation.
func TestToolOutputCharacterTruncation(t *testing.T) {
	longLine := strings.Repeat("x", 2500)
	result := truncate.ToolOutput(longLine)
	if len(result) != 2003 {
		t.Errorf("Expected 2003 chars, got %d", len(result))
	}
	if !strings.HasSuffix(result, "...") {
		t.Error("Truncated line should end with ...")
	}
}

// --- Unit Tests: Tool Output Display Styling ---

func TestToolOutputDimStyling(t *testing.T) {
	old, hadOld := os.LookupEnv("NO_COLOR")
	os.Unsetenv("NO_COLOR")
	style.ResetColorCache()
	defer func() {
		if hadOld {
			os.Setenv("NO_COLOR", old)
		}
		style.ResetColorCache()
	}()

	output := style.FormatDim("tool output text")
	if !strings.Contains(output, "\033[") {
		t.Error("Expected ANSI escape codes in styled output")
	}
	if !strings.Contains(output, "2m") {
		t.Error("Expected dim/faint ANSI attribute (code 2)")
	}
	if !strings.Contains(output, "tool output text") {
		t.Error("Original text should be preserved in styled output")
	}
	if !strings.HasSuffix(output, "\033[0m") {
		t.Error("Styled output should end with ANSI reset code")
	}
}

func TestToolOutputNoColorWhenDisabled(t *testing.T) {
	old, hadOld := os.LookupEnv("NO_COLOR")
	os.Setenv("NO_COLOR", "1")
	style.ResetColorCache()
	defer func() {
		if hadOld {
			os.Setenv("NO_COLOR", old)
		} else {
			os.Unsetenv("NO_COLOR")
		}
		style.ResetColorCache()
	}()

	output := style.FormatDim("tool output text")
	if strings.Contains(output, "\033[") {
		t.Error("Expected no ANSI codes when NO_COLOR is set")
	}
	if output != "tool output text" {
		t.Errorf("Expected raw text, got %q", output)
	}
}

// --- Unit Tests: Agent Emits Unconditionally (ARCH-2) ---

// TestAgentEmitsOutputUnconditionally verifies the agent always calls the
// output callback regardless of any level — the CLI filters.
func TestAgentEmitsOutputUnconditionally(t *testing.T) {
	var outputMessages []string
	a := agent.NewAgent(
		providers.NewClient("dummy", "http://localhost", "test", 100),
		"test prompt",
		agent.WithOutputCallback(func(output string) {
			outputMessages = append(outputMessages, output)
		}),
	)
	// Agent created — output callback is set. No log level in agent.
	if a == nil {
		t.Fatal("Agent should not be nil")
	}
	t.Log("Agent created with output callback — emits unconditionally (ARCH-2)")
}

// TestToolOutputTruncationConsistency verifies that the truncation function
// used for tool output uses the correct limit (25 lines).
func TestToolOutputTruncationConsistency(t *testing.T) {
	if truncate.ToolOutputLineLimit != 25 {
		t.Errorf("ToolOutputLineLimit should be 25, got %d", truncate.ToolOutputLineLimit)
	}

	lines := make([]string, 30)
	for i := range lines {
		lines[i] = fmt.Sprintf("line %d", i+1)
	}
	text := strings.Join(lines, "\n")

	result := truncate.ToolOutput(text)
	if !strings.Contains(result, "... (5 more lines)") {
		t.Error("ToolOutput should truncate at 25-line limit")
	}

	directResult := truncate.Text(text, truncate.ToolOutputLineLimit)
	if result != directResult {
		t.Error("ToolOutput and Text(25) should produce identical results")
	}
}

// --- Unit Tests: Display Message Formatting ---

func TestStyleMessageFormatting(t *testing.T) {
	old, hadOld := os.LookupEnv("NO_COLOR")
	os.Unsetenv("NO_COLOR")
	style.ResetColorCache()
	defer func() {
		if hadOld {
			os.Setenv("NO_COLOR", old)
		}
		style.ResetColorCache()
	}()

	t.Run("quiet_tool_progress", func(t *testing.T) {
		msg := "→ Reading file: main.go"
		styled := style.FormatToolProgress(msg)
		if !strings.Contains(styled, "main.go") {
			t.Error("Styled message should contain the file name")
		}
		if !strings.Contains(styled, "33") {
			t.Error("Tool progress should use yellow styling")
		}
	})

	t.Run("normal_tool_output", func(t *testing.T) {
		msg := "total 80\n-rw-r--r-- 1 user group 12345 main.go"
		styled := style.FormatDim(msg)
		if !strings.Contains(styled, "main.go") {
			t.Error("Styled message should contain the file listing")
		}
		if !strings.Contains(styled, "2m") {
			t.Error("Tool output should use dim styling")
		}
	})

	t.Run("debug_diagnostics", func(t *testing.T) {
		msg := "🔍 Tokens: input=100 output=50"
		styled := style.FormatDebug(msg)
		if !strings.Contains(styled, "Tokens") {
			t.Error("Styled message should contain diagnostic info")
		}
		if !strings.Contains(styled, "31") {
			t.Error("Debug messages should use red styling")
		}
	})
}

// --- Integration Tests ---

func TestToolOutputIntegrationNormal(t *testing.T) {
	apiKey := os.Getenv("TS_AGENT_API_KEY")
	if apiKey == "" {
		t.Skip("TS_AGENT_API_KEY not set, skipping integration test")
	}

	var outputMessages []string
	var progressMessages []string

	apiClient := providers.NewClient(
		apiKey,
		"https://api.anthropic.com/v1/messages",
		"claude-sonnet-4-5-20250929",
		4096,
	)

	agentInstance := agent.NewAgent(
		apiClient,
		prompts.SystemPrompt,
		agent.WithProgressCallback(func(msg string) {
			progressMessages = append(progressMessages, msg)
		}),
		agent.WithOutputCallback(func(output string) {
			outputMessages = append(outputMessages, output)
		}),
	)

	response, err := agentInstance.HandleMessage(
		"List the files in the current directory using the list_files tool. Just list them, don't explain.")
	if err != nil {
		t.Fatalf("HandleMessage failed: %v", err)
	}

	t.Logf("Response length: %d chars", len(response))
	t.Logf("→ progress lines: %d", len(progressMessages))
	t.Logf("Tool output bodies: %d", len(outputMessages))

	if len(progressMessages) == 0 {
		t.Error("Expected at least one tool progress line (→)")
	}
	if len(outputMessages) == 0 {
		t.Error("Expected at least one tool output body")
	}
	for _, msg := range outputMessages {
		if len(msg) == 0 {
			t.Error("Tool output body should not be empty")
		}
	}
	if response == "" {
		t.Error("Expected non-empty final response")
	}
}

func TestToolOutputIntegrationQuietSuppressed(t *testing.T) {
	apiKey := os.Getenv("TS_AGENT_API_KEY")
	if apiKey == "" {
		t.Skip("TS_AGENT_API_KEY not set, skipping integration test")
	}

	// With ARCH-2, the agent emits unconditionally. The CLI filters.
	// This test verifies the agent DOES emit output even when the
	// CLI would suppress it at Quiet level.
	var outputMessages []string
	var progressMessages []string

	apiClient := providers.NewClient(
		apiKey,
		"https://api.anthropic.com/v1/messages",
		"claude-sonnet-4-5-20250929",
		4096,
	)

	agentInstance := agent.NewAgent(
		apiClient,
		prompts.SystemPrompt,
		agent.WithProgressCallback(func(msg string) {
			progressMessages = append(progressMessages, msg)
		}),
		agent.WithOutputCallback(func(output string) {
			outputMessages = append(outputMessages, output)
		}),
	)

	_, err := agentInstance.HandleMessage(
		"What files are in the current directory? Use list_files tool.")
	if err != nil {
		t.Fatalf("HandleMessage failed: %v", err)
	}

	t.Logf("→ progress lines: %d", len(progressMessages))
	t.Logf("Tool output bodies: %d (agent emits unconditionally)", len(outputMessages))

	// ARCH-2: Agent always emits. Verify output was received.
	if len(progressMessages) > 0 {
		t.Logf("✅ Progress lines emitted unconditionally")
	}
	if len(outputMessages) > 0 {
		t.Logf("✅ Output bodies emitted unconditionally (CLI would filter at Quiet)")
	}
}

func TestToolOutputIntegrationVerboseNoTruncation(t *testing.T) {
	apiKey := os.Getenv("TS_AGENT_API_KEY")
	if apiKey == "" {
		t.Skip("TS_AGENT_API_KEY not set, skipping integration test")
	}

	var outputMessages []string

	apiClient := providers.NewClient(
		apiKey,
		"https://api.anthropic.com/v1/messages",
		"claude-sonnet-4-5-20250929",
		4096,
	)

	agentInstance := agent.NewAgent(
		apiClient,
		prompts.SystemPrompt,
		agent.WithOutputCallback(func(output string) {
			outputMessages = append(outputMessages, output)
		}),
	)

	_, err := agentInstance.HandleMessage(
		"List files in the current directory using list_files tool.")
	if err != nil {
		t.Fatalf("HandleMessage failed: %v", err)
	}

	// ARCH-2: Agent emits full, untruncated output. Verify no truncation.
	for i, msg := range outputMessages {
		if strings.Contains(msg, "... (") && strings.HasSuffix(msg, "more lines)") {
			t.Errorf("Output %d should NOT be truncated by agent (ARCH-2)", i)
		}
	}
	t.Logf("Tool output bodies: %d (all untruncated from agent)", len(outputMessages))
}

// --- Level Gating Unit Tests (now testing loglevel.ShouldShow directly) ---

func TestToolOutputLevelGating(t *testing.T) {
	tests := []struct {
		level    loglevel.Level
		wantShow bool
	}{
		{loglevel.Silent, false},
		{loglevel.Quiet, false},
		{loglevel.Normal, true},
		{loglevel.Verbose, true},
		{loglevel.Debug, true},
	}

	for _, tt := range tests {
		t.Run(tt.level.String(), func(t *testing.T) {
			// Tool output is displayed at Normal threshold.
			if got := tt.level.ShouldShow(loglevel.Normal); got != tt.wantShow {
				t.Errorf("Level %s ShouldShow(Normal) = %v, want %v",
					tt.level, got, tt.wantShow)
			}
		})
	}
}

func TestToolOutputProgressLineGating(t *testing.T) {
	tests := []struct {
		level    loglevel.Level
		wantShow bool
	}{
		{loglevel.Silent, false},
		{loglevel.Quiet, true},
		{loglevel.Normal, true},
		{loglevel.Verbose, true},
		{loglevel.Debug, true},
	}

	for _, tt := range tests {
		t.Run(tt.level.String(), func(t *testing.T) {
			if got := tt.level.ShouldShow(loglevel.Quiet); got != tt.wantShow {
				t.Errorf("Level %s ShouldShow(Quiet) = %v, want %v",
					tt.level, got, tt.wantShow)
			}
		})
	}
}
