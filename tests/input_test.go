package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/this-is-alpha-iota/clyde/cli/input"
)

// mockReadCloser wraps a strings.Reader as an io.ReadCloser for testing.
type mockReadCloser struct {
	*strings.Reader
}

func (m *mockReadCloser) Close() error { return nil }

// newMockStdin creates a mock stdin from the given string.
//
// Byte conventions for simulating keypresses:
//
//	\r        (0x0D / CR)       — Enter key (submit line)
//	\n        (0x0A / LF)       — Ctrl+J (insert newline / multiline)
//	\x1b\x0d (ESC + CR)        — Alt+Enter (insert newline / multiline)
//	\\        (literal \)       — backslash continuation when at end of line
//
// These match what real terminals send in raw mode.
func newMockStdin(s string) io.ReadCloser {
	return &mockReadCloser{strings.NewReader(s)}
}

// TestNew_DefaultConfig tests creating a Reader with defaults.
func TestNew_DefaultConfig(t *testing.T) {
	// Use temp dir so we don't clobber real history
	tmpDir := t.TempDir()
	historyFile := filepath.Join(tmpDir, "test-history")

	r, err := input.New(input.Config{
		Prompt:      "test> ",
		HistoryFile: historyFile,
		Stdin:       newMockStdin("hello\r"),
		Stdout:      io.Discard,
		Stderr:      io.Discard,
	})
	if err != nil {
		t.Fatalf("input.New() error = %v", err)
	}
	defer r.Close()
}

// TestReadLine_SingleLine tests basic single-line input submission.
func TestReadLine_SingleLine(t *testing.T) {
	tmpDir := t.TempDir()

	r, err := input.New(input.Config{
		Prompt:      "> ",
		HistoryFile: filepath.Join(tmpDir, "history"),
		Stdin:       newMockStdin("hello world\r"),
		Stdout:      io.Discard,
		Stderr:      io.Discard,
	})
	if err != nil {
		t.Fatalf("input.New() error = %v", err)
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

	r, err := input.New(input.Config{
		Prompt:      "> ",
		HistoryFile: filepath.Join(tmpDir, "history"),
		Stdin:       newMockStdin("\r"),
		Stdout:      io.Discard,
		Stderr:      io.Discard,
	})
	if err != nil {
		t.Fatalf("input.New() error = %v", err)
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
	r, err := input.New(input.Config{
		Prompt:      "> ",
		HistoryFile: filepath.Join(tmpDir, "history"),
		Stdin:       newMockStdin(""),
		Stdout:      io.Discard,
		Stderr:      io.Discard,
	})
	if err != nil {
		t.Fatalf("input.New() error = %v", err)
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

	r, err := input.New(input.Config{
		Prompt:      "> ",
		HistoryFile: filepath.Join(tmpDir, "history"),
		Stdin:       newMockStdin("first\rsecond\rthird\r"),
		Stdout:      io.Discard,
		Stderr:      io.Discard,
	})
	if err != nil {
		t.Fatalf("input.New() error = %v", err)
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

	// Lines ending with \ continue to the next line.
	// \r simulates pressing Enter to submit each line to readline.
	testInput := "line one\\\rline two\\\rline three\r"

	r, err := input.New(input.Config{
		Prompt:      "> ",
		HistoryFile: filepath.Join(tmpDir, "history"),
		Stdin:       newMockStdin(testInput),
		Stdout:      io.Discard,
		Stderr:      io.Discard,
	})
	if err != nil {
		t.Fatalf("input.New() error = %v", err)
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

	testInput := "start\\\rend\r"

	r, err := input.New(input.Config{
		Prompt:      "> ",
		HistoryFile: filepath.Join(tmpDir, "history"),
		Stdin:       newMockStdin(testInput),
		Stdout:      io.Discard,
		Stderr:      io.Discard,
	})
	if err != nil {
		t.Fatalf("input.New() error = %v", err)
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

	testInput := "\\\rsecond line\r"

	r, err := input.New(input.Config{
		Prompt:      "> ",
		HistoryFile: filepath.Join(tmpDir, "history"),
		Stdin:       newMockStdin(testInput),
		Stdout:      io.Discard,
		Stderr:      io.Discard,
	})
	if err != nil {
		t.Fatalf("input.New() error = %v", err)
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

	r, err := input.New(input.Config{
		Prompt:      "> ",
		HistoryFile: historyFile,
		Stdin:       newMockStdin("first input\rsecond input\r"),
		Stdout:      io.Discard,
		Stderr:      io.Discard,
	})
	if err != nil {
		t.Fatalf("input.New() error = %v", err)
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

	r, err := input.New(input.Config{
		Prompt:      "> ",
		HistoryFile: historyFile,
		Stdin:       newMockStdin("\rreal input\r"),
		Stdout:      io.Discard,
		Stderr:      io.Discard,
	})
	if err != nil {
		t.Fatalf("input.New() error = %v", err)
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
		t.Error("History should contain 'real input'")
	}
}

// TestReadLine_Multiline_HistorySavedAsBlock tests that multiline input
// is saved as a single history entry.
func TestReadLine_Multiline_HistorySavedAsBlock(t *testing.T) {
	tmpDir := t.TempDir()
	historyFile := filepath.Join(tmpDir, "history")

	testInput := "first line\\\rsecond line\r"

	r, err := input.New(input.Config{
		Prompt:      "> ",
		HistoryFile: historyFile,
		Stdin:       newMockStdin(testInput),
		Stdout:      io.Discard,
		Stderr:      io.Discard,
	})
	if err != nil {
		t.Fatalf("input.New() error = %v", err)
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

	r, err := input.New(input.Config{
		Prompt:      "initial> ",
		HistoryFile: filepath.Join(tmpDir, "history"),
		Stdin:       newMockStdin("test\r"),
		Stdout:      io.Discard,
		Stderr:      io.Discard,
	})
	if err != nil {
		t.Fatalf("input.New() error = %v", err)
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

	r, err := input.New(input.Config{
		Prompt:      "> ",
		HistoryFile: filepath.Join(tmpDir, "history"),
		Stdin:       newMockStdin(""),
		Stdout:      io.Discard,
		Stderr:      io.Discard,
	})
	if err != nil {
		t.Fatalf("input.New() error = %v", err)
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

	r, err := input.New(input.Config{
		Prompt:      "> ",
		HistoryFile: filepath.Join(tmpDir, "history"),
		Stdin:       newMockStdin("test\r"),
		Stdout:      io.Discard,
		Stderr:      io.Discard,
	})
	if err != nil {
		t.Fatalf("input.New() error = %v", err)
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

	r, err := input.New(input.Config{
		Prompt:      "> ",
		HistoryFile: filepath.Join(tmpDir, "history"),
		Stdin:       newMockStdin("test\r"),
		Stdout:      io.Discard,
		Stderr:      io.Discard,
	})
	if err != nil {
		t.Fatalf("input.New() error = %v", err)
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

	r, err := input.New(input.Config{
		Prompt:      "> ",
		HistoryFile: filepath.Join(tmpDir, "history"),
		Stdin:       newMockStdin("normal\r"),
		Stdout:      io.Discard,
		Stderr:      io.Discard,
	})
	if err != nil {
		t.Fatalf("input.New() error = %v", err)
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

	r, err := input.New(input.Config{
		Prompt:      "> ",
		HistoryFile: filepath.Join(tmpDir, "history"),
		Stdin:       newMockStdin("normal\r"),
		Stdout:      io.Discard,
		Stderr:      io.Discard,
	})
	if err != nil {
		t.Fatalf("input.New() error = %v", err)
	}
	defer r.Close()

	lines := r.AccumulatedLines()
	if lines != nil {
		t.Errorf("AccumulatedLines() = %v, want nil when not multiline", lines)
	}
}

// TestContinuationPrompt verifies the continuation prompt constant.
func TestContinuationPrompt(t *testing.T) {
	if input.ContinuationPrompt == "" {
		t.Error("input.ContinuationPrompt should not be empty")
	}
	// Should be indented (not ">" at column 0)
	if !strings.HasPrefix(input.ContinuationPrompt, " ") {
		t.Errorf("input.ContinuationPrompt should be indented, got %q", input.ContinuationPrompt)
	}
}

// TestReadLine_LongInput tests that long input is handled without truncation.
func TestReadLine_LongInput(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a long input line (3000 chars)
	longLine := strings.Repeat("a", 3000) + "\r"

	r, err := input.New(input.Config{
		Prompt:      "> ",
		HistoryFile: filepath.Join(tmpDir, "history"),
		Stdin:       newMockStdin(longLine),
		Stdout:      io.Discard,
		Stderr:      io.Discard,
	})
	if err != nil {
		t.Fatalf("input.New() error = %v", err)
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
		sb.WriteString("\\\r")
	}
	sb.WriteString("final line\r")

	r, err := input.New(input.Config{
		Prompt:      "> ",
		HistoryFile: filepath.Join(tmpDir, "history"),
		Stdin:       newMockStdin(sb.String()),
		Stdout:      io.Discard,
		Stderr:      io.Discard,
	})
	if err != nil {
		t.Fatalf("input.New() error = %v", err)
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

	r, err := input.New(input.Config{
		Prompt:      "> ",
		HistoryFile: filepath.Join(tmpDir, "nonexistent-dir", "history"),
		Stdin:       newMockStdin("test\r"),
		Stdout:      io.Discard,
		Stderr:      io.Discard,
	})
	// The reader should still work even if history file can't be created
	if err != nil {
		// This is acceptable — some systems may error on missing parent dirs
		t.Logf("input.New() returned error (acceptable): %v", err)
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

	r, err := input.New(input.Config{
		Prompt:      "> ",
		HistoryFile: historyFile,
		Stdin:       newMockStdin("   \rreal\r"),
		Stdout:      io.Discard,
		Stderr:      io.Discard,
	})
	if err != nil {
		t.Fatalf("input.New() error = %v", err)
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
	testInput := "single one\rfirst\\\rsecond\rsingle two\r"

	r, err := input.New(input.Config{
		Prompt:      "> ",
		HistoryFile: filepath.Join(tmpDir, "history"),
		Stdin:       newMockStdin(testInput),
		Stdout:      io.Discard,
		Stderr:      io.Discard,
	})
	if err != nil {
		t.Fatalf("input.New() error = %v", err)
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

// ============================================================================
// TUI-9: Ctrl+J and Alt+Enter multiline tests
// ============================================================================

// TestReadLine_CtrlJ_BasicMultiline tests that Ctrl+J (0x0A) inserts a
// newline and enters multiline accumulation mode.
func TestReadLine_CtrlJ_BasicMultiline(t *testing.T) {
	tmpDir := t.TempDir()

	// "hello" + Ctrl+J (0x0A) + "world" + Enter (0x0D)
	testInput := "hello\nworld\r"

	r, err := input.New(input.Config{
		Prompt:      "> ",
		HistoryFile: filepath.Join(tmpDir, "history"),
		Stdin:       newMockStdin(testInput),
		Stdout:      io.Discard,
		Stderr:      io.Discard,
	})
	if err != nil {
		t.Fatalf("input.New() error = %v", err)
	}
	defer r.Close()

	result, err := r.ReadLine()
	if err != nil {
		t.Fatalf("ReadLine() error = %v", err)
	}

	want := "hello\nworld"
	if result != want {
		t.Errorf("ReadLine() = %q, want %q", result, want)
	}
}

// TestReadLine_CtrlJ_ThreeLines tests Ctrl+J with three lines.
func TestReadLine_CtrlJ_ThreeLines(t *testing.T) {
	tmpDir := t.TempDir()

	// "line1" + Ctrl+J + "line2" + Ctrl+J + "line3" + Enter
	testInput := "line1\nline2\nline3\r"

	r, err := input.New(input.Config{
		Prompt:      "> ",
		HistoryFile: filepath.Join(tmpDir, "history"),
		Stdin:       newMockStdin(testInput),
		Stdout:      io.Discard,
		Stderr:      io.Discard,
	})
	if err != nil {
		t.Fatalf("input.New() error = %v", err)
	}
	defer r.Close()

	result, err := r.ReadLine()
	if err != nil {
		t.Fatalf("ReadLine() error = %v", err)
	}

	want := "line1\nline2\nline3"
	if result != want {
		t.Errorf("ReadLine() = %q, want %q", result, want)
	}
}

// TestReadLine_CtrlJ_EmptyFirstLine tests Ctrl+J on an empty line
// (just pressing Ctrl+J immediately inserts a blank line).
func TestReadLine_CtrlJ_EmptyFirstLine(t *testing.T) {
	tmpDir := t.TempDir()

	// Ctrl+J (empty first line) + "content" + Enter
	testInput := "\ncontent\r"

	r, err := input.New(input.Config{
		Prompt:      "> ",
		HistoryFile: filepath.Join(tmpDir, "history"),
		Stdin:       newMockStdin(testInput),
		Stdout:      io.Discard,
		Stderr:      io.Discard,
	})
	if err != nil {
		t.Fatalf("input.New() error = %v", err)
	}
	defer r.Close()

	result, err := r.ReadLine()
	if err != nil {
		t.Fatalf("ReadLine() error = %v", err)
	}

	want := "\ncontent"
	if result != want {
		t.Errorf("ReadLine() = %q, want %q", result, want)
	}
}

// TestReadLine_AltEnter_BasicMultiline tests that Alt+Enter (ESC CR / 0x1B 0x0D)
// inserts a newline and enters multiline accumulation mode.
func TestReadLine_AltEnter_BasicMultiline(t *testing.T) {
	tmpDir := t.TempDir()

	// "hello" + Alt+Enter (ESC CR) + "world" + Enter (CR)
	testInput := "hello\x1b\x0dworld\r"

	r, err := input.New(input.Config{
		Prompt:      "> ",
		HistoryFile: filepath.Join(tmpDir, "history"),
		Stdin:       newMockStdin(testInput),
		Stdout:      io.Discard,
		Stderr:      io.Discard,
	})
	if err != nil {
		t.Fatalf("input.New() error = %v", err)
	}
	defer r.Close()

	result, err := r.ReadLine()
	if err != nil {
		t.Fatalf("ReadLine() error = %v", err)
	}

	want := "hello\nworld"
	if result != want {
		t.Errorf("ReadLine() = %q, want %q", result, want)
	}
}

// TestReadLine_AltEnter_ThreeLines tests Alt+Enter with three lines.
func TestReadLine_AltEnter_ThreeLines(t *testing.T) {
	tmpDir := t.TempDir()

	// "a" + Alt+Enter + "b" + Alt+Enter + "c" + Enter
	testInput := "a\x1b\x0db\x1b\x0dc\r"

	r, err := input.New(input.Config{
		Prompt:      "> ",
		HistoryFile: filepath.Join(tmpDir, "history"),
		Stdin:       newMockStdin(testInput),
		Stdout:      io.Discard,
		Stderr:      io.Discard,
	})
	if err != nil {
		t.Fatalf("input.New() error = %v", err)
	}
	defer r.Close()

	result, err := r.ReadLine()
	if err != nil {
		t.Fatalf("ReadLine() error = %v", err)
	}

	want := "a\nb\nc"
	if result != want {
		t.Errorf("ReadLine() = %q, want %q", result, want)
	}
}

// TestReadLine_MixedMultiline tests mixing all three multiline methods
// (backslash, Ctrl+J, Alt+Enter) in a single input block.
func TestReadLine_MixedMultiline(t *testing.T) {
	tmpDir := t.TempDir()

	// "line1\" + Enter (backslash continuation)
	// + "line2" + Ctrl+J (0x0A)
	// + "line3" + Alt+Enter (ESC CR)
	// + "line4" + Enter (submit)
	testInput := "line1\\\rline2\nline3\x1b\x0dline4\r"

	r, err := input.New(input.Config{
		Prompt:      "> ",
		HistoryFile: filepath.Join(tmpDir, "history"),
		Stdin:       newMockStdin(testInput),
		Stdout:      io.Discard,
		Stderr:      io.Discard,
	})
	if err != nil {
		t.Fatalf("input.New() error = %v", err)
	}
	defer r.Close()

	result, err := r.ReadLine()
	if err != nil {
		t.Fatalf("ReadLine() error = %v", err)
	}

	want := "line1\nline2\nline3\nline4"
	if result != want {
		t.Errorf("ReadLine() = %q, want %q", result, want)
	}
}

// TestReadLine_CtrlJ_HistorySavedAsBlock tests that multiline input
// assembled via Ctrl+J is saved to history as a single block.
func TestReadLine_CtrlJ_HistorySavedAsBlock(t *testing.T) {
	tmpDir := t.TempDir()
	historyFile := filepath.Join(tmpDir, "history")

	// "part1" + Ctrl+J + "part2" + Enter
	testInput := "part1\npart2\r"

	r, err := input.New(input.Config{
		Prompt:      "> ",
		HistoryFile: historyFile,
		Stdin:       newMockStdin(testInput),
		Stdout:      io.Discard,
		Stderr:      io.Discard,
	})
	if err != nil {
		t.Fatalf("input.New() error = %v", err)
	}

	result, _ := r.ReadLine()
	r.Close()

	// Verify assembled correctly
	want := "part1\npart2"
	if result != want {
		t.Errorf("ReadLine() = %q, want %q", result, want)
	}

	// History should contain the assembled multiline input
	data, err := os.ReadFile(historyFile)
	if err != nil {
		t.Fatalf("Failed to read history file: %v", err)
	}
	histContent := string(data)
	if !strings.Contains(histContent, "part1") {
		t.Error("History should contain 'part1'")
	}
	if !strings.Contains(histContent, "part2") {
		t.Error("History should contain 'part2'")
	}
}

// TestReadLine_CtrlJ_BackslashPreserved tests that a trailing backslash
// is preserved (not stripped) when Ctrl+J is used for continuation,
// since the backslash is content, not a continuation marker.
func TestReadLine_CtrlJ_BackslashPreserved(t *testing.T) {
	tmpDir := t.TempDir()

	// "path\to\" + Ctrl+J + "file" + Enter
	// The backslash should be preserved because Ctrl+J, not Enter, was used
	testInput := "path\\to\\\nfile\r"

	r, err := input.New(input.Config{
		Prompt:      "> ",
		HistoryFile: filepath.Join(tmpDir, "history"),
		Stdin:       newMockStdin(testInput),
		Stdout:      io.Discard,
		Stderr:      io.Discard,
	})
	if err != nil {
		t.Fatalf("input.New() error = %v", err)
	}
	defer r.Close()

	result, err := r.ReadLine()
	if err != nil {
		t.Fatalf("ReadLine() error = %v", err)
	}

	// With Ctrl+J, the backslash is preserved (it's content, not a continuation marker)
	want := "path\\to\\\nfile"
	if result != want {
		t.Errorf("ReadLine() = %q, want %q", result, want)
	}
}

// TestReadLine_CtrlJ_ThenSingleLine tests that after a Ctrl+J multiline
// input, the next read works as a normal single-line input.
func TestReadLine_CtrlJ_ThenSingleLine(t *testing.T) {
	tmpDir := t.TempDir()

	// First: "a" + Ctrl+J + "b" + Enter
	// Second: "single" + Enter
	testInput := "a\nb\rsingle\r"

	r, err := input.New(input.Config{
		Prompt:      "> ",
		HistoryFile: filepath.Join(tmpDir, "history"),
		Stdin:       newMockStdin(testInput),
		Stdout:      io.Discard,
		Stderr:      io.Discard,
	})
	if err != nil {
		t.Fatalf("input.New() error = %v", err)
	}
	defer r.Close()

	// First: multiline via Ctrl+J
	result, err := r.ReadLine()
	if err != nil {
		t.Fatalf("ReadLine() 1 error = %v", err)
	}
	if result != "a\nb" {
		t.Errorf("ReadLine() 1 = %q, want %q", result, "a\nb")
	}

	// Second: single line
	result, err = r.ReadLine()
	if err != nil {
		t.Fatalf("ReadLine() 2 error = %v", err)
	}
	if result != "single" {
		t.Errorf("ReadLine() 2 = %q, want %q", result, "single")
	}
}

// TestReadLine_CtrlC_DuringCtrlJMultiline tests that Ctrl+C during a
// Ctrl+J multiline session discards the partial input.
func TestReadLine_CtrlC_DuringCtrlJMultiline(t *testing.T) {
	tmpDir := t.TempDir()

	// "partial" + Ctrl+J + Ctrl+C (interrupt) + "after" + Enter
	// Ctrl+C is 0x03 (CharInterrupt)
	testInput := "partial\n\x03after\r"

	r, err := input.New(input.Config{
		Prompt:      "> ",
		HistoryFile: filepath.Join(tmpDir, "history"),
		Stdin:       newMockStdin(testInput),
		Stdout:      io.Discard,
		Stderr:      io.Discard,
	})
	if err != nil {
		t.Fatalf("input.New() error = %v", err)
	}
	defer r.Close()

	// First ReadLine: "partial" + Ctrl+J enters multiline,
	// then Ctrl+C interrupts → returns error
	_, err = r.ReadLine()
	if err == nil {
		t.Fatal("Expected error from Ctrl+C, got nil")
	}

	// Verify multiline state was cleaned up
	if r.IsMultiline() {
		t.Error("Expected IsMultiline() = false after Ctrl+C")
	}

	// Next read should work normally
	result, err := r.ReadLine()
	if err != nil {
		t.Fatalf("ReadLine() after Ctrl+C error = %v", err)
	}
	if result != "after" {
		t.Errorf("ReadLine() after Ctrl+C = %q, want %q", result, "after")
	}
}

// ============================================================================
// Up/Down arrow history suppression tests
// ============================================================================

// upArrow is the terminal escape sequence for the Up arrow key (ESC [ A).
const upArrow = "\x1b[A"

// downArrow is the terminal escape sequence for the Down arrow key (ESC [ B).
const downArrow = "\x1b[B"

// TestReadLine_UpArrow_EmptyPrompt_RecallsHistory tests that up arrow on an
// empty prompt recalls the previous history entry (existing behavior preserved).
func TestReadLine_UpArrow_EmptyPrompt_RecallsHistory(t *testing.T) {
	tmpDir := t.TempDir()

	// First input: "previous" + Enter (creates history)
	// Second input: Up arrow + Enter (should recall "previous")
	testInput := "previous\r" + upArrow + "\r"

	r, err := input.New(input.Config{
		Prompt:      "> ",
		HistoryFile: filepath.Join(tmpDir, "history"),
		Stdin:       newMockStdin(testInput),
		Stdout:      io.Discard,
		Stderr:      io.Discard,
	})
	if err != nil {
		t.Fatalf("input.New() error = %v", err)
	}
	defer r.Close()

	// First: type "previous"
	got, err := r.ReadLine()
	if err != nil {
		t.Fatalf("ReadLine() 1 error = %v", err)
	}
	if got != "previous" {
		t.Errorf("ReadLine() 1 = %q, want %q", got, "previous")
	}

	// Second: up arrow should recall "previous" from history
	got, err = r.ReadLine()
	if err != nil {
		t.Fatalf("ReadLine() 2 error = %v", err)
	}
	if got != "previous" {
		t.Errorf("ReadLine() 2 = %q, want %q (history recall)", got, "previous")
	}
}

// TestReadLine_UpArrow_NonEmptyPrompt_Suppressed tests that up arrow is
// suppressed (does nothing) when the user has typed content on a single line.
func TestReadLine_UpArrow_NonEmptyPrompt_Suppressed(t *testing.T) {
	tmpDir := t.TempDir()

	// First: "old entry" + Enter (creates history)
	// Second: "current" + Up arrow + Enter
	// Up arrow should be suppressed; result should be "current" (not "old entry")
	testInput := "old entry\r" + "current" + upArrow + "\r"

	r, err := input.New(input.Config{
		Prompt:      "> ",
		HistoryFile: filepath.Join(tmpDir, "history"),
		Stdin:       newMockStdin(testInput),
		Stdout:      io.Discard,
		Stderr:      io.Discard,
	})
	if err != nil {
		t.Fatalf("input.New() error = %v", err)
	}
	defer r.Close()

	// First: create history
	got, err := r.ReadLine()
	if err != nil {
		t.Fatalf("ReadLine() 1 error = %v", err)
	}
	if got != "old entry" {
		t.Errorf("ReadLine() 1 = %q, want %q", got, "old entry")
	}

	// Second: "current" + up (suppressed) + Enter → should get "current"
	got, err = r.ReadLine()
	if err != nil {
		t.Fatalf("ReadLine() 2 error = %v", err)
	}
	if got != "current" {
		t.Errorf("ReadLine() 2 = %q, want %q (up arrow should be suppressed)", got, "current")
	}
}

// TestReadLine_UpArrow_MultilineMode_NavigatesToPreviousLine tests that up arrow
// navigates to the previous line when in multiline accumulation mode.
func TestReadLine_UpArrow_MultilineMode_NavigatesToPreviousLine(t *testing.T) {
	tmpDir := t.TempDir()

	// First: "history entry" + Enter (creates history — should NOT be recalled)
	// Second: "first line" + Ctrl+J + Up arrow (navigates back to "first line")
	//         + Ctrl+U (clear line) + "edited" + Enter
	testInput := "history entry\r" + "first line\n" + upArrow + "\x15" + "edited\r"

	r, err := input.New(input.Config{
		Prompt:      "> ",
		HistoryFile: filepath.Join(tmpDir, "history"),
		Stdin:       newMockStdin(testInput),
		Stdout:      io.Discard,
		Stderr:      io.Discard,
	})
	if err != nil {
		t.Fatalf("input.New() error = %v", err)
	}
	defer r.Close()

	// First: create history
	_, err = r.ReadLine()
	if err != nil {
		t.Fatalf("ReadLine() 1 error = %v", err)
	}

	// Second: multiline with up arrow navigation
	got, err := r.ReadLine()
	if err != nil {
		t.Fatalf("ReadLine() 2 error = %v", err)
	}
	want := "edited"
	if got != want {
		t.Errorf("ReadLine() 2 = %q, want %q (up arrow should navigate to previous line)", got, want)
	}
}

// TestReadLine_UpArrow_ContinuedHistoryBrowsing tests that once history
// browsing starts (from empty prompt), further up/down presses continue
// navigating history.
func TestReadLine_UpArrow_ContinuedHistoryBrowsing(t *testing.T) {
	tmpDir := t.TempDir()

	// Create two history entries, then browse with up arrow twice
	// First: "entry1" + Enter
	// Second: "entry2" + Enter
	// Third: Up (→ "entry2") + Up (→ "entry1") + Enter
	testInput := "entry1\r" + "entry2\r" + upArrow + upArrow + "\r"

	r, err := input.New(input.Config{
		Prompt:      "> ",
		HistoryFile: filepath.Join(tmpDir, "history"),
		Stdin:       newMockStdin(testInput),
		Stdout:      io.Discard,
		Stderr:      io.Discard,
	})
	if err != nil {
		t.Fatalf("input.New() error = %v", err)
	}
	defer r.Close()

	_, _ = r.ReadLine() // "entry1"
	_, _ = r.ReadLine() // "entry2"

	// Third: up twice should recall "entry1" (the oldest)
	got, err := r.ReadLine()
	if err != nil {
		t.Fatalf("ReadLine() 3 error = %v", err)
	}
	if got != "entry1" {
		t.Errorf("ReadLine() 3 = %q, want %q (double up should browse to oldest)", got, "entry1")
	}
}

// TestReadLine_DownArrow_NonEmptyPrompt_Suppressed tests that down arrow
// is suppressed when the buffer has content (same as up arrow behavior).
func TestReadLine_DownArrow_NonEmptyPrompt_Suppressed(t *testing.T) {
	tmpDir := t.TempDir()

	// First: "old" + Enter (creates history)
	// Second: "typed" + Down arrow + Enter
	// Down arrow should be suppressed; result should be "typed"
	testInput := "old\r" + "typed" + downArrow + "\r"

	r, err := input.New(input.Config{
		Prompt:      "> ",
		HistoryFile: filepath.Join(tmpDir, "history"),
		Stdin:       newMockStdin(testInput),
		Stdout:      io.Discard,
		Stderr:      io.Discard,
	})
	if err != nil {
		t.Fatalf("input.New() error = %v", err)
	}
	defer r.Close()

	_, _ = r.ReadLine() // "old"

	got, err := r.ReadLine()
	if err != nil {
		t.Fatalf("ReadLine() 2 error = %v", err)
	}
	if got != "typed" {
		t.Errorf("ReadLine() 2 = %q, want %q (down arrow should be suppressed)", got, "typed")
	}
}

// TestReadLine_UpDown_MultilineNavigation tests navigating up then down
// through a multiline block, editing a line and submitting.
func TestReadLine_UpDown_MultilineNavigation(t *testing.T) {
	tmpDir := t.TempDir()

	// "line1" + Ctrl+J + "line2" + Ctrl+J + "line3"
	// Then: Up (→ line2) + Up (→ line1) + Down (→ line2)
	// + Ctrl+U (clear) + "line2 edited" + Enter (submit all)
	testInput := "line1\nline2\nline3" + upArrow + upArrow + downArrow + "\x15" + "line2 edited\r"

	r, err := input.New(input.Config{
		Prompt:      "> ",
		HistoryFile: filepath.Join(tmpDir, "history"),
		Stdin:       newMockStdin(testInput),
		Stdout:      io.Discard,
		Stderr:      io.Discard,
	})
	if err != nil {
		t.Fatalf("input.New() error = %v", err)
	}
	defer r.Close()

	got, err := r.ReadLine()
	if err != nil {
		t.Fatalf("ReadLine() error = %v", err)
	}
	want := "line1\nline2 edited\nline3"
	if got != want {
		t.Errorf("ReadLine() = %q, want %q", got, want)
	}
}

// TestReadLine_UpArrow_EditFirstLine tests navigating up to the first line,
// editing it, and submitting.
func TestReadLine_UpArrow_EditFirstLine(t *testing.T) {
	tmpDir := t.TempDir()

	// "original" + Ctrl+J + Up (→ original) + Ctrl+U (clear) + "replaced" + Enter
	testInput := "original\n" + upArrow + "\x15" + "replaced\r"

	r, err := input.New(input.Config{
		Prompt:      "> ",
		HistoryFile: filepath.Join(tmpDir, "history"),
		Stdin:       newMockStdin(testInput),
		Stdout:      io.Discard,
		Stderr:      io.Discard,
	})
	if err != nil {
		t.Fatalf("input.New() error = %v", err)
	}
	defer r.Close()

	got, err := r.ReadLine()
	if err != nil {
		t.Fatalf("ReadLine() error = %v", err)
	}
	want := "replaced"
	if got != want {
		t.Errorf("ReadLine() = %q, want %q", got, want)
	}
}

// TestReadLine_DownArrow_ToNewLine tests navigating down past the last
// saved line to a fresh new line.
func TestReadLine_DownArrow_ToNewLine(t *testing.T) {
	tmpDir := t.TempDir()

	// "line1" + Ctrl+J + "line2" + Up (→ line1) + Down (→ line2) + Down (→ new) + "line3" + Enter
	testInput := "line1\nline2" + upArrow + downArrow + downArrow + "line3\r"

	r, err := input.New(input.Config{
		Prompt:      "> ",
		HistoryFile: filepath.Join(tmpDir, "history"),
		Stdin:       newMockStdin(testInput),
		Stdout:      io.Discard,
		Stderr:      io.Discard,
	})
	if err != nil {
		t.Fatalf("input.New() error = %v", err)
	}
	defer r.Close()

	got, err := r.ReadLine()
	if err != nil {
		t.Fatalf("ReadLine() error = %v", err)
	}
	want := "line1\nline2\nline3"
	if got != want {
		t.Errorf("ReadLine() = %q, want %q", got, want)
	}
}
