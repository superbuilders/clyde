// Package loglevel defines verbosity levels for Clyde's output.
//
// The five levels control what is printed during agent operation:
//
//   - Silent:  Nothing is printed to stdout or stderr (side-effects only).
//   - Quiet:   Only → tool progress lines and the final agent response.
//   - Normal:  Tool output bodies and thinking traces (truncated).
//   - Verbose: All truncation removed.
//   - Debug:   Additional harness diagnostics (token counts, latency, etc.).
package loglevel

import "fmt"

// Level represents a verbosity level for Clyde's output.
type Level int

const (
	// Silent suppresses all output. Only side-effects (file writes, etc.) occur.
	Silent Level = iota
	// Quiet shows only → tool progress lines and the final agent response.
	Quiet
	// Normal is the default. Shows tool output bodies and thinking traces (truncated).
	Normal
	// Verbose removes all truncation from output.
	Verbose
	// Debug adds harness diagnostics: token counts, latency, request/response sizes.
	Debug
)

// String returns the human-readable name of the log level.
func (l Level) String() string {
	switch l {
	case Silent:
		return "silent"
	case Quiet:
		return "quiet"
	case Normal:
		return "normal"
	case Verbose:
		return "verbose"
	case Debug:
		return "debug"
	default:
		return fmt.Sprintf("Level(%d)", int(l))
	}
}

// ShouldShow returns true if content at the given threshold level should be
// displayed at the current log level. For example, if the current level is
// Normal (2) and the threshold is Quiet (1), then the content should be shown
// because Normal >= Quiet.
func (l Level) ShouldShow(threshold Level) bool {
	return l >= threshold
}

// ParseFlags parses CLI flags and returns the appropriate log level.
// It scans the provided args for verbosity flags and returns the level
// plus the remaining args with verbosity flags removed.
//
// Recognized flags:
//   --silent        → Silent
//   -q, --quiet     → Quiet
//   (no flag)       → Normal
//   -v, --verbose   → Verbose
//   --debug         → Debug
//
// If multiple flags are provided, the last one wins.
func ParseFlags(args []string) (Level, []string) {
	level := Normal
	remaining := make([]string, 0, len(args))

	for _, arg := range args {
		switch arg {
		case "--silent":
			level = Silent
		case "-q", "--quiet":
			level = Quiet
		case "-v", "--verbose":
			level = Verbose
		case "--debug":
			level = Debug
		default:
			remaining = append(remaining, arg)
		}
	}

	return level, remaining
}
