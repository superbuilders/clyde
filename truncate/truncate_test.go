package truncate

import (
	"strings"
	"testing"

	"github.com/this-is-alpha-iota/clyde/loglevel"
)

// helper to build a string with N lines
func nLines(n int) string {
	lines := make([]string, n)
	for i := range lines {
		lines[i] = "line " + strings.Repeat("x", 10)
	}
	return strings.Join(lines, "\n")
}

// --- Lines tests ---

func TestLinesNoTruncationBelowLimit(t *testing.T) {
	text := nLines(24)
	result := Lines(text, 25, loglevel.Normal)
	if result != text {
		t.Error("24 lines should not be truncated at limit 25")
	}
}

func TestLinesNoTruncationAtExactLimit(t *testing.T) {
	text := nLines(25)
	result := Lines(text, 25, loglevel.Normal)
	if result != text {
		t.Error("25 lines should not be truncated at limit 25")
	}
}

func TestLinesTruncationOneOverLimit(t *testing.T) {
	text := nLines(26)
	result := Lines(text, 25, loglevel.Normal)

	resultLines := strings.Split(result, "\n")
	// 25 kept lines + 1 overflow message = 26 lines in result
	if len(resultLines) != 26 {
		t.Errorf("Expected 26 result lines (25 kept + 1 overflow), got %d", len(resultLines))
	}

	lastLine := resultLines[len(resultLines)-1]
	if lastLine != "... (1 more lines)" {
		t.Errorf("Expected overflow message, got %q", lastLine)
	}
}

func TestLinesTruncationManyOver(t *testing.T) {
	text := nLines(100)
	result := Lines(text, 25, loglevel.Normal)

	if !strings.Contains(result, "... (75 more lines)") {
		t.Error("Expected overflow message showing 75 more lines")
	}
}

func TestLinesBypassedAtVerbose(t *testing.T) {
	text := nLines(100)
	result := Lines(text, 25, loglevel.Verbose)
	if result != text {
		t.Error("Verbose level should bypass line truncation")
	}
}

func TestLinesBypassedAtDebug(t *testing.T) {
	text := nLines(100)
	result := Lines(text, 25, loglevel.Debug)
	if result != text {
		t.Error("Debug level should bypass line truncation")
	}
}

func TestLinesAppliedAtNormal(t *testing.T) {
	text := nLines(30)
	result := Lines(text, 25, loglevel.Normal)
	if !strings.Contains(result, "... (5 more lines)") {
		t.Error("Normal level should truncate")
	}
}

func TestLinesAppliedAtQuiet(t *testing.T) {
	text := nLines(30)
	result := Lines(text, 25, loglevel.Quiet)
	if !strings.Contains(result, "... (5 more lines)") {
		t.Error("Quiet level should truncate")
	}
}

func TestLinesAppliedAtSilent(t *testing.T) {
	text := nLines(30)
	result := Lines(text, 25, loglevel.Silent)
	if !strings.Contains(result, "... (5 more lines)") {
		t.Error("Silent level should truncate")
	}
}

// --- Chars tests ---

func TestCharsNoTruncationBelowLimit(t *testing.T) {
	line := strings.Repeat("x", 1999)
	result := Chars(line, loglevel.Normal)
	if result != line {
		t.Error("1999 chars should not be truncated")
	}
}

func TestCharsNoTruncationAtExactLimit(t *testing.T) {
	line := strings.Repeat("x", 2000)
	result := Chars(line, loglevel.Normal)
	if result != line {
		t.Error("2000 chars should not be truncated")
	}
}

func TestCharsTruncationOneOverLimit(t *testing.T) {
	line := strings.Repeat("x", 2001)
	result := Chars(line, loglevel.Normal)
	if len(result) != 2003 { // 2000 + "..."
		t.Errorf("Expected length 2003 (2000 + ...), got %d", len(result))
	}
	if !strings.HasSuffix(result, "...") {
		t.Error("Truncated line should end with ...")
	}
}

func TestCharsBypassedAtVerbose(t *testing.T) {
	line := strings.Repeat("x", 5000)
	result := Chars(line, loglevel.Verbose)
	if result != line {
		t.Error("Verbose level should bypass character truncation")
	}
}

func TestCharsBypassedAtDebug(t *testing.T) {
	line := strings.Repeat("x", 5000)
	result := Chars(line, loglevel.Debug)
	if result != line {
		t.Error("Debug level should bypass character truncation")
	}
}

// --- Text tests (combined line + char truncation) ---

func TestTextCombinedTruncation(t *testing.T) {
	// Build text with 30 lines, each 3000 chars
	lines := make([]string, 30)
	for i := range lines {
		lines[i] = strings.Repeat("x", 3000)
	}
	text := strings.Join(lines, "\n")

	result := Text(text, 25, loglevel.Normal)

	resultLines := strings.Split(result, "\n")

	// Should have 25 kept lines + 1 overflow message = 26 lines
	if len(resultLines) != 26 {
		t.Errorf("Expected 26 result lines, got %d", len(resultLines))
	}

	// Each kept line should be truncated to 2000 + "..."
	for i := 0; i < 25; i++ {
		if len(resultLines[i]) != 2003 {
			t.Errorf("Line %d: expected length 2003, got %d", i, len(resultLines[i]))
			break
		}
	}

	// Last line should be overflow message
	if resultLines[25] != "... (5 more lines)" {
		t.Errorf("Expected overflow message, got %q", resultLines[25])
	}
}

func TestTextBypassedAtVerbose(t *testing.T) {
	lines := make([]string, 100)
	for i := range lines {
		lines[i] = strings.Repeat("x", 3000)
	}
	text := strings.Join(lines, "\n")

	result := Text(text, 25, loglevel.Verbose)
	if result != text {
		t.Error("Verbose should bypass all truncation")
	}
}

// --- Thinking convenience wrapper ---

func TestThinkingUsesCorrectLimit(t *testing.T) {
	text := nLines(55)
	result := Thinking(text, loglevel.Normal)

	if !strings.Contains(result, "... (5 more lines)") {
		t.Error("Thinking should truncate at 50 lines, showing 5 overflow")
	}
}

func TestThinkingNoTruncationAtLimit(t *testing.T) {
	text := nLines(50)
	result := Thinking(text, loglevel.Normal)
	if result != text {
		t.Error("50 lines should not be truncated at ThinkingLineLimit=50")
	}
}

func TestThinkingBypassedAtVerbose(t *testing.T) {
	text := nLines(100)
	result := Thinking(text, loglevel.Verbose)
	if result != text {
		t.Error("Verbose should bypass thinking truncation")
	}
}

// --- ToolOutput convenience wrapper ---

func TestToolOutputUsesCorrectLimit(t *testing.T) {
	text := nLines(30)
	result := ToolOutput(text, loglevel.Normal)

	if !strings.Contains(result, "... (5 more lines)") {
		t.Error("ToolOutput should truncate at 25 lines, showing 5 overflow")
	}
}

func TestToolOutputNoTruncationAtLimit(t *testing.T) {
	text := nLines(25)
	result := ToolOutput(text, loglevel.Normal)
	if result != text {
		t.Error("25 lines should not be truncated at ToolOutputLineLimit=25")
	}
}

func TestToolOutputBypassedAtVerbose(t *testing.T) {
	text := nLines(100)
	result := ToolOutput(text, loglevel.Verbose)
	if result != text {
		t.Error("Verbose should bypass tool output truncation")
	}
}

// --- Empty/edge cases ---

func TestLinesEmptyString(t *testing.T) {
	result := Lines("", 25, loglevel.Normal)
	if result != "" {
		t.Errorf("Empty string should remain empty, got %q", result)
	}
}

func TestLinesSingleLine(t *testing.T) {
	result := Lines("hello", 25, loglevel.Normal)
	if result != "hello" {
		t.Errorf("Single line should remain unchanged, got %q", result)
	}
}

func TestCharsEmptyString(t *testing.T) {
	result := Chars("", loglevel.Normal)
	if result != "" {
		t.Errorf("Empty string should remain empty, got %q", result)
	}
}

// --- Single-line commands are never truncated tests ---
// This verifies the design principle that single-line bash commands
// are not subject to line truncation (they're 1 line, so always under limit).

func TestSingleLineCommandNeverTruncated(t *testing.T) {
	// A very long single-line command. Line truncation (25 lines) doesn't apply
	// because it's only 1 line. Character truncation still applies at Normal.
	longCmd := "run_bash: " + strings.Repeat("x", 500)
	result := Lines(longCmd, ToolOutputLineLimit, loglevel.Normal)
	if result != longCmd {
		t.Error("Single-line command should never be line-truncated")
	}
}

func TestMultiLineBashFollowsStandardTruncation(t *testing.T) {
	// A multi-line bash command/output should follow standard truncation
	text := nLines(30)
	result := Lines(text, ToolOutputLineLimit, loglevel.Normal)
	if !strings.Contains(result, "... (5 more lines)") {
		t.Error("Multi-line content should be truncated at 25 lines")
	}
}
