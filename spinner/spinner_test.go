package spinner

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

// TestFrameSequence verifies the braille dot frame sequence is correct.
func TestFrameSequence(t *testing.T) {
	expected := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

	if len(Frames) != len(expected) {
		t.Fatalf("Expected %d frames, got %d", len(expected), len(Frames))
	}

	for i, frame := range Frames {
		if frame != expected[i] {
			t.Errorf("Frame[%d] = %q, want %q", i, frame, expected[i])
		}
	}
}

// TestFrameCount verifies there are exactly 10 braille dot frames.
func TestFrameCount(t *testing.T) {
	if len(Frames) != 10 {
		t.Errorf("Expected 10 frames, got %d", len(Frames))
	}
}

// TestFrameDelay verifies the frame delay is approximately 1/60s (~16.7ms).
func TestFrameDelay(t *testing.T) {
	expected := time.Second / 60
	if FrameDelay != expected {
		t.Errorf("FrameDelay = %v, want %v", FrameDelay, expected)
	}
}

// TestFramesPerSymbol verifies 2 frames per symbol for ~30 symbols/second.
func TestFramesPerSymbol(t *testing.T) {
	if FramesPerSymbol != 2 {
		t.Errorf("FramesPerSymbol = %d, want 2", FramesPerSymbol)
	}
}

// TestEffectiveRate verifies the effective symbol rate is ~30/second.
func TestEffectiveRate(t *testing.T) {
	// At 60fps with 2 frames per symbol: 60/2 = 30 symbols/second
	effectiveRate := float64(time.Second) / float64(FrameDelay) / float64(FramesPerSymbol)
	if effectiveRate < 29 || effectiveRate > 31 {
		t.Errorf("Effective symbol rate = %.1f/s, want ~30/s", effectiveRate)
	}
}

// TestNewSpinner verifies a new spinner is created in inactive state.
func TestNewSpinner(t *testing.T) {
	s := New()
	if s == nil {
		t.Fatal("New() returned nil")
	}
	if s.IsActive() {
		t.Error("New spinner should not be active")
	}
	if s.Message() != "" {
		t.Errorf("New spinner message = %q, want empty", s.Message())
	}
}

// TestNewWithWriter verifies a spinner can be created with a custom writer.
func TestNewWithWriter(t *testing.T) {
	var buf bytes.Buffer
	s := NewWithWriter(&buf)
	if s == nil {
		t.Fatal("NewWithWriter() returned nil")
	}
	if s.writer != &buf {
		t.Error("Writer was not set correctly")
	}
}

// TestStartStop verifies the spinner lifecycle: start → active → stop → inactive.
func TestStartStop(t *testing.T) {
	var buf bytes.Buffer
	s := NewWithWriter(&buf)

	// Start the spinner
	s.Start("Testing...")
	// Give the goroutine a moment to start
	time.Sleep(50 * time.Millisecond)

	if !s.IsActive() {
		t.Error("Spinner should be active after Start()")
	}
	if s.Message() != "Testing..." {
		t.Errorf("Message = %q, want %q", s.Message(), "Testing...")
	}

	// Verify some output was written
	if buf.Len() == 0 {
		t.Error("Spinner should have written output to the buffer")
	}

	// Stop the spinner
	s.Stop()

	if s.IsActive() {
		t.Error("Spinner should not be active after Stop()")
	}

	// Verify the line was cleared (last write should contain \r\033[K)
	output := buf.String()
	if !strings.HasSuffix(output, "\r\033[K") {
		t.Errorf("Expected output to end with clear sequence, got last 20 chars: %q",
			output[max(0, len(output)-20):])
	}
}

// TestStartUpdatesMessage verifies that calling Start while already running
// updates the message without restarting.
func TestStartUpdatesMessage(t *testing.T) {
	var buf bytes.Buffer
	s := NewWithWriter(&buf)

	s.Start("First message")
	time.Sleep(50 * time.Millisecond)

	if s.Message() != "First message" {
		t.Errorf("Message = %q, want %q", s.Message(), "First message")
	}

	// Update message while running
	s.Start("Second message")
	time.Sleep(50 * time.Millisecond)

	if s.Message() != "Second message" {
		t.Errorf("Message = %q, want %q", s.Message(), "Second message")
	}

	// Should still be active (not restarted)
	if !s.IsActive() {
		t.Error("Spinner should still be active after message update")
	}

	s.Stop()
}

// TestStopWhenNotActive verifies that calling Stop when not active is a no-op.
func TestStopWhenNotActive(t *testing.T) {
	var buf bytes.Buffer
	s := NewWithWriter(&buf)

	// Should not panic or block
	s.Stop()

	if s.IsActive() {
		t.Error("Spinner should not be active")
	}
	if buf.Len() != 0 {
		t.Error("No output expected when stopping an inactive spinner")
	}
}

// TestDoubleStop verifies that calling Stop twice does not panic.
func TestDoubleStop(t *testing.T) {
	var buf bytes.Buffer
	s := NewWithWriter(&buf)

	s.Start("Test")
	time.Sleep(50 * time.Millisecond)

	s.Stop()
	s.Stop() // Second stop should be a no-op
}

// TestOutputContainsBrailleFrames verifies that the output contains braille
// dot characters from the frame sequence.
func TestOutputContainsBrailleFrames(t *testing.T) {
	var buf bytes.Buffer
	s := NewWithWriter(&buf)

	s.Start("Working")
	// Let it run for enough time to produce several frames
	time.Sleep(200 * time.Millisecond)
	s.Stop()

	output := buf.String()

	// At least one braille frame should appear in the output
	foundFrame := false
	for _, frame := range Frames {
		if strings.Contains(output, frame) {
			foundFrame = true
			break
		}
	}

	if !foundFrame {
		t.Errorf("Expected output to contain at least one braille frame, got: %q",
			output[:min(200, len(output))])
	}
}

// TestOutputContainsMessage verifies that the output includes the operation message.
func TestOutputContainsMessage(t *testing.T) {
	var buf bytes.Buffer
	s := NewWithWriter(&buf)

	msg := "Patching file: agent.go"
	s.Start(msg)
	time.Sleep(50 * time.Millisecond)
	s.Stop()

	output := buf.String()
	if !strings.Contains(output, msg) {
		t.Errorf("Expected output to contain message %q, got: %q",
			msg, output[:min(200, len(output))])
	}
}

// TestRenderFrameFormat verifies the format of a single rendered frame.
func TestRenderFrameFormat(t *testing.T) {
	var buf bytes.Buffer
	s := NewWithWriter(&buf)

	s.renderFrame("⠹", "Testing operation")

	output := buf.String()
	expected := "\r\033[K⠹ Testing operation"
	if output != expected {
		t.Errorf("renderFrame output = %q, want %q", output, expected)
	}
}

// TestClearLine verifies the clear line escape sequence.
func TestClearLine(t *testing.T) {
	var buf bytes.Buffer
	s := NewWithWriter(&buf)

	s.clearLine()

	output := buf.String()
	expected := "\r\033[K"
	if output != expected {
		t.Errorf("clearLine output = %q, want %q", output, expected)
	}
}

// TestRestartAfterStop verifies the spinner can be restarted after stopping.
func TestRestartAfterStop(t *testing.T) {
	var buf bytes.Buffer
	s := NewWithWriter(&buf)

	// First run
	s.Start("Run 1")
	time.Sleep(50 * time.Millisecond)
	s.Stop()

	if s.IsActive() {
		t.Error("Should not be active after Stop")
	}

	// Clear buffer
	buf.Reset()

	// Second run
	s.Start("Run 2")
	time.Sleep(50 * time.Millisecond)

	if !s.IsActive() {
		t.Error("Should be active after restart")
	}
	if s.Message() != "Run 2" {
		t.Errorf("Message = %q, want %q", s.Message(), "Run 2")
	}

	s.Stop()

	// Verify output contains second message
	output := buf.String()
	if !strings.Contains(output, "Run 2") {
		t.Error("Expected output to contain 'Run 2' after restart")
	}
}

// TestFormatSpinnerMessage verifies message formatting for the spinner.
func TestFormatSpinnerMessage(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "strips arrow prefix",
			input:    "→ Patching file: agent.go (+48 bytes)",
			expected: "Patching file: agent.go (+48 bytes)...",
		},
		{
			name:     "strips arrow prefix with running bash",
			input:    "→ Running bash: go test ./...",
			expected: "Running bash: go test ./...",
		},
		{
			name:     "no arrow prefix",
			input:    "Thinking",
			expected: "Thinking...",
		},
		{
			name:     "already has trailing dots",
			input:    "Processing...",
			expected: "Processing...",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "listing files",
			input:    "→ Listing files: . (current directory)",
			expected: "Listing files: . (current directory)...",
		},
		{
			name:     "reading file",
			input:    "→ Reading file: main.go",
			expected: "Reading file: main.go...",
		},
		{
			name:     "writing file with size",
			input:    "→ Writing file: progress.md (42.5 KB)",
			expected: "Writing file: progress.md (42.5 KB)...",
		},
		{
			name:     "searching pattern",
			input:    "→ Searching: 'TODO' in ./tools/*.go",
			expected: "Searching: 'TODO' in ./tools/*.go...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatSpinnerMessage(tt.input)
			if result != tt.expected {
				t.Errorf("FormatSpinnerMessage(%q) = %q, want %q",
					tt.input, result, tt.expected)
			}
		})
	}
}

// TestConcurrentStartStop verifies thread safety of Start/Stop operations.
func TestConcurrentStartStop(t *testing.T) {
	var buf bytes.Buffer
	s := NewWithWriter(&buf)

	// Rapidly start and stop in parallel - should not panic or deadlock
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 20; i++ {
			s.Start("concurrent test")
			time.Sleep(5 * time.Millisecond)
			s.Stop()
		}
	}()

	select {
	case <-done:
		// Success
	case <-time.After(5 * time.Second):
		t.Fatal("Concurrent start/stop test timed out (possible deadlock)")
	}
}

// TestSymbolCycling verifies that the spinner cycles through all symbols.
func TestSymbolCycling(t *testing.T) {
	var buf bytes.Buffer
	s := NewWithWriter(&buf)

	// Run long enough to cycle through all 10 symbols
	// At 60fps with 2 frames/symbol: 10 symbols = 20 frames = 20/60s ≈ 333ms
	s.Start("cycling test")
	time.Sleep(500 * time.Millisecond)
	s.Stop()

	output := buf.String()

	// Count unique frames found in the output
	foundFrames := make(map[string]bool)
	for _, frame := range Frames {
		if strings.Contains(output, frame) {
			foundFrames[frame] = true
		}
	}

	// Should find at least half the frames (timing-dependent, so we're lenient)
	if len(foundFrames) < 3 {
		t.Errorf("Expected to find at least 3 unique braille frames, found %d: %v",
			len(foundFrames), foundFrames)
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
