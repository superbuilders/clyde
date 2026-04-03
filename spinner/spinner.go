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
	s.clearLine()
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
			s.renderFrame(Frames[symbolIdx], msg)
			frameCount++
		}
	}
}

// renderFrame writes a single spinner frame to the writer.
func (s *Spinner) renderFrame(symbol, message string) {
	// \r returns cursor to start of line
	// \033[K clears from cursor to end of line (prevents leftover chars)
	fmt.Fprintf(s.writer, "\r\033[K%s %s", symbol, message)
}

// clearLine clears the spinner line.
func (s *Spinner) clearLine() {
	fmt.Fprintf(s.writer, "\r\033[K")
}

// FormatSpinnerMessage formats a tool progress message for the spinner.
// It strips the "→ " prefix (used in permanent logs) and adds "..." suffix.
//
// Example:
//
//	"→ Patching file: agent.go (+48 bytes)" → "Patching file: agent.go (+48 bytes)..."
//	"→ Running bash: go test ./..." → "Running bash: go test ./..."
func FormatSpinnerMessage(progressMsg string) string {
	// Strip the "→ " prefix if present
	msg := progressMsg
	if len(msg) >= 4 && msg[:4] == "→ " {
		msg = msg[4:]
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
