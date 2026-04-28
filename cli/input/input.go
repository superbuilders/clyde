// Package input provides a rich text input widget for Clyde's REPL mode.
//
// It provides:
//   - Cursor movement (left/right arrow, Home/End)
//   - Multiline input via three methods:
//     1. Backslash continuation: end a line with \ to continue
//     2. Ctrl+J: inserts a newline without submitting (universal, works everywhere)
//     3. Alt+Enter: inserts a newline without submitting (requires Meta key)
//   - Session-level history recall (up/down arrows, only on empty prompt)
//   - Up/down navigation between lines in multiline mode
//   - No artificial length limit
//   - Dynamic prompt updates (git branch, context %, You: label)
//
// The package is used in REPL mode only. CLI mode reads from args/stdin
// and does not use this package.
package input

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ErrInterrupt is returned by ReadLine when the user presses Ctrl+C.
var ErrInterrupt = errors.New("interrupted")

// ContinuationPrompt is shown for subsequent lines in multiline mode.
// It is indented to align with the content after "You: " in the main prompt.
const ContinuationPrompt = "  > "

// Reader provides rich line-editing for the REPL.
type Reader struct {
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer

	prompt     string       // main prompt (line 0)
	contPrompt string       // continuation prompt (lines 1+)
	lines      []lineBuffer // lines in the current input block
	activeIdx  int          // which line the cursor is on (may be len(lines) = virtual new line)
	multiline  bool         // true once Ctrl+J / backslash continuation enters multiline mode

	browsingHistory bool // true while up/down is cycling through history
	history         *history

	// Terminal state (only set when stdin is a real terminal)
	isTTY           bool
	fd              int
	termWidth       int
	restoreTerminal func()
	displayedRows   int // rows below the first row occupied by the editing block
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
// If Stdin is nil (real REPL mode), the terminal is placed in raw mode so
// that keystrokes can be read individually. If raw mode fails (e.g., stdin
// is not a terminal), an error is returned and the caller should fall back
// to basic input.
//
// If Stdin is provided (testing), raw mode is skipped and display output
// is suppressed.
func New(cfg Config) (*Reader, error) {
	historyFile := cfg.HistoryFile
	if historyFile == "" {
		if homeDir, err := os.UserHomeDir(); err == nil {
			historyFile = filepath.Join(homeDir, ".clyde", "history")
		}
	}

	r := &Reader{
		prompt:     cfg.Prompt,
		contPrompt: ContinuationPrompt,
		stdout:     cfg.Stdout,
		stderr:     cfg.Stderr,
	}
	if r.stdout == nil {
		r.stdout = os.Stdout
	}
	if r.stderr == nil {
		r.stderr = os.Stderr
	}

	if cfg.Stdin != nil {
		// Testing / non-interactive mode
		r.stdin = cfg.Stdin
	} else {
		// Real REPL mode — enter raw terminal mode
		r.stdin = os.Stdin
		fd := int(os.Stdin.Fd())
		restore, width, err := setupRawMode(fd)
		if err != nil {
			return nil, fmt.Errorf("input: %w", err)
		}
		r.isTTY = true
		r.fd = fd
		r.termWidth = width
		r.restoreTerminal = restore
	}

	r.history = newHistory(historyFile, 1000)
	r.history.Load()

	return r, nil
}

// ReadLine reads a line (or multiline block) of input from the user.
//
// Returns:
//   - The assembled input string
//   - An error: nil on success, io.EOF on Ctrl+D/EOF, ErrInterrupt on Ctrl+C
//
// Multiline mode is entered by any of three methods:
//  1. Backslash continuation: end a line with \ to continue on next line
//  2. Ctrl+J: inserts a newline (universal, works everywhere)
//  3. Alt+Enter: same as Ctrl+J (translated from ESC+CR at the byte level)
//
// All three methods can be mixed freely within the same input block.
// Plain Enter submits the accumulated multiline input (or single-line input).
// Ctrl+C during multiline mode discards the partial input.
// History saves the complete assembled block as a single entry.
func (r *Reader) ReadLine() (string, error) {
	r.multiline = false
	r.browsingHistory = false
	r.lines = []lineBuffer{{}}
	r.activeIdx = 0
	r.displayedRows = 0
	r.history.Reset()

	r.redraw()

	for {
		k, err := readKey(r.stdin)
		if err != nil {
			return "", err
		}

		// Reset history browsing on any key except Up/Down
		if k.special != keyUp && k.special != keyDown {
			r.browsingHistory = false
		}

		switch k.special {
		case keyEnter:
			lineStr := r.activeLine().String()
			// Backslash continuation: strip trailing \ and continue
			if strings.HasSuffix(lineStr, "\\") {
				r.activeLine().set(lineStr[:len(lineStr)-1])
				r.addNewLine()
				r.redraw()
				continue
			}
			// Submit
			result := r.assembleResult()
			r.finishDisplay()
			if strings.TrimSpace(result) != "" {
				r.history.Add(result)
			}
			return result, nil

		case keyCtrlJ:
			r.addNewLine()
			r.redraw()
			continue

		case keyCtrlC:
			r.multiline = false
			r.finishDisplay()
			return "", ErrInterrupt

		case keyCtrlD:
			if r.allEmpty() {
				r.finishDisplay()
				return "", io.EOF
			}
			r.activeLine().delete()

		case keyCtrlU:
			if r.activeIdx < len(r.lines) {
				r.lines[r.activeIdx].clear()
			}

		case keyCtrlL:
			r.clearScreen()
			continue

		case keyUp:
			r.handleUp()

		case keyDown:
			r.handleDown()

		case keyLeft:
			if r.activeIdx < len(r.lines) {
				r.lines[r.activeIdx].moveLeft()
			}

		case keyRight:
			if r.activeIdx < len(r.lines) {
				r.lines[r.activeIdx].moveRight()
			}

		case keyHome:
			if r.activeIdx < len(r.lines) {
				r.lines[r.activeIdx].moveHome()
			}

		case keyEnd:
			if r.activeIdx < len(r.lines) {
				r.lines[r.activeIdx].moveEnd()
			}

		case keyBackspace:
			if r.activeIdx < len(r.lines) {
				r.lines[r.activeIdx].backspace()
			}

		case keyDelete:
			if r.activeIdx < len(r.lines) {
				r.lines[r.activeIdx].delete()
			}

		default:
			if k.r != 0 {
				r.activeLine().insert(k.r)
			} else {
				continue // unknown key — skip redraw
			}
		}

		r.redraw()
	}
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

// activeLine returns a pointer to the current line buffer, materializing
// a new empty line if activeIdx is at the virtual "new line" position
// (one past the end of r.lines).
func (r *Reader) activeLine() *lineBuffer {
	for r.activeIdx >= len(r.lines) {
		r.lines = append(r.lines, lineBuffer{})
	}
	return &r.lines[r.activeIdx]
}

// activeLineLen returns the rune count of the active line (0 if virtual).
func (r *Reader) activeLineLen() int {
	if r.activeIdx >= len(r.lines) {
		return 0
	}
	return r.lines[r.activeIdx].Len()
}

// addNewLine materializes the current position (if virtual), then moves
// the cursor to a new virtual line at the end of the block.
func (r *Reader) addNewLine() {
	r.activeLine() // materialize current position
	r.activeIdx = len(r.lines)
	r.multiline = true
}

// assembleResult joins all materialized lines into a single string.
func (r *Reader) assembleResult() string {
	parts := make([]string, len(r.lines))
	for i := range r.lines {
		parts[i] = r.lines[i].String()
	}
	return strings.Join(parts, "\n")
}

// allEmpty returns true if every line (including any virtual new line) is empty.
func (r *Reader) allEmpty() bool {
	for i := range r.lines {
		if r.lines[i].Len() > 0 {
			return false
		}
	}
	return true
}

// handleUp handles the Up arrow key: multiline navigation or history browsing.
func (r *Reader) handleUp() {
	if r.multiline {
		if r.activeIdx > 0 {
			r.activeIdx--
		}
		return
	}
	// Non-multiline: history browsing
	if r.browsingHistory || r.activeLineLen() == 0 {
		entry, ok := r.history.Prev()
		if ok {
			r.browsingHistory = true
			r.activeLine().set(entry)
		}
	}
}

// handleDown handles the Down arrow key: multiline navigation or history browsing.
func (r *Reader) handleDown() {
	if r.multiline {
		// Allow navigating to the virtual new-line position (len(r.lines))
		if r.activeIdx < len(r.lines) {
			r.activeIdx++
		}
		return
	}
	// Non-multiline: history browsing
	if r.browsingHistory {
		entry, ok := r.history.Next()
		if ok {
			if entry == "" {
				r.browsingHistory = false
			}
			r.activeLine().set(entry)
		}
	}
}

// ---------------------------------------------------------------------------
// Public accessors
// ---------------------------------------------------------------------------

// SetPrompt updates the prompt string displayed to the user.
func (r *Reader) SetPrompt(prompt string) {
	r.prompt = prompt
}

// Close restores the terminal to its original state. Must be called on exit.
func (r *Reader) Close() error {
	if r.restoreTerminal != nil {
		r.restoreTerminal()
	}
	return nil
}

// Stdout returns a writer that can be used for output while editing is active.
func (r *Reader) Stdout() io.Writer { return r.stdout }

// Stderr returns a writer that can be used for error output while editing is active.
func (r *Reader) Stderr() io.Writer { return r.stderr }

// IsMultiline returns true if the reader is currently in multiline
// accumulation mode (for testing).
func (r *Reader) IsMultiline() bool { return r.multiline }

// AccumulatedLines returns the lines accumulated so far in multiline mode
// (for testing). Returns nil if not in multiline mode.
func (r *Reader) AccumulatedLines() []string {
	if !r.multiline {
		return nil
	}
	result := make([]string, len(r.lines))
	for i := range r.lines {
		result[i] = r.lines[i].String()
	}
	return result
}
