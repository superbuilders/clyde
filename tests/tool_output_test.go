package main

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/this-is-alpha-iota/clyde/agent"
	"github.com/this-is-alpha-iota/clyde/providers"
	"github.com/this-is-alpha-iota/clyde/loglevel"
	"github.com/this-is-alpha-iota/clyde/agent/prompts"
	"github.com/this-is-alpha-iota/clyde/cli/style"
	"github.com/this-is-alpha-iota/clyde/agent/truncate"
)

// --- Unit Tests: Tool Output Level Gating ---

// TestToolOutputLevelGating verifies that tool output bodies are shown at
// Normal, Verbose, and Debug levels, but suppressed at Quiet and Silent.
// Tool output is emitted at Normal threshold via agent.emit(loglevel.Normal, ...).
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
			// Tool output is emitted at Normal threshold.
			// ShouldShow determines whether the callback fires.
			if got := tt.level.ShouldShow(loglevel.Normal); got != tt.wantShow {
				t.Errorf("Level %s ShouldShow(Normal) = %v, want %v",
					tt.level, got, tt.wantShow)
			}

			// Verify the agent can be created at this level with a callback
			var messages []string
			a := agent.NewAgent(
				providers.NewClient("dummy", "http://localhost", "test", 100),
				"test prompt",
				agent.WithLogLevel(tt.level),
				agent.WithProgressCallback(func(lvl loglevel.Level, msg string) {
					messages = append(messages, msg)
				}),
			)
			if a.LogLevel() != tt.level {
				t.Errorf("Expected level %s, got %s", tt.level, a.LogLevel())
			}
		})
	}
}

// TestToolOutputProgressLineGating verifies that → progress lines (Quiet
// threshold) are shown at Quiet and above, but suppressed at Silent.
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
		result := truncate.ToolOutput(text, loglevel.Normal)
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
		result := truncate.ToolOutput(text, loglevel.Normal)
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
		result := truncate.ToolOutput(text, loglevel.Normal)
		resultLines := strings.Split(result, "\n")
		// 25 kept + 1 overflow message = 26 output lines
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
	result := truncate.ToolOutput(text, loglevel.Normal)

	if !strings.Contains(result, "... (75 more lines)") {
		t.Error("Expected overflow message for 75 extra lines")
	}

	// First line preserved
	if !strings.HasPrefix(result, "output line 1:") {
		t.Error("First line should be preserved")
	}

	// Line 25 is the last kept line
	if !strings.Contains(result, "output line 25:") {
		t.Error("Line 25 should be the last kept line")
	}

	// Line 26 should NOT appear (it's truncated)
	if strings.Contains(result, "output line 26:") {
		t.Error("Line 26 should be truncated away")
	}
}

// TestToolOutputNoTruncationAtVerbose verifies tool output is passed through
// unmodified at Verbose level.
func TestToolOutputNoTruncationAtVerbose(t *testing.T) {
	lines := make([]string, 100)
	for i := range lines {
		lines[i] = fmt.Sprintf("line %d", i+1)
	}
	text := strings.Join(lines, "\n")
	result := truncate.ToolOutput(text, loglevel.Verbose)
	if result != text {
		t.Error("Verbose level should not truncate tool output")
	}
}

// TestToolOutputNoTruncationAtDebug verifies tool output is passed through
// unmodified at Debug level.
func TestToolOutputNoTruncationAtDebug(t *testing.T) {
	lines := make([]string, 100)
	for i := range lines {
		lines[i] = fmt.Sprintf("line %d", i+1)
	}
	text := strings.Join(lines, "\n")
	result := truncate.ToolOutput(text, loglevel.Debug)
	if result != text {
		t.Error("Debug level should not truncate tool output")
	}
}

// TestToolOutputCharacterTruncation verifies per-line character truncation
// is applied to tool output at Normal level (2000 char limit per line).
func TestToolOutputCharacterTruncation(t *testing.T) {
	longLine := strings.Repeat("x", 2500)
	result := truncate.ToolOutput(longLine, loglevel.Normal)

	if len(result) != 2003 { // 2000 + "..."
		t.Errorf("Expected 2003 chars, got %d", len(result))
	}
	if !strings.HasSuffix(result, "...") {
		t.Error("Truncated line should end with ...")
	}
}

// --- Unit Tests: Tool Output Display Styling ---

// TestToolOutputDimStyling verifies tool output bodies are styled with
// dim/faint ANSI attribute when color is enabled.
func TestToolOutputDimStyling(t *testing.T) {
	// Save and clear NO_COLOR to enable color
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

	// Should contain ANSI escape codes
	if !strings.Contains(output, "\033[") {
		t.Error("Expected ANSI escape codes in styled output")
	}
	// Should contain dim/faint attribute (code "2")
	if !strings.Contains(output, "2m") {
		t.Error("Expected dim/faint ANSI attribute (code 2)")
	}
	// Original text preserved
	if !strings.Contains(output, "tool output text") {
		t.Error("Original text should be preserved in styled output")
	}
	// Should end with reset code
	if !strings.HasSuffix(output, "\033[0m") {
		t.Error("Styled output should end with ANSI reset code")
	}
}

// TestToolOutputNoColorWhenDisabled verifies no ANSI codes when NO_COLOR is set.
func TestToolOutputNoColorWhenDisabled(t *testing.T) {
	// Set NO_COLOR
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

// --- Unit Tests: Agent Callback Behavior ---

// TestToolOutputAgentCallbackSetup verifies the agent properly accepts
// and stores the progress callback for tool output delivery.
func TestToolOutputAgentCallbackSetup(t *testing.T) {
	tests := []struct {
		level      loglevel.Level
		expectEmit bool
		// expectTrunc only meaningful when expectEmit is true:
		//   true = truncation applied (Normal)
		//   false = no truncation (Verbose, Debug)
		expectTrunc bool
	}{
		{loglevel.Silent, false, false},
		{loglevel.Quiet, false, false},
		{loglevel.Normal, true, true},
		{loglevel.Verbose, true, false},
		{loglevel.Debug, true, false},
	}

	for _, tt := range tests {
		t.Run(tt.level.String(), func(t *testing.T) {
			a := agent.NewAgent(
				providers.NewClient("dummy", "http://localhost", "test", 100),
				"test prompt",
				agent.WithLogLevel(tt.level),
				agent.WithProgressCallback(func(lvl loglevel.Level, msg string) {}),
			)

			// Verify agent has correct level
			if a.LogLevel() != tt.level {
				t.Errorf("Expected level %s, got %s", tt.level, a.LogLevel())
			}

			// Verify the gating logic matches expectations
			canShow := tt.level.ShouldShow(loglevel.Normal)
			if canShow != tt.expectEmit {
				t.Errorf("ShouldShow(Normal) = %v, want %v", canShow, tt.expectEmit)
			}

			// Only verify truncation behavior when output is actually emitted
			if tt.expectEmit {
				longOutput := make([]string, 30)
				for i := range longOutput {
					longOutput[i] = fmt.Sprintf("line %d", i+1)
				}
				text := strings.Join(longOutput, "\n")

				truncated := truncate.ToolOutput(text, tt.level)
				hasTruncation := strings.Contains(truncated, "more lines)")

				if tt.expectTrunc && !hasTruncation {
					t.Error("Expected truncation at this level")
				}
				if !tt.expectTrunc && hasTruncation {
					t.Error("Did not expect truncation at this level")
				}
			}
		})
	}
}

// TestToolOutputTruncationConsistency verifies that the truncation function
// used for tool output (ToolOutput) uses the correct limit (25 lines).
func TestToolOutputTruncationConsistency(t *testing.T) {
	// Verify the ToolOutputLineLimit constant
	if truncate.ToolOutputLineLimit != 25 {
		t.Errorf("ToolOutputLineLimit should be 25, got %d", truncate.ToolOutputLineLimit)
	}

	// Verify ToolOutput uses ToolOutputLineLimit
	lines := make([]string, 30)
	for i := range lines {
		lines[i] = fmt.Sprintf("line %d", i+1)
	}
	text := strings.Join(lines, "\n")

	// ToolOutput should truncate at 25 lines
	result := truncate.ToolOutput(text, loglevel.Normal)
	if !strings.Contains(result, "... (5 more lines)") {
		t.Error("ToolOutput should truncate at 25-line limit")
	}

	// Direct Text call with the same limit should produce identical results
	directResult := truncate.Text(text, truncate.ToolOutputLineLimit, loglevel.Normal)
	if result != directResult {
		t.Error("ToolOutput and Text(25) should produce identical results")
	}
}

// --- Unit Tests: Display Message Formatting ---

// TestStyleMessageFormatting verifies that styleMessage produces the correct
// styling for each log level.
func TestStyleMessageFormatting(t *testing.T) {
	// Save and clear NO_COLOR to enable color
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
		// Tool progress lines should use bold yellow styling
		msg := "→ Reading file: main.go"
		styled := style.FormatToolProgress(msg)
		if !strings.Contains(styled, "main.go") {
			t.Error("Styled message should contain the file name")
		}
		// Should contain yellow color code
		if !strings.Contains(styled, "33") { // yellow
			t.Error("Tool progress should use yellow styling")
		}
	})

	t.Run("normal_tool_output", func(t *testing.T) {
		// Tool output bodies should use dim styling
		msg := "total 80\n-rw-r--r-- 1 user group 12345 main.go"
		styled := style.FormatDim(msg)
		if !strings.Contains(styled, "main.go") {
			t.Error("Styled message should contain the file listing")
		}
		// Should contain dim attribute
		if !strings.Contains(styled, "2m") { // dim/faint
			t.Error("Tool output should use dim styling")
		}
	})

	t.Run("debug_diagnostics", func(t *testing.T) {
		msg := "🔍 Tokens: input=100 output=50"
		styled := style.FormatDebug(msg)
		if !strings.Contains(styled, "Tokens") {
			t.Error("Styled message should contain diagnostic info")
		}
		// Should contain red color code
		if !strings.Contains(styled, "31") { // red
			t.Error("Debug messages should use red styling")
		}
	})
}

// --- Integration Tests ---

// TestToolOutputIntegrationNormal makes a real API call that triggers tool
// use and verifies both the → progress line and tool output body are emitted
// at Normal level.
func TestToolOutputIntegrationNormal(t *testing.T) {
	apiKey := os.Getenv("TS_AGENT_API_KEY")
	if apiKey == "" {
		t.Skip("TS_AGENT_API_KEY not set, skipping integration test")
	}

	var normalMessages []string
	var quietMessages []string

	apiClient := providers.NewClient(
		apiKey,
		"https://api.anthropic.com/v1/messages",
		"claude-sonnet-4-5-20250929",
		4096,
	)

	agentInstance := agent.NewAgent(
		apiClient,
		prompts.SystemPrompt,
		agent.WithLogLevel(loglevel.Normal),
		agent.WithProgressCallback(func(lvl loglevel.Level, msg string) {
			switch lvl {
			case loglevel.Quiet:
				quietMessages = append(quietMessages, msg)
			case loglevel.Normal:
				normalMessages = append(normalMessages, msg)
			}
		}),
	)

	// Ask a question that will trigger list_files tool
	response, err := agentInstance.HandleMessage(
		"List the files in the current directory using the list_files tool. Just list them, don't explain.")
	if err != nil {
		t.Fatalf("HandleMessage failed: %v", err)
	}

	t.Logf("Response length: %d chars", len(response))
	t.Logf("→ progress lines: %d", len(quietMessages))
	t.Logf("Tool output bodies: %d", len(normalMessages))

	// Verify we got at least one → progress line
	if len(quietMessages) == 0 {
		t.Error("Expected at least one tool progress line (→)")
	} else {
		for i, msg := range quietMessages {
			t.Logf("  Progress %d: %s", i, msg)
		}
	}

	// Verify we got at least one tool output body
	if len(normalMessages) == 0 {
		t.Error("Expected at least one tool output body at Normal level")
	} else {
		for i, msg := range normalMessages {
			preview := msg
			if len(preview) > 200 {
				preview = preview[:200] + "..."
			}
			t.Logf("  Output %d: %s", i, preview)
		}
	}

	// Verify output body contains actual content
	for _, msg := range normalMessages {
		if len(msg) == 0 {
			t.Error("Tool output body should not be empty")
		}
	}

	// Verify we got a final text response
	if response == "" {
		t.Error("Expected non-empty final response")
	}
}

// TestToolOutputIntegrationQuietSuppressed verifies that at Quiet level,
// tool output bodies are suppressed while → progress lines are still shown.
func TestToolOutputIntegrationQuietSuppressed(t *testing.T) {
	apiKey := os.Getenv("TS_AGENT_API_KEY")
	if apiKey == "" {
		t.Skip("TS_AGENT_API_KEY not set, skipping integration test")
	}

	var normalMessages []string
	var quietMessages []string

	apiClient := providers.NewClient(
		apiKey,
		"https://api.anthropic.com/v1/messages",
		"claude-sonnet-4-5-20250929",
		4096,
	)

	agentInstance := agent.NewAgent(
		apiClient,
		prompts.SystemPrompt,
		agent.WithLogLevel(loglevel.Quiet),
		agent.WithProgressCallback(func(lvl loglevel.Level, msg string) {
			switch lvl {
			case loglevel.Quiet:
				quietMessages = append(quietMessages, msg)
			case loglevel.Normal:
				normalMessages = append(normalMessages, msg)
			}
		}),
	)

	_, err := agentInstance.HandleMessage(
		"What files are in the current directory? Use list_files tool.")
	if err != nil {
		t.Fatalf("HandleMessage failed: %v", err)
	}

	t.Logf("→ progress lines: %d", len(quietMessages))
	t.Logf("Tool output bodies: %d", len(normalMessages))

	// At Quiet level, tool output bodies should be suppressed
	if len(normalMessages) > 0 {
		t.Errorf("Expected no tool output bodies at Quiet level, got %d", len(normalMessages))
		for i, msg := range normalMessages {
			t.Logf("  Unexpected output %d: %s", i, msg[:min(len(msg), 100)])
		}
	}

	// → progress lines should still be visible at Quiet level
	// (they may or may not appear depending on whether Claude uses tools)
	if len(quietMessages) > 0 {
		t.Logf("✅ → progress lines visible at Quiet level")
	}
}

// TestToolOutputIntegrationVerboseNoTruncation verifies that at Verbose level,
// tool output bodies are shown without truncation.
func TestToolOutputIntegrationVerboseNoTruncation(t *testing.T) {
	apiKey := os.Getenv("TS_AGENT_API_KEY")
	if apiKey == "" {
		t.Skip("TS_AGENT_API_KEY not set, skipping integration test")
	}

	var normalMessages []string

	apiClient := providers.NewClient(
		apiKey,
		"https://api.anthropic.com/v1/messages",
		"claude-sonnet-4-5-20250929",
		4096,
	)

	agentInstance := agent.NewAgent(
		apiClient,
		prompts.SystemPrompt,
		agent.WithLogLevel(loglevel.Verbose),
		agent.WithProgressCallback(func(lvl loglevel.Level, msg string) {
			if lvl == loglevel.Normal {
				normalMessages = append(normalMessages, msg)
			}
		}),
	)

	_, err := agentInstance.HandleMessage(
		"List files in the current directory using list_files tool.")
	if err != nil {
		t.Fatalf("HandleMessage failed: %v", err)
	}

	// At Verbose level, output should not be truncated
	for i, msg := range normalMessages {
		if strings.Contains(msg, "... (") && strings.HasSuffix(msg, "more lines)") {
			t.Errorf("Output %d should not be truncated at Verbose level", i)
		}
	}
	t.Logf("Tool output bodies at Verbose: %d (all untruncated)", len(normalMessages))
}
