// Package truncate provides configurable text truncation for Clyde's output.
//
// The truncation engine enforces line and character limits at Normal log level
// and passes content through unmodified at Verbose and Debug levels.
//
// Limits (at Normal level):
//   - Thinking traces: 50 lines max, then "... (N more lines)"
//   - Tool output bodies: 25 lines max, then "... (N more lines)"
//   - Any single line: 2000 characters max, then "..." appended
//   - Single-line bash commands: never truncated
//   - Multi-line bash commands: 25-line limit applies
//
// At Verbose and Debug levels, all truncation is disabled.
package truncate

import (
	"fmt"
	"strings"

	"github.com/this-is-alpha-iota/clyde/loglevel"
)

const (
	// ThinkingLineLimit is the maximum number of lines for thinking traces
	// at Normal log level.
	ThinkingLineLimit = 50

	// ToolOutputLineLimit is the maximum number of lines for tool output
	// bodies at Normal log level.
	ToolOutputLineLimit = 25

	// MaxCharsPerLine is the maximum number of characters per line at
	// Normal log level. Lines exceeding this are truncated with "...".
	MaxCharsPerLine = 2000
)

// Lines truncates text to the given maximum number of lines.
// If the text exceeds maxLines, it is truncated and a message like
// "... (N more lines)" is appended.
//
// At Verbose or Debug level, the text is returned unmodified.
// At any level, if the text has maxLines or fewer lines, it is returned as-is.
func Lines(text string, maxLines int, level loglevel.Level) string {
	if level.ShouldShow(loglevel.Verbose) {
		return text
	}

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
//
// At Verbose or Debug level, the line is returned unmodified.
func Chars(line string, level loglevel.Level) string {
	if level.ShouldShow(loglevel.Verbose) {
		return line
	}

	if len(line) <= MaxCharsPerLine {
		return line
	}

	return line[:MaxCharsPerLine] + "..."
}

// Text applies both line truncation and per-line character truncation.
// The maxLines parameter controls the line limit. Each individual line
// is also subject to MaxCharsPerLine character truncation.
//
// At Verbose or Debug level, the text is returned unmodified.
func Text(text string, maxLines int, level loglevel.Level) string {
	if level.ShouldShow(loglevel.Verbose) {
		return text
	}

	lines := strings.Split(text, "\n")

	// Apply character truncation to each line
	for i, line := range lines {
		lines[i] = Chars(line, level)
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
func Thinking(text string, level loglevel.Level) string {
	return Text(text, ThinkingLineLimit, level)
}

// ToolOutput truncates tool output text using ToolOutputLineLimit.
// Convenience wrapper around Text with the tool-output-specific limit.
func ToolOutput(text string, level loglevel.Level) string {
	return Text(text, ToolOutputLineLimit, level)
}
