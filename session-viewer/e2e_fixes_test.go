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
)

// ── HTTP helpers ──

func httpDelete(t *testing.T, url string) (int, []byte) {
	t.Helper()
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		t.Fatalf("DELETE request creation failed: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE %s failed: %v", url, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, body
}

// ── Delete Message Tests ──

func TestE2E_DeleteMessage(t *testing.T) {
	homeDir := t.TempDir()
	homeDir, _ = filepath.EvalSymlinks(homeDir)

	projectDir := filepath.Join(homeDir, "test-project")
	sessID := "2026-07-20T10-00-00_testuser"
	sessDir := filepath.Join(projectDir, ".clyde", "sessions", sessID)
	os.MkdirAll(sessDir, 0755)

	// Create test message files
	msgFile1 := "2026-07-20T10-00-01.000_user.md"
	msgFile2 := "2026-07-20T10-00-02.000_assistant.md"
	os.WriteFile(filepath.Join(sessDir, msgFile1), []byte("**You:** hello"), 0644)
	os.WriteFile(filepath.Join(sessDir, msgFile2), []byte("hi back"), 0644)

	baseURL, cleanup := startTestServer(t, homeDir)
	defer cleanup()

	// Inject into cache
	cacheMu.Lock()
	cache.Sessions[cacheKey(projectDir, sessID)] = &CachedSession{
		ID: sessID, CWD: projectDir, Project: "test-project",
		MessageCount: 2, LastModified: time.Now().Format(time.RFC3339), Read: true,
	}
	cacheMu.Unlock()

	// Delete the first message
	url := fmt.Sprintf("%s/api/sessions/%s/messages/%s?cwd=%s",
		baseURL, sessID, msgFile1, projectDir)
	status, body := httpDelete(t, url)
	if status != 200 {
		t.Fatalf("DELETE returned %d: %s", status, body)
	}

	var resp map[string]string
	json.Unmarshal(body, &resp)
	if resp["status"] != "ok" {
		t.Errorf("expected status ok, got %v", resp["status"])
	}

	// Verify file is gone from disk
	if _, err := os.Stat(filepath.Join(sessDir, msgFile1)); !os.IsNotExist(err) {
		t.Error("message file still exists after deletion")
	}

	// Verify second file still exists
	if _, err := os.Stat(filepath.Join(sessDir, msgFile2)); os.IsNotExist(err) {
		t.Error("second message file was incorrectly deleted")
	}

	// Verify cache message count decremented
	cacheMu.RLock()
	s := cache.Sessions[cacheKey(projectDir, sessID)]
	cacheMu.RUnlock()
	if s == nil {
		t.Fatal("session not found in cache")
	}
	if s.MessageCount != 1 {
		t.Errorf("message count = %d, want 1", s.MessageCount)
	}
}

func TestE2E_DeleteMessage_NotFound(t *testing.T) {
	homeDir := t.TempDir()
	homeDir, _ = filepath.EvalSymlinks(homeDir)

	projectDir := filepath.Join(homeDir, "test-project")
	sessID := "2026-07-20T10-00-00_testuser"
	sessDir := filepath.Join(projectDir, ".clyde", "sessions", sessID)
	os.MkdirAll(sessDir, 0755)

	baseURL, cleanup := startTestServer(t, homeDir)
	defer cleanup()

	// Try to delete a non-existent message
	url := fmt.Sprintf("%s/api/sessions/%s/messages/%s?cwd=%s",
		baseURL, sessID, "nonexistent.md", projectDir)
	status, _ := httpDelete(t, url)
	if status != 404 {
		t.Errorf("expected 404 for non-existent message, got %d", status)
	}
}

func TestE2E_DeleteMessage_PathTraversal(t *testing.T) {
	homeDir := t.TempDir()
	homeDir, _ = filepath.EvalSymlinks(homeDir)

	projectDir := filepath.Join(homeDir, "test-project")
	sessID := "2026-07-20T10-00-00_testuser"
	os.MkdirAll(filepath.Join(projectDir, ".clyde", "sessions", sessID), 0755)

	baseURL, cleanup := startTestServer(t, homeDir)
	defer cleanup()

	// Try path traversal with ".."
	url := fmt.Sprintf("%s/api/sessions/%s/messages/%s?cwd=%s",
		baseURL, sessID, "..%2F..%2Fetc%2Fpasswd", projectDir)
	status, body := httpDelete(t, url)
	if status != 400 {
		t.Errorf("expected 400 for path traversal, got %d: %s", status, body)
	}
}

func TestE2E_DeleteMessage_MissingParams(t *testing.T) {
	homeDir := t.TempDir()
	baseURL, cleanup := startTestServer(t, homeDir)
	defer cleanup()

	// Missing cwd
	url := fmt.Sprintf("%s/api/sessions/someid/messages/somefile.md", baseURL)
	status, _ := httpDelete(t, url)
	if status != 400 {
		t.Errorf("expected 400 for missing cwd, got %d", status)
	}
}

// ── Mark All Read Tests ──

func TestE2E_MarkAllRead_Global(t *testing.T) {
	homeDir := t.TempDir()
	homeDir, _ = filepath.EvalSymlinks(homeDir)

	baseURL, cleanup := startTestServer(t, homeDir)
	defer cleanup()

	// Create several unread sessions in different projects
	cacheMu.Lock()
	cache.Sessions[cacheKey("/project-a", "sess-1")] = &CachedSession{
		ID: "sess-1", CWD: "/project-a", Project: "project-a",
		MessageCount: 5, Read: false, LastModified: time.Now().Format(time.RFC3339),
	}
	cache.Sessions[cacheKey("/project-b", "sess-2")] = &CachedSession{
		ID: "sess-2", CWD: "/project-b", Project: "project-b",
		MessageCount: 3, Read: false, LastModified: time.Now().Format(time.RFC3339),
	}
	cache.Sessions[cacheKey("/project-a", "sess-3")] = &CachedSession{
		ID: "sess-3", CWD: "/project-a", Project: "project-a",
		MessageCount: 2, Read: true, // already read
		LastModified: time.Now().Format(time.RFC3339),
	}
	cacheMu.Unlock()

	// Mark all read globally
	status, body := httpPost(t, baseURL+"/api/sessions/mark-all-read", `{"scope": "all"}`)
	if status != 200 {
		t.Fatalf("POST mark-all-read returned %d: %s", status, body)
	}

	var resp map[string]interface{}
	json.Unmarshal(body, &resp)
	if resp["status"] != "ok" {
		t.Errorf("expected status ok, got %v", resp["status"])
	}
	// Should mark 2 (sess-1 and sess-2 were unread)
	if count, ok := resp["count"].(float64); !ok || count != 2 {
		t.Errorf("expected count 2, got %v", resp["count"])
	}

	// Verify all sessions are now read
	cacheMu.RLock()
	for key, s := range cache.Sessions {
		if !s.Read {
			t.Errorf("session %s is still unread", key)
		}
	}
	cacheMu.RUnlock()
}

func TestE2E_MarkAllRead_ByCWD(t *testing.T) {
	homeDir := t.TempDir()
	homeDir, _ = filepath.EvalSymlinks(homeDir)

	baseURL, cleanup := startTestServer(t, homeDir)
	defer cleanup()

	// Create unread sessions in different CWDs
	cacheMu.Lock()
	cache.Sessions[cacheKey("/project-a", "sess-1")] = &CachedSession{
		ID: "sess-1", CWD: "/project-a", Project: "project-a",
		MessageCount: 5, Read: false, LastModified: time.Now().Format(time.RFC3339),
	}
	cache.Sessions[cacheKey("/project-a", "sess-2")] = &CachedSession{
		ID: "sess-2", CWD: "/project-a", Project: "project-a",
		MessageCount: 3, Read: false, LastModified: time.Now().Format(time.RFC3339),
	}
	cache.Sessions[cacheKey("/project-b", "sess-3")] = &CachedSession{
		ID: "sess-3", CWD: "/project-b", Project: "project-b",
		MessageCount: 2, Read: false, LastModified: time.Now().Format(time.RFC3339),
	}
	cacheMu.Unlock()

	// Mark only project-a as read
	payload := `{"scope": "cwd", "cwd": "/project-a"}`
	status, body := httpPost(t, baseURL+"/api/sessions/mark-all-read", payload)
	if status != 200 {
		t.Fatalf("POST mark-all-read returned %d: %s", status, body)
	}

	var resp map[string]interface{}
	json.Unmarshal(body, &resp)
	if count, ok := resp["count"].(float64); !ok || count != 2 {
		t.Errorf("expected count 2, got %v", resp["count"])
	}

	// Verify project-a sessions are read, project-b still unread
	cacheMu.RLock()
	defer cacheMu.RUnlock()

	sA1 := cache.Sessions[cacheKey("/project-a", "sess-1")]
	sA2 := cache.Sessions[cacheKey("/project-a", "sess-2")]
	sB3 := cache.Sessions[cacheKey("/project-b", "sess-3")]

	if sA1 == nil || !sA1.Read {
		t.Error("sess-1 should be read")
	}
	if sA2 == nil || !sA2.Read {
		t.Error("sess-2 should be read")
	}
	if sB3 == nil || sB3.Read {
		t.Error("sess-3 should still be unread")
	}
}

func TestE2E_MarkAllRead_ByWorktree(t *testing.T) {
	homeDir := t.TempDir()
	homeDir, _ = filepath.EvalSymlinks(homeDir)

	baseURL, cleanup := startTestServer(t, homeDir)
	defer cleanup()

	// Create unread sessions with worktree info
	wtParent := "/code/myproject"
	cacheMu.Lock()
	cache.Sessions[cacheKey("/code/myproject/main", "sess-1")] = &CachedSession{
		ID: "sess-1", CWD: "/code/myproject/main", Project: "main",
		WorktreeParent: wtParent, WorktreeParentName: "myproject",
		MessageCount: 5, Read: false, LastModified: time.Now().Format(time.RFC3339),
	}
	cache.Sessions[cacheKey("/code/myproject/feature", "sess-2")] = &CachedSession{
		ID: "sess-2", CWD: "/code/myproject/feature", Project: "feature",
		WorktreeParent: wtParent, WorktreeParentName: "myproject",
		MessageCount: 3, Read: false, LastModified: time.Now().Format(time.RFC3339),
	}
	cache.Sessions[cacheKey("/other-project", "sess-3")] = &CachedSession{
		ID: "sess-3", CWD: "/other-project", Project: "other",
		MessageCount: 2, Read: false, LastModified: time.Now().Format(time.RFC3339),
	}
	cacheMu.Unlock()

	// Mark worktree group as read
	payload := fmt.Sprintf(`{"scope": "worktree", "worktree_parent": %q}`, wtParent)
	status, body := httpPost(t, baseURL+"/api/sessions/mark-all-read", payload)
	if status != 200 {
		t.Fatalf("POST mark-all-read returned %d: %s", status, body)
	}

	var resp map[string]interface{}
	json.Unmarshal(body, &resp)
	if count, ok := resp["count"].(float64); !ok || count != 2 {
		t.Errorf("expected count 2, got %v", resp["count"])
	}

	// Verify worktree sessions are read, other project still unread
	cacheMu.RLock()
	defer cacheMu.RUnlock()

	s1 := cache.Sessions[cacheKey("/code/myproject/main", "sess-1")]
	s2 := cache.Sessions[cacheKey("/code/myproject/feature", "sess-2")]
	s3 := cache.Sessions[cacheKey("/other-project", "sess-3")]

	if s1 == nil || !s1.Read {
		t.Error("sess-1 (worktree) should be read")
	}
	if s2 == nil || !s2.Read {
		t.Error("sess-2 (worktree) should be read")
	}
	if s3 == nil || s3.Read {
		t.Error("sess-3 (other project) should still be unread")
	}
}

// ── Delete Worktree Tests ──

func TestE2E_DeleteWorktree_MoveSessions(t *testing.T) {
	parentDir, mainDir, wtDir := setupWorktreeRepo(t)

	// Create sessions in the feature worktree
	wtSessRoot := filepath.Join(wtDir, ".clyde", "sessions")
	mainSessRoot := filepath.Join(mainDir, ".clyde", "sessions")
	sessID1 := "2026-07-20T10-00-00_test"
	sessID2 := "2026-07-20T11-00-00_test"

	os.MkdirAll(filepath.Join(wtSessRoot, sessID1), 0755)
	os.WriteFile(filepath.Join(wtSessRoot, sessID1, "2026-07-20T10-00-01.000_user.md"), []byte("hello"), 0644)
	os.MkdirAll(filepath.Join(wtSessRoot, sessID2), 0755)
	os.WriteFile(filepath.Join(wtSessRoot, sessID2, "2026-07-20T11-00-01.000_user.md"), []byte("world"), 0644)
	os.MkdirAll(mainSessRoot, 0755)

	homeDir := t.TempDir()
	baseURL, cleanup := startTestServer(t, homeDir)
	defer cleanup()

	// Inject sessions into cache
	cacheMu.Lock()
	cache.Sessions[cacheKey(wtDir, sessID1)] = &CachedSession{
		ID: sessID1, CWD: wtDir, Project: filepath.Base(wtDir),
		MessageCount: 1, Read: true, LastModified: time.Now().Format(time.RFC3339),
	}
	cache.Sessions[cacheKey(wtDir, sessID2)] = &CachedSession{
		ID: sessID2, CWD: wtDir, Project: filepath.Base(wtDir),
		MessageCount: 1, Read: true, LastModified: time.Now().Format(time.RFC3339),
	}
	cacheMu.Unlock()

	// Delete the feature worktree
	payload := fmt.Sprintf(`{"worktree_path": %q, "parent_path": %q}`, wtDir, parentDir)
	status, body := httpPost(t, baseURL+"/api/worktrees/delete", payload)
	if status != 200 {
		t.Fatalf("POST /api/worktrees/delete returned %d: %s", status, body)
	}

	var resp map[string]interface{}
	json.Unmarshal(body, &resp)
	if resp["status"] != "ok" {
		t.Errorf("expected status ok, got %v", resp["status"])
	}
	if movedSessions, ok := resp["moved_sessions"].(float64); !ok || movedSessions != 2 {
		t.Errorf("expected moved_sessions=2, got %v", resp["moved_sessions"])
	}

	// Verify sessions were moved to the main worktree
	if _, err := os.Stat(filepath.Join(mainSessRoot, sessID1)); os.IsNotExist(err) {
		t.Error("session 1 was not moved to main worktree")
	}
	if _, err := os.Stat(filepath.Join(mainSessRoot, sessID2)); os.IsNotExist(err) {
		t.Error("session 2 was not moved to main worktree")
	}

	// Verify the message files were moved too
	if _, err := os.Stat(filepath.Join(mainSessRoot, sessID1, "2026-07-20T10-00-01.000_user.md")); os.IsNotExist(err) {
		t.Error("message file was not moved with session 1")
	}

	// Verify the worktree directory was removed
	if _, err := os.Stat(wtDir); !os.IsNotExist(err) {
		t.Error("worktree directory still exists after deletion")
	}

	// Verify git no longer lists the worktree
	out, err := exec.Command("git", "-C", mainDir, "worktree", "list", "--porcelain").Output()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "feature-branch") {
		t.Error("git worktree list still shows deleted worktree")
	}

	// Verify cache was updated
	cacheMu.RLock()
	defer cacheMu.RUnlock()
	if cache.Sessions[cacheKey(wtDir, sessID1)] != nil {
		t.Error("old cache entry for session 1 still exists")
	}
	if cache.Sessions[cacheKey(wtDir, sessID2)] != nil {
		t.Error("old cache entry for session 2 still exists")
	}
	if s := cache.Sessions[cacheKey(mainDir, sessID1)]; s == nil {
		t.Error("new cache entry for session 1 not found")
	} else if s.CWD != mainDir {
		t.Errorf("session 1 CWD = %q, want %q", s.CWD, mainDir)
	}
}

func TestE2E_DeleteWorktree_Validation(t *testing.T) {
	homeDir := t.TempDir()
	baseURL, cleanup := startTestServer(t, homeDir)
	defer cleanup()

	// Missing fields
	status, _ := httpPost(t, baseURL+"/api/worktrees/delete", `{"worktree_path": ""}`)
	if status != 400 {
		t.Errorf("expected 400 for empty fields, got %d", status)
	}

	// Non-worktree directory
	nonWtDir := t.TempDir()
	payload := fmt.Sprintf(`{"worktree_path": %q, "parent_path": %q}`, nonWtDir, filepath.Dir(nonWtDir))
	status, _ = httpPost(t, baseURL+"/api/worktrees/delete", payload)
	if status != 400 {
		t.Errorf("expected 400 for non-worktree directory, got %d", status)
	}
}

func TestE2E_DeleteWorktree_LastWorktree(t *testing.T) {
	// Create a repo with no linked worktrees (just the main one)
	dir := t.TempDir()
	dir, _ = filepath.EvalSymlinks(dir)
	repoDir := filepath.Join(dir, "only-repo")
	os.MkdirAll(repoDir, 0755)
	run(t, repoDir, "git", "init")
	run(t, repoDir, "git", "config", "user.email", "test@test.com")
	run(t, repoDir, "git", "config", "user.name", "Test")
	os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("hello"), 0644)
	run(t, repoDir, "git", "add", ".")
	run(t, repoDir, "git", "commit", "-m", "init")

	homeDir := t.TempDir()
	baseURL, cleanup := startTestServer(t, homeDir)
	defer cleanup()

	// detectWorktreeGroup returns nil for a single-worktree repo, so this should fail
	payload := fmt.Sprintf(`{"worktree_path": %q, "parent_path": %q}`, repoDir, dir)
	status, body := httpPost(t, baseURL+"/api/worktrees/delete", payload)
	if status != 400 {
		t.Errorf("expected 400 for repo with no linked worktrees, got %d: %s", status, body)
	}
}

// ── Open In Terminal Tests ──

func TestE2E_OpenTerminal_Validation(t *testing.T) {
	homeDir := t.TempDir()
	baseURL, cleanup := startTestServer(t, homeDir)
	defer cleanup()

	// Missing cwd
	url := fmt.Sprintf("%s/api/sessions/someid/open-terminal", baseURL)
	status, _ := httpPost(t, url, "{}")
	if status != 400 {
		t.Errorf("expected 400 for missing cwd, got %d", status)
	}

	// Missing id (empty path param — will get a different route)
	url = fmt.Sprintf("%s/api/sessions//open-terminal?cwd=/tmp", baseURL)
	resp, err := http.Post(url, "application/json", nil)
	if err == nil {
		resp.Body.Close()
		// The route pattern may not match an empty :id, so we just verify it doesn't crash
	}
}

// ── Messages API Tests ──

func TestE2E_GetMessages_WithTypes(t *testing.T) {
	homeDir := t.TempDir()
	homeDir, _ = filepath.EvalSymlinks(homeDir)

	projectDir := filepath.Join(homeDir, "test-project")
	sessID := "2026-07-20T10-00-00_testuser"
	sessDir := filepath.Join(projectDir, ".clyde", "sessions", sessID)
	os.MkdirAll(sessDir, 0755)

	// Create messages of different types
	os.WriteFile(filepath.Join(sessDir, "2026-07-20T10-00-01.000_user.md"), []byte("hello"), 0644)
	os.WriteFile(filepath.Join(sessDir, "2026-07-20T10-00-02.000_thinking.md"), []byte("thinking..."), 0644)
	os.WriteFile(filepath.Join(sessDir, "2026-07-20T10-00-03.000_tool-use.md"), []byte("tool call"), 0644)
	os.WriteFile(filepath.Join(sessDir, "2026-07-20T10-00-04.000_tool-result.md"), []byte("tool result"), 0644)
	os.WriteFile(filepath.Join(sessDir, "2026-07-20T10-00-05.000_diagnostic.md"), []byte("diag info"), 0644)
	os.WriteFile(filepath.Join(sessDir, "2026-07-20T10-00-06.000_assistant.md"), []byte("response"), 0644)

	baseURL, cleanup := startTestServer(t, homeDir)
	defer cleanup()

	// Fetch all types
	url := fmt.Sprintf("%s/api/sessions/%s/messages?cwd=%s&types=user,assistant,thinking,tool-use,tool-result,diagnostic",
		baseURL, sessID, projectDir)
	status, body := httpGet(t, url)
	if status != 200 {
		t.Fatalf("GET messages returned %d: %s", status, body)
	}

	var data struct {
		Messages []MessageFile `json:"messages"`
	}
	json.Unmarshal(body, &data)
	if len(data.Messages) != 6 {
		t.Errorf("expected 6 messages, got %d", len(data.Messages))
	}

	// Verify types are present
	typeCount := map[string]int{}
	for _, m := range data.Messages {
		typeCount[m.Type]++
	}
	if typeCount["user"] != 1 || typeCount["assistant"] != 1 ||
		typeCount["thinking"] != 1 || typeCount["tool-use"] != 1 ||
		typeCount["tool-result"] != 1 || typeCount["diagnostic"] != 1 {
		t.Errorf("unexpected type distribution: %v", typeCount)
	}
}

// ── Worktree Visibility Tests ──

func TestE2E_ProjectVisibility_Worktrees(t *testing.T) {
	homeDir := t.TempDir()
	homeDir, _ = filepath.EvalSymlinks(homeDir)

	baseURL, cleanup := startTestServer(t, homeDir)
	defer cleanup()

	// Create sessions with worktree info (sharing a parent)
	wtParent := "/code/myproject"
	cacheMu.Lock()
	cache.Sessions[cacheKey("/code/myproject/main", "sess-1")] = &CachedSession{
		ID: "sess-1", CWD: "/code/myproject/main", Project: "main",
		WorktreeParent: wtParent, WorktreeParentName: "myproject",
		MessageCount: 1, Read: true, LastModified: time.Now().Format(time.RFC3339),
	}
	cache.Sessions[cacheKey("/code/myproject/feature", "sess-2")] = &CachedSession{
		ID: "sess-2", CWD: "/code/myproject/feature", Project: "feature",
		WorktreeParent: wtParent, WorktreeParentName: "myproject",
		MessageCount: 1, Read: true, LastModified: time.Now().Format(time.RFC3339),
	}
	cache.Sessions[cacheKey("/other-project", "sess-3")] = &CachedSession{
		ID: "sess-3", CWD: "/other-project", Project: "other",
		MessageCount: 1, Read: true, LastModified: time.Now().Format(time.RFC3339),
	}
	cacheMu.Unlock()

	// Get initial sessions — all 3 should appear
	status, body := httpGet(t, baseURL+"/api/sessions?days=0")
	if status != 200 {
		t.Fatalf("GET sessions returned %d: %s", status, body)
	}
	var data struct {
		Sessions []SessionResponse `json:"sessions"`
	}
	json.Unmarshal(body, &data)
	if len(data.Sessions) != 3 {
		t.Errorf("expected 3 sessions, got %d", len(data.Sessions))
	}

	// Verify worktree sessions have correct parent info
	for _, s := range data.Sessions {
		if s.CWD == "/code/myproject/main" || s.CWD == "/code/myproject/feature" {
			if s.WorktreeParentName != "myproject" {
				t.Errorf("session %s: worktree_parent_name = %q, want 'myproject'", s.ID, s.WorktreeParentName)
			}
		}
	}

	// Hide the worktree group by parent name
	hiddenProjects := map[string]bool{"myproject": true}
	patchBody := fmt.Sprintf(`{"hidden_projects": %s}`, mustJSON(hiddenProjects))
	status, _ = httpPatchHelper(t, baseURL+"/api/preferences", patchBody)
	if status != 200 {
		t.Fatalf("PATCH preferences returned %d", status)
	}

	// Verify preferences were saved
	status, body = httpGet(t, baseURL+"/api/preferences")
	if status != 200 {
		t.Fatalf("GET preferences returned %d: %s", status, body)
	}
	var prefs Preferences
	json.Unmarshal(body, &prefs)
	if !prefs.HiddenProjects["myproject"] {
		t.Error("'myproject' should be hidden in preferences")
	}
}

// ── Session State Tests ──

func TestE2E_SessionReadState(t *testing.T) {
	homeDir := t.TempDir()
	homeDir, _ = filepath.EvalSymlinks(homeDir)

	baseURL, cleanup := startTestServer(t, homeDir)
	defer cleanup()

	sessID := "2026-07-20T10-00-00_testuser"
	projectDir := filepath.Join(homeDir, "test-project")

	cacheMu.Lock()
	cache.Sessions[cacheKey(projectDir, sessID)] = &CachedSession{
		ID: sessID, CWD: projectDir, Project: "test-project",
		MessageCount: 5, Read: true, LastReadCount: 5,
		LastModified: time.Now().Format(time.RFC3339),
	}
	cacheMu.Unlock()

	// Mark as unread via PATCH
	patchURL := fmt.Sprintf("%s/api/sessions/%s?cwd=%s", baseURL, sessID, projectDir)
	status, _ := httpPatchHelper(t, patchURL, `{"read": false}`)
	if status != 200 {
		t.Fatalf("PATCH session returned %d", status)
	}

	// Verify it shows as unread in sessions list
	status, body := httpGet(t, baseURL+"/api/sessions?days=0")
	if status != 200 {
		t.Fatalf("GET sessions returned %d", status)
	}
	var data struct {
		Sessions []SessionResponse `json:"sessions"`
	}
	json.Unmarshal(body, &data)
	found := false
	for _, s := range data.Sessions {
		if s.ID == sessID {
			found = true
			if !s.Unread {
				t.Error("session should be unread")
			}
		}
	}
	if !found {
		t.Error("session not found in response")
	}

	// Mark back as read
	status, _ = httpPatchHelper(t, patchURL, `{"read": true}`)
	if status != 200 {
		t.Fatalf("PATCH session returned %d", status)
	}

	// Verify read state and LastReadCount updated
	cacheMu.RLock()
	s := cache.Sessions[cacheKey(projectDir, sessID)]
	cacheMu.RUnlock()
	if s == nil || !s.Read {
		t.Error("session should be marked as read")
	}
	if s.LastReadCount != 5 {
		t.Errorf("LastReadCount = %d, want 5", s.LastReadCount)
	}
}

// ── Helper functions ──

func httpPatchHelper(t *testing.T, url string, jsonBody string) (int, []byte) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPatch, url, strings.NewReader(jsonBody))
	if err != nil {
		t.Fatalf("PATCH request creation failed: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PATCH %s failed: %v", url, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, body
}

func mustJSON(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}
