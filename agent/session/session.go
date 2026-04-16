// Package session provides file-based session persistence for Clyde.
//
// Each session is a directory of timestamped Markdown files — one per message
// or content block. The filesystem is the index: filenames encode timestamps
// and message types, enabling trivial filtering with Unix globs.
//
// Session location:
//   - Inside a git repo: <repo>/.clyde/sessions/
//   - Outside any git repo: ~/.clyde/sessions/
//
// File naming: <timestamp>_<type>.md
//   - Timestamp: ISO-8601 with milliseconds, hyphens for colons
//   - Type: user, assistant, system, thinking, tool-use, tool-result, diagnostic, compaction
//
// Design: see docs/sessions-history.md
package session

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// MessageType identifies the kind of message being persisted.
type MessageType string

const (
	TypeUser       MessageType = "user"
	TypeAssistant  MessageType = "assistant"
	TypeSystem     MessageType = "system"
	TypeThinking   MessageType = "thinking"
	TypeToolUse    MessageType = "tool-use"
	TypeToolResult MessageType = "tool-result"
	TypeDiagnostic MessageType = "diagnostic"
	TypeCompaction MessageType = "compaction"
)

// Session represents an active session with its directory and state.
type Session struct {
	// Dir is the absolute path to the session directory.
	Dir string

	// SessionsRoot is the parent directory containing all sessions.
	SessionsRoot string

	mu            sync.Mutex
	lastTimestamp time.Time // monotonicity guard
}

// New creates a new session. It determines the session location (git repo root
// or ~/.clyde/), creates the session directory, and optionally updates .gitignore.
//
// The session directory name is: <timestamp>_<username>
func New() (*Session, error) {
	sessionsRoot, inGitRepo := findSessionsRoot()

	// Create sessions root if needed
	isNewRoot := false
	if _, err := os.Stat(sessionsRoot); os.IsNotExist(err) {
		isNewRoot = true
		if err := os.MkdirAll(sessionsRoot, 0755); err != nil {
			return nil, fmt.Errorf("failed to create sessions directory %s: %w", sessionsRoot, err)
		}
	}

	// Add .clyde/sessions/ to .gitignore if this is a new directory inside a git repo
	if isNewRoot && inGitRepo {
		if err := ensureGitignore(sessionsRoot); err != nil {
			// Non-fatal — warn but continue
			fmt.Fprintf(os.Stderr, "Warning: could not update .gitignore: %v\n", err)
		}
	}

	// Determine username
	username := getUsername()

	// Create session directory
	now := time.Now()
	dirName := FormatTimestampDir(now) + "_" + username
	sessionDir := filepath.Join(sessionsRoot, dirName)

	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create session directory %s: %w", sessionDir, err)
	}

	return &Session{
		Dir:          sessionDir,
		SessionsRoot: sessionsRoot,
	}, nil
}

// WriteMessage writes a message file to the session directory.
// The file is named <timestamp>_<type>.md and contains the provided content.
// Content should be ANSI-stripped, debug-level Markdown.
//
// This method is safe for concurrent use.
func (s *Session) WriteMessage(msgType MessageType, content string) error {
	s.mu.Lock()
	now := s.monotonicNow()
	s.mu.Unlock()

	filename := FormatTimestampFile(now) + "_" + string(msgType) + ".md"
	path := filepath.Join(s.Dir, filename)

	return os.WriteFile(path, []byte(content), 0644)
}

// RelativeDir returns the session directory relative to the current working directory,
// or the absolute path if it can't be made relative.
func (s *Session) RelativeDir() string {
	cwd, err := os.Getwd()
	if err != nil {
		return s.Dir
	}
	rel, err := filepath.Rel(cwd, s.Dir)
	if err != nil {
		return s.Dir
	}
	return rel
}

// monotonicNow returns the current time truncated to millisecond precision,
// ensuring it is strictly greater than the last returned timestamp. If the
// system clock has not advanced past the last millisecond, the timestamp is
// bumped by 1 millisecond. Must be called with s.mu held.
//
// Millisecond truncation is essential because the filename format only has
// millisecond precision. Without it, two calls in the same millisecond could
// produce identical filenames, causing overwrites.
func (s *Session) monotonicNow() time.Time {
	now := time.Now().Truncate(time.Millisecond)
	if !now.After(s.lastTimestamp) {
		now = s.lastTimestamp.Add(time.Millisecond)
	}
	s.lastTimestamp = now
	return now
}

// FormatTimestampDir formats a time for use in a session directory name.
// Format: 2026-07-14T09-32-00 (no milliseconds, hyphens for colons).
func FormatTimestampDir(t time.Time) string {
	return t.Format("2006-01-02T15-04-05")
}

// FormatTimestampFile formats a time for use in a message filename.
// Format: 2026-07-14T09-32-05.123 (with milliseconds, hyphens for colons).
func FormatTimestampFile(t time.Time) string {
	return t.Format("2006-01-02T15-04-05.000")
}

// FindSessionsRoot determines where sessions should be stored.
// Returns (path, inGitRepo). Exported for testing.
func FindSessionsRoot() (string, bool) {
	return findSessionsRoot()
}

// findSessionsRoot determines where sessions should be stored.
// Returns (path, inGitRepo).
func findSessionsRoot() (string, bool) {
	// Try git repo root first
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	output, err := cmd.Output()
	if err == nil {
		repoRoot := strings.TrimSpace(string(output))
		if repoRoot != "" {
			return filepath.Join(repoRoot, ".clyde", "sessions"), true
		}
	}

	// Fallback to ~/.clyde/sessions/
	homeDir, err := os.UserHomeDir()
	if err != nil {
		// Last resort: use current directory
		return filepath.Join(".clyde", "sessions"), false
	}
	return filepath.Join(homeDir, ".clyde", "sessions"), false
}

// GetUsername returns the normalized username for session directory naming.
// Exported for testing.
func GetUsername() string {
	return getUsername()
}

// getUsername returns the normalized username for session directory naming.
// Tries git config user.name first, then falls back to $USER.
// Lowercase, spaces replaced with hyphens.
func getUsername() string {
	// Try git config user.name
	cmd := exec.Command("git", "config", "user.name")
	output, err := cmd.Output()
	if err == nil {
		name := strings.TrimSpace(string(output))
		if name != "" {
			return NormalizeUsername(name)
		}
	}

	// Fallback to $USER environment variable
	if envUser := os.Getenv("USER"); envUser != "" {
		return NormalizeUsername(envUser)
	}

	// Fallback to os/user
	if u, err := user.Current(); err == nil && u.Username != "" {
		return NormalizeUsername(u.Username)
	}

	return "unknown"
}

// NormalizeUsername lowercases a name and replaces spaces/special chars with hyphens.
// Exported for testing.
func NormalizeUsername(name string) string {
	name = strings.ToLower(name)
	name = strings.ReplaceAll(name, " ", "-")
	// Remove any characters that aren't alphanumeric or hyphens
	re := regexp.MustCompile(`[^a-z0-9\-]`)
	name = re.ReplaceAllString(name, "")
	// Collapse multiple hyphens
	re2 := regexp.MustCompile(`-+`)
	name = re2.ReplaceAllString(name, "-")
	name = strings.Trim(name, "-")
	if name == "" {
		return "unknown"
	}
	return name
}

// ensureGitignore adds .clyde/sessions/ to the repo's .gitignore if not already present.
func ensureGitignore(sessionsRoot string) error {
	// Find the repo root (parent of .clyde/sessions/)
	// sessionsRoot is <repo>/.clyde/sessions/
	clydeDir := filepath.Dir(sessionsRoot) // <repo>/.clyde
	repoRoot := filepath.Dir(clydeDir)     // <repo>
	gitignorePath := filepath.Join(repoRoot, ".gitignore")

	entry := ".clyde/sessions/"

	// Read existing .gitignore
	content, err := os.ReadFile(gitignorePath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to read .gitignore: %w", err)
	}

	// Check if already present
	if strings.Contains(string(content), entry) {
		return nil // already there
	}

	// Append the entry
	f, err := os.OpenFile(gitignorePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open .gitignore: %w", err)
	}
	defer f.Close()

	// Ensure we start on a new line
	if len(content) > 0 && content[len(content)-1] != '\n' {
		if _, err := f.WriteString("\n"); err != nil {
			return err
		}
	}

	// Add comment and entry
	if _, err := f.WriteString("\n# Clyde session history\n" + entry + "\n"); err != nil {
		return err
	}

	return nil
}

// StripANSI removes ANSI escape codes from a string.
// Used to ensure session files contain clean Markdown without terminal styling.
func StripANSI(s string) string {
	re := regexp.MustCompile(`\x1b\[[0-9;]*m`)
	return re.ReplaceAllString(s, "")
}

// FormatToolUseID appends the tool use ID to the first line of a progress
// message in brackets. For multi-line display messages (e.g. run_bash with
// newlines), the ID is always placed on line 1 so that extractToolUseMetadata
// can find it reliably during session reconstruction.
//
// Example (single-line):  "→ Reading file: agent.go" → "→ Reading file: agent.go [toolu_abc123]"
// Example (multi-line):   "→ Running bash: cd /tmp\nls" → "→ Running bash: cd /tmp [toolu_abc123]\nls"
func FormatToolUseID(progressMsg, toolUseID string) string {
	if toolUseID == "" {
		return progressMsg
	}
	if idx := strings.Index(progressMsg, "\n"); idx >= 0 {
		// Multi-line: insert ID at end of first line, keep remaining lines after
		return fmt.Sprintf("%s [%s]%s", progressMsg[:idx], toolUseID, progressMsg[idx:])
	}
	return fmt.Sprintf("%s [%s]", progressMsg, toolUseID)
}
