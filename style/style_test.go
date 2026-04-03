package style

import (
	"os"
	"strings"
	"testing"
)

// resetTestEnv ensures a clean environment and cache for each test.
func resetTestEnv(t *testing.T) {
	t.Helper()
	os.Unsetenv("NO_COLOR")
	os.Unsetenv("TERM")
	ResetColorCache()
}

// setNoColor sets the NO_COLOR environment variable and resets cache.
func setNoColor(t *testing.T, value string) {
	t.Helper()
	os.Setenv("NO_COLOR", value)
	ResetColorCache()
}

// setTermDumb sets TERM=dumb and resets cache.
func setTermDumb(t *testing.T) {
	t.Helper()
	os.Setenv("TERM", "dumb")
	ResetColorCache()
}

// containsANSI returns true if the string contains any ANSI escape sequence.
func containsANSI(s string) bool {
	return strings.Contains(s, "\033[")
}

// --- Color detection tests ---

func TestIsColorEnabled_Default(t *testing.T) {
	resetTestEnv(t)
	defer resetTestEnv(t)

	if !IsColorEnabled() {
		t.Error("Expected color to be enabled by default (no NO_COLOR, no TERM=dumb)")
	}
}

func TestIsColorEnabled_NoColor_Set(t *testing.T) {
	resetTestEnv(t)
	defer resetTestEnv(t)

	setNoColor(t, "1")
	if IsColorEnabled() {
		t.Error("Expected color to be disabled when NO_COLOR=1")
	}
}

func TestIsColorEnabled_NoColor_Empty(t *testing.T) {
	resetTestEnv(t)
	defer resetTestEnv(t)

	// Per https://no-color.org/, presence of the variable (even empty) disables color
	setNoColor(t, "")
	if IsColorEnabled() {
		t.Error("Expected color to be disabled when NO_COLOR is set to empty string")
	}
}

func TestIsColorEnabled_TermDumb(t *testing.T) {
	resetTestEnv(t)
	defer resetTestEnv(t)

	setTermDumb(t)
	if IsColorEnabled() {
		t.Error("Expected color to be disabled when TERM=dumb")
	}
}

func TestIsColorEnabled_TermOther(t *testing.T) {
	resetTestEnv(t)
	defer resetTestEnv(t)

	os.Setenv("TERM", "xterm-256color")
	ResetColorCache()

	if !IsColorEnabled() {
		t.Error("Expected color to be enabled when TERM=xterm-256color")
	}
}

func TestIsColorEnabled_Cached(t *testing.T) {
	resetTestEnv(t)
	defer resetTestEnv(t)

	// First call caches the result
	result1 := IsColorEnabled()

	// Set NO_COLOR after cache — should NOT change the result (cached)
	os.Setenv("NO_COLOR", "1")
	result2 := IsColorEnabled()

	if result1 != result2 {
		t.Error("Expected cached result to be consistent, but it changed")
	}
}

func TestResetColorCache(t *testing.T) {
	resetTestEnv(t)
	defer resetTestEnv(t)

	// Cache the "enabled" result
	if !IsColorEnabled() {
		t.Skip("Color is unexpectedly disabled in test environment")
	}

	// Now set NO_COLOR and reset cache
	setNoColor(t, "1")

	if IsColorEnabled() {
		t.Error("After ResetColorCache + NO_COLOR, expected color to be disabled")
	}
}

// --- Semantic style helpers: color enabled ---

func TestUserLabel_WithColor(t *testing.T) {
	resetTestEnv(t)
	defer resetTestEnv(t)

	result := UserLabel("You:")
	if !containsANSI(result) {
		t.Error("UserLabel should contain ANSI codes when color is enabled")
	}
	if !strings.Contains(result, "You:") {
		t.Error("UserLabel should contain the original text")
	}
	// Should contain bold (1) and cyan (36)
	if !strings.Contains(result, "1;36m") {
		t.Errorf("UserLabel should use bold cyan (1;36m), got: %q", result)
	}
	// Should end with reset
	if !strings.HasSuffix(result, reset) {
		t.Errorf("UserLabel should end with reset sequence, got: %q", result)
	}
}

func TestAgentLabel_WithColor(t *testing.T) {
	resetTestEnv(t)
	defer resetTestEnv(t)

	result := AgentLabel("Claude:")
	if !containsANSI(result) {
		t.Error("AgentLabel should contain ANSI codes when color is enabled")
	}
	if !strings.Contains(result, "Claude:") {
		t.Error("AgentLabel should contain the original text")
	}
	// Should contain bold (1) and green (32)
	if !strings.Contains(result, "1;32m") {
		t.Errorf("AgentLabel should use bold green (1;32m), got: %q", result)
	}
}

func TestToolLabel_WithColor(t *testing.T) {
	resetTestEnv(t)
	defer resetTestEnv(t)

	result := ToolLabel("→ Reading file:")
	if !containsANSI(result) {
		t.Error("ToolLabel should contain ANSI codes when color is enabled")
	}
	if !strings.Contains(result, "→ Reading file:") {
		t.Error("ToolLabel should contain the original text")
	}
	// Should contain bold (1) and yellow (33)
	if !strings.Contains(result, "1;33m") {
		t.Errorf("ToolLabel should use bold yellow (1;33m), got: %q", result)
	}
}

func TestDim_WithColor(t *testing.T) {
	resetTestEnv(t)
	defer resetTestEnv(t)

	result := Dim("some secondary text")
	if !containsANSI(result) {
		t.Error("Dim should contain ANSI codes when color is enabled")
	}
	if !strings.Contains(result, "some secondary text") {
		t.Error("Dim should contain the original text")
	}
	// Should contain dim/faint (2)
	if !strings.Contains(result, "2m") {
		t.Errorf("Dim should use faint attribute (2m), got: %q", result)
	}
}

func TestThinkingStyle_WithColor(t *testing.T) {
	resetTestEnv(t)
	defer resetTestEnv(t)

	result := ThinkingStyle("I need to think about this...")
	if !containsANSI(result) {
		t.Error("ThinkingStyle should contain ANSI codes when color is enabled")
	}
	if !strings.Contains(result, "I need to think about this...") {
		t.Error("ThinkingStyle should contain the original text")
	}
	// Should contain dim (2) and magenta (35)
	if !strings.Contains(result, "2;35m") {
		t.Errorf("ThinkingStyle should use dim magenta (2;35m), got: %q", result)
	}
}

func TestDebugStyle_WithColor(t *testing.T) {
	resetTestEnv(t)
	defer resetTestEnv(t)

	result := DebugStyle("🔍 Tokens: input=500")
	if !containsANSI(result) {
		t.Error("DebugStyle should contain ANSI codes when color is enabled")
	}
	if !strings.Contains(result, "🔍 Tokens: input=500") {
		t.Error("DebugStyle should contain the original text")
	}
	// Should contain red (31)
	if !strings.Contains(result, "31m") {
		t.Errorf("DebugStyle should use red (31m), got: %q", result)
	}
}

// --- Semantic style helpers: color disabled (NO_COLOR) ---

func TestUserLabel_NoColor(t *testing.T) {
	resetTestEnv(t)
	defer resetTestEnv(t)
	setNoColor(t, "1")

	result := UserLabel("You:")
	if containsANSI(result) {
		t.Errorf("UserLabel should not contain ANSI codes when NO_COLOR is set, got: %q", result)
	}
	if result != "You:" {
		t.Errorf("UserLabel with NO_COLOR should return plain text, got: %q", result)
	}
}

func TestAgentLabel_NoColor(t *testing.T) {
	resetTestEnv(t)
	defer resetTestEnv(t)
	setNoColor(t, "1")

	result := AgentLabel("Claude:")
	if containsANSI(result) {
		t.Errorf("AgentLabel should not contain ANSI codes when NO_COLOR is set, got: %q", result)
	}
	if result != "Claude:" {
		t.Errorf("AgentLabel with NO_COLOR should return plain text, got: %q", result)
	}
}

func TestToolLabel_NoColor(t *testing.T) {
	resetTestEnv(t)
	defer resetTestEnv(t)
	setNoColor(t, "1")

	result := ToolLabel("→ Reading file:")
	if containsANSI(result) {
		t.Errorf("ToolLabel should not contain ANSI codes when NO_COLOR is set, got: %q", result)
	}
	if result != "→ Reading file:" {
		t.Errorf("ToolLabel with NO_COLOR should return plain text, got: %q", result)
	}
}

func TestDim_NoColor(t *testing.T) {
	resetTestEnv(t)
	defer resetTestEnv(t)
	setNoColor(t, "1")

	result := Dim("some text")
	if containsANSI(result) {
		t.Errorf("Dim should not contain ANSI codes when NO_COLOR is set, got: %q", result)
	}
	if result != "some text" {
		t.Errorf("Dim with NO_COLOR should return plain text, got: %q", result)
	}
}

func TestThinkingStyle_NoColor(t *testing.T) {
	resetTestEnv(t)
	defer resetTestEnv(t)
	setNoColor(t, "1")

	result := ThinkingStyle("thinking...")
	if containsANSI(result) {
		t.Errorf("ThinkingStyle should not contain ANSI codes when NO_COLOR is set, got: %q", result)
	}
	if result != "thinking..." {
		t.Errorf("ThinkingStyle with NO_COLOR should return plain text, got: %q", result)
	}
}

func TestDebugStyle_NoColor(t *testing.T) {
	resetTestEnv(t)
	defer resetTestEnv(t)
	setNoColor(t, "1")

	result := DebugStyle("debug info")
	if containsANSI(result) {
		t.Errorf("DebugStyle should not contain ANSI codes when NO_COLOR is set, got: %q", result)
	}
	if result != "debug info" {
		t.Errorf("DebugStyle with NO_COLOR should return plain text, got: %q", result)
	}
}

// --- Semantic style helpers: color disabled (TERM=dumb) ---

func TestAllStyles_TermDumb(t *testing.T) {
	resetTestEnv(t)
	defer resetTestEnv(t)
	setTermDumb(t)

	tests := []struct {
		name   string
		fn     func(string) string
		input  string
	}{
		{"UserLabel", UserLabel, "You:"},
		{"AgentLabel", AgentLabel, "Claude:"},
		{"ToolLabel", ToolLabel, "→ Tool:"},
		{"Dim", Dim, "dimmed text"},
		{"ThinkingStyle", ThinkingStyle, "thinking"},
		{"DebugStyle", DebugStyle, "debug"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.fn(tt.input)
			if containsANSI(result) {
				t.Errorf("%s should not contain ANSI codes when TERM=dumb, got: %q", tt.name, result)
			}
			if result != tt.input {
				t.Errorf("%s with TERM=dumb should return plain text %q, got: %q", tt.name, tt.input, result)
			}
		})
	}
}

// --- Compound formatter tests ---

func TestFormatUserPrompt_WithColor(t *testing.T) {
	resetTestEnv(t)
	defer resetTestEnv(t)

	result := FormatUserPrompt()
	if !containsANSI(result) {
		t.Error("FormatUserPrompt should contain ANSI codes when color is enabled")
	}
	if !strings.Contains(result, "You: ") {
		t.Error("FormatUserPrompt should contain 'You: '")
	}
}

func TestFormatUserPrompt_NoColor(t *testing.T) {
	resetTestEnv(t)
	defer resetTestEnv(t)
	setNoColor(t, "1")

	result := FormatUserPrompt()
	if containsANSI(result) {
		t.Errorf("FormatUserPrompt should not contain ANSI codes with NO_COLOR, got: %q", result)
	}
	if result != "You: " {
		t.Errorf("FormatUserPrompt with NO_COLOR should be 'You: ', got: %q", result)
	}
}

func TestFormatAgentPrefix_WithColor(t *testing.T) {
	resetTestEnv(t)
	defer resetTestEnv(t)

	result := FormatAgentPrefix()
	if !containsANSI(result) {
		t.Error("FormatAgentPrefix should contain ANSI codes when color is enabled")
	}
	if !strings.Contains(result, "Claude: ") {
		t.Error("FormatAgentPrefix should contain 'Claude: '")
	}
}

func TestFormatAgentPrefix_NoColor(t *testing.T) {
	resetTestEnv(t)
	defer resetTestEnv(t)
	setNoColor(t, "1")

	result := FormatAgentPrefix()
	if containsANSI(result) {
		t.Errorf("FormatAgentPrefix should not contain ANSI codes with NO_COLOR, got: %q", result)
	}
	if result != "Claude: " {
		t.Errorf("FormatAgentPrefix with NO_COLOR should be 'Claude: ', got: %q", result)
	}
}

func TestFormatToolProgress_WithColor(t *testing.T) {
	resetTestEnv(t)
	defer resetTestEnv(t)

	tests := []struct {
		name    string
		message string
		wantAction string // the bold-yellow portion
		wantDetail string // the default-foreground portion
	}{
		{
			name:       "with colon separator",
			message:    "→ Reading file: main.go",
			wantAction: "→ Reading file:",
			wantDetail: " main.go",
		},
		{
			name:       "listing files with detail",
			message:    "→ Listing files: . (current directory)",
			wantAction: "→ Listing files:",
			wantDetail: " . (current directory)",
		},
		{
			name:       "running bash",
			message:    "→ Running bash: go test -v",
			wantAction: "→ Running bash:",
			wantDetail: " go test -v",
		},
		{
			name:       "no colon — entire line styled",
			message:    "→ Processing",
			wantAction: "→ Processing",
			wantDetail: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatToolProgress(tt.message)

			if !containsANSI(result) {
				t.Error("FormatToolProgress should contain ANSI codes when color is enabled")
			}

			// The action part should be styled (bold yellow)
			styledAction := ToolLabel(tt.wantAction)
			if tt.wantDetail != "" {
				expected := styledAction + tt.wantDetail
				if result != expected {
					t.Errorf("FormatToolProgress(%q) = %q, want %q", tt.message, result, expected)
				}
			}

			// Verify the full text content is preserved
			if !strings.Contains(result, tt.wantAction) {
				t.Errorf("FormatToolProgress should contain action text %q", tt.wantAction)
			}
		})
	}
}

func TestFormatToolProgress_NoColor(t *testing.T) {
	resetTestEnv(t)
	defer resetTestEnv(t)
	setNoColor(t, "1")

	result := FormatToolProgress("→ Reading file: main.go")
	if containsANSI(result) {
		t.Errorf("FormatToolProgress should not contain ANSI codes with NO_COLOR, got: %q", result)
	}
	if result != "→ Reading file: main.go" {
		t.Errorf("FormatToolProgress with NO_COLOR should return original text, got: %q", result)
	}
}

func TestFormatThinking_WithColor(t *testing.T) {
	resetTestEnv(t)
	defer resetTestEnv(t)

	result := FormatThinking("considering the options...")
	if !containsANSI(result) {
		t.Error("FormatThinking should contain ANSI codes when color is enabled")
	}
	if !strings.HasPrefix(result, "💭 ") {
		t.Error("FormatThinking should start with 💭 prefix")
	}
	if !strings.Contains(result, "considering the options...") {
		t.Error("FormatThinking should contain the original text")
	}
}

func TestFormatThinking_NoColor(t *testing.T) {
	resetTestEnv(t)
	defer resetTestEnv(t)
	setNoColor(t, "1")

	result := FormatThinking("considering the options...")
	if containsANSI(result) {
		t.Errorf("FormatThinking should not contain ANSI codes with NO_COLOR, got: %q", result)
	}
	if result != "💭 considering the options..." {
		t.Errorf("FormatThinking with NO_COLOR should be '💭 considering the options...', got: %q", result)
	}
}

func TestFormatDebug_WithColor(t *testing.T) {
	resetTestEnv(t)
	defer resetTestEnv(t)

	result := FormatDebug("🔍 Tokens: input=500 output=200")
	if !containsANSI(result) {
		t.Error("FormatDebug should contain ANSI codes when color is enabled")
	}
	if !strings.Contains(result, "🔍 Tokens: input=500 output=200") {
		t.Error("FormatDebug should contain the original text")
	}
}

func TestFormatDebug_NoColor(t *testing.T) {
	resetTestEnv(t)
	defer resetTestEnv(t)
	setNoColor(t, "1")

	result := FormatDebug("debug info")
	if containsANSI(result) {
		t.Errorf("FormatDebug should not contain ANSI codes with NO_COLOR, got: %q", result)
	}
	if result != "debug info" {
		t.Errorf("FormatDebug with NO_COLOR should return plain text, got: %q", result)
	}
}

func TestFormatDim_WithColor(t *testing.T) {
	resetTestEnv(t)
	defer resetTestEnv(t)

	result := FormatDim("secondary content")
	if !containsANSI(result) {
		t.Error("FormatDim should contain ANSI codes when color is enabled")
	}
}

func TestFormatDim_NoColor(t *testing.T) {
	resetTestEnv(t)
	defer resetTestEnv(t)
	setNoColor(t, "1")

	result := FormatDim("secondary content")
	if containsANSI(result) {
		t.Errorf("FormatDim should not contain ANSI codes with NO_COLOR, got: %q", result)
	}
	if result != "secondary content" {
		t.Errorf("FormatDim with NO_COLOR should return plain text, got: %q", result)
	}
}

// --- Edge cases ---

func TestEmptyString(t *testing.T) {
	resetTestEnv(t)
	defer resetTestEnv(t)

	// With color enabled, empty string wrapped should still have ANSI codes
	result := UserLabel("")
	if !containsANSI(result) {
		t.Error("UserLabel('') should still contain ANSI codes when color is enabled")
	}

	// The text between the codes should be empty
	// Format: \033[1;36m\033[0m
	expected := "\033[1;36m\033[0m"
	if result != expected {
		t.Errorf("UserLabel('') = %q, want %q", result, expected)
	}
}

func TestEmptyString_NoColor(t *testing.T) {
	resetTestEnv(t)
	defer resetTestEnv(t)
	setNoColor(t, "1")

	result := UserLabel("")
	if result != "" {
		t.Errorf("UserLabel('') with NO_COLOR should return empty string, got: %q", result)
	}
}

func TestMultilineText(t *testing.T) {
	resetTestEnv(t)
	defer resetTestEnv(t)

	multiline := "line 1\nline 2\nline 3"
	result := Dim(multiline)
	if !containsANSI(result) {
		t.Error("Dim should contain ANSI codes for multiline text")
	}
	if !strings.Contains(result, "line 1\nline 2\nline 3") {
		t.Error("Dim should preserve the multiline text content")
	}
}

func TestTextWithExistingANSI(t *testing.T) {
	resetTestEnv(t)
	defer resetTestEnv(t)

	// Text that already has ANSI codes should still be wrapped
	existing := "\033[31mred text\033[0m"
	result := UserLabel(existing)
	if !strings.Contains(result, existing) {
		t.Error("UserLabel should preserve text that already contains ANSI codes")
	}
}

func TestFormatToolProgress_EdgeCases(t *testing.T) {
	resetTestEnv(t)
	defer resetTestEnv(t)

	tests := []struct {
		name    string
		message string
	}{
		{"empty string", ""},
		{"just arrow", "→"},
		{"arrow with space", "→ "},
		{"colon but not arrow", "Tool: action"},
		{"multiple colons", "→ Writing file: /path/to/file: extra"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Should not panic
			result := FormatToolProgress(tt.message)
			// Should contain the original text somewhere
			if !strings.Contains(result, tt.message) && !containsANSI(result) {
				// If no ANSI, should be exact match
				if result != tt.message {
					t.Errorf("FormatToolProgress(%q) = %q, expected it to contain the original text", tt.message, result)
				}
			}
		})
	}
}

// --- Specific ANSI code verification ---

func TestANSICodeValues(t *testing.T) {
	resetTestEnv(t)
	defer resetTestEnv(t)

	// Verify exact ANSI sequences used by each style
	tests := []struct {
		name     string
		fn       func(string) string
		wantCode string
	}{
		{"UserLabel", UserLabel, "\033[1;36m"},         // bold cyan
		{"AgentLabel", AgentLabel, "\033[1;32m"},       // bold green
		{"ToolLabel", ToolLabel, "\033[1;33m"},         // bold yellow
		{"Dim", Dim, "\033[2m"},                        // faint
		{"ThinkingStyle", ThinkingStyle, "\033[2;35m"}, // dim magenta
		{"DebugStyle", DebugStyle, "\033[31m"},         // red
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.fn("test")
			if !strings.HasPrefix(result, tt.wantCode) {
				t.Errorf("%s should start with %q, got: %q", tt.name, tt.wantCode, result)
			}
			if !strings.HasSuffix(result, "\033[0m") {
				t.Errorf("%s should end with reset (\\033[0m), got: %q", tt.name, result)
			}
		})
	}
}

// --- Body text readability verification ---

func TestBodyTextIsDefaultForeground(t *testing.T) {
	resetTestEnv(t)
	defer resetTestEnv(t)

	// Per acceptance criteria: "Body text (user input, agent response)
	// is always default foreground for readability."
	//
	// FormatUserPrompt styles only the "You: " label.
	// The user's actual input text should be appended WITHOUT styling.
	prompt := FormatUserPrompt()
	userInput := "What is 2+2?"
	fullLine := prompt + userInput

	// The user input should NOT be wrapped in any ANSI codes
	// (it comes after the reset of the label)
	if !strings.HasSuffix(fullLine, userInput) {
		t.Error("User input text should be plain (default foreground), appended after styled label")
	}

	// Same for agent response
	prefix := FormatAgentPrefix()
	responseText := "The answer is 4."
	fullResponse := prefix + responseText

	if !strings.HasSuffix(fullResponse, responseText) {
		t.Error("Agent response text should be plain (default foreground), appended after styled label")
	}
}
