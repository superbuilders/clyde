package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
)

// ── WT-2 Unit Tests: parseWorktreeList ──

func TestParseWorktreeList(t *testing.T) {
	input := `worktree /Users/aj/code/myproject/myproject
HEAD abc123def456
branch refs/heads/main

worktree /Users/aj/code/myproject/feature-x
HEAD def456abc789
branch refs/heads/feature-x

worktree /Users/aj/code/myproject/bugfix-y
HEAD 111222333444
branch refs/heads/bugfix-y
`
	entries := parseWorktreeList(input)
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}

	// Entry 1
	if entries[0].Path != "/Users/aj/code/myproject/myproject" {
		t.Errorf("entry 0 path = %q", entries[0].Path)
	}
	if entries[0].Branch != "main" {
		t.Errorf("entry 0 branch = %q (want 'main')", entries[0].Branch)
	}
	if entries[0].Name != "myproject" {
		t.Errorf("entry 0 name = %q (want 'myproject')", entries[0].Name)
	}

	// Entry 2
	if entries[1].Path != "/Users/aj/code/myproject/feature-x" {
		t.Errorf("entry 1 path = %q", entries[1].Path)
	}
	if entries[1].Branch != "feature-x" {
		t.Errorf("entry 1 branch = %q", entries[1].Branch)
	}

	// Entry 3
	if entries[2].Branch != "bugfix-y" {
		t.Errorf("entry 2 branch = %q", entries[2].Branch)
	}
}

func TestParseWorktreeListNoTrailingNewline(t *testing.T) {
	input := `worktree /tmp/repo
HEAD abc123
branch refs/heads/main`
	entries := parseWorktreeList(input)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Branch != "main" {
		t.Errorf("branch = %q", entries[0].Branch)
	}
}

func TestParseWorktreeListDetached(t *testing.T) {
	// A detached worktree has no branch line
	input := `worktree /tmp/repo
HEAD abc123
branch refs/heads/main

worktree /tmp/repo-detached
HEAD def456
detached
`
	entries := parseWorktreeList(input)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[1].Branch != "" {
		t.Errorf("detached worktree should have empty branch, got %q", entries[1].Branch)
	}
}

// ── WT-2 Integration Tests: detectWorktreeGroup ──

// setupWorktreeRepo creates a temp parent dir with a main repo and a linked worktree.
// Returns (parentDir, mainDir, worktreeDir, cleanup).
func setupWorktreeRepo(t *testing.T) (string, string, string) {
	t.Helper()
	parentDir := t.TempDir()
	// Resolve symlinks for macOS /var → /private/var
	parentDir, _ = filepath.EvalSymlinks(parentDir)
	mainDir := filepath.Join(parentDir, "main-repo")
	os.MkdirAll(mainDir, 0755)

	// Init git repo
	run(t, mainDir, "git", "init")
	run(t, mainDir, "git", "config", "user.email", "test@test.com")
	run(t, mainDir, "git", "config", "user.name", "Test")
	// Create initial commit (needed for worktree add)
	os.WriteFile(filepath.Join(mainDir, "README.md"), []byte("hello"), 0644)
	run(t, mainDir, "git", "add", ".")
	run(t, mainDir, "git", "commit", "-m", "init")

	// Create linked worktree
	wtDir := filepath.Join(parentDir, "feature-branch")
	run(t, mainDir, "git", "worktree", "add", "-b", "feature-branch", wtDir)

	return parentDir, mainDir, wtDir
}

func run(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("command %s %v failed in %s: %v\n%s", name, args, dir, err, out)
	}
}

func TestDetectWorktreeGroup_FromMainWorktree(t *testing.T) {
	parentDir, mainDir, _ := setupWorktreeRepo(t)

	group := detectWorktreeGroup(mainDir)
	if group == nil {
		t.Fatal("expected non-nil WorktreeGroup from main worktree")
	}
	if group.ParentDir != parentDir {
		t.Errorf("ParentDir = %q, want %q", group.ParentDir, parentDir)
	}
	if group.ParentName != filepath.Base(parentDir) {
		t.Errorf("ParentName = %q", group.ParentName)
	}
	if len(group.Worktrees) != 2 {
		t.Fatalf("expected 2 worktrees, got %d", len(group.Worktrees))
	}
}

func TestDetectWorktreeGroup_FromLinkedWorktree(t *testing.T) {
	parentDir, _, wtDir := setupWorktreeRepo(t)

	group := detectWorktreeGroup(wtDir)
	if group == nil {
		t.Fatal("expected non-nil WorktreeGroup from linked worktree")
	}
	if group.ParentDir != parentDir {
		t.Errorf("ParentDir = %q, want %q", group.ParentDir, parentDir)
	}
	if len(group.Worktrees) != 2 {
		t.Fatalf("expected 2 worktrees, got %d", len(group.Worktrees))
	}
}

func TestDetectWorktreeGroup_IdenticalFromBothSides(t *testing.T) {
	_, mainDir, wtDir := setupWorktreeRepo(t)

	g1 := detectWorktreeGroup(mainDir)
	g2 := detectWorktreeGroup(wtDir)

	if g1 == nil || g2 == nil {
		t.Fatal("expected non-nil groups from both sides")
	}
	if g1.ParentDir != g2.ParentDir {
		t.Errorf("ParentDir mismatch: %q vs %q", g1.ParentDir, g2.ParentDir)
	}
	if len(g1.Worktrees) != len(g2.Worktrees) {
		t.Errorf("Worktree count mismatch: %d vs %d", len(g1.Worktrees), len(g2.Worktrees))
	}
}

func TestDetectWorktreeGroup_NormalRepo(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "git", "init")
	run(t, dir, "git", "config", "user.email", "test@test.com")
	run(t, dir, "git", "config", "user.name", "Test")
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("hello"), 0644)
	run(t, dir, "git", "add", ".")
	run(t, dir, "git", "commit", "-m", "init")

	group := detectWorktreeGroup(dir)
	if group != nil {
		t.Error("expected nil for normal repo without worktrees")
	}
}

func TestDetectWorktreeGroup_NotGitDir(t *testing.T) {
	dir := t.TempDir()
	group := detectWorktreeGroup(dir)
	if group != nil {
		t.Error("expected nil for non-git directory")
	}
}

func TestDetectWorktreeGroup_NonSiblingWorktrees(t *testing.T) {
	parentDir := t.TempDir()
	mainDir := filepath.Join(parentDir, "main-repo")
	os.MkdirAll(mainDir, 0755)
	run(t, mainDir, "git", "init")
	run(t, mainDir, "git", "config", "user.email", "test@test.com")
	run(t, mainDir, "git", "config", "user.name", "Test")
	os.WriteFile(filepath.Join(mainDir, "README.md"), []byte("hello"), 0644)
	run(t, mainDir, "git", "add", ".")
	run(t, mainDir, "git", "commit", "-m", "init")

	// Create worktree in a DIFFERENT parent directory (non-sibling)
	otherParent := t.TempDir()
	wtDir := filepath.Join(otherParent, "non-sibling-wt")
	run(t, mainDir, "git", "worktree", "add", "-b", "non-sibling", wtDir)

	group := detectWorktreeGroup(mainDir)
	if group != nil {
		t.Error("expected nil for non-sibling worktrees")
	}
}

// ── WT-1/3/4/5/6 E2E Tests: full server tests ──

// startTestServer starts the session viewer on a random port with a custom HOME.
// Returns the base URL and a cleanup function.
func startTestServer(t *testing.T, homeDir string) (string, func()) {
	t.Helper()

	// Find a free port
	port := 18700 + os.Getpid()%1000

	// Override global state for testing
	origCachePath := cachePath

	// Set up a clean cache
	cacheMu.Lock()
	cache = newEmptyCache()
	cacheMu.Unlock()

	cacheDir := filepath.Join(homeDir, ".clyde")
	os.MkdirAll(cacheDir, 0755)
	cachePath = filepath.Join(cacheDir, "viewer-cache-test.json")

	baseURL := fmt.Sprintf("http://localhost:%d", port)

	// Start echo server in background
	e := setupEcho()
	go func() {
		e.Start(fmt.Sprintf(":%d", port))
	}()

	// Wait for server to be ready
	for i := 0; i < 30; i++ {
		time.Sleep(100 * time.Millisecond)
		resp, err := http.Get(baseURL + "/api/sessions?days=30")
		if err == nil {
			resp.Body.Close()
			break
		}
	}

	cleanup := func() {
		e.Close()
		cachePath = origCachePath
	}
	return baseURL, cleanup
}

// setupEcho creates the Echo server with all routes (extracted for testability)
func setupEcho() *echo.Echo {
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true
	api := e.Group("/api")
	api.GET("/sessions", getSessions)
	api.GET("/sessions/:id/messages", getSessionMessages)
	api.POST("/sessions/:id/messages", postSessionMessage)
	api.POST("/sessions/:id/stop", stopSession)
	api.GET("/sessions/:id/status", getSessionStatus)
	api.PATCH("/sessions/:id", patchSession)
	api.POST("/sessions/scan", triggerScan)
	api.POST("/sessions/new", createSession)
	api.POST("/upload", uploadFile)
	api.GET("/projects", getProjects)
	api.POST("/worktrees", createWorktree)
	api.POST("/worktrees/delete", deleteWorktreeHandler)
	api.DELETE("/sessions/:id/messages/:filename", deleteSessionMessage)
	api.POST("/sessions/mark-all-read", markAllRead)
	api.POST("/sessions/:id/open-terminal", openInTerminal)
	api.GET("/preferences", func(c echo.Context) error {
		cacheMu.RLock()
		defer cacheMu.RUnlock()
		return c.JSON(http.StatusOK, cache.Preferences)
	})
	api.PATCH("/preferences", patchPreferences)
	return e
}

func httpGet(t *testing.T, url string) (int, []byte) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s failed: %v", url, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, body
}

func httpPost(t *testing.T, url string, jsonBody string) (int, []byte) {
	t.Helper()
	resp, err := http.Post(url, "application/json", strings.NewReader(jsonBody))
	if err != nil {
		t.Fatalf("POST %s failed: %v", url, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, body
}

func TestE2E_WT3_SessionsAnnotatedWithWorktreeInfo(t *testing.T) {
	parentDir, mainDir, wtDir := setupWorktreeRepo(t)

	// Create sessions in both worktrees
	mainSessRoot := filepath.Join(mainDir, ".clyde", "sessions")
	wtSessRoot := filepath.Join(wtDir, ".clyde", "sessions")
	os.MkdirAll(filepath.Join(mainSessRoot, "2026-07-16T10-00-00_testuser"), 0755)
	os.MkdirAll(filepath.Join(wtSessRoot, "2026-07-16T11-00-00_testuser"), 0755)

	// Write a message file so last_modified is populated
	os.WriteFile(filepath.Join(mainSessRoot, "2026-07-16T10-00-00_testuser", "2026-07-16T10-00-01.000_user.md"), []byte("hello"), 0644)
	os.WriteFile(filepath.Join(wtSessRoot, "2026-07-16T11-00-00_testuser", "2026-07-16T11-00-01.000_user.md"), []byte("hello"), 0644)

	homeDir := t.TempDir()
	// Create code dir structure that discoverProjectDirs expects
	codeDir := filepath.Join(homeDir, "code")
	os.MkdirAll(codeDir, 0755)
	// Symlink our test dirs into ~/code so they're discoverable
	os.Symlink(mainDir, filepath.Join(codeDir, filepath.Base(mainDir)))
	os.Symlink(wtDir, filepath.Join(codeDir, filepath.Base(wtDir)))

	baseURL, cleanup := startTestServer(t, homeDir)
	defer cleanup()

	// Manually inject into cache by running scan logic
	// We'll directly put sessions in the cache for this test
	cacheMu.Lock()
	bc := make(map[string]string)
	group := detectWorktreeGroup(mainDir)
	if group == nil {
		t.Fatal("detectWorktreeGroup returned nil")
	}
	cache.Sessions[cacheKey(mainDir, "2026-07-16T10-00-00_testuser")] = &CachedSession{
		ID: "2026-07-16T10-00-00_testuser", CWD: mainDir,
		Project: filepath.Base(mainDir), Branch: getBranch(mainDir, bc),
		MessageCount: 1, LastModified: time.Now().Format(time.RFC3339),
		Read: true, LastReadCount: 1,
		WorktreeParent:     group.ParentDir,
		WorktreeParentName: group.ParentName,
	}
	cache.Sessions[cacheKey(wtDir, "2026-07-16T11-00-00_testuser")] = &CachedSession{
		ID: "2026-07-16T11-00-00_testuser", CWD: wtDir,
		Project: filepath.Base(wtDir), Branch: getBranch(wtDir, bc),
		MessageCount: 1, LastModified: time.Now().Format(time.RFC3339),
		Read: true, LastReadCount: 1,
		WorktreeParent:     group.ParentDir,
		WorktreeParentName: group.ParentName,
	}
	cacheMu.Unlock()

	// GET /api/sessions
	status, body := httpGet(t, baseURL+"/api/sessions?days=30")
	if status != 200 {
		t.Fatalf("GET /api/sessions returned %d: %s", status, body)
	}

	var resp struct {
		Sessions []SessionResponse `json:"sessions"`
	}
	json.Unmarshal(body, &resp)
	if len(resp.Sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(resp.Sessions))
	}

	for _, s := range resp.Sessions {
		if s.WorktreeParent == "" {
			t.Errorf("session %s missing worktree_parent", s.ID)
		}
		if s.WorktreeParentName == "" {
			t.Errorf("session %s missing worktree_parent_name", s.ID)
		}
		if s.WorktreeParent != parentDir {
			t.Errorf("session %s worktree_parent = %q, want %q", s.ID, s.WorktreeParent, parentDir)
		}
	}
}

func TestE2E_WT5_CreateWorktree(t *testing.T) {
	parentDir, mainDir, _ := setupWorktreeRepo(t)

	homeDir := t.TempDir()
	baseURL, cleanup := startTestServer(t, homeDir)
	defer cleanup()

	// Test: create a new worktree
	payload := fmt.Sprintf(`{"parent_path": %q, "branch_name": "new-feature"}`, parentDir)
	status, body := httpPost(t, baseURL+"/api/worktrees", payload)
	if status != 200 {
		t.Fatalf("POST /api/worktrees returned %d: %s", status, body)
	}

	var resp map[string]interface{}
	json.Unmarshal(body, &resp)
	if resp["status"] != "ok" {
		t.Errorf("expected status ok, got %v", resp["status"])
	}
	expectedPath := filepath.Join(parentDir, "new-feature")
	if resp["path"] != expectedPath {
		t.Errorf("path = %v, want %v", resp["path"], expectedPath)
	}
	if resp["branch"] != "new-feature" {
		t.Errorf("branch = %v", resp["branch"])
	}

	// Verify the worktree was actually created
	if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
		t.Error("worktree directory was not created")
	}
	// Verify .clyde/sessions/ was created
	if _, err := os.Stat(filepath.Join(expectedPath, ".clyde", "sessions")); os.IsNotExist(err) {
		t.Error(".clyde/sessions/ was not created in new worktree")
	}

	// Verify git recognizes it
	out, err := exec.Command("git", "-C", mainDir, "worktree", "list", "--porcelain").Output()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "new-feature") {
		t.Error("git worktree list does not show new-feature")
	}
}

func TestE2E_WT5_CreateWorktree_SlashInBranchName(t *testing.T) {
	parentDir, mainDir, _ := setupWorktreeRepo(t)

	homeDir := t.TempDir()
	baseURL, cleanup := startTestServer(t, homeDir)
	defer cleanup()

	// Branch name with "/" should create a flat sibling directory with "-" instead
	payload := fmt.Sprintf(`{"parent_path": %q, "branch_name": "feature/payments"}`, parentDir)
	status, body := httpPost(t, baseURL+"/api/worktrees", payload)
	if status != 200 {
		t.Fatalf("POST /api/worktrees returned %d: %s", status, body)
	}

	var resp map[string]interface{}
	json.Unmarshal(body, &resp)
	if resp["status"] != "ok" {
		t.Errorf("expected status ok, got %v", resp["status"])
	}

	// Directory name should have "/" replaced with "-"
	expectedPath := filepath.Join(parentDir, "feature-payments")
	if resp["path"] != expectedPath {
		t.Errorf("path = %v, want %v", resp["path"], expectedPath)
	}
	// Branch name should be preserved as-is
	if resp["branch"] != "feature/payments" {
		t.Errorf("branch = %v, want feature/payments", resp["branch"])
	}

	// Verify the worktree was created as a flat sibling
	if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
		t.Error("worktree directory was not created at expected flat path")
	}
	// Should NOT have created nested dirs
	nestedPath := filepath.Join(parentDir, "feature", "payments")
	if _, err := os.Stat(nestedPath); err == nil {
		t.Error("worktree created as nested directory — should be flat sibling")
	}

	// Verify git recognizes the branch
	out, err := exec.Command("git", "-C", mainDir, "worktree", "list", "--porcelain").Output()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "feature/payments") {
		t.Error("git worktree list does not show feature/payments branch")
	}

	// Verify all worktrees are still siblings (grouping should work)
	group := detectWorktreeGroup(mainDir)
	if group == nil {
		t.Fatal("worktree group detection failed after creating slash-named branch")
	}
	if len(group.Worktrees) < 3 {
		t.Errorf("expected at least 3 worktrees in group, got %d", len(group.Worktrees))
	}
}

func TestE2E_WT5_CreateWorktree_Validation(t *testing.T) {
	parentDir, _, _ := setupWorktreeRepo(t)

	homeDir := t.TempDir()
	baseURL, cleanup := startTestServer(t, homeDir)
	defer cleanup()

	// Test: missing fields
	status, _ := httpPost(t, baseURL+"/api/worktrees", `{"parent_path": ""}`)
	if status != 400 {
		t.Errorf("expected 400 for empty fields, got %d", status)
	}

	// Test: invalid branch name with spaces
	payload := fmt.Sprintf(`{"parent_path": %q, "branch_name": "bad name"}`, parentDir)
	status, _ = httpPost(t, baseURL+"/api/worktrees", payload)
	if status != 400 {
		t.Errorf("expected 400 for branch with spaces, got %d", status)
	}

	// Test: invalid branch name with ..
	payload = fmt.Sprintf(`{"parent_path": %q, "branch_name": "bad..name"}`, parentDir)
	status, _ = httpPost(t, baseURL+"/api/worktrees", payload)
	if status != 400 {
		t.Errorf("expected 400 for branch with .., got %d", status)
	}

	// Test: no git dir in parent
	emptyDir := t.TempDir()
	payload = fmt.Sprintf(`{"parent_path": %q, "branch_name": "anything"}`, emptyDir)
	status, _ = httpPost(t, baseURL+"/api/worktrees", payload)
	if status != 400 {
		t.Errorf("expected 400 for no git dir, got %d", status)
	}
}

func TestE2E_WT5_CreateWorktree_DuplicateBranch(t *testing.T) {
	parentDir, _, _ := setupWorktreeRepo(t)

	homeDir := t.TempDir()
	baseURL, cleanup := startTestServer(t, homeDir)
	defer cleanup()

	// feature-branch already exists from setupWorktreeRepo
	payload := fmt.Sprintf(`{"parent_path": %q, "branch_name": "feature-branch"}`, parentDir)
	status, body := httpPost(t, baseURL+"/api/worktrees", payload)
	if status != 409 {
		t.Errorf("expected 409 for duplicate branch, got %d: %s", status, body)
	}
}

func TestE2E_WT6_GetProjectsWithWorktreeInfo(t *testing.T) {
	homeDir := t.TempDir()
	homeDir, _ = filepath.EvalSymlinks(homeDir)

	// Set HOME so discoverProjectDirs finds our test dirs
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", homeDir)
	defer os.Setenv("HOME", origHome)

	// Create worktree repos directly inside ~/code/
	codeDir := filepath.Join(homeDir, "code")
	parentDir := filepath.Join(codeDir, "myproject")
	mainDir := filepath.Join(parentDir, "main-repo")
	os.MkdirAll(mainDir, 0755)

	run(t, mainDir, "git", "init")
	run(t, mainDir, "git", "config", "user.email", "test@test.com")
	run(t, mainDir, "git", "config", "user.name", "Test")
	os.WriteFile(filepath.Join(mainDir, "README.md"), []byte("hello"), 0644)
	run(t, mainDir, "git", "add", ".")
	run(t, mainDir, "git", "commit", "-m", "init")

	wtDir := filepath.Join(parentDir, "feature-branch")
	run(t, mainDir, "git", "worktree", "add", "-b", "feature-branch", wtDir)

	// Create .clyde/sessions in both worktrees so they're discoverable
	os.MkdirAll(filepath.Join(mainDir, ".clyde", "sessions"), 0755)
	os.MkdirAll(filepath.Join(wtDir, ".clyde", "sessions"), 0755)

	baseURL, cleanup := startTestServer(t, homeDir)
	defer cleanup()

	status, body := httpGet(t, baseURL+"/api/projects")
	if status != 200 {
		t.Fatalf("GET /api/projects returned %d: %s", status, body)
	}

	var projects []ProjectInfo
	json.Unmarshal(body, &projects)

	// Find projects that are part of our worktree group
	var wtProjects []ProjectInfo
	for _, p := range projects {
		if p.ParentPath == parentDir || p.Path == parentDir {
			wtProjects = append(wtProjects, p)
		}
	}

	if len(wtProjects) == 0 {
		t.Log("Projects returned:", string(body))
		t.Fatal("no projects found with our worktree parent path")
	}

	// At least one should be marked as worktree group
	hasGroup := false
	for _, p := range wtProjects {
		if p.IsWorktreeGroup && len(p.Worktrees) >= 2 {
			hasGroup = true
		}
	}
	if !hasGroup {
		t.Error("no project marked as worktree group with worktrees")
	}
}

// ── WT-1 Test: findSessionsRoot respects existing .clyde/sessions/ ──

func TestWT1_FindSessionsRoot_ExistingClydeDir(t *testing.T) {
	// This test needs to be in the agent/session package, but we can test
	// the concept here by verifying the session-viewer's createSession
	// works with non-git directories that have .clyde/sessions/

	homeDir := t.TempDir()
	nonGitDir := filepath.Join(homeDir, "my-project-parent")
	os.MkdirAll(filepath.Join(nonGitDir, ".clyde", "sessions"), 0755)

	baseURL, cleanup := startTestServer(t, homeDir)
	defer cleanup()

	// Creating a session in a non-git dir should work
	payload := fmt.Sprintf(`{"cwd": %q}`, nonGitDir)
	status, body := httpPost(t, baseURL+"/api/sessions/new", payload)
	if status != 200 {
		t.Fatalf("POST /api/sessions/new returned %d: %s", status, body)
	}

	var resp map[string]string
	json.Unmarshal(body, &resp)
	if resp["status"] != "ok" {
		t.Errorf("expected status ok, got %v", resp["status"])
	}
	if resp["session_id"] == "" {
		t.Error("expected non-empty session_id")
	}
	if resp["cwd"] != nonGitDir {
		t.Errorf("cwd = %q, want %q", resp["cwd"], nonGitDir)
	}

	// Verify the session directory was created on disk
	sessDir := filepath.Join(nonGitDir, ".clyde", "sessions", resp["session_id"])
	if _, err := os.Stat(sessDir); os.IsNotExist(err) {
		t.Error("session directory was not created on disk")
	}
}

// ── Full integration: create worktree, scan, verify sessions API shows it ──

func TestE2E_FullWorktreeFlow(t *testing.T) {
	homeDir := t.TempDir()
	homeDir, _ = filepath.EvalSymlinks(homeDir)

	// Override HOME for discoverProjectDirs
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", homeDir)
	defer os.Setenv("HOME", origHome)

	// Create worktree repos directly inside ~/code/
	codeDir := filepath.Join(homeDir, "code")
	parentDir := filepath.Join(codeDir, "fullflow-project")
	mainDir := filepath.Join(parentDir, "main-repo")
	os.MkdirAll(mainDir, 0755)

	run(t, mainDir, "git", "init")
	run(t, mainDir, "git", "config", "user.email", "test@test.com")
	run(t, mainDir, "git", "config", "user.name", "Test")
	os.WriteFile(filepath.Join(mainDir, "README.md"), []byte("hello"), 0644)
	run(t, mainDir, "git", "add", ".")
	run(t, mainDir, "git", "commit", "-m", "init")

	wtDir := filepath.Join(parentDir, "feature-branch")
	run(t, mainDir, "git", "worktree", "add", "-b", "feature-branch", wtDir)

	// Create sessions in existing worktrees
	os.MkdirAll(filepath.Join(mainDir, ".clyde", "sessions", "2026-07-16T10-00-00_test"), 0755)
	os.WriteFile(filepath.Join(mainDir, ".clyde", "sessions", "2026-07-16T10-00-00_test", "2026-07-16T10-00-01.000_user.md"), []byte("**You:** hello from main"), 0644)

	os.MkdirAll(filepath.Join(wtDir, ".clyde", "sessions", "2026-07-16T11-00-00_test"), 0755)
	os.WriteFile(filepath.Join(wtDir, ".clyde", "sessions", "2026-07-16T11-00-00_test", "2026-07-16T11-00-01.000_user.md"), []byte("**You:** hello from feature"), 0644)

	baseURL, cleanup := startTestServer(t, homeDir)
	defer cleanup()

	// Run a background scan
	httpPost(t, baseURL+"/api/sessions/scan", "{}")
	time.Sleep(2 * time.Second)

	// Check sessions are annotated
	status, body := httpGet(t, baseURL+"/api/sessions?days=0")
	if status != 200 {
		t.Fatalf("GET /api/sessions: %d %s", status, body)
	}

	var sessResp struct {
		Sessions []SessionResponse `json:"sessions"`
	}
	json.Unmarshal(body, &sessResp)

	worktreeSessions := 0
	for _, s := range sessResp.Sessions {
		if s.WorktreeParent == parentDir {
			worktreeSessions++
			if s.WorktreeParentName != filepath.Base(parentDir) {
				t.Errorf("session %s: worktree_parent_name = %q, want %q", s.ID, s.WorktreeParentName, filepath.Base(parentDir))
			}
		}
	}
	if worktreeSessions < 2 {
		t.Errorf("expected at least 2 worktree sessions, found %d (total sessions: %d)", worktreeSessions, len(sessResp.Sessions))
		for _, s := range sessResp.Sessions {
			t.Logf("  session: id=%s cwd=%s project=%s wt_parent=%s", s.ID, s.CWD, s.Project, s.WorktreeParent)
		}
	}

	// Now create a new worktree via API
	payload := fmt.Sprintf(`{"parent_path": %q, "branch_name": "api-created-wt"}`, parentDir)
	status, body = httpPost(t, baseURL+"/api/worktrees", payload)
	if status != 200 {
		t.Fatalf("POST /api/worktrees: %d %s", status, body)
	}

	// Verify it exists
	newWtDir := filepath.Join(parentDir, "api-created-wt")
	if _, err := os.Stat(newWtDir); os.IsNotExist(err) {
		t.Fatal("new worktree directory not created")
	}
	if _, err := os.Stat(filepath.Join(newWtDir, ".clyde", "sessions")); os.IsNotExist(err) {
		t.Fatal("new worktree .clyde/sessions/ not created")
	}

	// Wait for rescan
	time.Sleep(3 * time.Second)

	// Check that projects API shows the worktree group
	status, body = httpGet(t, baseURL+"/api/projects")
	if status != 200 {
		t.Fatalf("GET /api/projects: %d %s", status, body)
	}

	var projects []ProjectInfo
	json.Unmarshal(body, &projects)

	foundNewWt := false
	for _, p := range projects {
		if p.Path == newWtDir {
			foundNewWt = true
			if p.ParentPath != parentDir {
				t.Errorf("new worktree parent_path = %q, want %q", p.ParentPath, parentDir)
			}
		}
	}
	if !foundNewWt {
		t.Log("Projects:", string(body))
		t.Error("new worktree not found in GET /api/projects")
	}
}
