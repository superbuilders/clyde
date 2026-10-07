package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/superbuilders/clyde/agent/session"
	"github.com/superbuilders/clyde/cli"
	"github.com/superbuilders/clyde/cli/loglevel"
	"github.com/superbuilders/clyde/cli/style"
)

// TestErrorMessageTypeExists verifies the dedicated error record type.
func TestErrorMessageTypeExists(t *testing.T) {
	if string(session.TypeError) != "error" {
		t.Errorf("expected session.TypeError to be \"error\", got %q", session.TypeError)
	}
	if session.TypeError == session.TypeDiagnostic {
		t.Error("error type must be distinct from the diagnostic (debug note) type")
	}
}

// TestErrorPersistedAsErrorFile verifies error records are written to disk with
// an _error.md suffix so they are a first-class record type in the history.
func TestErrorPersistedAsErrorFile(t *testing.T) {
	dir := t.TempDir()
	sess := &session.Session{Dir: dir, SessionsRoot: dir}
	if err := sess.WriteMessage(session.TypeError, "❌ Error: boom\n"); err != nil {
		t.Fatalf("WriteMessage: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	found := ""
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), "_error.md") {
			found = e.Name()
		}
	}
	if found == "" {
		t.Fatal("no _error.md file written")
	}
	if got := session.MessageTypeFromFilename(found); got != "error" {
		t.Errorf("MessageTypeFromFilename(%q) = %q, want \"error\"", found, got)
	}
	content, _ := os.ReadFile(filepath.Join(dir, found))
	if !strings.Contains(string(content), "boom") {
		t.Errorf("error content not persisted: %q", content)
	}
}

// TestErrorsSkippedInHistoryReconstruction verifies errors are not replayed
// into the API conversation (like diagnostics and compaction markers).
func TestErrorsSkippedInHistoryReconstruction(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	write("2026-07-20T10-00-00.000_user.md", "**You:**\n\nhello\n")
	write("2026-07-20T10-00-01.000_error.md", "❌ Error: boom\n")
	write("2026-07-20T10-00-02.000_assistant.md", "**Claude:**\n\nhi\n")

	msgs, warnings, err := session.ReconstructHistory(dir)
	if err != nil {
		t.Fatalf("ReconstructHistory: %v", err)
	}
	for _, w := range warnings {
		if strings.Contains(w, "unknown message type") && strings.Contains(w, "error") {
			t.Errorf("error type treated as unknown: %s", w)
		}
	}
	for _, m := range msgs {
		if strings.Contains(contentString(m.Content), "❌ Error") {
			t.Error("error record leaked into reconstructed API history")
		}
	}
}

func contentString(c interface{}) string {
	if s, ok := c.(string); ok {
		return s
	}
	return ""
}

// TestReplayShowsErrorsAtEveryLevelExceptSilent is the core acceptance check:
// errors must render in every verbosity mode except silent, and turning debug
// output off must not hide them.
func TestReplayShowsErrorsAtEveryLevelExceptSilent(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "2026-07-20T10-00-00.000_user.md"), []byte("**You:**\n\nhello\n"), 0644)
	os.WriteFile(filepath.Join(dir, "2026-07-20T10-00-01.000_error.md"), []byte("❌ Error: kaboom\n"), 0644)
	os.WriteFile(filepath.Join(dir, "2026-07-20T10-00-02.000_diagnostic.md"), []byte("🔍 Tokens: input=1 output=2\n"), 0644)

	cases := []struct {
		level     loglevel.Level
		wantError bool
	}{
		{loglevel.Silent, false},
		{loglevel.Quiet, true},
		{loglevel.Normal, true},
		{loglevel.Verbose, true},
		{loglevel.Debug, true},
	}
	for _, tc := range cases {
		out := captureReplay(t, dir, tc.level)
		if got := strings.Contains(out, "kaboom"); got != tc.wantError {
			t.Errorf("level %s: error visible = %v, want %v (output: %q)", tc.level, got, tc.wantError, out)
		}
		// Debug notes stay debug-only; errors must not follow that rule.
		if tc.level < loglevel.Debug && strings.Contains(out, "🔍 Tokens") {
			t.Errorf("level %s: debug note leaked into output", tc.level)
		}
	}
}

// TestReplayLegacyErrorDiagnosticDegradesGracefully verifies sessions written
// before the error type existed still show their errors when debug is hidden.
func TestReplayLegacyErrorDiagnosticDegradesGracefully(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "2026-07-20T10-00-01.000_diagnostic.md"), []byte("❌ Error: legacy boom\n"), 0644)

	for _, lvl := range []loglevel.Level{loglevel.Quiet, loglevel.Normal, loglevel.Verbose, loglevel.Debug} {
		out := captureReplay(t, dir, lvl)
		if !strings.Contains(out, "legacy boom") {
			t.Errorf("level %s: legacy error-as-diagnostic was hidden (output: %q)", lvl, out)
		}
	}
	if out := captureReplay(t, dir, loglevel.Silent); strings.Contains(out, "legacy boom") {
		t.Errorf("silent mode should print nothing, got %q", out)
	}
}

func TestIsLegacyErrorDiagnostic(t *testing.T) {
	if !session.IsLegacyErrorDiagnostic("❌ Error: boom") {
		t.Error("expected legacy error diagnostic to be recognized")
	}
	if session.IsLegacyErrorDiagnostic("🔍 Tokens: input=1") {
		t.Error("token diagnostics must not be treated as errors")
	}
}

func TestFormatErrorIsDistinctFromDebug(t *testing.T) {
	e := style.FormatError("boom")
	d := style.FormatDebug("boom")
	if e == d {
		t.Error("errors must be visually distinguished from debug notes")
	}
	if !strings.Contains(e, "❌") {
		t.Errorf("error formatting should carry an error prefix, got %q", e)
	}
	if strings.Count(style.FormatError("❌ Error: boom"), "❌") != 1 {
		t.Error("error prefix should not be duplicated")
	}
}

// captureReplay runs the session replay at a level and captures stdout.
func captureReplay(t *testing.T, dir string, level loglevel.Level) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	cli.ReplaySession(dir, level)
	w.Close()
	os.Stdout = old
	buf := make([]byte, 64*1024)
	n, _ := r.Read(buf)
	r.Close()
	return string(buf[:n])
}
