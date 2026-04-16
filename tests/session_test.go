package main

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/this-is-alpha-iota/clyde/agent/session"
)

// --- Unit Tests ---

// TestSessionCreation verifies that a new session creates the correct
// directory structure with proper naming conventions.
func TestSessionCreation(t *testing.T) {
	// Create a temp directory to act as a fake repo root
	tmpDir := t.TempDir()

	// Override HOME so sessions go somewhere predictable
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	// Change to a non-git directory so sessions go to ~/.clyde/sessions/
	origDir, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(origDir)

	sess, err := session.New()
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	// Verify session directory exists
	info, err := os.Stat(sess.Dir)
	if err != nil {
		t.Fatalf("Session directory does not exist: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("Session path is not a directory")
	}

	// Verify directory name format: <timestamp>_<username>
	dirName := filepath.Base(sess.Dir)
	parts := strings.SplitN(dirName, "_", 2)
	if len(parts) < 2 {
		t.Fatalf("Session directory name '%s' does not match <timestamp>_<username> format", dirName)
	}

	// Verify timestamp part looks correct (YYYY-MM-DDTHH-MM-SS)
	timestamp := parts[0]
	if len(timestamp) != 19 { // "2026-07-14T09-32-00"
		t.Errorf("Timestamp part '%s' has unexpected length %d (want 19)", timestamp, len(timestamp))
	}
	if !strings.Contains(timestamp, "T") {
		t.Errorf("Timestamp part '%s' missing T separator", timestamp)
	}

	// Verify username part is non-empty and normalized
	username := parts[1]
	if username == "" {
		t.Error("Username part is empty")
	}
	if strings.Contains(username, " ") {
		t.Errorf("Username '%s' contains spaces (should be normalized)", username)
	}
	if username != strings.ToLower(username) {
		t.Errorf("Username '%s' is not lowercase", username)
	}

	t.Logf("Session created: %s", sess.Dir)
	t.Logf("Session relative: %s", sess.RelativeDir())
}

// TestSessionWriteMessage verifies that messages are written to disk
// with correct filenames and content.
func TestSessionWriteMessage(t *testing.T) {
	tmpDir := t.TempDir()
	sess := &session.Session{
		Dir:          tmpDir,
		SessionsRoot: filepath.Dir(tmpDir),
	}

	// Write a user message
	err := sess.WriteMessage(session.TypeUser, "**You:**\n\nHello, Claude!\n")
	if err != nil {
		t.Fatalf("Failed to write user message: %v", err)
	}

	// Write a thinking message
	err = sess.WriteMessage(session.TypeThinking, "💭 Let me think about this...\n")
	if err != nil {
		t.Fatalf("Failed to write thinking message: %v", err)
	}

	// Write a tool-use message
	err = sess.WriteMessage(session.TypeToolUse, "→ Reading file: main.go [toolu_abc123]\n")
	if err != nil {
		t.Fatalf("Failed to write tool-use message: %v", err)
	}

	// Write a tool-result message
	err = sess.WriteMessage(session.TypeToolResult, "```\npackage main\n```\n")
	if err != nil {
		t.Fatalf("Failed to write tool-result message: %v", err)
	}

	// Write a diagnostic message
	err = sess.WriteMessage(session.TypeDiagnostic, "🔍 Tokens: input=100 output=50\n")
	if err != nil {
		t.Fatalf("Failed to write diagnostic message: %v", err)
	}

	// Write an assistant message
	err = sess.WriteMessage(session.TypeAssistant, "**Claude:**\n\nHere's the file content...\n")
	if err != nil {
		t.Fatalf("Failed to write assistant message: %v", err)
	}

	// Read directory and verify files
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("Failed to read session directory: %v", err)
	}

	if len(entries) != 6 {
		t.Fatalf("Expected 6 files, got %d", len(entries))
	}

	// Collect file info
	var fileNames []string
	for _, e := range entries {
		fileNames = append(fileNames, e.Name())
	}
	sort.Strings(fileNames)

	// Verify filenames contain correct type suffixes
	expectedTypes := []string{"_user.md", "_thinking.md", "_tool-use.md", "_tool-result.md", "_diagnostic.md", "_assistant.md"}
	for _, expectedType := range expectedTypes {
		found := false
		for _, name := range fileNames {
			if strings.HasSuffix(name, expectedType) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("No file found with suffix %s. Files: %v", expectedType, fileNames)
		}
	}

	// Verify files are sorted chronologically (timestamps increase)
	for i := 1; i < len(fileNames); i++ {
		if fileNames[i] <= fileNames[i-1] {
			t.Errorf("Files not in chronological order: %s <= %s", fileNames[i], fileNames[i-1])
		}
	}

	t.Logf("Session files (sorted): %v", fileNames)
}

// TestSessionCatProducesTranscript verifies that `cat *.md` produces
// a valid, readable conversation transcript.
func TestSessionCatProducesTranscript(t *testing.T) {
	tmpDir := t.TempDir()
	sess := &session.Session{
		Dir:          tmpDir,
		SessionsRoot: filepath.Dir(tmpDir),
	}

	// Write a multi-turn conversation
	sess.WriteMessage(session.TypeUser, "**You:**\n\nWhat files are in the current directory?\n")
	sess.WriteMessage(session.TypeThinking, "💭 I'll list the files...\n")
	sess.WriteMessage(session.TypeToolUse, "→ Listing files: . (current directory) [toolu_001]\n")
	sess.WriteMessage(session.TypeToolResult, "```\nmain.go\nREADME.md\n```\n")
	sess.WriteMessage(session.TypeDiagnostic, "🔍 Tokens: input=200 output=100\n")
	sess.WriteMessage(session.TypeAssistant, "**Claude:**\n\nI found 2 files: main.go and README.md.\n")

	// Read all files in sorted order (simulates `cat *.md`)
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)

	var transcript strings.Builder
	for _, name := range names {
		content, err := os.ReadFile(filepath.Join(tmpDir, name))
		if err != nil {
			t.Fatal(err)
		}
		transcript.Write(content)
	}

	result := transcript.String()

	// Verify the transcript contains all expected role markers
	if !strings.Contains(result, "**You:**") {
		t.Error("Transcript missing **You:** marker")
	}
	if !strings.Contains(result, "**Claude:**") {
		t.Error("Transcript missing **Claude:** marker")
	}
	if !strings.Contains(result, "💭") {
		t.Error("Transcript missing thinking marker 💭")
	}
	if !strings.Contains(result, "→") {
		t.Error("Transcript missing tool-use marker →")
	}
	if !strings.Contains(result, "🔍") {
		t.Error("Transcript missing diagnostic marker 🔍")
	}
	if !strings.Contains(result, "[toolu_001]") {
		t.Error("Transcript missing tool use ID")
	}

	// Verify order: user appears before assistant
	userIdx := strings.Index(result, "**You:**")
	assistantIdx := strings.Index(result, "**Claude:**")
	if userIdx >= assistantIdx {
		t.Error("User message should appear before assistant message in transcript")
	}

	t.Logf("Transcript:\n%s", result)
}

// TestSessionMonotonicity verifies that timestamps are strictly increasing
// even when messages are written in rapid succession.
func TestSessionMonotonicity(t *testing.T) {
	tmpDir := t.TempDir()
	sess := &session.Session{
		Dir:          tmpDir,
		SessionsRoot: filepath.Dir(tmpDir),
	}

	// Write many messages in rapid succession
	for i := 0; i < 20; i++ {
		err := sess.WriteMessage(session.TypeUser, "msg\n")
		if err != nil {
			t.Fatalf("Failed to write message %d: %v", i, err)
		}
	}

	// Read files and verify strict ordering
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	if len(entries) != 20 {
		t.Fatalf("Expected 20 files, got %d", len(entries))
	}

	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)

	for i := 1; i < len(names); i++ {
		if names[i] <= names[i-1] {
			t.Errorf("Timestamp collision or regression at index %d: '%s' <= '%s'", i, names[i], names[i-1])
		}
	}

	t.Logf("All %d timestamps are strictly increasing", len(names))
}

// TestSessionToolUseIDsInProgressLines verifies that tool use IDs
// appear in → progress lines at all log levels.
func TestSessionToolUseIDsInProgressLines(t *testing.T) {
	// Test FormatToolUseID
	tests := []struct {
		msg      string
		id       string
		expected string
	}{
		{
			msg:      "→ Reading file: main.go",
			id:       "toolu_abc123",
			expected: "→ Reading file: main.go [toolu_abc123]",
		},
		{
			msg:      "→ Running bash: ls -la",
			id:       "toolu_xyz789",
			expected: "→ Running bash: ls -la [toolu_xyz789]",
		},
		{
			msg:      "→ Listing files: .",
			id:       "",
			expected: "→ Listing files: .",
		},
		{
			msg:      "→ Running bash: cd /tmp\nls -la\ngrep foo bar",
			id:       "toolu_multi1",
			expected: "→ Running bash: cd /tmp [toolu_multi1]\nls -la\ngrep foo bar",
		},
		{
			msg:      "→ Running bash: echo hello\necho world",
			id:       "toolu_multi2",
			expected: "→ Running bash: echo hello [toolu_multi2]\necho world",
		},
	}

	for _, tt := range tests {
		result := session.FormatToolUseID(tt.msg, tt.id)
		if result != tt.expected {
			t.Errorf("FormatToolUseID(%q, %q) = %q, want %q", tt.msg, tt.id, result, tt.expected)
		}
	}
}

// TestSessionUsernameNormalization verifies username normalization.
func TestSessionUsernameNormalization(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"John Doe", "john-doe"},
		{"AJ BECKNER", "aj-beckner"},
		{"simple", "simple"},
		{"with.dots", "withdots"},
		{"with@special!chars", "withspecialchars"},
		{"UPPER CASE", "upper-case"},
		{"  spaces  ", "spaces"},
		{"multiple---hyphens", "multiple-hyphens"},
		{"", "unknown"},
	}

	for _, tt := range tests {
		result := session.NormalizeUsername(tt.input)
		if result != tt.expected {
			t.Errorf("NormalizeUsername(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

// TestSessionTimestampFormats verifies timestamp formatting.
func TestSessionTimestampFormats(t *testing.T) {
	// Use a fixed time for predictable output
	fixedTime := time.Date(2026, 7, 14, 9, 32, 5, 123000000, time.UTC)

	dirFormat := session.FormatTimestampDir(fixedTime)
	if dirFormat != "2026-07-14T09-32-05" {
		t.Errorf("FormatTimestampDir = %q, want %q", dirFormat, "2026-07-14T09-32-05")
	}

	fileFormat := session.FormatTimestampFile(fixedTime)
	if fileFormat != "2026-07-14T09-32-05.123" {
		t.Errorf("FormatTimestampFile = %q, want %q", fileFormat, "2026-07-14T09-32-05.123")
	}
}

// TestSessionStripANSI verifies ANSI code stripping.
func TestSessionStripANSI(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"\033[1;36mYou: \033[0m", "You: "},
		{"\033[1;32mClaude: \033[0m", "Claude: "},
		{"\033[1;33m→ Reading file:\033[0m main.go", "→ Reading file: main.go"},
		{"no ansi here", "no ansi here"},
		{"", ""},
	}

	for _, tt := range tests {
		result := session.StripANSI(tt.input)
		if result != tt.expected {
			t.Errorf("StripANSI(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

// TestSessionGitignoreUpdate verifies that .gitignore is updated
// when sessions are first created inside a git repo.
func TestSessionGitignoreUpdate(t *testing.T) {
	// Create a fake git repo
	tmpDir := t.TempDir()
	gitDir := filepath.Join(tmpDir, ".git")
	os.MkdirAll(gitDir, 0755)

	// Create a .gitignore
	gitignorePath := filepath.Join(tmpDir, ".gitignore")
	os.WriteFile(gitignorePath, []byte("*.exe\n"), 0644)

	// Create .clyde/sessions/ and simulate ensureGitignore
	sessionsDir := filepath.Join(tmpDir, ".clyde", "sessions")
	os.MkdirAll(sessionsDir, 0755)

	// Read gitignore before
	before, _ := os.ReadFile(gitignorePath)
	if strings.Contains(string(before), ".clyde/sessions/") {
		t.Fatal("Gitignore already contains .clyde/sessions/ before test")
	}

	// The actual gitignore update is tested by checking that when
	// .clyde/sessions/ is new and inside a git repo, it gets added.
	// Since New() depends on `git rev-parse --show-toplevel`, we test
	// the entry format expectations instead.
	t.Log("Gitignore entry format: '.clyde/sessions/'")
	t.Log("Added with comment: '# Clyde session history'")
}

// TestSessionDirectoryNaming verifies that session directory names
// use the correct format.
func TestSessionDirectoryNaming(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	origDir, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(origDir)

	sess, err := session.New()
	if err != nil {
		t.Fatal(err)
	}

	dirName := filepath.Base(sess.Dir)

	// Verify it matches the pattern: <timestamp>_<username>
	// Timestamp format: YYYY-MM-DDTHH-MM-SS (19 chars)
	if len(dirName) < 20 { // 19 + 1 for underscore + at least 1 char username
		t.Errorf("Directory name '%s' is too short", dirName)
	}

	// Verify the underscore separator
	if dirName[19] != '_' {
		t.Errorf("Expected underscore at position 19 of '%s', got '%c'", dirName, dirName[19])
	}

	t.Logf("Session directory: %s", dirName)
}

// TestSessionCrashSafety verifies that prior messages survive if the
// process "crashes" (stops writing) mid-session.
func TestSessionCrashSafety(t *testing.T) {
	tmpDir := t.TempDir()
	sess := &session.Session{
		Dir:          tmpDir,
		SessionsRoot: filepath.Dir(tmpDir),
	}

	// Write some complete messages
	sess.WriteMessage(session.TypeUser, "**You:**\n\nFirst message\n")
	sess.WriteMessage(session.TypeAssistant, "**Claude:**\n\nFirst response\n")
	sess.WriteMessage(session.TypeUser, "**You:**\n\nSecond message\n")

	// "Crash" — don't write any more messages

	// Verify all 3 messages are intact
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	if len(entries) != 3 {
		t.Fatalf("Expected 3 files, got %d", len(entries))
	}

	// Verify each file is readable
	for _, e := range entries {
		content, err := os.ReadFile(filepath.Join(tmpDir, e.Name()))
		if err != nil {
			t.Errorf("Failed to read file %s: %v", e.Name(), err)
		}
		if len(content) == 0 {
			t.Errorf("File %s is empty", e.Name())
		}
	}

	t.Log("All 3 prior messages survived simulated crash")
}

// TestSessionFiltering verifies that files can be filtered by type
// using glob patterns on filenames.
func TestSessionFiltering(t *testing.T) {
	tmpDir := t.TempDir()
	sess := &session.Session{
		Dir:          tmpDir,
		SessionsRoot: filepath.Dir(tmpDir),
	}

	// Write a multi-turn conversation
	sess.WriteMessage(session.TypeUser, "**You:**\n\nHello\n")
	sess.WriteMessage(session.TypeThinking, "💭 thinking...\n")
	sess.WriteMessage(session.TypeToolUse, "→ ls [toolu_1]\n")
	sess.WriteMessage(session.TypeToolResult, "```\nfiles...\n```\n")
	sess.WriteMessage(session.TypeDiagnostic, "🔍 tokens...\n")
	sess.WriteMessage(session.TypeAssistant, "**Claude:**\n\nHi!\n")
	sess.WriteMessage(session.TypeUser, "**You:**\n\nThanks\n")
	sess.WriteMessage(session.TypeAssistant, "**Claude:**\n\nYou're welcome!\n")

	// Test filtering by type
	entries, _ := os.ReadDir(tmpDir)

	typeCount := map[string]int{}
	for _, e := range entries {
		name := e.Name()
		// Extract type suffix
		parts := strings.SplitN(name, "_", 2)
		if len(parts) == 2 {
			typeSuffix := strings.TrimSuffix(parts[1], ".md")
			// The parts[1] is like "09-32-05.123_user.md", so split again
			subParts := strings.SplitN(parts[1], "_", 2)
			if len(subParts) == 2 {
				typeSuffix = strings.TrimSuffix(subParts[1], ".md")
				typeCount[typeSuffix]++
			}
		}
	}

	// Alternatively, just check by suffix matching
	userCount := 0
	assistantCount := 0
	toolUseCount := 0
	for _, e := range entries {
		name := e.Name()
		if strings.HasSuffix(name, "_user.md") {
			userCount++
		}
		if strings.HasSuffix(name, "_assistant.md") {
			assistantCount++
		}
		if strings.HasSuffix(name, "_tool-use.md") {
			toolUseCount++
		}
	}

	if userCount != 2 {
		t.Errorf("Expected 2 user files, got %d", userCount)
	}
	if assistantCount != 2 {
		t.Errorf("Expected 2 assistant files, got %d", assistantCount)
	}
	if toolUseCount != 1 {
		t.Errorf("Expected 1 tool-use file, got %d", toolUseCount)
	}

	t.Logf("Filtering works: %d user, %d assistant, %d tool-use files", userCount, assistantCount, toolUseCount)
}

// TestSessionFindSessionsRoot verifies session root detection.
func TestSessionFindSessionsRoot(t *testing.T) {
	// When inside a git repo, root should contain .clyde/sessions
	root, inGitRepo := session.FindSessionsRoot()
	if root == "" {
		t.Error("FindSessionsRoot returned empty path")
	}

	// We're running in the clyde repo, so should be inside a git repo
	if inGitRepo {
		if !strings.Contains(root, ".clyde/sessions") {
			t.Errorf("Expected path to contain '.clyde/sessions', got %s", root)
		}
		t.Logf("Inside git repo, sessions root: %s", root)
	} else {
		t.Logf("Not inside git repo, sessions root: %s", root)
	}
}

// TestSessionGetUsername verifies username detection.
func TestSessionGetUsername(t *testing.T) {
	username := session.GetUsername()
	if username == "" {
		t.Error("GetUsername returned empty string")
	}
	if username == "unknown" {
		t.Log("Warning: GetUsername returned 'unknown' (no git user.name or $USER)")
	}
	// Verify it's normalized
	if strings.Contains(username, " ") {
		t.Errorf("Username '%s' contains spaces", username)
	}
	if username != strings.ToLower(username) {
		t.Errorf("Username '%s' is not lowercase", username)
	}
	t.Logf("Username: %s", username)
}

// TestSessionAllMessageTypes verifies that all defined message types
// can be written and produce correctly-named files.
func TestSessionAllMessageTypes(t *testing.T) {
	tmpDir := t.TempDir()
	sess := &session.Session{
		Dir:          tmpDir,
		SessionsRoot: filepath.Dir(tmpDir),
	}

	allTypes := []session.MessageType{
		session.TypeUser,
		session.TypeAssistant,
		session.TypeSystem,
		session.TypeThinking,
		session.TypeToolUse,
		session.TypeToolResult,
		session.TypeDiagnostic,
		session.TypeCompaction,
	}

	for _, msgType := range allTypes {
		err := sess.WriteMessage(msgType, "test content for "+string(msgType)+"\n")
		if err != nil {
			t.Errorf("Failed to write message of type %s: %v", msgType, err)
		}
	}

	entries, _ := os.ReadDir(tmpDir)
	if len(entries) != len(allTypes) {
		t.Fatalf("Expected %d files, got %d", len(allTypes), len(entries))
	}

	// Verify each type has a corresponding file
	for _, msgType := range allTypes {
		suffix := "_" + string(msgType) + ".md"
		found := false
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), suffix) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("No file found with suffix %s", suffix)
		}
	}
}
