package main

import (
	"strings"
	"testing"

	"github.com/this-is-alpha-iota/clyde/cli/truncate"
)

// --- Lines ---

func TestLinesUnderLimit(t *testing.T) {
	text := strings.Repeat("line\n", 24) + "line"
	result := truncate.Lines(text, 25)
	if result != text {
		t.Error("24 lines should not be truncated at limit 25")
	}
}

func TestLinesAtLimit(t *testing.T) {
	text := strings.Repeat("line\n", 24) + "last line"
	result := truncate.Lines(text, 25)
	if result != text {
		t.Error("25 lines should not be truncated at limit 25")
	}
}

func TestLinesOverLimit(t *testing.T) {
	text := strings.Repeat("line\n", 25) + "extra line"
	result := truncate.Lines(text, 25)
	if !strings.Contains(result, "... (1 more lines)") {
		t.Error("26 lines should be truncated at limit 25")
	}
}

func TestLinesWellOverLimit(t *testing.T) {
	lines := make([]string, 50)
	for i := range lines {
		lines[i] = "line"
	}
	text := strings.Join(lines, "\n")
	result := truncate.Lines(text, 25)
	if !strings.Contains(result, "... (25 more lines)") {
		t.Error("Expected 25 overflow lines")
	}
}

// Lines always truncates (no level bypass). Caller decides whether to call it.
func TestLinesAlwaysTruncates(t *testing.T) {
	lines := make([]string, 30)
	for i := range lines {
		lines[i] = "line"
	}
	text := strings.Join(lines, "\n")
	result := truncate.Lines(text, 25)
	if !strings.Contains(result, "more lines)") {
		t.Error("Lines should always truncate when over limit")
	}
}

func TestLinesEmptyString(t *testing.T) {
	result := truncate.Lines("", 25)
	if result != "" {
		t.Error("Empty string should pass through")
	}
}

func TestLinesSingleLine(t *testing.T) {
	result := truncate.Lines("hello", 25)
	if result != "hello" {
		t.Error("Single line should pass through")
	}
}

// --- Chars ---

func TestCharsUnderLimit(t *testing.T) {
	line := strings.Repeat("x", 1999)
	result := truncate.Chars(line)
	if result != line {
		t.Error("1999 chars should not be truncated")
	}
}

func TestCharsAtLimit(t *testing.T) {
	line := strings.Repeat("x", 2000)
	result := truncate.Chars(line)
	if result != line {
		t.Error("2000 chars should not be truncated")
	}
}

func TestCharsOverLimit(t *testing.T) {
	line := strings.Repeat("x", 2001)
	result := truncate.Chars(line)
	if !strings.HasSuffix(result, "...") {
		t.Error("Over-limit should end with ...")
	}
	if len(result) != 2003 { // 2000 + "..."
		t.Errorf("Expected 2003 chars, got %d", len(result))
	}
}

func TestCharsEmpty(t *testing.T) {
	result := truncate.Chars("")
	if result != "" {
		t.Error("Empty string should pass through")
	}
}

// --- Text (combined line + char truncation) ---

func TestTextCombinedTruncation(t *testing.T) {
	// Build 30 lines, each 2500 chars
	lines := make([]string, 30)
	for i := range lines {
		lines[i] = strings.Repeat("x", 2500)
	}
	text := strings.Join(lines, "\n")

	result := truncate.Text(text, 25)

	// Should have 25 kept + overflow
	if !strings.Contains(result, "... (5 more lines)") {
		t.Error("Expected 5 overflow lines")
	}

	// Each kept line should be truncated to 2000 + "..."
	resultLines := strings.Split(result, "\n")
	for i := 0; i < 25; i++ {
		if len(resultLines[i]) != 2003 {
			t.Errorf("Line %d: expected 2003 chars, got %d", i, len(resultLines[i]))
		}
	}
}

// --- Convenience wrappers ---

func TestThinkingTruncation(t *testing.T) {
	lines := make([]string, 55)
	for i := range lines {
		lines[i] = "thinking"
	}
	text := strings.Join(lines, "\n")
	result := truncate.Thinking(text)
	if !strings.Contains(result, "... (5 more lines)") {
		t.Error("Thinking should truncate at 50 lines, showing 5 overflow")
	}
}

func TestThinkingAtLimit(t *testing.T) {
	lines := make([]string, 50)
	for i := range lines {
		lines[i] = "thinking"
	}
	text := strings.Join(lines, "\n")
	result := truncate.Thinking(text)
	if result != text {
		t.Error("50 lines should not be truncated at ThinkingLineLimit=50")
	}
}

func TestToolOutputTruncation(t *testing.T) {
	lines := make([]string, 30)
	for i := range lines {
		lines[i] = "output"
	}
	text := strings.Join(lines, "\n")
	result := truncate.ToolOutput(text)
	if !strings.Contains(result, "... (5 more lines)") {
		t.Error("ToolOutput should truncate at 25 lines, showing 5 overflow")
	}
}

func TestToolOutputAtLimit(t *testing.T) {
	lines := make([]string, 25)
	for i := range lines {
		lines[i] = "output"
	}
	text := strings.Join(lines, "\n")
	result := truncate.ToolOutput(text)
	if result != text {
		t.Error("25 lines should not be truncated at truncate.ToolOutputLineLimit=25")
	}
}

// --- Single-line commands are never truncated tests ---

func TestSingleLineCommandNeverTruncated(t *testing.T) {
	longCmd := "go test -v -count=1 -run 'TestSomethingVeryLongAndComplicated' ./pkg/something/very/deeply/nested/..."
	result := truncate.Lines(longCmd, truncate.ToolOutputLineLimit)
	if result != longCmd {
		t.Error("Single-line command should never be line-truncated")
	}
}

func TestMultiLineCommandTruncated(t *testing.T) {
	lines := make([]string, 30)
	for i := range lines {
		lines[i] = "command line"
	}
	text := strings.Join(lines, "\n")
	result := truncate.Lines(text, truncate.ToolOutputLineLimit)
	if !strings.Contains(result, "more lines)") {
		t.Error("Multi-line content should be truncated at 25 lines")
	}
}
