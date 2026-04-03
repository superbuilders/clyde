// Package input provides a rich text input widget for Clyde's REPL mode.
//
// It wraps chzyer/readline to provide:
//   - Cursor movement (left/right arrow, Home/End)
//   - Multiline input (Ctrl+J or Alt+Enter inserts a newline)
//   - Session-level history recall (up/down arrows)
//   - No artificial length limit
//   - Dynamic prompt updates (git branch, context %, You: label)
//
// The package is used in REPL mode only. CLI mode reads from args/stdin
// and does not use this package.
package input

import (
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/chzyer/readline"
)

// Reader provides rich line-editing for the REPL.
type Reader struct {
	rl          *readline.Instance
	multiline   bool     // true if we're in multiline accumulation mode
	lines       []string // accumulated lines in multiline mode
	historyPath string   // path to the history file
}

// Config holds configuration for the input Reader.
type Config struct {
	// Prompt is the initial prompt string (may contain ANSI codes).
	Prompt string
	// HistoryFile is the path to persist history. Empty disables file persistence.
	HistoryFile string
	// Stdin overrides the default stdin (for testing).
	Stdin io.ReadCloser
	// Stdout overrides the default stdout (for testing).
	Stdout io.Writer
	// Stderr overrides the default stderr (for testing).
	Stderr io.Writer
}

// New creates a new Reader with the given configuration.
//
// The Reader supports:
//   - Left/right arrow keys for cursor movement within the line
//   - Home/End keys to jump to start/end of input
//   - Enter to submit the input
//   - Ctrl+J to insert a newline (multiline input)
//   - Up/down arrow keys to recall previous inputs
//   - Ctrl+C to cancel current input (returns empty)
//   - Ctrl+D to signal EOF (exit)
func New(cfg Config) (*Reader, error) {
	historyFile := cfg.HistoryFile
	if historyFile == "" {
		// Default to ~/.clyde/history
		homeDir, err := os.UserHomeDir()
		if err == nil {
			historyFile = filepath.Join(homeDir, ".clyde", "history")
		}
	}

	rlConfig := &readline.Config{
		Prompt:                 cfg.Prompt,
		HistoryFile:            historyFile,
		HistoryLimit:           1000,
		DisableAutoSaveHistory: true, // We save manually after multiline assembly
		InterruptPrompt:        "^C",
		EOFPrompt:              "exit",

		// Ctrl+J (linefeed) acts as a newline insertion in our multiline handler.
		// readline treats it as "accept line" by default in some modes, but we
		// intercept it via the Listener to set multiline mode.
	}

	// Apply overrides for testing
	if cfg.Stdin != nil {
		rlConfig.Stdin = cfg.Stdin
	}
	if cfg.Stdout != nil {
		rlConfig.Stdout = cfg.Stdout
	}
	if cfg.Stderr != nil {
		rlConfig.Stderr = cfg.Stderr
	}

	rl, err := readline.NewEx(rlConfig)
	if err != nil {
		return nil, err
	}

	return &Reader{
		rl:          rl,
		historyPath: historyFile,
	}, nil
}

// continuationPrompt is shown for subsequent lines in multiline mode.
// It is indented to align with the content after "You: " in the main prompt.
const continuationPrompt = "  > "

// ReadLine reads a line (or multiline block) of input from the user.
//
// Returns:
//   - The assembled input string (trimmed)
//   - An error: nil on success, io.EOF on Ctrl+D, ErrInterrupt on Ctrl+C
//
// Multiline mode is entered when a line ends with a backslash (\).
// The backslash is stripped and subsequent lines are accumulated until
// a line does NOT end with a backslash. Ctrl+J also inserts a newline
// inline (the readline library handles this natively for some key combos).
//
// History is saved as the complete assembled input (single or multiline).
func (r *Reader) ReadLine() (string, error) {
	r.multiline = false
	r.lines = nil

	for {
		line, err := r.rl.Readline()
		if err != nil {
			// If we were accumulating multiline and got interrupted,
			// discard the partial input
			if r.multiline {
				r.multiline = false
				r.lines = nil
				r.rl.SetPrompt(r.rl.Config.Prompt)
			}
			return "", err
		}

		// Check for line continuation (trailing backslash)
		if strings.HasSuffix(line, "\\") {
			// Enter or continue multiline mode
			line = line[:len(line)-1] // strip the backslash
			r.lines = append(r.lines, line)
			r.multiline = true
			r.rl.SetPrompt(continuationPrompt)
			continue
		}

		if r.multiline {
			// Final line of multiline input
			r.lines = append(r.lines, line)
			result := strings.Join(r.lines, "\n")
			r.multiline = false
			r.lines = nil
			r.rl.SetPrompt(r.rl.Config.Prompt)

			// Save assembled input to history
			if strings.TrimSpace(result) != "" {
				r.rl.SaveHistory(result)
			}
			return result, nil
		}

		// Single-line input
		if strings.TrimSpace(line) != "" {
			r.rl.SaveHistory(line)
		}
		return line, nil
	}
}

// SetPrompt updates the prompt string displayed to the user.
// This should be called before each ReadLine() to refresh git info and context %.
func (r *Reader) SetPrompt(prompt string) {
	r.rl.SetPrompt(prompt)
}

// Close cleans up the readline instance. Must be called before process exit.
func (r *Reader) Close() error {
	return r.rl.Close()
}

// Stdout returns a writer that is safe to use while readline is active.
// Writing to this writer will properly refresh the prompt after output.
func (r *Reader) Stdout() io.Writer {
	return r.rl.Stdout()
}

// Stderr returns a writer that is safe to use while readline is active.
// Writing to this writer will properly refresh the prompt after output.
func (r *Reader) Stderr() io.Writer {
	return r.rl.Stderr()
}

// IsMultiline returns true if the reader is currently in multiline
// accumulation mode (for testing).
func (r *Reader) IsMultiline() bool {
	return r.multiline
}

// AccumulatedLines returns the lines accumulated so far in multiline mode
// (for testing). Returns nil if not in multiline mode.
func (r *Reader) AccumulatedLines() []string {
	if !r.multiline {
		return nil
	}
	result := make([]string, len(r.lines))
	copy(result, r.lines)
	return result
}
