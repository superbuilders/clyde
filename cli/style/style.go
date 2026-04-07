// Package style provides semantic color helpers for Clyde's terminal output.
//
// Each helper wraps text in ANSI escape codes for a specific semantic role:
//
//   - UserLabel:     Bold cyan    — the "You:" prompt label
//   - AgentLabel:    Bold green   — the "Claude:" response label
//   - ToolLabel:     Bold yellow  — tool name in "→ ToolName:" progress lines
//   - Dim:           Faint        — secondary content (tool output bodies)
//   - ThinkingStyle: Dim magenta  — thinking trace text
//   - DebugStyle:    Red          — debug-level diagnostic lines
//
// Colors are automatically disabled when the NO_COLOR environment variable is
// set (any value, per https://no-color.org/) or when TERM=dumb.
//
// All helpers use named ANSI colors (not hardcoded RGB or black/white) to work
// well on both dark and light terminal themes.
package style

import (
	"fmt"
	"os"
	"strings"
	"sync"
)

// ANSI escape code components
const (
	esc   = "\033["
	reset = esc + "0m"
)

// Reset is the ANSI reset sequence, exported for testing.
const Reset = "\033[0m"

const (

	bold = "1"
	dim  = "2"

	fgCyan    = "36"
	fgGreen   = "32"
	fgYellow  = "33"
	fgMagenta = "35"
	fgRed     = "31"
)

// colorEnabled caches whether color output is enabled.
// It is computed once on first access.
var (
	colorEnabled     bool
	colorEnabledOnce sync.Once
)

// IsColorEnabled returns true if ANSI color output is enabled.
// Color is disabled when:
//   - The NO_COLOR environment variable is set (any value, including empty)
//   - TERM is set to "dumb"
//
// The result is cached after the first call for performance.
func IsColorEnabled() bool {
	colorEnabledOnce.Do(func() {
		colorEnabled = detectColor()
	})
	return colorEnabled
}

// detectColor checks environment variables to determine if color is supported.
func detectColor() bool {
	// NO_COLOR convention: https://no-color.org/
	// "When set, command-line software should not output ANSI color escape codes."
	// Presence of the variable (even empty) disables color.
	if _, exists := os.LookupEnv("NO_COLOR"); exists {
		return false
	}

	// TERM=dumb is the traditional Unix signal for a terminal that
	// doesn't support escape sequences.
	if os.Getenv("TERM") == "dumb" {
		return false
	}

	return true
}

// ResetColorCache clears the cached color detection result.
// This is primarily useful for testing — production code should not call this.
func ResetColorCache() {
	colorEnabledOnce = sync.Once{}
}

// wrap applies ANSI attributes to text. If color is disabled, returns text unchanged.
func wrap(text string, attrs ...string) string {
	if !IsColorEnabled() {
		return text
	}
	code := esc + strings.Join(attrs, ";") + "m"
	return code + text + reset
}

// --- Semantic style helpers ---

// UserLabel styles text as a user input label (bold cyan).
// Used for the "You:" prompt label.
func UserLabel(text string) string {
	return wrap(text, bold, fgCyan)
}

// AgentLabel styles text as an agent response label (bold green).
// Used for the "Claude:" response label.
func AgentLabel(text string) string {
	return wrap(text, bold, fgGreen)
}

// ToolLabel styles text as a tool label (bold yellow).
// Used for the tool name in "→ ToolName:" progress lines.
func ToolLabel(text string) string {
	return wrap(text, bold, fgYellow)
}

// Dim styles text as secondary/de-emphasized content (faint attribute).
// Used for tool output bodies.
func Dim(text string) string {
	return wrap(text, dim)
}

// ThinkingStyle styles text as thinking trace content (dim magenta).
// Used for Claude's thinking blocks, prefixed with 💭.
func ThinkingStyle(text string) string {
	return wrap(text, dim, fgMagenta)
}

// DebugStyle styles text as debug-level output (red).
// Used for harness diagnostics at Debug log level.
func DebugStyle(text string) string {
	return wrap(text, fgRed)
}

// --- Compound formatters ---

// FormatUserPrompt returns a styled "You: " label.
// The user's input text itself is NOT styled (default foreground for readability).
func FormatUserPrompt() string {
	return UserLabel("You: ")
}

// FormatAgentPrefix returns a styled "Claude: " label.
// The agent's response text itself is NOT styled (default foreground for readability).
func FormatAgentPrefix() string {
	return AgentLabel("Claude: ")
}

// FormatToolProgress formats a tool progress line with the tool name in bold yellow
// and the rest in default color. The arrow "→" is part of the tool label styling.
//
// Example: "→ Reading file: main.go" → the "→ Reading file:" part is bold yellow.
func FormatToolProgress(message string) string {
	// Find the colon that separates the tool action from the detail
	// Pattern: "→ Action: detail" or "→ Action"
	if idx := strings.Index(message, ": "); idx > 0 && strings.HasPrefix(message, "→") {
		action := message[:idx+1] // "→ Reading file:"
		detail := message[idx+1:] // " main.go"
		return ToolLabel(action) + detail
	}
	// No colon found — style the entire line
	return ToolLabel(message)
}

// FormatThinking formats a thinking trace line with the 💭 prefix and dim magenta styling.
func FormatThinking(text string) string {
	return fmt.Sprintf("💭 %s", ThinkingStyle(text))
}

// FormatDebug formats a debug-level line in red.
func FormatDebug(text string) string {
	return DebugStyle(text)
}

// FormatDim formats text as dim/faint secondary content.
func FormatDim(text string) string {
	return Dim(text)
}
