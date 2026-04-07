package main

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/this-is-alpha-iota/clyde/cli/prompt"
	"github.com/this-is-alpha-iota/clyde/cli/style"
)

// TestGetGitInfo tests the live git info retrieval.
// This test runs in the actual clyde repo, so we know it's a git repo.
func TestGetGitInfo(t *testing.T) {
	info := prompt.GetGitInfo()

	if !info.IsRepo {
		t.Fatal("Expected IsRepo=true when running inside the clyde git repo")
	}

	if info.Branch == "" {
		t.Error("Expected non-empty branch name")
	}

	t.Logf("Git info: branch=%q dirty=%v", info.Branch, info.Dirty)
}

// TestGetGitInfoWith_CleanRepo tests prompt formatting for a clean git repo.
func TestGetGitInfoWith_CleanRepo(t *testing.T) {
	runner := func(args ...string) (string, error) {
		switch args[0] {
		case "rev-parse":
			if args[1] == "--abbrev-ref" {
				return "main\n", nil
			}
		case "status":
			return "", nil // empty porcelain = clean
		}
		return "", fmt.Errorf("unexpected git command: %v", args)
	}

	info := prompt.GetGitInfoWith(runner)

	if !info.IsRepo {
		t.Error("Expected IsRepo=true")
	}
	if info.Branch != "main" {
		t.Errorf("Branch = %q, want %q", info.Branch, "main")
	}
	if info.Dirty {
		t.Error("Expected Dirty=false for clean repo")
	}
}

// TestGetGitInfoWith_DirtyRepo tests prompt formatting for a dirty git repo.
func TestGetGitInfoWith_DirtyRepo(t *testing.T) {
	runner := func(args ...string) (string, error) {
		switch args[0] {
		case "rev-parse":
			if args[1] == "--abbrev-ref" {
				return "feature/my-branch\n", nil
			}
		case "status":
			return " M main.go\n?? untracked.txt\n", nil // porcelain output
		}
		return "", fmt.Errorf("unexpected git command: %v", args)
	}

	info := prompt.GetGitInfoWith(runner)

	if !info.IsRepo {
		t.Error("Expected IsRepo=true")
	}
	if info.Branch != "feature/my-branch" {
		t.Errorf("Branch = %q, want %q", info.Branch, "feature/my-branch")
	}
	if !info.Dirty {
		t.Error("Expected Dirty=true for dirty repo")
	}
}

// TestGetGitInfoWith_DetachedHead tests prompt formatting for detached HEAD state.
func TestGetGitInfoWith_DetachedHead(t *testing.T) {
	runner := func(args ...string) (string, error) {
		switch args[0] {
		case "rev-parse":
			if len(args) > 1 && args[1] == "--abbrev-ref" {
				return "HEAD\n", nil // detached HEAD
			}
			if len(args) > 1 && args[1] == "--short" {
				return "a1b2c3d\n", nil
			}
		case "status":
			return "", nil
		}
		return "", fmt.Errorf("unexpected git command: %v", args)
	}

	info := prompt.GetGitInfoWith(runner)

	if !info.IsRepo {
		t.Error("Expected IsRepo=true")
	}
	if info.Branch != "a1b2c3d" {
		t.Errorf("Branch = %q, want %q (short hash)", info.Branch, "a1b2c3d")
	}
	if info.Dirty {
		t.Error("Expected Dirty=false")
	}
}

// TestGetGitInfoWith_NotAGitRepo tests when not in a git repository.
func TestGetGitInfoWith_NotAGitRepo(t *testing.T) {
	runner := func(args ...string) (string, error) {
		return "", fmt.Errorf("fatal: not a git repository")
	}

	info := prompt.GetGitInfoWith(runner)

	if info.IsRepo {
		t.Error("Expected IsRepo=false when not in a git repo")
	}
	if info.Branch != "" {
		t.Errorf("Expected empty branch, got %q", info.Branch)
	}
	if info.Dirty {
		t.Error("Expected Dirty=false when not in a git repo")
	}
}

// TestGetGitInfoWith_StatusFails tests graceful handling when git status fails.
func TestGetGitInfoWith_StatusFails(t *testing.T) {
	runner := func(args ...string) (string, error) {
		switch args[0] {
		case "rev-parse":
			if args[1] == "--abbrev-ref" {
				return "main\n", nil
			}
		case "status":
			return "", fmt.Errorf("error: could not get status")
		}
		return "", fmt.Errorf("unexpected git command: %v", args)
	}

	info := prompt.GetGitInfoWith(runner)

	if !info.IsRepo {
		t.Error("Expected IsRepo=true (branch lookup succeeded)")
	}
	if info.Branch != "main" {
		t.Errorf("Branch = %q, want %q", info.Branch, "main")
	}
	if info.Dirty {
		t.Error("Expected Dirty=false when status fails (graceful fallback)")
	}
}

// TestFormatPrompt tests the prompt line formatting with various combinations.
func TestFormatPrompt(t *testing.T) {
	// Ensure color is enabled for these tests (we check for content, not exact ANSI codes)
	os.Unsetenv("NO_COLOR")
	os.Setenv("TERM", "xterm-256color")
	style.ResetColorCache()
	t.Cleanup(func() {
		style.ResetColorCache()
	})

	tests := []struct {
		name           string
		git            prompt.GitInfo
		contextPercent int
		wantContains   []string
		wantAbsent     []string
	}{
		{
			name:           "clean repo with context",
			git:            prompt.GitInfo{IsRepo: true, Branch: "main", Dirty: false},
			contextPercent: 12,
			wantContains:   []string{"main", "12%", "You: "},
			wantAbsent:     []string{"*"},
		},
		{
			name:           "dirty repo with context",
			git:            prompt.GitInfo{IsRepo: true, Branch: "main", Dirty: true},
			contextPercent: 45,
			wantContains:   []string{"main*", "45%", "You: "},
		},
		{
			name:           "detached head",
			git:            prompt.GitInfo{IsRepo: true, Branch: "a1b2c3d", Dirty: false},
			contextPercent: 5,
			wantContains:   []string{"a1b2c3d", "5%", "You: "},
		},
		{
			name:           "not a git repo with context",
			git:            prompt.GitInfo{IsRepo: false},
			contextPercent: 30,
			wantContains:   []string{"30%", "You: "},
			wantAbsent:     []string{"main"},
		},
		{
			name:           "no context yet (before first API call)",
			git:            prompt.GitInfo{IsRepo: true, Branch: "main", Dirty: false},
			contextPercent: -1,
			wantContains:   []string{"main", "You: "},
			wantAbsent:     []string{"%"},
		},
		{
			name:           "not a git repo and no context",
			git:            prompt.GitInfo{IsRepo: false},
			contextPercent: -1,
			wantContains:   []string{"You: "},
			wantAbsent:     []string{"%", "main"},
		},
		{
			name:           "0% context",
			git:            prompt.GitInfo{IsRepo: true, Branch: "develop", Dirty: false},
			contextPercent: 0,
			wantContains:   []string{"develop", "0%", "You: "},
		},
		{
			name:           "99% context",
			git:            prompt.GitInfo{IsRepo: true, Branch: "main", Dirty: true},
			contextPercent: 99,
			wantContains:   []string{"main*", "99%", "You: "},
		},
		{
			name:           "feature branch with slash",
			git:            prompt.GitInfo{IsRepo: true, Branch: "feature/tui-4", Dirty: true},
			contextPercent: 50,
			wantContains:   []string{"feature/tui-4*", "50%", "You: "},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := prompt.FormatPrompt(tt.git, tt.contextPercent)

			// Strip ANSI codes for content checking
			stripped := stripANSI(result)

			for _, want := range tt.wantContains {
				if !strings.Contains(stripped, want) {
					t.Errorf("prompt.FormatPrompt() stripped = %q, want to contain %q", stripped, want)
				}
			}

			for _, absent := range tt.wantAbsent {
				if strings.Contains(stripped, absent) {
					t.Errorf("prompt.FormatPrompt() stripped = %q, should NOT contain %q", stripped, absent)
				}
			}
		})
	}
}

// TestFormatPrompt_NoColor tests that FormatPrompt works without ANSI codes.
func TestFormatPrompt_NoColor(t *testing.T) {
	os.Setenv("NO_COLOR", "1")
	style.ResetColorCache()
	t.Cleanup(func() {
		os.Unsetenv("NO_COLOR")
		style.ResetColorCache()
	})

	result := prompt.FormatPrompt(
		prompt.GitInfo{IsRepo: true, Branch: "main", Dirty: true},
		25,
	)

	// No ANSI codes should be present
	if strings.Contains(result, "\033[") {
		t.Errorf("prompt.FormatPrompt() with NO_COLOR should not contain ANSI codes, got %q", result)
	}

	// Content should still be correct
	if !strings.Contains(result, "main*") {
		t.Errorf("Expected 'main*' in prompt, got %q", result)
	}
	if !strings.Contains(result, "25%") {
		t.Errorf("Expected '25%%' in prompt, got %q", result)
	}
	if !strings.Contains(result, "You: ") {
		t.Errorf("Expected 'You: ' in prompt, got %q", result)
	}
}

// TestFormatPrompt_Ordering verifies the order: git, context%, You:
func TestFormatPrompt_Ordering(t *testing.T) {
	os.Setenv("NO_COLOR", "1")
	style.ResetColorCache()
	t.Cleanup(func() {
		os.Unsetenv("NO_COLOR")
		style.ResetColorCache()
	})

	result := prompt.FormatPrompt(
		prompt.GitInfo{IsRepo: true, Branch: "main", Dirty: true},
		42,
	)

	gitIdx := strings.Index(result, "main*")
	pctIdx := strings.Index(result, "42%")
	youIdx := strings.Index(result, "You:")

	if gitIdx < 0 || pctIdx < 0 || youIdx < 0 {
		t.Fatalf("Missing expected content in %q", result)
	}

	if gitIdx >= pctIdx {
		t.Errorf("Git info should come before context%%: git=%d, pct=%d", gitIdx, pctIdx)
	}
	if pctIdx >= youIdx {
		t.Errorf("Context%% should come before You:: pct=%d, you=%d", pctIdx, youIdx)
	}
}

// TestCalculateContextPercent tests the context percentage calculation.
func TestCalculateContextPercent(t *testing.T) {
	tests := []struct {
		name        string
		inputTokens int
		windowSize  int
		want        int
	}{
		{"0 tokens used", 0, 200000, 0},
		{"12% usage", 24000, 200000, 12},
		{"50% usage", 100000, 200000, 50},
		{"99% usage", 198000, 200000, 99},
		{"100% usage", 200000, 200000, 100},
		{"over 100% clamped", 250000, 200000, 100},
		{"small window", 50, 1000, 5},
		{"1% boundary", 2000, 200000, 1},
		{"unknown window size", 1000, 0, -1},
		{"negative window size", 1000, -1, -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := prompt.CalculateContextPercent(tt.inputTokens, tt.windowSize)
			if got != tt.want {
				t.Errorf("prompt.CalculateContextPercent(%d, %d) = %d, want %d",
					tt.inputTokens, tt.windowSize, got, tt.want)
			}
		})
	}
}

// TestFormatPrompt_UserLabelStyled verifies the You: label is bold cyan when color enabled.
func TestFormatPrompt_UserLabelStyled(t *testing.T) {
	os.Unsetenv("NO_COLOR")
	os.Setenv("TERM", "xterm-256color")
	style.ResetColorCache()
	t.Cleanup(func() {
		style.ResetColorCache()
	})

	result := prompt.FormatPrompt(prompt.GitInfo{IsRepo: false}, -1)

	// Should contain bold cyan ANSI code for "You: "
	// Bold cyan = \033[1;36m
	if !strings.Contains(result, "\033[1;36m") {
		t.Errorf("Expected bold cyan ANSI code in prompt, got %q", result)
	}
	if !strings.Contains(result, "You: ") {
		t.Errorf("Expected 'You: ' in prompt, got %q", result)
	}
}

// TestFormatPrompt_GitInfoDimmed verifies git info is rendered in dim style.
func TestFormatPrompt_GitInfoDimmed(t *testing.T) {
	os.Unsetenv("NO_COLOR")
	os.Setenv("TERM", "xterm-256color")
	style.ResetColorCache()
	t.Cleanup(func() {
		style.ResetColorCache()
	})

	result := prompt.FormatPrompt(
		prompt.GitInfo{IsRepo: true, Branch: "main", Dirty: false},
		-1,
	)

	// Should contain dim ANSI code (\033[2m) for git info
	if !strings.Contains(result, "\033[2m") {
		t.Errorf("Expected dim ANSI code for git info, got %q", result)
	}
}

// stripANSI removes ANSI escape sequences from a string for content testing.
func stripANSI(s string) string {
	var result strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == '\033' && i+1 < len(s) && s[i+1] == '[' {
			// Skip until we find the terminating letter
			j := i + 2
			for j < len(s) && !((s[j] >= 'A' && s[j] <= 'Z') || (s[j] >= 'a' && s[j] <= 'z')) {
				j++
			}
			if j < len(s) {
				i = j + 1
			} else {
				i = j
			}
		} else {
			result.WriteByte(s[i])
			i++
		}
	}
	return result.String()
}
