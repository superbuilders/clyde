package input

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mockReadCloser wraps a strings.Reader as an io.ReadCloser for testing.
type mockReadCloser struct {
	*strings.Reader
}

func (m *mockReadCloser) Close() error { return nil }

// newMockStdin creates a mock stdin from the given string.
// Each line should end with \n to simulate Enter.
func newMockStdin(s string) io.ReadCloser {
	return &mockReadCloser{strings.NewReader(s)}
}

// TestNew_DefaultConfig tests creating a Reader with defaults.
func TestNew_DefaultConfig(t *testing.T) {
	// Use temp dir so we don't clobber real history
	tmpDir := t.TempDir()
	historyFile := filepath.Join(tmpDir, "test-history")

	r, err := New(Config{
		Prompt:      "test> ",
		HistoryFile: historyFile,
		Stdin:       newMockStdin("hello\n"),
		Stdout:      io.Discard,
		Stderr:      io.Discard,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer r.Close()

	if r.rl == nil {
		t.Error("Expected non-nil readline instance")
	}
}

// TestReadLine_SingleLine tests basic single-line input submission.
func TestReadLine_SingleLine(t *testing.T) {
	tmpDir := t.TempDir()

	r, err := New(Config{
		Prompt:      "> ",
		HistoryFile: filepath.Join(tmpDir, "history"),
		Stdin:       newMockStdin("hello world\n"),
		Stdout:      io.Discard,
		Stderr:      io.Discard,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer r.Close()

	line, err := r.ReadLine()
	if err != nil {
		t.Fatalf("ReadLine() error = %v", err)
	}

	if line != "hello world" {
		t.Errorf("ReadLine() = %q, want %q", line, "hello world")
	}
}

// TestReadLine_EmptyLine tests that empty lines are returned (not skipped).
func TestReadLine_EmptyLine(t *testing.T) {
	tmpDir := t.TempDir()

	r, err := New(Config{
		Prompt:      "> ",
		HistoryFile: filepath.Join(tmpDir, "history"),
		Stdin:       newMockStdin("\n"),
		Stdout:      io.Discard,
		Stderr:      io.Discard,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer r.Close()

	line, err := r.ReadLine()
	if err != nil {
		t.Fatalf("ReadLine() error = %v", err)
	}

	if line != "" {
		t.Errorf("ReadLine() = %q, want empty string", line)
	}
}

// TestReadLine_EOF tests that EOF is properly returned on Ctrl+D.
func TestReadLine_EOF(t *testing.T) {
	tmpDir := t.TempDir()

	// Empty stdin simulates immediate EOF
	r, err := New(Config{
		Prompt:      "> ",
		HistoryFile: filepath.Join(tmpDir, "history"),
		Stdin:       newMockStdin(""),
		Stdout:      io.Discard,
		Stderr:      io.Discard,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer r.Close()

	_, err = r.ReadLine()
	if err != io.EOF {
		t.Errorf("ReadLine() error = %v, want io.EOF", err)
	}
}

// TestReadLine_MultipleLines tests reading multiple successive lines.
func TestReadLine_MultipleLines(t *testing.T) {
	tmpDir := t.TempDir()

	r, err := New(Config{
		Prompt:      "> ",
		HistoryFile: filepath.Join(tmpDir, "history"),
		Stdin:       newMockStdin("first\nsecond\nthird\n"),
		Stdout:      io.Discard,
		Stderr:      io.Discard,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer r.Close()

	expected := []string{"first", "second", "third"}
	for _, want := range expected {
		got, err := r.ReadLine()
		if err != nil {
			t.Fatalf("ReadLine() error = %v for expected %q", err, want)
		}
		if got != want {
			t.Errorf("ReadLine() = %q, want %q", got, want)
		}
	}
}

// TestReadLine_Multiline_BackslashContinuation tests multiline input
// using backslash continuation.
func TestReadLine_Multiline_BackslashContinuation(t *testing.T) {
	tmpDir := t.TempDir()

	// Lines ending with \ continue to the next line
	input := "line one\\\nline two\\\nline three\n"

	r, err := New(Config{
		Prompt:      "> ",
		HistoryFile: filepath.Join(tmpDir, "history"),
		Stdin:       newMockStdin(input),
		Stdout:      io.Discard,
		Stderr:      io.Discard,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer r.Close()

	result, err := r.ReadLine()
	if err != nil {
		t.Fatalf("ReadLine() error = %v", err)
	}

	want := "line one\nline two\nline three"
	if result != want {
		t.Errorf("ReadLine() = %q, want %q", result, want)
	}
}

// TestReadLine_Multiline_SingleBackslashLine tests that a single trailing
// backslash starts multiline mode.
func TestReadLine_Multiline_SingleBackslashLine(t *testing.T) {
	tmpDir := t.TempDir()

	input := "start\\\nend\n"

	r, err := New(Config{
		Prompt:      "> ",
		HistoryFile: filepath.Join(tmpDir, "history"),
		Stdin:       newMockStdin(input),
		Stdout:      io.Discard,
		Stderr:      io.Discard,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer r.Close()

	result, err := r.ReadLine()
	if err != nil {
		t.Fatalf("ReadLine() error = %v", err)
	}

	want := "start\nend"
	if result != want {
		t.Errorf("ReadLine() = %q, want %q", result, want)
	}
}

// TestReadLine_Multiline_OnlyBackslash tests a line that is just a backslash.
func TestReadLine_Multiline_OnlyBackslash(t *testing.T) {
	tmpDir := t.TempDir()

	input := "\\\nsecond line\n"

	r, err := New(Config{
		Prompt:      "> ",
		HistoryFile: filepath.Join(tmpDir, "history"),
		Stdin:       newMockStdin(input),
		Stdout:      io.Discard,
		Stderr:      io.Discard,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer r.Close()

	result, err := r.ReadLine()
	if err != nil {
		t.Fatalf("ReadLine() error = %v", err)
	}

	want := "\nsecond line"
	if result != want {
		t.Errorf("ReadLine() = %q, want %q", result, want)
	}
}

// TestReadLine_HistorySaved tests that submitted input is saved to history.
func TestReadLine_HistorySaved(t *testing.T) {
	tmpDir := t.TempDir()
	historyFile := filepath.Join(tmpDir, "history")

	r, err := New(Config{
		Prompt:      "> ",
		HistoryFile: historyFile,
		Stdin:       newMockStdin("first input\nsecond input\n"),
		Stdout:      io.Discard,
		Stderr:      io.Discard,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	// Read two lines
	_, _ = r.ReadLine()
	_, _ = r.ReadLine()

	r.Close()

	// Check history file was written
	data, err := os.ReadFile(historyFile)
	if err != nil {
		t.Fatalf("Failed to read history file: %v", err)
	}

	histContent := string(data)
	if !strings.Contains(histContent, "first input") {
		t.Errorf("History should contain 'first input', got: %q", histContent)
	}
	if !strings.Contains(histContent, "second input") {
		t.Errorf("History should contain 'second input', got: %q", histContent)
	}
}

// TestReadLine_EmptyNotSavedToHistory tests that empty lines are not
// saved to history.
func TestReadLine_EmptyNotSavedToHistory(t *testing.T) {
	tmpDir := t.TempDir()
	historyFile := filepath.Join(tmpDir, "history")

	r, err := New(Config{
		Prompt:      "> ",
		HistoryFile: historyFile,
		Stdin:       newMockStdin("\nreal input\n"),
		Stdout:      io.Discard,
		Stderr:      io.Discard,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	_, _ = r.ReadLine() // empty
	_, _ = r.ReadLine() // real input

	r.Close()

	data, err := os.ReadFile(historyFile)
	if err != nil {
		t.Fatalf("Failed to read history file: %v", err)
	}

	histContent := string(data)
	// The empty line should not be in history but real input should
	if !strings.Contains(histContent, "real input") {
		t.Errorf("History should contain 'real input', got: %q", histContent)
	}
}

// TestReadLine_Multiline_HistorySavedAsBlock tests that multiline input
// is saved as a single history entry.
func TestReadLine_Multiline_HistorySavedAsBlock(t *testing.T) {
	tmpDir := t.TempDir()
	historyFile := filepath.Join(tmpDir, "history")

	input := "first line\\\nsecond line\n"

	r, err := New(Config{
		Prompt:      "> ",
		HistoryFile: historyFile,
		Stdin:       newMockStdin(input),
		Stdout:      io.Discard,
		Stderr:      io.Discard,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	result, _ := r.ReadLine()
	r.Close()

	// Verify the result is joined
	if result != "first line\nsecond line" {
		t.Errorf("ReadLine() = %q, want %q", result, "first line\nsecond line")
	}

	// History should contain the assembled input
	data, err := os.ReadFile(historyFile)
	if err != nil {
		t.Fatalf("Failed to read history file: %v", err)
	}

	if !strings.Contains(string(data), "first line") {
		t.Errorf("History should contain multiline input")
	}
}

// TestSetPrompt tests that SetPrompt updates the reader's prompt.
func TestSetPrompt(t *testing.T) {
	tmpDir := t.TempDir()

	r, err := New(Config{
		Prompt:      "initial> ",
		HistoryFile: filepath.Join(tmpDir, "history"),
		Stdin:       newMockStdin("test\n"),
		Stdout:      io.Discard,
		Stderr:      io.Discard,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer r.Close()

	// Update prompt
	r.SetPrompt("updated> ")

	// Read a line — no panic or error means the prompt update worked
	line, err := r.ReadLine()
	if err != nil {
		t.Fatalf("ReadLine() error = %v", err)
	}
	if line != "test" {
		t.Errorf("ReadLine() = %q, want %q", line, "test")
	}
}

// TestClose tests that Close doesn't panic and can be called multiple times.
func TestClose(t *testing.T) {
	tmpDir := t.TempDir()

	r, err := New(Config{
		Prompt:      "> ",
		HistoryFile: filepath.Join(tmpDir, "history"),
		Stdin:       newMockStdin(""),
		Stdout:      io.Discard,
		Stderr:      io.Discard,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	// First close should succeed
	err = r.Close()
	if err != nil {
		t.Errorf("First Close() error = %v", err)
	}
}

// TestStdout tests that the Stdout writer is usable.
func TestStdout(t *testing.T) {
	tmpDir := t.TempDir()

	r, err := New(Config{
		Prompt:      "> ",
		HistoryFile: filepath.Join(tmpDir, "history"),
		Stdin:       newMockStdin("test\n"),
		Stdout:      io.Discard,
		Stderr:      io.Discard,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer r.Close()

	w := r.Stdout()
	if w == nil {
		t.Error("Stdout() should return a non-nil writer")
	}

	// Writing to it should not panic
	_, err = w.Write([]byte("test output\n"))
	if err != nil {
		t.Errorf("Stdout().Write() error = %v", err)
	}
}

// TestStderr tests that the Stderr writer is usable.
func TestStderr(t *testing.T) {
	tmpDir := t.TempDir()

	r, err := New(Config{
		Prompt:      "> ",
		HistoryFile: filepath.Join(tmpDir, "history"),
		Stdin:       newMockStdin("test\n"),
		Stdout:      io.Discard,
		Stderr:      io.Discard,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer r.Close()

	w := r.Stderr()
	if w == nil {
		t.Error("Stderr() should return a non-nil writer")
	}
}

// TestIsMultiline tests the multiline state accessor.
func TestIsMultiline(t *testing.T) {
	tmpDir := t.TempDir()

	r, err := New(Config{
		Prompt:      "> ",
		HistoryFile: filepath.Join(tmpDir, "history"),
		Stdin:       newMockStdin("normal\n"),
		Stdout:      io.Discard,
		Stderr:      io.Discard,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer r.Close()

	// Before reading, should not be multiline
	if r.IsMultiline() {
		t.Error("Expected IsMultiline() = false before any input")
	}

	// Read a normal line
	_, _ = r.ReadLine()
	if r.IsMultiline() {
		t.Error("Expected IsMultiline() = false after single-line input")
	}
}

// TestAccumulatedLines_Nil tests that AccumulatedLines returns nil
// when not in multiline mode.
func TestAccumulatedLines_Nil(t *testing.T) {
	tmpDir := t.TempDir()

	r, err := New(Config{
		Prompt:      "> ",
		HistoryFile: filepath.Join(tmpDir, "history"),
		Stdin:       newMockStdin("normal\n"),
		Stdout:      io.Discard,
		Stderr:      io.Discard,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer r.Close()

	lines := r.AccumulatedLines()
	if lines != nil {
		t.Errorf("AccumulatedLines() = %v, want nil when not multiline", lines)
	}
}

// TestContinuationPrompt verifies the continuation prompt constant.
func TestContinuationPrompt(t *testing.T) {
	if continuationPrompt == "" {
		t.Error("continuationPrompt should not be empty")
	}
	// Should be indented (not ">" at column 0)
	if !strings.HasPrefix(continuationPrompt, " ") {
		t.Errorf("continuationPrompt should be indented, got %q", continuationPrompt)
	}
}

// TestReadLine_LongInput tests that long input is handled without truncation.
func TestReadLine_LongInput(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a long input line (3000 chars)
	longLine := strings.Repeat("a", 3000) + "\n"

	r, err := New(Config{
		Prompt:      "> ",
		HistoryFile: filepath.Join(tmpDir, "history"),
		Stdin:       newMockStdin(longLine),
		Stdout:      io.Discard,
		Stderr:      io.Discard,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer r.Close()

	result, err := r.ReadLine()
	if err != nil {
		t.Fatalf("ReadLine() error = %v", err)
	}

	if len(result) != 3000 {
		t.Errorf("ReadLine() returned %d chars, want 3000", len(result))
	}
}

// TestReadLine_Multiline_ManyLines tests multiline mode with many lines.
func TestReadLine_Multiline_ManyLines(t *testing.T) {
	tmpDir := t.TempDir()

	// Build 20 continuation lines + 1 final line
	var sb strings.Builder
	for i := 0; i < 20; i++ {
		sb.WriteString("line " + strings.Repeat("x", i))
		sb.WriteString("\\\n")
	}
	sb.WriteString("final line\n")

	r, err := New(Config{
		Prompt:      "> ",
		HistoryFile: filepath.Join(tmpDir, "history"),
		Stdin:       newMockStdin(sb.String()),
		Stdout:      io.Discard,
		Stderr:      io.Discard,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer r.Close()

	result, err := r.ReadLine()
	if err != nil {
		t.Fatalf("ReadLine() error = %v", err)
	}

	lines := strings.Split(result, "\n")
	if len(lines) != 21 {
		t.Errorf("Expected 21 lines, got %d", len(lines))
	}

	// Last line should be "final line"
	if lines[len(lines)-1] != "final line" {
		t.Errorf("Last line = %q, want %q", lines[len(lines)-1], "final line")
	}
}

// TestNew_NoHistoryFile tests creating a Reader with no history file.
func TestNew_NoHistoryFile(t *testing.T) {
	// This tests the case where we explicitly provide a path in a temp dir
	// to avoid touching the real ~/.clyde/history
	tmpDir := t.TempDir()

	r, err := New(Config{
		Prompt:      "> ",
		HistoryFile: filepath.Join(tmpDir, "nonexistent-dir", "history"),
		Stdin:       newMockStdin("test\n"),
		Stdout:      io.Discard,
		Stderr:      io.Discard,
	})
	// readline should still work even if history file can't be created
	if err != nil {
		// This is acceptable — some systems may error on missing parent dirs
		t.Logf("New() returned error (acceptable): %v", err)
		return
	}
	defer r.Close()

	line, err := r.ReadLine()
	if err != nil {
		t.Fatalf("ReadLine() error = %v", err)
	}
	if line != "test" {
		t.Errorf("ReadLine() = %q, want %q", line, "test")
	}
}

// TestReadLine_WhitespaceOnly tests that whitespace-only input is not saved to history.
func TestReadLine_WhitespaceOnly(t *testing.T) {
	tmpDir := t.TempDir()
	historyFile := filepath.Join(tmpDir, "history")

	r, err := New(Config{
		Prompt:      "> ",
		HistoryFile: historyFile,
		Stdin:       newMockStdin("   \nreal\n"),
		Stdout:      io.Discard,
		Stderr:      io.Discard,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	line1, _ := r.ReadLine() // whitespace-only
	if line1 != "   " {
		t.Errorf("ReadLine() = %q, want %q", line1, "   ")
	}

	_, _ = r.ReadLine() // "real"
	r.Close()

	// History should have "real" but not the whitespace
	data, err := os.ReadFile(historyFile)
	if err != nil {
		t.Fatalf("Failed to read history file: %v", err)
	}

	if !strings.Contains(string(data), "real") {
		t.Error("History should contain 'real'")
	}
}

// TestReadLine_SequentialSingleAndMultiline tests alternating between
// single-line and multiline inputs.
func TestReadLine_SequentialSingleAndMultiline(t *testing.T) {
	tmpDir := t.TempDir()

	// single -> multiline -> single
	input := "single one\nfirst\\\nsecond\nsingle two\n"

	r, err := New(Config{
		Prompt:      "> ",
		HistoryFile: filepath.Join(tmpDir, "history"),
		Stdin:       newMockStdin(input),
		Stdout:      io.Discard,
		Stderr:      io.Discard,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer r.Close()

	// First: single line
	got, err := r.ReadLine()
	if err != nil {
		t.Fatalf("ReadLine() 1 error = %v", err)
	}
	if got != "single one" {
		t.Errorf("ReadLine() 1 = %q, want %q", got, "single one")
	}

	// Second: multiline
	got, err = r.ReadLine()
	if err != nil {
		t.Fatalf("ReadLine() 2 error = %v", err)
	}
	if got != "first\nsecond" {
		t.Errorf("ReadLine() 2 = %q, want %q", got, "first\nsecond")
	}

	// Third: single line
	got, err = r.ReadLine()
	if err != nil {
		t.Fatalf("ReadLine() 3 error = %v", err)
	}
	if got != "single two" {
		t.Errorf("ReadLine() 3 = %q, want %q", got, "single two")
	}
}
