// Package input provides a rich text input widget for Clyde's REPL mode.
//
// It wraps chzyer/readline to provide:
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
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

	"github.com/chzyer/readline"
)

// Reader provides rich line-editing for the REPL.
type Reader struct {
	rl          *readline.Instance
	multiline   bool     // true if we're in multiline accumulation mode
	lines       []string // accumulated lines in multiline mode
	lineIdx     int      // current editing position within lines (len(lines) = new line)
	historyPath string   // path to the history file

	// ctrlJPressed is set atomically by FuncFilterInputRune (runs in
	// readline's ioloop goroutine) and read by ReadLine (main goroutine).
	// When true, the next line accepted by readline should be accumulated
	// as a multiline continuation instead of returned.
	ctrlJPressed atomic.Bool

	// browsingHistory tracks whether the user is currently cycling through
	// history entries with up/down arrows. Once history browsing starts
	// (from an empty, non-multiline prompt), further up/down presses
	// continue navigating history until any other key is pressed.
	browsingHistory atomic.Bool

	// inMultiline mirrors the multiline field for safe cross-goroutine
	// access from FuncFilterInputRune (runs in readline's ioloop goroutine).
	inMultiline atomic.Bool

	// currentBufLen tracks the current readline buffer length, updated by
	// the Listener callback after each keystroke in the ioloop goroutine.
	// Used by FuncFilterInputRune to decide whether to allow history
	// navigation (only when the buffer is empty).
	currentBufLen atomic.Int32

	// lineIdxAtomic and numLinesAtomic mirror lineIdx and len(lines) for
	// safe access from FuncFilterInputRune in the ioloop goroutine.
	// Used for boundary checks when navigating between multiline lines.
	lineIdxAtomic  atomic.Int32
	numLinesAtomic atomic.Int32

	// navigateDir is set by FuncFilterInputRune and consumed by the
	// Listener callback, both running in readline's ioloop goroutine.
	// -1 = navigate up, +1 = navigate down, 0 = no navigation pending.
	// The Listener replaces the readline buffer with the target line's
	// content when this is non-zero.
	navigateDir atomic.Int32
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
//   - Ctrl+J to insert a newline (multiline input) — works on all terminals
//   - Alt+Enter to insert a newline (multiline input) — requires Meta key
//   - Backslash at end of line to continue on next line
//   - Up/down arrow keys to recall previous inputs (empty prompt only)
//   - Up/down arrow keys to navigate lines (in multiline mode)
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

	reader := &Reader{
		historyPath: historyFile,
	}

	rlConfig := &readline.Config{
		Prompt:                 cfg.Prompt,
		HistoryFile:            historyFile,
		HistoryLimit:           1000,
		DisableAutoSaveHistory: true, // We save manually after multiline assembly
		InterruptPrompt:        "^C",
		EOFPrompt:              "exit",

		// Listener runs in readline's ioloop goroutine after each keystroke.
		// It has two responsibilities:
		//   1. Track the current buffer length (for history navigation gating)
		//   2. Handle multiline navigation: when navigateDir is set by
		//      FuncFilterInputRune, save the current line, update lineIdx,
		//      and replace the buffer with the target line's content.
		//
		// Navigation is handled here (in the ioloop) rather than in ReadLine
		// (main goroutine) to avoid a race condition: the terminal continues
		// reading stdin after processing arrow keys, so by the time the main
		// goroutine could call ReadlineWithDefault, subsequent characters may
		// have already been consumed. The Listener replaces the buffer in-place
		// within the same ioloop iteration, before any subsequent characters
		// are processed.
		Listener: readline.FuncListener(func(line []rune, pos int, key rune) ([]rune, int, bool) {
			reader.currentBufLen.Store(int32(len(line)))

			dir := reader.navigateDir.Load()
			if dir != 0 {
				reader.navigateDir.Store(0)

				// Save current line at lineIdx
				curLine := string(line)
				if reader.lineIdx >= len(reader.lines) {
					// At "new line" position — only save if non-empty
					// to avoid phantom empty lines
					if curLine != "" {
						reader.lines = append(reader.lines, curLine)
					}
				} else {
					reader.lines[reader.lineIdx] = curLine
				}

				// Navigate
				reader.lineIdx += int(dir)

				// Load target line content
				var newBuf []rune
				if reader.lineIdx < len(reader.lines) {
					newBuf = []rune(reader.lines[reader.lineIdx])
				}
				// else: new-line position, empty buffer

				// Update atomics for FuncFilterInputRune boundary checks
				reader.lineIdxAtomic.Store(int32(reader.lineIdx))
				reader.numLinesAtomic.Store(int32(len(reader.lines)))

				return newBuf, len(newBuf), true // replace buffer
			}

			return line, pos, false // observe only
		}),

		// FuncFilterInputRune intercepts special keys before readline
		// processes them. Runs in readline's ioloop goroutine.
		//
		// 1. Ctrl+J (0x0A / LF) — translate to Enter but flag for multiline
		//    accumulation. Alt+Enter (ESC+CR) arrives as Ctrl+J via the
		//    metaCRReader translation layer.
		//
		// 2. Up/Down arrows (CharPrev/CharNext) — in multiline mode, set
		//    navigateDir and translate to CharBell (a no-op that triggers the
		//    Listener to perform the buffer swap). In non-multiline mode,
		//    allow history browsing only from an empty prompt.
		FuncFilterInputRune: func(r rune) (rune, bool) {
			if r == readline.CharCtrlJ { // 0x0A / LF
				reader.ctrlJPressed.Store(true)
				reader.browsingHistory.Store(false)
				return readline.CharEnter, true // Accept line; ReadLine will accumulate
			}

			// Up/Down arrow handling
			if r == readline.CharPrev || r == readline.CharNext {
				// --- Multiline mode: navigate between lines ---
				if reader.inMultiline.Load() {
					if r == readline.CharPrev {
						if reader.lineIdxAtomic.Load() > 0 {
							reader.navigateDir.Store(-1)
							return readline.CharBell, true // trigger Listener
						}
						return r, false // already at first line
					}
					// CharNext (down)
					if reader.lineIdxAtomic.Load() < reader.numLinesAtomic.Load() {
						reader.navigateDir.Store(1)
						return readline.CharBell, true // trigger Listener
					}
					return r, false // already at/past last line
				}

				// --- Non-multiline: history browsing ---
				if reader.browsingHistory.Load() {
					return r, true
				}
				if reader.currentBufLen.Load() == 0 {
					reader.browsingHistory.Store(true)
					return r, true
				}
				return r, false // buffer has content, suppress
			}

			// Any other key exits history browsing mode
			reader.browsingHistory.Store(false)
			return r, true
		},
	}

	// Wrap stdin in metaCRReader to translate Alt+Enter (ESC CR / 0x1B 0x0D)
	// into Ctrl+J (LF / 0x0A) before readline processes escape sequences.
	// Without this, readline's terminal layer consumes the ESC and passes
	// through plain CR, making Alt+Enter indistinguishable from Enter.
	if cfg.Stdin != nil {
		// Testing mode: wrap the provided mock stdin
		rlConfig.Stdin = &metaCRReader{rc: cfg.Stdin}
	} else {
		// REPL mode: wrap os.Stdin through CancelableStdin for proper shutdown
		cancelable := readline.NewCancelableStdin(os.Stdin)
		rlConfig.Stdin = &metaCRReader{rc: cancelable}
	}

	// Apply output overrides for testing
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

	reader.rl = rl
	return reader, nil
}

// ContinuationPrompt is shown for subsequent lines in multiline mode.
// It is indented to align with the content after "You: " in the main prompt.
const ContinuationPrompt = "  > "

// ReadLine reads a line (or multiline block) of input from the user.
//
// Returns:
//   - The assembled input string (trimmed)
//   - An error: nil on success, io.EOF on Ctrl+D, ErrInterrupt on Ctrl+C
//
// Multiline mode is entered by any of three methods:
//  1. Backslash continuation: end a line with \ to continue on next line
//  2. Ctrl+J: inserts a newline (the current line is accumulated, not submitted)
//  3. Alt+Enter: same as Ctrl+J (translated at the byte level by metaCRReader)
//
// All three methods can be mixed freely within the same input block.
// Plain Enter submits the accumulated multiline input (or single-line input).
// Ctrl+C during multiline mode discards the partial input.
// History saves the complete assembled block as a single entry.
//
// In multiline mode, up/down arrow keys navigate between accumulated lines
// (handled by the Listener in the ioloop goroutine — the buffer is swapped
// in-place without returning to ReadLine). The user can edit any previously
// entered line; pressing Enter from any position submits the entire block.
func (r *Reader) ReadLine() (string, error) {
	r.multiline = false
	r.inMultiline.Store(false)
	r.browsingHistory.Store(false)
	r.navigateDir.Store(0)
	r.lines = nil
	r.lineIdx = 0
	r.syncAtomics()

	for {
		line, err := r.rl.Readline()
		if err != nil {
			// If we were accumulating multiline and got interrupted,
			// discard the partial input
			if r.multiline {
				r.multiline = false
				r.inMultiline.Store(false)
				r.lines = nil
				r.lineIdx = 0
				r.syncAtomics()
				r.rl.SetPrompt(r.rl.Config.Prompt)
			}
			// Clear any stale flags from the interrupted line
			r.ctrlJPressed.Store(false)
			r.navigateDir.Store(0)
			return "", err
		}

		// --- Ctrl+J / Alt+Enter: insert newline, continue multiline ---
		if r.ctrlJPressed.Load() {
			r.ctrlJPressed.Store(false)
			r.saveLineAlways(line)
			// Move to a new line at the end of the block
			r.lineIdx = len(r.lines)
			r.multiline = true
			r.inMultiline.Store(true)
			r.syncAtomics()
			r.rl.SetPrompt(ContinuationPrompt)
			continue
		}

		// --- Backslash continuation ---
		if strings.HasSuffix(line, "\\") {
			line = line[:len(line)-1] // strip the backslash
			r.saveLineAlways(line)
			// Move to a new line at the end of the block
			r.lineIdx = len(r.lines)
			r.multiline = true
			r.inMultiline.Store(true)
			r.syncAtomics()
			r.rl.SetPrompt(ContinuationPrompt)
			continue
		}

		// --- Enter: submit ---
		if r.multiline {
			// Final submission of the multiline block.
			// Save the current line at whatever position we're editing
			// (the Listener may have changed lineIdx via navigation).
			r.saveLineAlways(line)
			result := strings.Join(r.lines, "\n")
			r.multiline = false
			r.inMultiline.Store(false)
			r.lines = nil
			r.lineIdx = 0
			r.syncAtomics()
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

// saveLineAlways saves line content at lineIdx, always appending when at end.
// Used for Ctrl+J, backslash continuation, and Enter submission where the
// line is intentional content (even if empty).
func (r *Reader) saveLineAlways(line string) {
	if r.lineIdx >= len(r.lines) {
		r.lines = append(r.lines, line)
	} else {
		r.lines[r.lineIdx] = line
	}
}

// syncAtomics updates the atomic mirrors of lineIdx and len(lines) so that
// FuncFilterInputRune (running in the ioloop goroutine) can perform
// boundary checks for multiline navigation.
func (r *Reader) syncAtomics() {
	r.lineIdxAtomic.Store(int32(r.lineIdx))
	r.numLinesAtomic.Store(int32(len(r.lines)))
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

// ---------------------------------------------------------------------------
// metaCRReader — translates Alt+Enter (ESC CR) to Ctrl+J (LF)
// ---------------------------------------------------------------------------

// metaCRReader wraps an io.ReadCloser and translates the byte sequence
// ESC CR (0x1B 0x0D) — sent by terminals when Alt+Enter is pressed —
// into a single LF byte (0x0A). This makes Alt+Enter arrive at readline's
// FuncFilterInputRune as CharCtrlJ, where it receives the same multiline
// treatment as a direct Ctrl+J keypress.
//
// All other byte sequences pass through unmodified, including:
//   - ESC followed by '[' or 'O' (ANSI escape sequences → arrow keys, etc.)
//   - ESC followed by letter keys (Meta+b, Meta+f, etc.)
//   - Standalone CR (plain Enter → 0x0D)
//   - Standalone LF (plain Ctrl+J → 0x0A)
//
// Thread safety: Read is called by readline's internal goroutine (terminal
// ioloop), so it must not race with other methods. The pending byte state
// is contained within Read's sequential call chain — no concurrent access.
// MetaCRReader wraps an io.ReadCloser and translates the byte sequence
// ESC CR (0x1B 0x0D) — sent by terminals when Alt+Enter is pressed —
// into a single LF byte (0x0A).
type MetaCRReader = metaCRReader

type metaCRReader struct {
	rc      io.ReadCloser
	buf     [1]byte
	pending byte // buffered byte from a non-Alt+Enter ESC sequence
	hasPend bool // true if pending contains a valid byte
}

// NewMetaCRReader creates a new MetaCRReader wrapping the given ReadCloser.
// Exported for testing.
func NewMetaCRReader(rc io.ReadCloser) *MetaCRReader {
	return &metaCRReader{rc: rc}
}

// Read implements io.Reader. It reads from the underlying reader one byte
// at a time, translating ESC+CR sequences to LF.
func (m *metaCRReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}

	// Return pending byte from a previous ESC+<non-CR> sequence
	if m.hasPend {
		p[0] = m.pending
		m.hasPend = false
		return 1, nil
	}

	// Read one byte from underlying reader
	n, err := m.rc.Read(m.buf[:])
	if n == 0 {
		return 0, err
	}

	b := m.buf[0]

	if b != 0x1B {
		// Not ESC — pass through unchanged
		p[0] = b
		return 1, nil
	}

	// Got ESC (0x1B) — peek at the next byte to check for Alt+Enter
	n, err = m.rc.Read(m.buf[:])
	if n == 0 {
		// ESC at EOF — pass ESC through
		p[0] = 0x1B
		return 1, err
	}

	next := m.buf[0]

	if next == 0x0D {
		// ESC + CR = Alt+Enter → translate to LF (Ctrl+J)
		p[0] = 0x0A
		return 1, nil
	}

	// ESC + something else — pass ESC through now, buffer the next byte
	// for the following Read call. This preserves escape sequences like
	// ESC [ (CSI), ESC O (SS3), ESC b (Meta+backward), etc.
	p[0] = 0x1B
	m.pending = next
	m.hasPend = true
	return 1, nil
}

// Close delegates to the underlying ReadCloser.
func (m *metaCRReader) Close() error {
	return m.rc.Close()
}
