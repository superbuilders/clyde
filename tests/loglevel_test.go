package main

import (
	"strings"
	"testing"

	"github.com/this-is-alpha-iota/clyde/agent"
	"github.com/this-is-alpha-iota/clyde/agent/providers"
	"github.com/this-is-alpha-iota/clyde/cli/loglevel"
	"github.com/this-is-alpha-iota/clyde/agent/prompts"
)

// TestLogLevelDefault verifies that a new agent can be created without log level (ARCH-2).
func TestLogLevelDefault(t *testing.T) {
	apiClient := providers.NewClient("test-key", "https://api.example.com", "test-model", 4096)
	a := agent.NewAgent(apiClient, prompts.SystemPrompt)
	if a == nil {
		t.Error("Agent should not be nil")
	}
}

// TestLogLevelCallbackWiring verifies that callbacks can be composed on the agent.
// With ARCH-2, the agent has no log level — it emits everything.
// The CLI filters using its own log level.
func TestLogLevelCallbackWiring(t *testing.T) {
	apiClient := providers.NewClient("test-key", "https://api.example.com", "test-model", 4096)

	var captured []string

	a := agent.NewAgent(apiClient, prompts.SystemPrompt,
		agent.WithProgressCallback(func(msg string) {
			captured = append(captured, msg)
		}),
	)

	if a == nil {
		t.Fatal("Agent should not be nil")
	}
}

// TestLogLevelParseFlagsIntegration tests parsing flags (loglevel is still used by CLI).
func TestLogLevelParseFlagsIntegration(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		expectedLevel loglevel.Level
		expectedArgs  []string
	}{
		{
			name:          "silent flag",
			args:          []string{"--silent", "Hello"},
			expectedLevel: loglevel.Silent,
			expectedArgs:  []string{"Hello"},
		},
		{
			name:          "quiet flag",
			args:          []string{"-q", "Hello"},
			expectedLevel: loglevel.Quiet,
			expectedArgs:  []string{"Hello"},
		},
		{
			name:          "verbose flag",
			args:          []string{"-v", "Hello"},
			expectedLevel: loglevel.Verbose,
			expectedArgs:  []string{"Hello"},
		},
		{
			name:          "debug flag",
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
		})
	}
}

// TestLogLevelCLIFlagStripping verifies that verbosity flags are stripped from args.
func TestLogLevelCLIFlagStripping(t *testing.T) {
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

// TestLogLevelCLIFileFlagPreserved verifies that -f flag is not consumed by log level parser.
func TestLogLevelCLIFileFlagPreserved(t *testing.T) {
	args := []string{"-v", "-f", "prompt.txt"}
	level, remaining := loglevel.ParseFlags(args)

	if level != loglevel.Verbose {
		t.Errorf("Expected Verbose, got %v", level)
	}

	if len(remaining) != 2 || remaining[0] != "-f" || remaining[1] != "prompt.txt" {
		t.Errorf("Expected [-f prompt.txt], got %v", remaining)
	}
}

// TestLogLevelCLIBinaryFlagParsing tests that CLI binary properly parses log level flags.
func TestLogLevelCLIBinaryFlagParsing(t *testing.T) {
	binaryPath := buildTestBinary(t)

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
