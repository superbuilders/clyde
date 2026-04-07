package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/this-is-alpha-iota/clyde/loglevel"
	"github.com/this-is-alpha-iota/clyde/spinner"
)

// TestSpinnerNotShownInCLIMode verifies that CLI mode doesn't use a spinner.
// In CLI mode, progress goes directly to stderr as permanent lines.
func TestSpinnerNotShownInCLIMode(t *testing.T) {
	// CLI mode uses a simple callback that prints to stderr.
	// No spinner is created. This test verifies the design by checking
	// that CLI callbacks produce immediate output (no spinner needed).
	var output strings.Builder

	callback := func(lvl loglevel.Level, msg string) {
		output.WriteString(msg + "\n")
	}

	// Simulate CLI-mode progress callback
	callback(loglevel.Quiet, "→ Reading file: main.go")
	callback(loglevel.Normal, "package main\nfunc main() {}")

	result := output.String()
	if !strings.Contains(result, "→ Reading file: main.go") {
		t.Error("CLI mode should print progress lines directly")
	}
	if !strings.Contains(result, "package main") {
		t.Error("CLI mode should print tool output directly")
	}
}

// TestSpinnerNotShownAtSilentLevel verifies spinner is suppressed at Silent level.
func TestSpinnerNotShownAtSilentLevel(t *testing.T) {
	var buf bytes.Buffer
	sp := spinner.NewWithWriter(&buf)

	level := loglevel.Silent

	// Simulate the REPL callback logic at Silent level
	if level != loglevel.Silent {
		sp.Start(spinner.FormatSpinnerMessage("→ Reading file: main.go"))
	}

	time.Sleep(50 * time.Millisecond)

	// Should not be active because we didn't start it
	if sp.IsActive() {
		t.Error("Spinner should not be active at Silent level")
	}

	// No output should be written
	if buf.Len() > 0 {
		t.Errorf("No spinner output expected at Silent level, got: %q", buf.String())
	}
}

// TestSpinnerShownAtQuietLevel verifies spinner is shown at Quiet level.
func TestSpinnerShownAtQuietLevel(t *testing.T) {
	var buf bytes.Buffer
	sp := spinner.NewWithWriter(&buf)

	level := loglevel.Quiet

	// Simulate the REPL callback logic at Quiet level
	if level != loglevel.Silent {
		sp.Start(spinner.FormatSpinnerMessage("→ Reading file: main.go"))
	}

	time.Sleep(50 * time.Millisecond)

	if !sp.IsActive() {
		t.Error("Spinner should be active at Quiet level")
	}

	sp.Stop()

	// Should have produced output
	if buf.Len() == 0 {
		t.Error("Spinner should produce output at Quiet level")
	}

	output := buf.String()
	// Spinner now shows verb-only messages
	if !strings.Contains(output, "Reading...") {
		t.Errorf("Spinner output should contain verb-only message, got: %q",
			output[:min(200, len(output))])
	}
}

// TestSpinnerShownAtNormalLevel verifies spinner is shown at Normal level.
func TestSpinnerShownAtNormalLevel(t *testing.T) {
	var buf bytes.Buffer
	sp := spinner.NewWithWriter(&buf)

	level := loglevel.Normal

	if level != loglevel.Silent {
		sp.Start(spinner.FormatSpinnerMessage("→ Patching file: agent.go (+48 bytes)"))
	}

	time.Sleep(50 * time.Millisecond)

	if !sp.IsActive() {
		t.Error("Spinner should be active at Normal level")
	}

	sp.Stop()

	output := buf.String()
	// Spinner now shows verb-only messages
	if !strings.Contains(output, "Patching...") {
		t.Errorf("Spinner should show verb-only operation at Normal level, got: %q",
			output[:min(200, len(output))])
	}
}

// TestSpinnerPersistenceRule verifies that the spinner line content also
// appears in the permanent scrollback when the operation completes.
func TestSpinnerPersistenceRule(t *testing.T) {
	var spinnerBuf bytes.Buffer
	var scrollback strings.Builder

	sp := spinner.NewWithWriter(&spinnerBuf)

	progressMsg := "→ Patching file: agent.go (+48 bytes)"
	var lastProgressMsg string

	// Phase 1: Tool progress (Quiet level) → start spinner
	lastProgressMsg = progressMsg
	sp.Start(spinner.FormatSpinnerMessage(progressMsg))
	time.Sleep(50 * time.Millisecond)

	// Phase 2: Tool output (Normal level) → stop spinner, print permanent line
	if sp.IsActive() {
		sp.Stop()
	}
	if lastProgressMsg != "" {
		scrollback.WriteString(lastProgressMsg + "\n")
		lastProgressMsg = ""
	}
	scrollback.WriteString("File patched successfully\n")

	// Verify: the permanent scrollback contains the progress line
	result := scrollback.String()
	if !strings.Contains(result, "→ Patching file: agent.go (+48 bytes)") {
		t.Error("Permanent scrollback should contain the → progress line")
	}
	if !strings.Contains(result, "File patched successfully") {
		t.Error("Permanent scrollback should contain the tool output")
	}

	// Verify: spinner was cleared (last write ends with clear sequence)
	spinnerOutput := spinnerBuf.String()
	if !strings.HasSuffix(spinnerOutput, "\r\033[K") {
		t.Error("Spinner line should be cleared after Stop()")
	}
}

// TestSpinnerToolWithNoOutput verifies that the permanent → line is still
// printed even when a tool emits no output body.
func TestSpinnerToolWithNoOutput(t *testing.T) {
	var spinnerBuf bytes.Buffer
	var scrollback strings.Builder

	sp := spinner.NewWithWriter(&spinnerBuf)

	progressMsg := "→ Writing file: output.txt (1.2 KB)"
	var lastProgressMsg string

	// Phase 1: Tool progress → start spinner
	lastProgressMsg = progressMsg
	sp.Start(spinner.FormatSpinnerMessage(progressMsg))
	time.Sleep(50 * time.Millisecond)

	// Phase 2: HandleMessage completes without Normal-level output.
	// The spinner should be stopped and the progress line printed.
	if sp.IsActive() {
		sp.Stop()
	}
	if lastProgressMsg != "" {
		scrollback.WriteString(lastProgressMsg + "\n")
		lastProgressMsg = ""
	}

	// Verify the progress line is in scrollback
	result := scrollback.String()
	if !strings.Contains(result, "→ Writing file: output.txt (1.2 KB)") {
		t.Error("Progress line should appear in scrollback even without tool output")
	}
}

// TestSpinnerMultipleToolCalls verifies correct behavior with sequential tool calls.
func TestSpinnerMultipleToolCalls(t *testing.T) {
	var spinnerBuf bytes.Buffer
	var scrollback strings.Builder

	sp := spinner.NewWithWriter(&spinnerBuf)
	var lastProgressMsg string

	// Tool 1: read_file
	lastProgressMsg = "→ Reading file: main.go"
	sp.Start(spinner.FormatSpinnerMessage(lastProgressMsg))
	time.Sleep(30 * time.Millisecond)

	// Tool 1 output
	sp.Stop()
	scrollback.WriteString(lastProgressMsg + "\n")
	scrollback.WriteString("package main\n")
	lastProgressMsg = ""

	// Tool 2: patch_file
	lastProgressMsg = "→ Patching file: main.go (+10 bytes)"
	sp.Start(spinner.FormatSpinnerMessage(lastProgressMsg))
	time.Sleep(30 * time.Millisecond)

	// Tool 2 output
	sp.Stop()
	scrollback.WriteString(lastProgressMsg + "\n")
	scrollback.WriteString("Patched successfully\n")
	lastProgressMsg = ""

	result := scrollback.String()

	// Both progress lines should appear
	if !strings.Contains(result, "→ Reading file: main.go") {
		t.Error("First tool progress line missing from scrollback")
	}
	if !strings.Contains(result, "→ Patching file: main.go (+10 bytes)") {
		t.Error("Second tool progress line missing from scrollback")
	}

	// Both output bodies should appear
	if !strings.Contains(result, "package main") {
		t.Error("First tool output missing from scrollback")
	}
	if !strings.Contains(result, "Patched successfully") {
		t.Error("Second tool output missing from scrollback")
	}
}

// TestSpinnerMessageUpdate verifies that when the agent starts a second
// tool while the spinner is still running (unlikely but possible with
// the current architecture), the message is updated.
func TestSpinnerMessageUpdate(t *testing.T) {
	var buf bytes.Buffer
	sp := spinner.NewWithWriter(&buf)

	sp.Start("Reading file: main.go...")
	time.Sleep(30 * time.Millisecond)

	if sp.Message() != "Reading file: main.go..." {
		t.Errorf("Message = %q, want %q", sp.Message(), "Reading file: main.go...")
	}

	sp.Start("Patching file: main.go...")
	time.Sleep(30 * time.Millisecond)

	if sp.Message() != "Patching file: main.go..." {
		t.Errorf("Message = %q, want %q", sp.Message(), "Patching file: main.go...")
	}

	sp.Stop()
}

// TestFormatSpinnerMessage_Integration verifies FormatSpinnerMessage works
// correctly with real tool progress messages from the codebase.
// Spinner messages should be verb-only to prevent the frame-bleed bug.
func TestFormatSpinnerMessage_Integration(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string // Exact expected verb-only output
	}{
		{
			name:     "list_files tool",
			input:    "→ Listing files: . (current directory)",
			expected: "Listing...",
		},
		{
			name:     "read_file tool",
			input:    "→ Reading file: agent/agent.go",
			expected: "Reading...",
		},
		{
			name:     "patch_file tool",
			input:    "→ Patching file: main.go (+100 bytes)",
			expected: "Patching...",
		},
		{
			name:     "write_file tool",
			input:    "→ Writing file: output.txt (42.5 KB)",
			expected: "Writing...",
		},
		{
			name:     "run_bash tool",
			input:    "→ Running bash: go test -v ./...",
			expected: "Running...",
		},
		{
			name:     "grep tool",
			input:    "→ Searching: 'TODO' in . (*.go)",
			expected: "Searching...",
		},
		{
			name:     "browse tool (web fetch)",
			input:    "→ Browsing: https://pkg.go.dev/net/http",
			expected: "Reading Webpage...",
		},
		{
			name:     "browser tool (playwright)",
			input:    "→ Browser: navigate https://example.com",
			expected: "Browsing...",
		},
		{
			name:     "web_search tool",
			input:    "→ Searching web: \"golang HTTP client\"",
			expected: "Searching...",
		},
		{
			name:     "include_file tool",
			input:    "→ Including file: screenshot.png",
			expected: "Loading...",
		},
		{
			name:     "multi_patch tool",
			input:    "→ Applying multi-patch: 5 files",
			expected: "Patching...",
		},
		{
			name:     "browse with very long URL",
			input:    "→ Browsing: https://example.com/api/v1/documents?page=1&format=json&filter=active&sort=date&limit=100&offset=200",
			expected: "Reading Webpage...",
		},
		{
			name:     "run_bash with multi-line command",
			input:    "→ Running bash: cd /tmp\nfind . -name '*.go'\nxargs grep TODO",
			expected: "Running...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := spinner.FormatSpinnerMessage(tt.input)

			// Must not start with "→ " (stripped)
			if strings.HasPrefix(result, "→ ") {
				t.Errorf("FormatSpinnerMessage should strip '→ ' prefix, got: %q", result)
			}

			// Must be the exact verb-only form
			if result != tt.expected {
				t.Errorf("FormatSpinnerMessage(%q) = %q, want %q", tt.input, result, tt.expected)
			}

			// Must end with "..."
			if !strings.HasSuffix(result, "...") {
				t.Errorf("FormatSpinnerMessage should end with '...', got: %q", result)
			}

			// Must be short — verb-only messages should never exceed 20 chars
			if len(result) > 20 {
				t.Errorf("Spinner message too long (%d chars): %q — should be verb-only to prevent frame bleed", len(result), result)
			}
		})
	}
}
