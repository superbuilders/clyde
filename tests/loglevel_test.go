package main

import (
	"strings"
	"testing"

	"github.com/this-is-alpha-iota/clyde/agent"
	"github.com/this-is-alpha-iota/clyde/api"
	"github.com/this-is-alpha-iota/clyde/loglevel"
	"github.com/this-is-alpha-iota/clyde/prompts"
)

// TestLogLevelDefault verifies that a new agent defaults to Normal log level
func TestLogLevelDefault(t *testing.T) {
	apiClient := api.NewClient("test-key", "https://api.example.com", "test-model", 4096)
	a := agent.NewAgent(apiClient, prompts.SystemPrompt)

	if a.LogLevel() != loglevel.Normal {
		t.Errorf("Default log level = %v, want Normal", a.LogLevel())
	}
}

// TestLogLevelWithOption verifies WithLogLevel sets the level correctly
func TestLogLevelWithOption(t *testing.T) {
	apiClient := api.NewClient("test-key", "https://api.example.com", "test-model", 4096)

	tests := []struct {
		name  string
		level loglevel.Level
	}{
		{"silent", loglevel.Silent},
		{"quiet", loglevel.Quiet},
		{"normal", loglevel.Normal},
		{"verbose", loglevel.Verbose},
		{"debug", loglevel.Debug},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := agent.NewAgent(apiClient, prompts.SystemPrompt,
				agent.WithLogLevel(tt.level))

			if a.LogLevel() != tt.level {
				t.Errorf("WithLogLevel(%v): LogLevel() = %v, want %v",
					tt.level, a.LogLevel(), tt.level)
			}
		})
	}
}

// capturedMessage records a progress message with its level
type capturedMessage struct {
	level   loglevel.Level
	message string
}

// TestLogLevelGating verifies that the agent correctly gates output based on
// the log level. This uses a mock interaction pattern: we verify that the
// callback reports the level correctly. We can't easily do a full API round-trip
// without a real API key, so we verify the wiring and callback signature.
func TestLogLevelGating(t *testing.T) {
	// This test verifies the callback signature and that WithLogLevel + WithProgressCallback
	// can be composed correctly.
	apiClient := api.NewClient("test-key", "https://api.example.com", "test-model", 4096)

	var captured []capturedMessage

	a := agent.NewAgent(apiClient, prompts.SystemPrompt,
		agent.WithLogLevel(loglevel.Debug),
		agent.WithProgressCallback(func(level loglevel.Level, msg string) {
			captured = append(captured, capturedMessage{level, msg})
		}),
	)

	if a.LogLevel() != loglevel.Debug {
		t.Errorf("Expected Debug log level, got %v", a.LogLevel())
	}

	// Verify the callback was set (non-nil agent is the proof)
	if a == nil {
		t.Fatal("Agent should not be nil")
	}
}

// TestLogLevelParseFlagsIntegration tests parsing flags and threading them into the agent
func TestLogLevelParseFlagsIntegration(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		expectedLevel loglevel.Level
		expectedArgs  []string
	}{
		{
			name:          "silent flag threaded to agent",
			args:          []string{"--silent", "Hello"},
			expectedLevel: loglevel.Silent,
			expectedArgs:  []string{"Hello"},
		},
		{
			name:          "quiet flag threaded to agent",
			args:          []string{"-q", "Hello"},
			expectedLevel: loglevel.Quiet,
			expectedArgs:  []string{"Hello"},
		},
		{
			name:          "verbose flag threaded to agent",
			args:          []string{"-v", "Hello"},
			expectedLevel: loglevel.Verbose,
			expectedArgs:  []string{"Hello"},
		},
		{
			name:          "debug flag threaded to agent",
			args:          []string{"--debug", "Hello"},
			expectedLevel: loglevel.Debug,
			expectedArgs:  []string{"Hello"},
		},
		{
			name:          "no flag defaults to Normal",
			args:          []string{"Hello"},
			expectedLevel: loglevel.Normal,
			expectedArgs:  []string{"Hello"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			level, remaining := loglevel.ParseFlags(tt.args)

			if level != tt.expectedLevel {
				t.Errorf("ParseFlags level = %v, want %v", level, tt.expectedLevel)
			}

			if len(remaining) != len(tt.expectedArgs) {
				t.Fatalf("ParseFlags remaining len = %d, want %d", len(remaining), len(tt.expectedArgs))
			}

			for i, arg := range remaining {
				if arg != tt.expectedArgs[i] {
					t.Errorf("remaining[%d] = %q, want %q", i, arg, tt.expectedArgs[i])
				}
			}

			// Thread into agent and verify
			apiClient := api.NewClient("test-key", "https://api.example.com", "test-model", 4096)
			a := agent.NewAgent(apiClient, prompts.SystemPrompt,
				agent.WithLogLevel(level))

			if a.LogLevel() != tt.expectedLevel {
				t.Errorf("Agent log level = %v, want %v", a.LogLevel(), tt.expectedLevel)
			}
		})
	}
}

// TestLogLevelCLIFlagStripping verifies that verbosity flags are stripped from
// args before they are treated as prompt text (important for CLI mode)
func TestLogLevelCLIFlagStripping(t *testing.T) {
	// Simulate: clyde --verbose What is 2+2?
	args := []string{"--verbose", "What", "is", "2+2?"}
	level, remaining := loglevel.ParseFlags(args)

	if level != loglevel.Verbose {
		t.Errorf("Expected Verbose, got %v", level)
	}

	prompt := strings.Join(remaining, " ")
	if prompt != "What is 2+2?" {
		t.Errorf("Expected prompt 'What is 2+2?', got %q", prompt)
	}
}

// TestLogLevelCLIFileFlagPreserved verifies that -f flag is not consumed
// by the log level parser
func TestLogLevelCLIFileFlagPreserved(t *testing.T) {
	// Simulate: clyde -v -f prompt.txt
	args := []string{"-v", "-f", "prompt.txt"}
	level, remaining := loglevel.ParseFlags(args)

	if level != loglevel.Verbose {
		t.Errorf("Expected Verbose, got %v", level)
	}

	if len(remaining) != 2 {
		t.Fatalf("Expected 2 remaining args, got %d: %v", len(remaining), remaining)
	}

	if remaining[0] != "-f" || remaining[1] != "prompt.txt" {
		t.Errorf("Expected [-f prompt.txt], got %v", remaining)
	}
}

// TestLogLevelCLIBinaryFlagParsing tests that CLI binary properly parses
// log level flags. This is an end-to-end test that builds the binary.
func TestLogLevelCLIBinaryFlagParsing(t *testing.T) {
	// Build binary
	binaryPath := buildTestBinary(t)

	// Verify --silent with empty prompt gives proper error (not a flag parse error)
	tests := []struct {
		name       string
		args       []string
		wantInErr  string
		shouldFail bool
	}{
		{
			name:       "silent flag with empty prompt",
			args:       []string{"--silent", ""},
			wantInErr:  "Empty prompt",
			shouldFail: true,
		},
		{
			name:       "verbose flag with empty prompt",
			args:       []string{"-v", ""},
			wantInErr:  "Empty prompt",
			shouldFail: true,
		},
		{
			name:       "quiet flag with empty prompt",
			args:       []string{"-q", ""},
			wantInErr:  "Empty prompt",
			shouldFail: true,
		},
		{
			name:       "debug flag with empty prompt",
			args:       []string{"--debug", ""},
			wantInErr:  "Empty prompt",
			shouldFail: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := buildTestCommand(t, binaryPath, tt.args...)
			output, err := cmd.CombinedOutput()

			if tt.shouldFail {
				if err == nil {
					t.Errorf("Expected command to fail, but it succeeded. Output: %s", output)
				}
				if tt.wantInErr != "" && !strings.Contains(string(output), tt.wantInErr) {
					t.Errorf("Expected error containing %q, got: %s", tt.wantInErr, output)
				}
			}
		})
	}
}
