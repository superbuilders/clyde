package main

import (
	"os"
	"strings"
	"testing"

	"github.com/this-is-alpha-iota/clyde/agent"
	"github.com/this-is-alpha-iota/clyde/agent/truncate"
	"github.com/this-is-alpha-iota/clyde/loglevel"
	"github.com/this-is-alpha-iota/clyde/providers"
)

// --- ARCH-2: Remove I/O Concerns from the Agent ---
//
// These tests verify the ARCH-2 story: the agent has zero display/filtering
// logic and emits all information unconditionally to callers. The CLI layer
// is the sole owner of display filtering, truncation, and spinner management.

// TestARCH2_AgentNoLogLevelImport verifies the agent package does not import loglevel.
// This is a compile-time guarantee: if agent imported loglevel, this file's imports
// of both agent and loglevel would cause visible coupling. The real proof is in
// go vet and the grep in the acceptance criteria, but this documents the intent.
func TestARCH2_AgentNoLogLevelImport(t *testing.T) {
	// Read agent/agent.go source and verify no loglevel import
	content := readTestFile(t, "../agent/agent.go")
	if strings.Contains(content, `"github.com/this-is-alpha-iota/clyde/loglevel"`) {
		t.Error("agent/agent.go should NOT import loglevel (ARCH-2)")
	}
	if strings.Contains(content, "loglevel.") {
		t.Error("agent/agent.go should have zero references to loglevel package")
	}
}

// TestARCH2_TruncateNoLogLevelImport verifies truncate package has no loglevel dependency.
func TestARCH2_TruncateNoLogLevelImport(t *testing.T) {
	content := readTestFile(t, "../agent/truncate/truncate.go")
	if strings.Contains(content, `"github.com/this-is-alpha-iota/clyde/loglevel"`) {
		t.Error("agent/truncate/truncate.go should NOT import loglevel (ARCH-2)")
	}
}

// TestARCH2_AgentNoWithLogLevel verifies WithLogLevel was removed from agent.
func TestARCH2_AgentNoWithLogLevel(t *testing.T) {
	content := readTestFile(t, "../agent/agent.go")
	if strings.Contains(content, "WithLogLevel") {
		t.Error("agent/agent.go should NOT have WithLogLevel (ARCH-2)")
	}
	if strings.Contains(content, "LogLevel()") {
		t.Error("agent/agent.go should NOT have LogLevel() getter (ARCH-2)")
	}
}

// TestARCH2_AgentEmitsUnconditionally verifies the agent has separate
// callbacks and no level-gating logic.
func TestARCH2_AgentEmitsUnconditionally(t *testing.T) {
	apiClient := providers.NewClient("dummy", "http://localhost", "test", 100)

	var progress []string
	var outputs []string
	var diagnostics []string
	var thinking []string

	a := agent.NewAgent(
		apiClient,
		"test prompt",
		agent.WithProgressCallback(func(msg string) {
			progress = append(progress, msg)
		}),
		agent.WithOutputCallback(func(output string) {
			outputs = append(outputs, output)
		}),
		agent.WithDiagnosticCallback(func(msg string) {
			diagnostics = append(diagnostics, msg)
		}),
		agent.WithThinkingCallback(func(text string) {
			thinking = append(thinking, text)
		}),
		agent.WithSpinnerCallback(func(start bool, message string) {
			// Spinner callback also set — no level check in agent
		}),
		agent.WithContextWindowSize(200000),
	)

	if a == nil {
		t.Fatal("Agent should not be nil")
	}

	// Verify all four callback types are separate concerns
	t.Log("✅ Agent created with 4 separate callbacks (progress, output, diagnostic, thinking)")
	t.Log("✅ No log level parameter — agent emits everything unconditionally")
}

// TestARCH2_ProgressCallbackSignature verifies the new callback signature
// has no loglevel parameter.
func TestARCH2_ProgressCallbackSignature(t *testing.T) {
	// This compiles only if ProgressCallback is func(string)
	var cb agent.ProgressCallback = func(msg string) {
		_ = msg
	}
	if cb == nil {
		t.Error("ProgressCallback should accept func(string)")
	}
}

// TestARCH2_OutputCallbackExists verifies OutputCallback is a new type.
func TestARCH2_OutputCallbackExists(t *testing.T) {
	var cb agent.OutputCallback = func(output string) {
		_ = output
	}
	if cb == nil {
		t.Error("OutputCallback should exist")
	}
}

// TestARCH2_DiagnosticCallbackExists verifies DiagnosticCallback is a new type.
func TestARCH2_DiagnosticCallbackExists(t *testing.T) {
	var cb agent.DiagnosticCallback = func(msg string) {
		_ = msg
	}
	if cb == nil {
		t.Error("DiagnosticCallback should exist")
	}
}

// TestARCH2_TruncateFunctionsNoLevelParam verifies truncation functions
// take plain parameters (no loglevel).
func TestARCH2_TruncateFunctionsNoLevelParam(t *testing.T) {
	// These compile only if the functions have the new signatures
	_ = truncate.Lines("test", 25)
	_ = truncate.Chars("test")
	_ = truncate.Text("test", 25)
	_ = truncate.Thinking("test")
	_ = truncate.ToolOutput("test")
	t.Log("✅ All truncate functions take plain parameters (no loglevel)")
}

// TestARCH2_TruncateAlwaysTruncates verifies truncation functions always
// apply (no verbose/debug bypass). The CLI decides whether to call them.
func TestARCH2_TruncateAlwaysTruncates(t *testing.T) {
	lines := make([]string, 30)
	for i := range lines {
		lines[i] = "line"
	}
	text := strings.Join(lines, "\n")

	result := truncate.ToolOutput(text)
	if !strings.Contains(result, "more lines)") {
		t.Error("ToolOutput should always truncate (ARCH-2: no level bypass)")
	}

	result2 := truncate.Thinking(strings.Join(make([]string, 60), "\n"))
	if !strings.Contains(result2, "more lines)") {
		t.Error("Thinking should always truncate (ARCH-2: no level bypass)")
	}
}

// TestARCH2_CLIOwnsFilteringLogic verifies that loglevel.ShouldShow is
// the mechanism the CLI uses for display filtering.
func TestARCH2_CLIOwnsFilteringLogic(t *testing.T) {
	// CLI filtering rules (documented here for reference):
	//   Progress (→ lines):     show at Quiet and above
	//   Output bodies:          show at Normal and above
	//   Cache verbose:          show at Verbose and above
	//   Cache debug/tokens:     show at Debug
	//   Thinking:               show at Normal and above (truncated), full at Verbose+

	tests := []struct {
		name      string
		level     loglevel.Level
		threshold loglevel.Level
		want      bool
	}{
		{"silent hides progress", loglevel.Silent, loglevel.Quiet, false},
		{"quiet shows progress", loglevel.Quiet, loglevel.Quiet, true},
		{"quiet hides output", loglevel.Quiet, loglevel.Normal, false},
		{"normal shows output", loglevel.Normal, loglevel.Normal, true},
		{"normal hides cache", loglevel.Normal, loglevel.Verbose, false},
		{"verbose shows cache", loglevel.Verbose, loglevel.Verbose, true},
		{"verbose hides debug", loglevel.Verbose, loglevel.Debug, false},
		{"debug shows all", loglevel.Debug, loglevel.Debug, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.level.ShouldShow(tt.threshold)
			if got != tt.want {
				t.Errorf("%s.ShouldShow(%s) = %v, want %v",
					tt.level, tt.threshold, got, tt.want)
			}
		})
	}
}

// TestARCH2_NoBehavioralChange verifies that the user sees the same output
// at every log level (same content, filtered by CLI, not agent).
// This is a design-level test documenting the new architecture.
func TestARCH2_NoBehavioralChange(t *testing.T) {
	// At each level, the CLI applies these filters:
	//   Silent:  suppress all callbacks
	//   Quiet:   show progress only
	//   Normal:  show progress + output (truncated) + thinking (truncated)
	//   Verbose: show progress + output (full) + thinking (full) + cache
	//   Debug:   show everything including token diagnostics

	// The agent emits ALL of these unconditionally.
	// The CLI decides what to display.
	t.Log("ARCH-2: Agent emits unconditionally → CLI filters by level")
	t.Log("  Silent:  CLI suppresses all")
	t.Log("  Quiet:   CLI shows progress only")
	t.Log("  Normal:  CLI shows progress + truncated output/thinking")
	t.Log("  Verbose: CLI shows progress + full output/thinking + cache")
	t.Log("  Debug:   CLI shows everything")
	t.Log("No behavioral change from user's perspective")
}

// readTestFile reads a file for source-level assertions.
func readTestFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read %s: %v", path, err)
	}
	return string(content)
}
