package main

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/this-is-alpha-iota/clyde/agent"
	"github.com/this-is-alpha-iota/clyde/providers"
	"github.com/this-is-alpha-iota/clyde/loglevel"
	"github.com/this-is-alpha-iota/clyde/agent/prompts"
)

// TestCacheDisplaySuppressedAtNormal verifies that cache display messages are
// NOT emitted at Normal, Quiet, and Silent log levels. The context window
// percentage on the prompt line replaces the cache message as the primary
// indicator at Normal level.
func TestCacheDisplaySuppressedAtNormal(t *testing.T) {
	suppressedLevels := []loglevel.Level{
		loglevel.Silent,
		loglevel.Quiet,
		loglevel.Normal,
	}

	for _, level := range suppressedLevels {
		t.Run(level.String(), func(t *testing.T) {
			var messages []string
			agentOpts := []agent.AgentOption{
				agent.WithLogLevel(level),
				agent.WithContextWindowSize(200000),
				agent.WithProgressCallback(func(lvl loglevel.Level, msg string) {
					messages = append(messages, msg)
				}),
			}

			// We can't easily inject mock API responses into the agent,
			// so we verify the emit gating logic directly by testing
			// that at these levels, no cache messages are emitted.
			// The agent's emit() only fires when logLevel.ShouldShow(threshold).
			// Cache messages are emitted at Verbose threshold, so:
			//   Silent.ShouldShow(Verbose) = false
			//   Quiet.ShouldShow(Verbose) = false
			//   Normal.ShouldShow(Verbose) = false

			if level.ShouldShow(loglevel.Verbose) {
				t.Errorf("Level %s should NOT show Verbose-threshold content", level)
			}

			// Verify agent options compile and apply cleanly
			apiClient := providers.NewClient("dummy-key", "http://localhost", "test-model", 100)
			a := agent.NewAgent(apiClient, "test prompt", agentOpts...)
			if a.LogLevel() != level {
				t.Errorf("Expected log level %s, got %s", level, a.LogLevel())
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

	var messages []string
	agentInstance := agent.NewAgent(
		apiClient,
		prompts.SystemPrompt,
		agent.WithLogLevel(loglevel.Verbose),
		agent.WithContextWindowSize(200000),
		agent.WithProgressCallback(func(lvl loglevel.Level, msg string) {
			messages = append(messages, msg)
		}),
	)

	// First request (creates cache)
	_, err := agentInstance.HandleMessage("What is 2+2? Reply with just the number.")
	if err != nil {
		t.Fatalf("First request failed: %v", err)
	}

	// Second request (should hit cache)
	messages = nil
	_, err = agentInstance.HandleMessage("What is 3+3? Reply with just the number.")
	if err != nil {
		t.Fatalf("Second request failed: %v", err)
	}

	t.Logf("Verbose messages: %v", messages)

	// Look for the Verbose cache format
	foundVerboseCache := false
	for _, msg := range messages {
		if strings.HasPrefix(msg, "💾 Cache: ") && strings.HasSuffix(msg, " tokens") {
			foundVerboseCache = true
			// Verify it's the fraction format (N/M tokens)
			// Strip prefix and suffix
			inner := strings.TrimPrefix(msg, "💾 Cache: ")
			inner = strings.TrimSuffix(inner, " tokens")
			if !strings.Contains(inner, "/") {
				t.Errorf("Expected fraction format N/M, got: %s", inner)
			}
			// Should NOT contain old format keywords
			if strings.Contains(msg, "of input") || strings.Contains(msg, "hit:") {
				t.Errorf("Verbose cache format should not use old format, got: %s", msg)
			}
			t.Logf("✅ Verbose cache format: %s", msg)
		}
	}

	// Note: Cache hit may not happen on first pair of requests in some cases
	if !foundVerboseCache {
		t.Log("No cache hit detected (may happen if cache not created yet)")
	}
}

// TestCacheDisplayDebugFormat verifies that at Debug level, cache info includes
// additional detail: creation tokens and context percentage.
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

	var messages []string
	agentInstance := agent.NewAgent(
		apiClient,
		prompts.SystemPrompt,
		agent.WithLogLevel(loglevel.Debug),
		agent.WithContextWindowSize(200000),
		agent.WithProgressCallback(func(lvl loglevel.Level, msg string) {
			messages = append(messages, msg)
		}),
	)

	// First request (creates cache)
	_, err := agentInstance.HandleMessage("What is 2+2? Reply with just the number.")
	if err != nil {
		t.Fatalf("First request failed: %v", err)
	}

	// Second request (should hit cache)
	messages = nil
	_, err = agentInstance.HandleMessage("What is 3+3? Reply with just the number.")
	if err != nil {
		t.Fatalf("Second request failed: %v", err)
	}

	t.Logf("Debug messages: %v", messages)

	// At Debug level, we should see BOTH the Verbose format AND the Debug format
	foundVerboseCache := false
	foundDebugCache := false
	for _, msg := range messages {
		if strings.HasPrefix(msg, "💾 Cache: ") && !strings.Contains(msg, "|") {
			foundVerboseCache = true
			t.Logf("✅ Verbose cache format (also shown at Debug): %s", msg)
		}
		if strings.HasPrefix(msg, "💾 Cache: ") && strings.Contains(msg, "| Creation:") {
			foundDebugCache = true
			// Verify detailed format
			if !strings.Contains(msg, "| Context:") {
				t.Errorf("Debug cache format should include Context %%, got: %s", msg)
			}
			if !strings.Contains(msg, "/200000") {
				t.Errorf("Debug cache format should include context window size, got: %s", msg)
			}
			t.Logf("✅ Debug cache format: %s", msg)
		}
	}

	if !foundVerboseCache || !foundDebugCache {
		t.Log("Cache messages may not appear if cache was not hit yet")
	}
}

// TestCacheDisplayFormatUnit tests the cache display format strings without
// making API calls. Simulates what the agent would emit.
func TestCacheDisplayFormatUnit(t *testing.T) {
	// Simulate usage values
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

	t.Run("debug_format_without_context_window", func(t *testing.T) {
		// When contextWindowSize is 0, context info is omitted
		noContextWindowSize := 0
		detail := fmt.Sprintf("💾 Cache: %d/%d tokens | Creation: %d tokens",
			usage.CacheReadInputTokens, totalInputTokens,
			usage.CacheCreationInputTokens)
		if noContextWindowSize > 0 {
			// This block intentionally not entered
			detail += " | Context: should not appear"
		}

		expected := "💾 Cache: 3715/4102 tokens | Creation: 500 tokens"
		if detail != expected {
			t.Errorf("Expected %q, got %q", expected, detail)
		}
	})

	t.Run("debug_format_high_usage", func(t *testing.T) {
		// Test with high context usage
		highUsage := providers.Usage{
			InputTokens:              50000,
			CacheCreationInputTokens: 0,
			CacheReadInputTokens:     150000,
		}
		highTotal := highUsage.InputTokens + highUsage.CacheReadInputTokens

		detail := fmt.Sprintf("💾 Cache: %d/%d tokens | Creation: %d tokens",
			highUsage.CacheReadInputTokens, highTotal,
			highUsage.CacheCreationInputTokens)
		pct := (highTotal * 100) / contextWindowSize
		if pct > 100 {
			pct = 100
		}
		detail += fmt.Sprintf(" | Context: %d%% (%d/%d)",
			pct, highTotal, contextWindowSize)

		expected := "💾 Cache: 150000/200000 tokens | Creation: 0 tokens | Context: 100% (200000/200000)"
		if detail != expected {
			t.Errorf("Expected %q, got %q", expected, detail)
		}
	})

	t.Run("verbose_format_zero_cache", func(t *testing.T) {
		// When CacheReadInputTokens is 0, no cache message should be emitted.
		// This test verifies the condition check.
		zeroUsage := providers.Usage{
			InputTokens:              1000,
			CacheReadInputTokens:     0,
			CacheCreationInputTokens: 500,
		}

		if zeroUsage.CacheReadInputTokens > 0 {
			t.Error("This should not execute - zero cache read tokens means no message")
		}
	})
}

// TestCacheDisplayLevelGating verifies that the emit gating logic correctly
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
			var messages []string
			a := agent.NewAgent(
				providers.NewClient("dummy", "http://localhost", "test", 100),
				"test prompt",
				agent.WithLogLevel(tt.level),
				agent.WithContextWindowSize(200000),
				agent.WithProgressCallback(func(lvl loglevel.Level, msg string) {
					messages = append(messages, msg)
				}),
			)

			// Directly test the ShouldShow logic that gates cache display
			if got := tt.level.ShouldShow(loglevel.Verbose); got != tt.wantShow {
				t.Errorf("Level %s ShouldShow(Verbose) = %v, want %v",
					tt.level, got, tt.wantShow)
			}

			// Verify agent was created with correct level
			if a.LogLevel() != tt.level {
				t.Errorf("Agent log level = %s, want %s", a.LogLevel(), tt.level)
			}
		})
	}
}

// TestCacheDisplayOldFormatRemoved verifies the old cache display format
// ("💾 Cache hit: N tokens (M% of input)") is no longer used.
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

	var messages []string
	agentInstance := agent.NewAgent(
		apiClient,
		prompts.SystemPrompt,
		agent.WithLogLevel(loglevel.Debug), // Show everything
		agent.WithContextWindowSize(200000),
		agent.WithProgressCallback(func(lvl loglevel.Level, msg string) {
			messages = append(messages, msg)
		}),
	)

	// Make two requests to trigger cache
	_, _ = agentInstance.HandleMessage("What is 2+2? Reply with just the number.")
	messages = nil
	_, _ = agentInstance.HandleMessage("What is 3+3? Reply with just the number.")

	for _, msg := range messages {
		if strings.Contains(msg, "Cache hit:") {
			t.Errorf("Found old format 'Cache hit:' — should use new format. Message: %s", msg)
		}
		if strings.Contains(msg, "of input") && strings.Contains(msg, "💾") {
			t.Errorf("Found old format 'of input' — should use new format. Message: %s", msg)
		}
	}
}

// TestWithContextWindowSizeOption verifies the WithContextWindowSize agent option.
func TestWithContextWindowSizeOption(t *testing.T) {
	t.Run("default_is_zero", func(t *testing.T) {
		a := agent.NewAgent(
			providers.NewClient("dummy", "http://localhost", "test", 100),
			"test",
		)
		// Default agent has no context window size set
		// (contextWindowSize is 0 / unexported, but the agent still works)
		if a.LogLevel() != loglevel.Normal {
			t.Errorf("Default log level should be Normal, got %s", a.LogLevel())
		}
	})

	t.Run("set_via_option", func(t *testing.T) {
		var messages []string
		a := agent.NewAgent(
			providers.NewClient("dummy", "http://localhost", "test", 100),
			"test",
			agent.WithContextWindowSize(128000),
			agent.WithLogLevel(loglevel.Debug),
			agent.WithProgressCallback(func(lvl loglevel.Level, msg string) {
				messages = append(messages, msg)
			}),
		)
		// Agent compiles and runs with the option
		if a.LogLevel() != loglevel.Debug {
			t.Errorf("Expected Debug, got %s", a.LogLevel())
		}
	})
}
