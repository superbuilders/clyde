// Package prompt provides the REPL prompt line formatter for Clyde.
//
// The prompt line shows:
//   - Git branch name (or short hash if detached HEAD)
//   - Dirty indicator (*) when there are uncommitted changes
//   - Context window usage percentage
//   - "You: " label styled in bold cyan
//
// Example: "main* 12% You: "
//
// Git info is omitted when not in a git repository.
// In CLI mode, there is no prompt line.
package prompt

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/this-is-alpha-iota/clyde/style"
)

// GitInfo holds the current git repository state.
type GitInfo struct {
	// Branch is the current branch name, or short commit hash if detached.
	Branch string
	// Dirty is true when there are uncommitted changes.
	Dirty bool
	// IsRepo is true when the current directory is inside a git repository.
	IsRepo bool
}

// GetGitInfo queries git for the current branch and dirty state.
// Returns GitInfo with IsRepo=false if not in a git repository.
func GetGitInfo() GitInfo {
	return getGitInfoWith(runGitCommand)
}

// gitRunner is a function that executes a git command and returns its output.
// It is used for dependency injection in tests.
type gitRunner func(args ...string) (string, error)

// getGitInfoWith queries git state using the provided runner function.
// This allows testing without actual git commands.
func getGitInfoWith(run gitRunner) GitInfo {
	// Get branch name
	branch, err := run("rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return GitInfo{IsRepo: false}
	}
	branch = strings.TrimSpace(branch)

	// If detached HEAD, rev-parse --abbrev-ref returns "HEAD"
	if branch == "HEAD" {
		// Get short hash instead
		hash, err := run("rev-parse", "--short", "HEAD")
		if err != nil {
			return GitInfo{IsRepo: false}
		}
		branch = strings.TrimSpace(hash)
	}

	// Check for dirty state
	porcelain, err := run("status", "--porcelain")
	if err != nil {
		// If status fails, still report what we have
		return GitInfo{IsRepo: true, Branch: branch, Dirty: false}
	}

	dirty := strings.TrimSpace(porcelain) != ""

	return GitInfo{
		IsRepo: true,
		Branch: branch,
		Dirty:  dirty,
	}
}

// runGitCommand executes a git command and returns its trimmed stdout.
func runGitCommand(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// FormatPrompt formats the complete REPL prompt line.
//
// Parameters:
//   - git: current git repository info (from GetGitInfo)
//   - contextPercent: context window usage as integer percentage (0–100)
//     Use -1 to indicate "no data yet" (before the first API call).
//
// Returns a styled string like: "main* 12% You: "
// If not in a git repo, returns: "12% You: "
// Before first API call: "main* You: " or just "You: "
func FormatPrompt(git GitInfo, contextPercent int) string {
	var parts []string

	// Git info
	if git.IsRepo {
		gitPart := git.Branch
		if git.Dirty {
			gitPart += "*"
		}
		parts = append(parts, style.Dim(gitPart))
	}

	// Context percentage (only shown after first API call)
	if contextPercent >= 0 {
		parts = append(parts, style.Dim(fmt.Sprintf("%d%%", contextPercent)))
	}

	// Build prefix (git + context %, space-separated)
	prefix := ""
	if len(parts) > 0 {
		prefix = strings.Join(parts, " ") + " "
	}

	return prefix + style.FormatUserPrompt()
}

// CalculateContextPercent computes the context window usage percentage.
//
// Parameters:
//   - inputTokens: total input tokens from the last API response
//     (sum of input_tokens and cache_read_input_tokens)
//   - contextWindowSize: the model's maximum context window in tokens
//
// Returns an integer percentage (0–100), clamped to 100.
// Returns -1 if contextWindowSize is 0 (unknown).
func CalculateContextPercent(inputTokens, contextWindowSize int) int {
	if contextWindowSize <= 0 {
		return -1
	}
	pct := (inputTokens * 100) / contextWindowSize
	if pct > 100 {
		pct = 100
	}
	return pct
}
