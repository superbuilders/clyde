package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/superbuilders/clyde/agent/session"
	"github.com/superbuilders/clyde/cli"
	"github.com/superbuilders/clyde/cli/input"
	"github.com/superbuilders/clyde/cli/loglevel"
)

// ============================================================================
// Fix 1: Back cursor movement with multiple lines
// ============================================================================

// TestReadLine_LeftArrow_CrossLineBoundary tests that pressing left at position 0
// of a multiline line crosses to the end of the previous line.
func TestReadLine_LeftArrow_CrossLineBoundary(t *testing.T) {
	tmpDir := t.TempDir()

	// "abc" + Ctrl+J + "def" + Home (Ctrl+A, go to pos 0 on line 2)
	// + Left (should cross to end of "abc", pos 3)
	// + Ctrl+U (clear line 1 = "abc") + Enter
	// Result: line 0 cleared → "" + "\n" + "def" = "\ndef"
	testInput := "abc\ndef\x01\x1b[D\x15\r"

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

	// After crossing to line 0 and clearing it, we get "" + "\n" + "def"
	want := "\ndef"
	if result != want {
		t.Errorf("ReadLine() = %q, want %q (left arrow should cross line boundary)", result, want)
	}
}

// TestReadLine_LeftArrow_CrossMultipleLines tests crossing multiple line
// boundaries with repeated left arrow presses.
func TestReadLine_LeftArrow_CrossMultipleLines(t *testing.T) {
	tmpDir := t.TempDir()

	// "ab" + Ctrl+J + "cd" + Ctrl+J + "ef"
	// + Home (go to pos 0 on line 2)
	// + Left (cross to end of "cd" on line 1)
	// + Left + Left (move to pos 0 on line 1)
	// + Left (cross to end of "ab" on line 0)
	// + Ctrl+U (clear line 0) + Enter
	testInput := "ab\ncd\nef\x01\x1b[D\x1b[D\x1b[D\x1b[D\x15\r"

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

	// After crossing to line 0 and clearing it: "" + "\n" + "cd" + "\n" + "ef"
	want := "\ncd\nef"
	if result != want {
		t.Errorf("ReadLine() = %q, want %q (left arrow should cross multiple lines)", result, want)
	}
}

// TestReadLine_RightArrow_CrossLineBoundary tests that pressing right at the
// end of a line crosses to the start of the next line.
func TestReadLine_RightArrow_CrossLineBoundary(t *testing.T) {
	tmpDir := t.TempDir()

	// "abc" + Ctrl+J + "def"
	// + Up (go to line 0) + End (go to end of "abc")
	// + Right (cross to start of "def" on line 1)
	// + delete 'd' + Enter
	testInput := "abc\ndef" + upArrow + "\x05\x1b[C\x1b[3~\r"

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

	// Right arrow crossed to start of line 1, Delete removed 'd': "abc\nef"
	want := "abc\nef"
	if result != want {
		t.Errorf("ReadLine() = %q, want %q (right arrow should cross line boundary)", result, want)
	}
}

// TestReadLine_LeftArrow_NoWrapOnFirstLine tests that left arrow at position 0
// of the first line does nothing (no wrap past the beginning).
func TestReadLine_LeftArrow_NoWrapOnFirstLine(t *testing.T) {
	tmpDir := t.TempDir()

	// "abc" + Ctrl+J + "def"
	// + Up (go to line 0) + Home (pos 0)
	// + Left (should do nothing — already at start of first line)
	// + 'X' + Enter
	testInput := "abc\ndef" + upArrow + "\x01\x1b[DX\r"

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

	// 'X' inserted at position 0 of line 0: "Xabc\ndef"
	want := "Xabc\ndef"
	if result != want {
		t.Errorf("ReadLine() = %q, want %q (left arrow should not wrap past first line)", result, want)
	}
}

// TestReadLine_RightArrow_NoWrapOnLastLine tests that right arrow at end
// of the last line does nothing (no wrap past the end).
func TestReadLine_RightArrow_NoWrapOnLastLine(t *testing.T) {
	tmpDir := t.TempDir()

	// "abc" + Ctrl+J + "def"
	// + Right (should do nothing — already at end of last line)
	// + 'X' + Enter
	testInput := "abc\ndef\x1b[CX\r"

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

	// 'X' inserted at end of "def": "abc\ndefX"
	want := "abc\ndefX"
	if result != want {
		t.Errorf("ReadLine() = %q, want %q (right arrow should not wrap past last line)", result, want)
	}
}

// TestReadLine_LeftArrow_SingleLine_NoChange tests that left arrow at position 0
// on a single line (non-multiline) does nothing — no crash or unexpected behavior.
func TestReadLine_LeftArrow_SingleLine_NoChange(t *testing.T) {
	tmpDir := t.TempDir()

	// Home + Left (at pos 0, single line, should do nothing) + 'X' + Enter
	testInput := "abc\x01\x1b[DX\r"

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

	// 'X' inserted at position 0: "Xabc"
	want := "Xabc"
	if result != want {
		t.Errorf("ReadLine() = %q, want %q", result, want)
	}
}

// TestReadLine_LeftRight_RoundTrip tests that crossing right then left
// returns the cursor to the original position.
func TestReadLine_LeftRight_RoundTrip(t *testing.T) {
	tmpDir := t.TempDir()

	// "abc" + Ctrl+J + "def"
	// + Up (line 0) + End (end of "abc")
	// + Right (cross to start of "def" on line 1)
	// + Left (cross back to end of "abc" on line 0)
	// + 'X' + Enter
	testInput := "abc\ndef" + upArrow + "\x05\x1b[C\x1b[DX\r"

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

	// Right then Left round-trip: cursor back at end of "abc", insert 'X': "abcX\ndef"
	want := "abcX\ndef"
	if result != want {
		t.Errorf("ReadLine() = %q, want %q (left-right round trip)", result, want)
	}
}

// ============================================================================
// Fix 2: Redraw chat on -r (session replay)
// ============================================================================

// TestReplaySession_BasicMessages tests that ReplaySession displays user and
// assistant messages from session files.
func TestReplaySession_BasicMessages(t *testing.T) {
	tmpDir := t.TempDir()

	// Create mock session files with proper timestamps
	writeSessionFile(t, tmpDir, "2026-07-20T10-00-00.001_user.md",
		"**You:**\n\nHello, world!\n")
	writeSessionFile(t, tmpDir, "2026-07-20T10-00-01.001_assistant.md",
		"**Claude:**\n\nHi there! How can I help?\n")

	// Capture output
	output := captureStdout(t, func() {
		cli.ReplaySession(tmpDir, loglevel.Normal)
	})

	// Verify user message appears
	if !strings.Contains(output, "Hello, world!") {
		t.Errorf("Replay should contain user message 'Hello, world!', got:\n%s", output)
	}

	// Verify assistant message appears
	if !strings.Contains(output, "Hi there! How can I help?") {
		t.Errorf("Replay should contain assistant message, got:\n%s", output)
	}
}

// TestReplaySession_ThinkingHiddenAtQuiet tests that thinking traces are
// hidden at Quiet verbosity level.
func TestReplaySession_ThinkingHiddenAtQuiet(t *testing.T) {
	tmpDir := t.TempDir()

	writeSessionFile(t, tmpDir, "2026-07-20T10-00-00.001_user.md",
		"**You:**\n\nTest\n")
	writeSessionFile(t, tmpDir, "2026-07-20T10-00-01.001_thinking.md",
		"💭 This is my thinking process\nsignature: abc123\n")
	writeSessionFile(t, tmpDir, "2026-07-20T10-00-02.001_assistant.md",
		"**Claude:**\n\nResponse\n")

	output := captureStdout(t, func() {
		cli.ReplaySession(tmpDir, loglevel.Quiet)
	})

	// Thinking should NOT appear at Quiet level
	if strings.Contains(output, "thinking process") {
		t.Errorf("Thinking traces should be hidden at Quiet level, got:\n%s", output)
	}

	// But user and assistant should still appear
	if !strings.Contains(output, "Test") {
		t.Errorf("User message should appear at Quiet level")
	}
	if !strings.Contains(output, "Response") {
		t.Errorf("Assistant message should appear at Quiet level")
	}
}

// TestReplaySession_ThinkingShownAtNormal tests that thinking traces are
// shown at Normal verbosity level.
func TestReplaySession_ThinkingShownAtNormal(t *testing.T) {
	tmpDir := t.TempDir()

	writeSessionFile(t, tmpDir, "2026-07-20T10-00-00.001_user.md",
		"**You:**\n\nTest\n")
	writeSessionFile(t, tmpDir, "2026-07-20T10-00-01.001_thinking.md",
		"💭 This is my thinking process\nsignature: abc123\n")
	writeSessionFile(t, tmpDir, "2026-07-20T10-00-02.001_assistant.md",
		"**Claude:**\n\nResponse\n")

	output := captureStdout(t, func() {
		cli.ReplaySession(tmpDir, loglevel.Normal)
	})

	// Thinking SHOULD appear at Normal level
	if !strings.Contains(output, "thinking process") {
		t.Errorf("Thinking traces should be shown at Normal level, got:\n%s", output)
	}
}

// TestReplaySession_ToolProgressAtQuiet tests that tool progress lines
// appear at Quiet level.
func TestReplaySession_ToolProgressAtQuiet(t *testing.T) {
	tmpDir := t.TempDir()

	writeSessionFile(t, tmpDir, "2026-07-20T10-00-00.001_user.md",
		"**You:**\n\nRead main.go\n")
	writeSessionFile(t, tmpDir, "2026-07-20T10-00-01.001_tool-use.md",
		"→ Reading file: main.go [toolu_abc123]\nname: read_file\ninput: {\"path\":\"main.go\"}\n")
	writeSessionFile(t, tmpDir, "2026-07-20T10-00-02.001_tool-result.md",
		"[toolu_abc123]\n```\npackage main\n```\n")
	writeSessionFile(t, tmpDir, "2026-07-20T10-00-03.001_assistant.md",
		"**Claude:**\n\nDone\n")

	output := captureStdout(t, func() {
		cli.ReplaySession(tmpDir, loglevel.Quiet)
	})

	// Tool progress should appear at Quiet
	if !strings.Contains(output, "Reading file") {
		t.Errorf("Tool progress should appear at Quiet level, got:\n%s", output)
	}

	// Tool result should NOT appear at Quiet (requires Normal)
	if strings.Contains(output, "package main") {
		t.Errorf("Tool result should not appear at Quiet level, got:\n%s", output)
	}
}

// TestReplaySession_ToolResultAtNormal tests that tool results appear at
// Normal verbosity level.
func TestReplaySession_ToolResultAtNormal(t *testing.T) {
	tmpDir := t.TempDir()

	writeSessionFile(t, tmpDir, "2026-07-20T10-00-00.001_user.md",
		"**You:**\n\nRead main.go\n")
	writeSessionFile(t, tmpDir, "2026-07-20T10-00-01.001_tool-use.md",
		"→ Reading file: main.go [toolu_abc123]\nname: read_file\ninput: {\"path\":\"main.go\"}\n")
	writeSessionFile(t, tmpDir, "2026-07-20T10-00-02.001_tool-result.md",
		"[toolu_abc123]\n```\npackage main\n```\n")
	writeSessionFile(t, tmpDir, "2026-07-20T10-00-03.001_assistant.md",
		"**Claude:**\n\nDone\n")

	output := captureStdout(t, func() {
		cli.ReplaySession(tmpDir, loglevel.Normal)
	})

	// Tool result SHOULD appear at Normal
	if !strings.Contains(output, "package main") {
		t.Errorf("Tool result should appear at Normal level, got:\n%s", output)
	}
}

// TestReplaySession_SilentOnlyShowsUserAssistant tests that at Silent level,
// only user and assistant messages are shown.
func TestReplaySession_SilentOnlyShowsUserAssistant(t *testing.T) {
	tmpDir := t.TempDir()

	writeSessionFile(t, tmpDir, "2026-07-20T10-00-00.001_user.md",
		"**You:**\n\nTest prompt\n")
	writeSessionFile(t, tmpDir, "2026-07-20T10-00-01.001_thinking.md",
		"💭 secret thinking\nsignature: sig\n")
	writeSessionFile(t, tmpDir, "2026-07-20T10-00-02.001_tool-use.md",
		"→ Reading file: x [toolu_1]\nname: read_file\ninput: {}\n")
	writeSessionFile(t, tmpDir, "2026-07-20T10-00-03.001_tool-result.md",
		"[toolu_1]\n```\nfile content\n```\n")
	writeSessionFile(t, tmpDir, "2026-07-20T10-00-04.001_assistant.md",
		"**Claude:**\n\nFinal answer\n")

	output := captureStdout(t, func() {
		cli.ReplaySession(tmpDir, loglevel.Silent)
	})

	// User and assistant messages always show
	if !strings.Contains(output, "Test prompt") {
		t.Errorf("User message should appear at Silent level")
	}
	if !strings.Contains(output, "Final answer") {
		t.Errorf("Assistant message should appear at Silent level")
	}

	// Everything else should be hidden
	if strings.Contains(output, "secret thinking") {
		t.Errorf("Thinking should be hidden at Silent level")
	}
	if strings.Contains(output, "Reading file") {
		t.Errorf("Tool progress should be hidden at Silent level")
	}
	if strings.Contains(output, "file content") {
		t.Errorf("Tool result should be hidden at Silent level")
	}
}

// TestReplaySession_CompactionBoundary tests that replay starts from the
// latest system message (compaction boundary).
func TestReplaySession_CompactionBoundary(t *testing.T) {
	tmpDir := t.TempDir()

	// Old messages before compaction
	writeSessionFile(t, tmpDir, "2026-07-20T09-00-00.001_user.md",
		"**You:**\n\nOld message\n")
	writeSessionFile(t, tmpDir, "2026-07-20T09-00-01.001_assistant.md",
		"**Claude:**\n\nOld response\n")

	// Compaction system message
	writeSessionFile(t, tmpDir, "2026-07-20T10-00-00.001_system.md",
		"**System:**\n\nCompaction summary here\n")

	// New messages after compaction
	writeSessionFile(t, tmpDir, "2026-07-20T10-00-01.001_user.md",
		"**You:**\n\nNew message\n")
	writeSessionFile(t, tmpDir, "2026-07-20T10-00-02.001_assistant.md",
		"**Claude:**\n\nNew response\n")

	output := captureStdout(t, func() {
		cli.ReplaySession(tmpDir, loglevel.Normal)
	})

	// Old messages should NOT appear (before compaction boundary)
	if strings.Contains(output, "Old message") {
		t.Errorf("Messages before compaction should not appear")
	}

	// New messages SHOULD appear
	if !strings.Contains(output, "New message") {
		t.Errorf("Messages after compaction should appear")
	}
	if !strings.Contains(output, "New response") {
		t.Errorf("Assistant messages after compaction should appear")
	}
}

// TestReplaySession_EmptyDir tests replay with an empty session directory.
func TestReplaySession_EmptyDir(t *testing.T) {
	tmpDir := t.TempDir()

	// Should not panic or error
	output := captureStdout(t, func() {
		cli.ReplaySession(tmpDir, loglevel.Normal)
	})

	if output != "" {
		t.Errorf("Empty session replay should produce no output, got: %q", output)
	}
}

// TestReplaySession_DiagnosticsAtDebug tests that diagnostics only appear
// at Debug verbosity.
func TestReplaySession_DiagnosticsAtDebug(t *testing.T) {
	tmpDir := t.TempDir()

	writeSessionFile(t, tmpDir, "2026-07-20T10-00-00.001_user.md",
		"**You:**\n\nTest\n")
	writeSessionFile(t, tmpDir, "2026-07-20T10-00-01.001_diagnostic.md",
		"🔍 Token usage: 5000 input, 1000 output\n")
	writeSessionFile(t, tmpDir, "2026-07-20T10-00-02.001_assistant.md",
		"**Claude:**\n\nDone\n")

	// At Normal: no diagnostics
	outputNormal := captureStdout(t, func() {
		cli.ReplaySession(tmpDir, loglevel.Normal)
	})
	if strings.Contains(outputNormal, "Token usage") {
		t.Errorf("Diagnostics should not appear at Normal level")
	}

	// At Debug: diagnostics shown
	outputDebug := captureStdout(t, func() {
		cli.ReplaySession(tmpDir, loglevel.Debug)
	})
	if !strings.Contains(outputDebug, "Token usage") {
		t.Errorf("Diagnostics should appear at Debug level")
	}
}

// ============================================================================
// Fix 3: /verbosity command (loglevel.FromString)
// ============================================================================

// TestFromString_ValidLevels tests that all valid level names are parsed.
func TestFromString_ValidLevels(t *testing.T) {
	tests := []struct {
		input string
		want  loglevel.Level
	}{
		{"silent", loglevel.Silent},
		{"quiet", loglevel.Quiet},
		{"q", loglevel.Quiet},
		{"normal", loglevel.Normal},
		{"n", loglevel.Normal},
		{"verbose", loglevel.Verbose},
		{"v", loglevel.Verbose},
		{"debug", loglevel.Debug},
		{"d", loglevel.Debug},
		{"SILENT", loglevel.Silent},
		{"Quiet", loglevel.Quiet},
		{"VERBOSE", loglevel.Verbose},
		{"  normal  ", loglevel.Normal},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, ok := loglevel.FromString(tt.input)
			if !ok {
				t.Errorf("FromString(%q) returned ok=false, want true", tt.input)
			}
			if got != tt.want {
				t.Errorf("FromString(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// TestFromString_Invalid tests that invalid level names return false.
func TestFromString_Invalid(t *testing.T) {
	invalid := []string{"", "foo", "verboose", "123", "silence"}
	for _, s := range invalid {
		t.Run(s, func(t *testing.T) {
			_, ok := loglevel.FromString(s)
			if ok {
				t.Errorf("FromString(%q) returned ok=true, want false", s)
			}
		})
	}
}

// TestParseVerbosityCommand tests the /verbosity command parsing.
func TestParseVerbosityCommand(t *testing.T) {
	tests := []struct {
		input   string
		want    loglevel.Level
		wantOK  bool
	}{
		{"/verbosity quiet", loglevel.Quiet, true},
		{"/verbosity verbose", loglevel.Verbose, true},
		{"/verbosity debug", loglevel.Debug, true},
		{"/verbosity normal", loglevel.Normal, true},
		{"/verbosity silent", loglevel.Silent, true},
		{"/verbosity", loglevel.Normal, false},        // missing level
		{"/verbosity foo bar", loglevel.Normal, false}, // too many args
		{"/verbosity invalid", loglevel.Normal, false}, // unknown level
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, ok := cli.ParseVerbosityCommand(tt.input)
			if ok != tt.wantOK {
				t.Errorf("ParseVerbosityCommand(%q) ok = %v, want %v", tt.input, ok, tt.wantOK)
			}
			if ok && got != tt.want {
				t.Errorf("ParseVerbosityCommand(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// ============================================================================
// Helper functions
// ============================================================================

// writeSessionFile creates a session message file in the given directory.
func writeSessionFile(t *testing.T, dir, filename, content string) {
	t.Helper()
	path := filepath.Join(dir, filename)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write session file %s: %v", filename, err)
	}
}

// captureStdout captures stdout output from a function and returns it as a string.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	// Save original stdout
	oldStdout := os.Stdout

	// Create pipe
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Failed to create pipe: %v", err)
	}

	// Redirect stdout
	os.Stdout = w

	// Run function
	fn()

	// Restore stdout and close writer
	w.Close()
	os.Stdout = oldStdout

	// Read captured output
	var buf strings.Builder
	io.Copy(&buf, r)
	r.Close()

	return buf.String()
}

// Ensure session package is used (prevent import cycle errors)
var _ = session.TypeUser
// Ensure fmt is used
var _ = fmt.Sprintf
