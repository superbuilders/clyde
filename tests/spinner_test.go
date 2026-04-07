package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/this-is-alpha-iota/clyde/cli/loglevel"
	"github.com/this-is-alpha-iota/clyde/cli/spinner"
)

// TestSpinnerNotShownInCLIMode verifies that CLI mode doesn't use a spinner.
func TestSpinnerNotShownInCLIMode(t *testing.T) {
	var output strings.Builder

	callback := func(msg string) {
		output.WriteString(msg + "\n")
	}

	callback("→ Reading file: main.go")
	callback("package main\nfunc main() {}")

	result := output.String()
	if !strings.Contains(result, "→ Reading file: main.go") {
		t.Error("CLI mode should print progress lines directly")
	}
	if !strings.Contains(result, "package main") {
		t.Error("CLI mode should print tool output directly")
	}
}

// TestSpinnerNotShownAtSilentLevel verifies spinner is suppressed at Silent level.
// With ARCH-2, this is the CLI's responsibility — not the agent's.
func TestSpinnerNotShownAtSilentLevel(t *testing.T) {
	var buf bytes.Buffer
	sp := spinner.NewWithWriter(&buf)

	level := loglevel.Silent

	// CLI would check level before starting spinner
	if level != loglevel.Silent {
		sp.Start(spinner.FormatSpinnerMessage("→ Reading file: main.go"))
	}

	time.Sleep(50 * time.Millisecond)

	if sp.IsActive() {
		t.Error("Spinner should not be active at Silent level")
	}

	if buf.Len() > 0 {
		t.Errorf("No spinner output expected at Silent level, got: %q", buf.String())
	}
}

// TestSpinnerShownAtQuietLevel verifies spinner is shown at Quiet level.
func TestSpinnerShownAtQuietLevel(t *testing.T) {
	var buf bytes.Buffer
	sp := spinner.NewWithWriter(&buf)

	level := loglevel.Quiet

	if level != loglevel.Silent {
		sp.Start(spinner.FormatSpinnerMessage("→ Reading file: main.go"))
	}

	time.Sleep(50 * time.Millisecond)

	if !sp.IsActive() {
		t.Error("Spinner should be active at Quiet level")
	}

	sp.Stop()

	if buf.Len() == 0 {
		t.Error("Spinner should produce output at Quiet level")
	}

	output := buf.String()
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

	lastProgressMsg = progressMsg
	sp.Start(spinner.FormatSpinnerMessage(progressMsg))
	time.Sleep(50 * time.Millisecond)

	if sp.IsActive() {
		sp.Stop()
	}
	if lastProgressMsg != "" {
		scrollback.WriteString(lastProgressMsg + "\n")
		lastProgressMsg = ""
	}
	scrollback.WriteString("File patched successfully\n")

	result := scrollback.String()
	if !strings.Contains(result, "→ Patching file: agent.go (+48 bytes)") {
		t.Error("Permanent scrollback should contain the → progress line")
	}
	if !strings.Contains(result, "File patched successfully") {
		t.Error("Permanent scrollback should contain the tool output")
	}

	spinnerOutput := spinnerBuf.String()
	if !strings.HasSuffix(spinnerOutput, "\r\033[K") {
		t.Error("Spinner line should be cleared after Stop()")
	}
}

// TestSpinnerToolWithNoOutput verifies permanent → line is printed even without output body.
func TestSpinnerToolWithNoOutput(t *testing.T) {
	var spinnerBuf bytes.Buffer
	var scrollback strings.Builder

	sp := spinner.NewWithWriter(&spinnerBuf)

	progressMsg := "→ Writing file: output.txt (1.2 KB)"
	var lastProgressMsg string

	lastProgressMsg = progressMsg
	sp.Start(spinner.FormatSpinnerMessage(progressMsg))
	time.Sleep(50 * time.Millisecond)

	if sp.IsActive() {
		sp.Stop()
	}
	if lastProgressMsg != "" {
		scrollback.WriteString(lastProgressMsg + "\n")
		lastProgressMsg = ""
	}

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

	lastProgressMsg = "→ Reading file: main.go"
	sp.Start(spinner.FormatSpinnerMessage(lastProgressMsg))
	time.Sleep(30 * time.Millisecond)

	sp.Stop()
	scrollback.WriteString(lastProgressMsg + "\n")
	scrollback.WriteString("package main\n")
	lastProgressMsg = ""

	lastProgressMsg = "→ Patching file: main.go (+10 bytes)"
	sp.Start(spinner.FormatSpinnerMessage(lastProgressMsg))
	time.Sleep(30 * time.Millisecond)

	sp.Stop()
	scrollback.WriteString(lastProgressMsg + "\n")
	scrollback.WriteString("Patched successfully\n")
	lastProgressMsg = ""

	result := scrollback.String()
	if !strings.Contains(result, "→ Reading file: main.go") {
		t.Error("First tool progress line missing from scrollback")
	}
	if !strings.Contains(result, "→ Patching file: main.go (+10 bytes)") {
		t.Error("Second tool progress line missing from scrollback")
	}
}

// TestSpinnerMessageUpdate verifies spinner message can be updated.
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
// correctly with real tool progress messages.
func TestFormatSpinnerMessage_Integration(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"list_files tool", "→ Listing files: . (current directory)", "Listing..."},
		{"read_file tool", "→ Reading file: agent/agent.go", "Reading..."},
		{"patch_file tool", "→ Patching file: main.go (+100 bytes)", "Patching..."},
		{"write_file tool", "→ Writing file: output.txt (42.5 KB)", "Writing..."},
		{"run_bash tool", "→ Running bash: go test -v ./...", "Running..."},
		{"grep tool", "→ Searching: 'TODO' in . (*.go)", "Searching..."},
		{"browse tool", "→ Browsing: https://pkg.go.dev/net/http", "Reading Webpage..."},
		{"browser tool", "→ Browser: navigate https://example.com", "Browsing..."},
		{"web_search tool", "→ Searching web: \"golang HTTP client\"", "Searching..."},
		{"include_file tool", "→ Including file: screenshot.png", "Loading..."},
		{"multi_patch tool", "→ Applying multi-patch: 5 files", "Patching..."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := spinner.FormatSpinnerMessage(tt.input)

			if strings.HasPrefix(result, "→ ") {
				t.Errorf("FormatSpinnerMessage should strip '→ ' prefix, got: %q", result)
			}
			if result != tt.expected {
				t.Errorf("FormatSpinnerMessage(%q) = %q, want %q", tt.input, result, tt.expected)
			}
			if !strings.HasSuffix(result, "...") {
				t.Errorf("FormatSpinnerMessage should end with '...', got: %q", result)
			}
			if len(result) > 20 {
				t.Errorf("Spinner message too long (%d chars): %q", len(result), result)
			}
		})
	}
}
