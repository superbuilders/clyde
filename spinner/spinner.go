// Package spinner provides a smooth animated braille-dot spinner for Clyde's
// REPL mode. The spinner occupies the second-to-last terminal line and is
// redrawn in place while an operation is in progress.
//
// The spinner is an ephemeral preview — any text shown on the spinner line
// also appears in the permanent scrollback log once the operation completes.
//
// Animation uses the braille dot Unicode set:
//
//	⠋ ⠙ ⠹ ⠸ ⠼ ⠴ ⠦ ⠧ ⠇ ⠏
//
// Parameters: 1/60s frame delay, 2 frames per symbol (~30 symbols/second).
package spinner

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

// Frames is the braille dot animation sequence.
var Frames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// FrameDelay is the time between animation frames (~16.7ms = 1/60s).
const FrameDelay = time.Second / 60

// FramesPerSymbol is the number of animation frames each symbol is held.
const FramesPerSymbol = 2

// Spinner manages an animated spinner on the terminal.
type Spinner struct {
	mu      sync.Mutex
	message string // Current operation text (e.g., "Patching file: agent.go...")
	active  bool   // Whether the spinner is currently running
	stopCh  chan struct{}
	doneCh  chan struct{}
	writer  io.Writer // Output writer (defaults to os.Stderr)
	frame   int       // Current symbol index (for testing)
}

// New creates a new Spinner that writes to os.Stderr.
func New() *Spinner {
	return &Spinner{
		writer: os.Stderr,
	}
}

// NewWithWriter creates a new Spinner that writes to the given writer.
// This is primarily useful for testing.
func NewWithWriter(w io.Writer) *Spinner {
	return &Spinner{
		writer: w,
	}
}

// Start begins the spinner animation with the given operation message.
// If the spinner is already running, it updates the message.
// The message should describe the current operation (e.g., "Patching file: agent.go...").
func (s *Spinner) Start(message string) {
	s.mu.Lock()
	if s.active {
		// Already running — just update the message
		s.message = message
		s.mu.Unlock()
		return
	}

	s.message = message
	s.active = true
	s.frame = 0
	s.stopCh = make(chan struct{})
	s.doneCh = make(chan struct{})
	s.mu.Unlock()

	go s.run()
}

// Stop stops the spinner animation and clears the spinner line.
func (s *Spinner) Stop() {
	s.mu.Lock()
	if !s.active {
		s.mu.Unlock()
		return
	}
	s.active = false
	close(s.stopCh)
	doneCh := s.doneCh
	s.mu.Unlock()

	// Wait for the animation goroutine to finish
	<-doneCh

	// Clear the spinner line
	s.ClearLine()
}

// IsActive returns whether the spinner is currently running.
func (s *Spinner) IsActive() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.active
}

// Message returns the current operation message.
func (s *Spinner) Message() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.message
}

// Frame returns the current symbol index (for testing).
func (s *Spinner) Frame() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.frame
}

// run is the animation loop goroutine.
func (s *Spinner) run() {
	defer close(s.doneCh)

	ticker := time.NewTicker(FrameDelay)
	defer ticker.Stop()

	frameCount := 0

	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.mu.Lock()
			symbolIdx := (frameCount / FramesPerSymbol) % len(Frames)
			msg := s.message
			s.frame = symbolIdx
			s.mu.Unlock()

			// Render: \r clears the line, then write spinner frame + message
			s.RenderFrame(Frames[symbolIdx], msg)
			frameCount++
		}
	}
}

// RenderFrame writes a single spinner frame to the writer.
func (s *Spinner) RenderFrame(symbol, message string) {
	// \r returns cursor to start of line
	// \033[K clears from cursor to end of line (prevents leftover chars)
	fmt.Fprintf(s.writer, "\r\033[K%s %s", symbol, message)
}

// ClearLine clears the spinner line.
func (s *Spinner) ClearLine() {
	fmt.Fprintf(s.writer, "\r\033[K")
}

// GetWriter returns the spinner's output writer. Exported for testing.
func (s *Spinner) GetWriter() io.Writer {
	return s.writer
}

// FormatSpinnerMessage extracts a short verb-only label from a tool progress
// message for the spinner. The spinner is an ephemeral preview — full details
// (URLs, file paths, commands, byte counts) appear in the permanent → scrollback
// line that prints when the operation completes.
//
// Verb-only messages are always short enough to fit on a single terminal line,
// which prevents the frame-bleed bug where \r\033[K fails to clear wrapped or
// multi-line content.
//
// Example:
//
//	"→ Browsing: https://example.com/very/long/path?q=foo" → "Reading Webpage..."
//	"→ Browser: navigate https://example.com"             → "Browsing..."
//	"→ Running bash: cd /tmp && find . -name '*.go'"       → "Running..."
//	"→ Patching file: agent.go (+48 bytes)"                → "Patching..."
//	"Thinking"                                             → "Thinking..."
func FormatSpinnerMessage(progressMsg string) string {
	// Strip the "→ " prefix if present
	msg := progressMsg
	if len(msg) >= 4 && msg[:4] == "→ " {
		msg = msg[4:]
	}

	// Match known verb prefixes → return short form
	for prefix, verb := range spinnerVerbs {
		if strings.HasPrefix(msg, prefix) {
			return verb
		}
	}

	// Fallback: take text up to first ":" or newline, add "..."
	if idx := strings.IndexAny(msg, ":\n"); idx > 0 {
		return msg[:idx] + "..."
	}

	// Add "..." if not already ending with dots
	if len(msg) >= 3 && msg[len(msg)-3:] == "..." {
		return msg
	}
	if len(msg) > 0 {
		msg += "..."
	}

	return msg
}

// spinnerVerbs maps the "→ Action" prefix of a progress message to a short
// verb for the spinner. The spinner is an ephemeral preview — full details
// appear in the permanent → scrollback line.
var spinnerVerbs = map[string]string{
	"Browsing":             "Reading Webpage...",
	"Browser":              "Browsing...",
	"Running bash":         "Running...",
	"Searching web":        "Searching...",
	"Searching":            "Searching...",
	"Reading file":         "Reading...",
	"Patching file":        "Patching...",
	"Writing file":         "Writing...",
	"Listing files":        "Listing...",
	"Finding files":        "Finding...",
	"Applying multi-patch": "Patching...",
	"Including file":       "Loading...",
}
