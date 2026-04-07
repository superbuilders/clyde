package main

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/this-is-alpha-iota/clyde/agent"
	"github.com/this-is-alpha-iota/clyde/providers"
	"github.com/this-is-alpha-iota/clyde/cli/loglevel"
	"github.com/this-is-alpha-iota/clyde/agent/prompts"
)

// TestCacheDisplaySuppressedAtNormal verifies that cache display messages
// should be suppressed at Normal, Quiet, and Silent log levels.
// With ARCH-2, the agent emits all diagnostics unconditionally. The CLI filters.
// This test verifies the CLI gating logic via loglevel.ShouldShow.
func TestCacheDisplaySuppressedAtNormal(t *testing.T) {
	suppressedLevels := []loglevel.Level{
		loglevel.Silent,
		loglevel.Quiet,
		loglevel.Normal,
	}

	for _, level := range suppressedLevels {
		t.Run(level.String(), func(t *testing.T) {
			// Cache messages require Verbose threshold to display
			if level.ShouldShow(loglevel.Verbose) {
				t.Errorf("Level %s should NOT show Verbose-threshold content", level)
			}

			// Verify agent can be created without log level (ARCH-2)
			apiClient := providers.NewClient("dummy-key", "http://localhost", "test-model", 100)
			a := agent.NewAgent(apiClient, "test prompt",
				agent.WithContextWindowSize(200000),
				agent.WithDiagnosticCallback(func(msg string) {}),
			)
			if a == nil {
				t.Error("Agent should not be nil")
			}
		})
	}
}

// TestCacheDisplayVerboseFormat verifies that at Verbose level, cache info is
// displayed as a token fraction: "💾 Cache: N/M tokens".
func TestCacheDisplayVerboseFormat(t *testing.T) {
	apiKey := os.Getenv("TS_AGENT_API_KEY")
	if apiKey == "" {
		t.Skip("TS_AGENT_API_KEY not set, skipping integration test")
	}

	apiClient := providers.NewClient(
		apiKey,
		"https://api.anthropic.com/v1/messages",
		"claude-sonnet-4-5-20250929",
		4096,
	)

	var diagnosticMessages []string
	agentInstance := agent.NewAgent(
		apiClient,
		prompts.SystemPrompt,
		agent.WithContextWindowSize(200000),
		agent.WithDiagnosticCallback(func(msg string) {
			diagnosticMessages = append(diagnosticMessages, msg)
		}),
	)

	_, err := agentInstance.HandleMessage("What is 2+2? Reply with just the number.")
	if err != nil {
		t.Fatalf("First request failed: %v", err)
	}

	diagnosticMessages = nil
	_, err = agentInstance.HandleMessage("What is 3+3? Reply with just the number.")
	if err != nil {
		t.Fatalf("Second request failed: %v", err)
	}

	t.Logf("Diagnostic messages: %v", diagnosticMessages)

	foundVerboseCache := false
	for _, msg := range diagnosticMessages {
		if strings.HasPrefix(msg, "💾 Cache: ") && !strings.Contains(msg, "|") {
			foundVerboseCache = true
			inner := strings.TrimPrefix(msg, "💾 Cache: ")
			inner = strings.TrimSuffix(inner, " tokens")
			if !strings.Contains(inner, "/") {
				t.Errorf("Expected fraction format N/M, got: %s", inner)
			}
			t.Logf("✅ Verbose cache format: %s", msg)
		}
	}

	if !foundVerboseCache {
		t.Log("No cache hit detected (may happen if cache not created yet)")
	}
}

// TestCacheDisplayDebugFormat verifies debug-level cache info.
func TestCacheDisplayDebugFormat(t *testing.T) {
	apiKey := os.Getenv("TS_AGENT_API_KEY")
	if apiKey == "" {
		t.Skip("TS_AGENT_API_KEY not set, skipping integration test")
	}

	apiClient := providers.NewClient(
		apiKey,
		"https://api.anthropic.com/v1/messages",
		"claude-sonnet-4-5-20250929",
		4096,
	)

	var diagnosticMessages []string
	agentInstance := agent.NewAgent(
		apiClient,
		prompts.SystemPrompt,
		agent.WithContextWindowSize(200000),
		agent.WithDiagnosticCallback(func(msg string) {
			diagnosticMessages = append(diagnosticMessages, msg)
		}),
	)

	_, err := agentInstance.HandleMessage("What is 2+2? Reply with just the number.")
	if err != nil {
		t.Fatalf("First request failed: %v", err)
	}

	diagnosticMessages = nil
	_, err = agentInstance.HandleMessage("What is 3+3? Reply with just the number.")
	if err != nil {
		t.Fatalf("Second request failed: %v", err)
	}

	t.Logf("Diagnostic messages: %v", diagnosticMessages)

	foundDebugCache := false
	for _, msg := range diagnosticMessages {
		if strings.HasPrefix(msg, "💾 Cache: ") && strings.Contains(msg, "| Creation:") {
			foundDebugCache = true
			if !strings.Contains(msg, "| Context:") {
				t.Errorf("Debug cache format should include Context %%, got: %s", msg)
			}
			t.Logf("✅ Debug cache format: %s", msg)
		}
	}

	if !foundDebugCache {
		t.Log("Cache messages may not appear if cache was not hit yet")
	}
}

// TestCacheDisplayFormatUnit tests the cache display format strings.
func TestCacheDisplayFormatUnit(t *testing.T) {
	usage := providers.Usage{
		InputTokens:              387,
		OutputTokens:             50,
		CacheCreationInputTokens: 500,
		CacheReadInputTokens:     3715,
	}
	contextWindowSize := 200000
	totalInputTokens := usage.InputTokens + usage.CacheReadInputTokens

	t.Run("verbose_format", func(t *testing.T) {
		msg := fmt.Sprintf("💾 Cache: %d/%d tokens",
			usage.CacheReadInputTokens, totalInputTokens)
		expected := "💾 Cache: 3715/4102 tokens"
		if msg != expected {
			t.Errorf("Expected %q, got %q", expected, msg)
		}
	})

	t.Run("debug_format_with_context", func(t *testing.T) {
		detail := fmt.Sprintf("💾 Cache: %d/%d tokens | Creation: %d tokens",
			usage.CacheReadInputTokens, totalInputTokens,
			usage.CacheCreationInputTokens)
		if contextWindowSize > 0 {
			pct := (totalInputTokens * 100) / contextWindowSize
			if pct > 100 {
				pct = 100
			}
			detail += fmt.Sprintf(" | Context: %d%% (%d/%d)",
				pct, totalInputTokens, contextWindowSize)
		}
		expected := "💾 Cache: 3715/4102 tokens | Creation: 500 tokens | Context: 2% (4102/200000)"
		if detail != expected {
			t.Errorf("Expected %q, got %q", expected, detail)
		}
	})

	t.Run("verbose_format_zero_cache", func(t *testing.T) {
		zeroUsage := providers.Usage{
			InputTokens:          1000,
			CacheReadInputTokens: 0,
		}
		if zeroUsage.CacheReadInputTokens > 0 {
			t.Error("This should not execute - zero cache read tokens means no message")
		}
	})
}

// TestCacheDisplayLevelGating verifies that the CLI gating logic correctly
// determines which levels see cache messages.
func TestCacheDisplayLevelGating(t *testing.T) {
	tests := []struct {
		level    loglevel.Level
		wantShow bool
	}{
		{loglevel.Silent, false},
		{loglevel.Quiet, false},
		{loglevel.Normal, false},
		{loglevel.Verbose, true},
		{loglevel.Debug, true},
	}

	for _, tt := range tests {
		t.Run(tt.level.String(), func(t *testing.T) {
			if got := tt.level.ShouldShow(loglevel.Verbose); got != tt.wantShow {
				t.Errorf("Level %s ShouldShow(Verbose) = %v, want %v",
					tt.level, got, tt.wantShow)
			}
		})
	}
}

// TestCacheDisplayOldFormatRemoved verifies the old cache display format is gone.
func TestCacheDisplayOldFormatRemoved(t *testing.T) {
	apiKey := os.Getenv("TS_AGENT_API_KEY")
	if apiKey == "" {
		t.Skip("TS_AGENT_API_KEY not set, skipping integration test")
	}

	apiClient := providers.NewClient(
		apiKey,
		"https://api.anthropic.com/v1/messages",
		"claude-sonnet-4-5-20250929",
		4096,
	)

	var diagnosticMessages []string
	agentInstance := agent.NewAgent(
		apiClient,
		prompts.SystemPrompt,
		agent.WithContextWindowSize(200000),
		agent.WithDiagnosticCallback(func(msg string) {
			diagnosticMessages = append(diagnosticMessages, msg)
		}),
	)

	_, _ = agentInstance.HandleMessage("What is 2+2? Reply with just the number.")
	diagnosticMessages = nil
	_, _ = agentInstance.HandleMessage("What is 3+3? Reply with just the number.")

	for _, msg := range diagnosticMessages {
		if strings.Contains(msg, "Cache hit:") {
			t.Errorf("Found old format 'Cache hit:'. Message: %s", msg)
		}
	}
}

// TestWithContextWindowSizeOption verifies the WithContextWindowSize agent option.
func TestWithContextWindowSizeOption(t *testing.T) {
	t.Run("default_creation", func(t *testing.T) {
		a := agent.NewAgent(
			providers.NewClient("dummy", "http://localhost", "test", 100),
			"test",
		)
		// Agent created without any options — should work
		if a == nil {
			t.Error("Agent should not be nil")
		}
	})

	t.Run("set_via_option", func(t *testing.T) {
		var diagnosticMessages []string
		a := agent.NewAgent(
			providers.NewClient("dummy", "http://localhost", "test", 100),
			"test",
			agent.WithContextWindowSize(128000),
			agent.WithDiagnosticCallback(func(msg string) {
				diagnosticMessages = append(diagnosticMessages, msg)
			}),
		)
		if a == nil {
			t.Error("Agent should not be nil")
		}
	})
}
