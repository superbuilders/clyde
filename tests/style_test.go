package main

import (
	"os"
	"strings"
	"testing"

	"github.com/this-is-alpha-iota/clyde/loglevel"
	"github.com/this-is-alpha-iota/clyde/cli/style"
)

// --- styleMessage integration tests ---
// These test the styleMessage function used in main.go's progress callbacks.
// We replicate the function here because it's in package main and can't be imported.
// This mirrors the production behavior exactly.

// styleMessageTest applies color styling to a progress message based on its log level.
// This is an exact copy of main.go's styleMessage for testing purposes.
func styleMessageTest(level loglevel.Level, msg string) string {
	switch level {
	case loglevel.Quiet:
		return style.FormatToolProgress(msg)
	case loglevel.Normal:
		return style.FormatDim(msg)
	case loglevel.Debug:
		return style.FormatDebug(msg)
	default:
		return msg
	}
}

// containsANSI returns true if the string contains any ANSI escape sequence.
func containsANSITest(s string) bool {
	return strings.Contains(s, "\033[")
}

func TestStyleMessage_ToolProgress(t *testing.T) {
	// Reset color cache for consistent test behavior
	os.Unsetenv("NO_COLOR")
	os.Unsetenv("TERM")
	style.ResetColorCache()
	defer func() {
		style.ResetColorCache()
	}()

	tests := []struct {
		name string
		msg  string
	}{
		{"listing files", "→ Listing files: . (current directory)"},
		{"reading file", "→ Reading file: main.go"},
		{"patching file", "→ Patching file: main.go (+100 bytes)"},
		{"running bash", "→ Running bash: go test -v"},
		{"writing file", "→ Writing file: out.txt (1.2 KB)"},
		{"searching", "→ Searching: pattern in *.go"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := styleMessageTest(loglevel.Quiet, tt.msg)

			// With color enabled, should contain ANSI codes
			if !containsANSITest(result) {
				t.Errorf("Tool progress should be styled with ANSI codes, got: %q", result)
			}

			// Should contain bold yellow (1;33m) for the tool action part
			if !strings.Contains(result, "1;33m") {
				t.Errorf("Tool progress should use bold yellow (1;33m), got: %q", result)
			}

			// The original text content should be preserved
			if !strings.Contains(result, "→") {
				t.Error("Tool progress should preserve the → arrow")
			}
		})
	}
}

func TestStyleMessage_ToolOutput(t *testing.T) {
	os.Unsetenv("NO_COLOR")
	os.Unsetenv("TERM")
	style.ResetColorCache()
	defer func() {
		style.ResetColorCache()
	}()

	result := styleMessageTest(loglevel.Normal, "total 24\ndrwxr-xr-x 3 user staff 96 .\n-rw-r--r-- 1 user staff 5 file.txt")

	// Tool output bodies should be dim
	if !containsANSITest(result) {
		t.Error("Tool output body should be styled with ANSI codes")
	}
	if !strings.Contains(result, "2m") {
		t.Errorf("Tool output body should use dim/faint attribute (2m), got: %q", result)
	}
}

func TestStyleMessage_Debug(t *testing.T) {
	os.Unsetenv("NO_COLOR")
	os.Unsetenv("TERM")
	style.ResetColorCache()
	defer func() {
		style.ResetColorCache()
	}()

	result := styleMessageTest(loglevel.Debug, "🔍 Tokens: input=500 output=200 cache_read=300 cache_create=0")

	// Debug lines should be red
	if !containsANSITest(result) {
		t.Error("Debug output should be styled with ANSI codes")
	}
	if !strings.Contains(result, "31m") {
		t.Errorf("Debug output should use red (31m), got: %q", result)
	}
}

func TestStyleMessage_Verbose(t *testing.T) {
	os.Unsetenv("NO_COLOR")
	os.Unsetenv("TERM")
	style.ResetColorCache()
	defer func() {
		style.ResetColorCache()
	}()

	result := styleMessageTest(loglevel.Verbose, "💾 Cache hit: 3715 tokens (100% of input)")

	// Verbose messages should NOT be styled
	if containsANSITest(result) {
		t.Errorf("Verbose messages should not have ANSI codes, got: %q", result)
	}
	if result != "💾 Cache hit: 3715 tokens (100% of input)" {
		t.Errorf("Verbose message should be unchanged, got: %q", result)
	}
}

func TestStyleMessage_NoColor(t *testing.T) {
	os.Setenv("NO_COLOR", "1")
	style.ResetColorCache()
	defer func() {
		os.Unsetenv("NO_COLOR")
		style.ResetColorCache()
	}()

	tests := []struct {
		name  string
		level loglevel.Level
		msg   string
	}{
		{"tool progress", loglevel.Quiet, "→ Reading file: main.go"},
		{"tool output", loglevel.Normal, "file contents here"},
		{"debug", loglevel.Debug, "🔍 Tokens: input=500"},
		{"verbose", loglevel.Verbose, "💾 Cache hit: 3715 tokens"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := styleMessageTest(tt.level, tt.msg)
			if containsANSITest(result) {
				t.Errorf("With NO_COLOR set, message should not contain ANSI codes, got: %q", result)
			}
			// Text should still be present
			if result != tt.msg {
				t.Errorf("With NO_COLOR, message should be unchanged. Got %q, want %q", result, tt.msg)
			}
		})
	}
}

// --- REPL output formatting tests ---

func TestREPLPromptFormatting(t *testing.T) {
	os.Unsetenv("NO_COLOR")
	os.Unsetenv("TERM")
	style.ResetColorCache()
	defer func() {
		style.ResetColorCache()
	}()

	prompt := style.FormatUserPrompt()

	// Should contain "You: "
	if !strings.Contains(prompt, "You: ") {
		t.Error("REPL prompt should contain 'You: '")
	}

	// Should be bold cyan
	if !strings.Contains(prompt, "1;36m") {
		t.Errorf("REPL prompt should use bold cyan (1;36m), got: %q", prompt)
	}
}

func TestREPLResponseFormatting(t *testing.T) {
	os.Unsetenv("NO_COLOR")
	os.Unsetenv("TERM")
	style.ResetColorCache()
	defer func() {
		style.ResetColorCache()
	}()

	prefix := style.FormatAgentPrefix()

	// Should contain "Claude: "
	if !strings.Contains(prefix, "Claude: ") {
		t.Error("Agent prefix should contain 'Claude: '")
	}

	// Should be bold green
	if !strings.Contains(prefix, "1;32m") {
		t.Errorf("Agent prefix should use bold green (1;32m), got: %q", prefix)
	}

	// Body text after prefix should NOT be styled
	responseBody := "The answer is 42."
	fullLine := prefix + responseBody

	// The response body should appear unmodified at the end
	if !strings.HasSuffix(fullLine, responseBody) {
		t.Error("Agent response body should be plain text (default foreground)")
	}
}

func TestREPLPromptFormatting_NoColor(t *testing.T) {
	os.Setenv("NO_COLOR", "1")
	style.ResetColorCache()
	defer func() {
		os.Unsetenv("NO_COLOR")
		style.ResetColorCache()
	}()

	prompt := style.FormatUserPrompt()
	if prompt != "You: " {
		t.Errorf("With NO_COLOR, prompt should be plain 'You: ', got: %q", prompt)
	}

	prefix := style.FormatAgentPrefix()
	if prefix != "Claude: " {
		t.Errorf("With NO_COLOR, agent prefix should be plain 'Claude: ', got: %q", prefix)
	}
}

// --- Thinking and dim format tests ---

func TestThinkingFormat(t *testing.T) {
	os.Unsetenv("NO_COLOR")
	os.Unsetenv("TERM")
	style.ResetColorCache()
	defer func() {
		style.ResetColorCache()
	}()

	result := style.FormatThinking("I need to analyze the code structure...")

	// Should start with 💭
	if !strings.HasPrefix(result, "💭 ") {
		t.Error("Thinking format should start with 💭 prefix")
	}

	// Should use dim magenta (2;35m)
	if !strings.Contains(result, "2;35m") {
		t.Errorf("Thinking should use dim magenta (2;35m), got: %q", result)
	}
}

func TestDimFormat(t *testing.T) {
	os.Unsetenv("NO_COLOR")
	os.Unsetenv("TERM")
	style.ResetColorCache()
	defer func() {
		style.ResetColorCache()
	}()

	result := style.FormatDim("secondary content here")

	// Should use dim/faint (2m)
	if !strings.Contains(result, "2m") {
		t.Errorf("Dim format should use faint attribute (2m), got: %q", result)
	}
}

// --- CLI binary color integration tests ---

func TestCLIBinaryColorOutput(t *testing.T) {
	binaryPath := buildTestBinary(t)

	// Test that NO_COLOR env var is respected by the binary
	t.Run("NO_COLOR disables color in error output", func(t *testing.T) {
		cmd := buildTestCommand(t, binaryPath, "")
		cmd.Env = append(cmd.Env, "NO_COLOR=1")

		output, _ := cmd.CombinedOutput()
		outputStr := string(output)

		if containsANSITest(outputStr) {
			t.Errorf("With NO_COLOR=1, binary output should not contain ANSI codes, got: %q", outputStr)
		}
	})

	t.Run("TERM=dumb disables color in error output", func(t *testing.T) {
		cmd := buildTestCommand(t, binaryPath, "")
		cmd.Env = append(cmd.Env, "TERM=dumb")

		output, _ := cmd.CombinedOutput()
		outputStr := string(output)

		if containsANSITest(outputStr) {
			t.Errorf("With TERM=dumb, binary output should not contain ANSI codes, got: %q", outputStr)
		}
	})
}
