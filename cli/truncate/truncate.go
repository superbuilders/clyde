// Package truncate provides configurable text truncation for Clyde's output.
//
// The truncation engine enforces line and character limits. Functions always
// truncate when limits are exceeded. The caller (typically the CLI layer)
// decides whether to call truncation based on its own display policy
// (e.g., bypass at Verbose/Debug levels).
//
// Limits:
//   - Thinking traces: 50 lines max, then "... (N more lines)"
//   - Tool output bodies: 25 lines max, then "... (N more lines)"
//   - Any single line: 2000 characters max, then "..." appended
//   - Single-line content: never line-truncated (only 1 line)
//   - Multi-line content: line limit applies
package truncate

import (
	"fmt"
	"strings"
)

const (
	// ThinkingLineLimit is the maximum number of lines for thinking traces.
	ThinkingLineLimit = 50

	// ToolOutputLineLimit is the maximum number of lines for tool output bodies.
	ToolOutputLineLimit = 25

	// MaxCharsPerLine is the maximum number of characters per line.
	// Lines exceeding this are truncated with "...".
	MaxCharsPerLine = 2000
)

// Lines truncates text to the given maximum number of lines.
// If the text exceeds maxLines, it is truncated and a message like
// "... (N more lines)" is appended.
func Lines(text string, maxLines int) string {
	lines := strings.Split(text, "\n")
	if len(lines) <= maxLines {
		return text
	}

	kept := lines[:maxLines]
	overflow := len(lines) - maxLines
	return strings.Join(kept, "\n") + fmt.Sprintf("\n... (%d more lines)", overflow)
}

// Chars truncates a single line to MaxCharsPerLine characters.
// If the line exceeds the limit, it is truncated and "..." is appended.
func Chars(line string) string {
	if len(line) <= MaxCharsPerLine {
		return line
	}

	return line[:MaxCharsPerLine] + "..."
}

// Text applies both line truncation and per-line character truncation.
// The maxLines parameter controls the line limit. Each individual line
// is also subject to MaxCharsPerLine character truncation.
func Text(text string, maxLines int) string {
	lines := strings.Split(text, "\n")

	// Apply character truncation to each line
	for i, line := range lines {
		lines[i] = Chars(line)
	}

	// Apply line truncation
	if len(lines) > maxLines {
		kept := lines[:maxLines]
		overflow := len(lines) - maxLines
		return strings.Join(kept, "\n") + fmt.Sprintf("\n... (%d more lines)", overflow)
	}

	return strings.Join(lines, "\n")
}

// Thinking truncates thinking trace text using ThinkingLineLimit.
// Convenience wrapper around Text with the thinking-specific limit.
func Thinking(text string) string {
	return Text(text, ThinkingLineLimit)
}

// ToolOutput truncates tool output text using ToolOutputLineLimit.
// Convenience wrapper around Text with the tool-output-specific limit.
func ToolOutput(text string) string {
	return Text(text, ToolOutputLineLimit)
}
