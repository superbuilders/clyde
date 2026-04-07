package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/this-is-alpha-iota/clyde/agent"
	"github.com/this-is-alpha-iota/clyde/providers"
	"github.com/this-is-alpha-iota/clyde/cli/prompt"
	"github.com/this-is-alpha-iota/clyde/agent/prompts"
	"github.com/this-is-alpha-iota/clyde/cli/style"
)

// TestPromptLine_CLIModeNoPrompt verifies that CLI mode does not render a prompt line.
// In CLI mode, there's no interactive prompt — just direct output.
func TestPromptLine_CLIModeNoPrompt(t *testing.T) {
	binaryPath := buildTestBinary(t)

	apiKey := os.Getenv("TS_AGENT_API_KEY")
	if apiKey == "" {
		t.Skip("Skipping: TS_AGENT_API_KEY not set")
	}

	cmd := buildTestCommand(t, binaryPath, "What is 1+1?")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("CLI mode failed: %v\nOutput: %s", err, output)
	}

	outputStr := string(output)
	// CLI mode should NOT contain "You: " prompt
	if strings.Contains(outputStr, "You: ") {
		t.Errorf("CLI mode should not contain 'You: ' prompt, got: %s", outputStr)
	}
}

// TestPromptLine_FormatWithAgent tests that the prompt can be constructed
// using agent LastUsage() data after an API call.
func TestPromptLine_FormatWithAgent(t *testing.T) {
	apiKey := os.Getenv("TS_AGENT_API_KEY")
	if apiKey == "" {
		t.Skip("Skipping: TS_AGENT_API_KEY not set")
	}

	apiClient := providers.NewClient(apiKey, "https://api.anthropic.com/v1/messages", "claude-sonnet-4-5-20250929", 4096)
	a := agent.NewAgent(apiClient, prompts.SystemPrompt)

	// Before any API call, LastUsage should be zero
	usage := a.LastUsage()
	if usage.InputTokens != 0 {
		t.Errorf("Before API call, InputTokens should be 0, got %d", usage.InputTokens)
	}

	// Make an API call
	_, err := a.HandleMessage("Say hi")
	if err != nil {
		t.Fatalf("HandleMessage failed: %v", err)
	}

	// After API call, LastUsage should have non-zero tokens
	usage = a.LastUsage()
	totalInput := usage.InputTokens + usage.CacheReadInputTokens
	if totalInput == 0 {
		t.Error("After API call, expected non-zero total input tokens")
	}

	// Calculate context percent
	contextPercent := prompt.CalculateContextPercent(totalInput, 200000)
	if contextPercent < 0 || contextPercent > 100 {
		t.Errorf("Context percent should be 0-100, got %d", contextPercent)
	}

	t.Logf("Usage: input=%d, cache_read=%d, total=%d, context=%.1f%%",
		usage.InputTokens, usage.CacheReadInputTokens, totalInput,
		float64(totalInput)/200000.0*100)

	// Format the prompt with real data
	gitInfo := prompt.GetGitInfo()
	promptLine := prompt.FormatPrompt(gitInfo, contextPercent)

	t.Logf("Prompt line: %q", promptLine)

	if !strings.Contains(promptLine, "You: ") {
		t.Error("Prompt should contain 'You: '")
	}
}

// TestPromptLine_GitInfoInRepo verifies git info appears in the prompt
// when running inside a git repository.
func TestPromptLine_GitInfoInRepo(t *testing.T) {
	os.Setenv("NO_COLOR", "1")
	style.ResetColorCache()
	t.Cleanup(func() {
		os.Unsetenv("NO_COLOR")
		style.ResetColorCache()
	})

	gitInfo := prompt.GetGitInfo()

	if !gitInfo.IsRepo {
		t.Skip("Not running in a git repo")
	}

	result := prompt.FormatPrompt(gitInfo, 10)

	if !strings.Contains(result, gitInfo.Branch) {
		t.Errorf("Prompt should contain branch %q, got %q", gitInfo.Branch, result)
	}

	if !strings.Contains(result, "10%") {
		t.Errorf("Prompt should contain '10%%', got %q", result)
	}

	if !strings.Contains(result, "You: ") {
		t.Errorf("Prompt should contain 'You: ', got %q", result)
	}

	t.Logf("Prompt: %q (branch=%q, dirty=%v)", result, gitInfo.Branch, gitInfo.Dirty)
}

// TestPromptLine_NonGitDirectory verifies git info is omitted outside a git repo.
func TestPromptLine_NonGitDirectory(t *testing.T) {
	os.Setenv("NO_COLOR", "1")
	style.ResetColorCache()
	t.Cleanup(func() {
		os.Unsetenv("NO_COLOR")
		style.ResetColorCache()
	})

	// Create a temp dir that is NOT a git repo
	tmpDir := t.TempDir()

	// Run git rev-parse in the temp dir to confirm it's not a git repo
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err == nil {
		t.Skip("Temp dir unexpectedly inside a git repo")
	}

	// Simulate non-repo state
	info := prompt.GitInfo{IsRepo: false}
	result := prompt.FormatPrompt(info, 5)

	// Should just have context % and You: — no branch info
	if strings.Contains(result, "main") || strings.Contains(result, "master") {
		t.Errorf("Non-git prompt should not contain branch name, got %q", result)
	}
	if !strings.Contains(result, "5%") {
		t.Errorf("Prompt should contain '5%%', got %q", result)
	}
	if !strings.Contains(result, "You: ") {
		t.Errorf("Prompt should contain 'You: ', got %q", result)
	}
}

// TestPromptLine_ContextPercentProgression verifies that context % is valid
// and that LastUsage() updates after each API call.
func TestPromptLine_ContextPercentProgression(t *testing.T) {
	apiKey := os.Getenv("TS_AGENT_API_KEY")
	if apiKey == "" {
		t.Skip("Skipping: TS_AGENT_API_KEY not set")
	}

	apiClient := providers.NewClient(apiKey, "https://api.anthropic.com/v1/messages", "claude-sonnet-4-5-20250929", 4096)
	a := agent.NewAgent(apiClient, prompts.SystemPrompt)

	// Before any call, usage should be zero
	usage0 := a.LastUsage()
	if usage0.InputTokens != 0 || usage0.OutputTokens != 0 {
		t.Errorf("Before any API call, expected zero usage, got input=%d output=%d",
			usage0.InputTokens, usage0.OutputTokens)
	}

	// First message
	_, err := a.HandleMessage("Say hello")
	if err != nil {
		t.Fatalf("First message failed: %v", err)
	}

	usage1 := a.LastUsage()
	total1 := usage1.InputTokens + usage1.CacheReadInputTokens
	pct1 := prompt.CalculateContextPercent(total1, 200000)

	t.Logf("Turn 1: input=%d cache_read=%d total=%d (%.1f%%)",
		usage1.InputTokens, usage1.CacheReadInputTokens, total1,
		float64(total1)/200000.0*100)

	// After first call, usage should be non-zero (either InputTokens or CacheReadInputTokens)
	if total1 == 0 && usage1.OutputTokens == 0 {
		t.Error("After first API call, expected non-zero token usage")
	}

	// Second message
	_, err = a.HandleMessage("Now say goodbye")
	if err != nil {
		t.Fatalf("Second message failed: %v", err)
	}

	usage2 := a.LastUsage()
	total2 := usage2.InputTokens + usage2.CacheReadInputTokens
	pct2 := prompt.CalculateContextPercent(total2, 200000)

	t.Logf("Turn 2: input=%d cache_read=%d total=%d (%.1f%%)",
		usage2.InputTokens, usage2.CacheReadInputTokens, total2,
		float64(total2)/200000.0*100)

	// Percentages should be valid (0-100)
	if pct1 < 0 || pct1 > 100 {
		t.Errorf("Turn 1 context percent out of range: %d", pct1)
	}
	if pct2 < 0 || pct2 > 100 {
		t.Errorf("Turn 2 context percent out of range: %d", pct2)
	}

	// LastUsage should have been updated (output tokens should be set)
	if usage2.OutputTokens == 0 {
		t.Error("After second API call, expected non-zero output tokens")
	}
}
