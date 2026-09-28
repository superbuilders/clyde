package main

import (
	"os"
	"path/filepath"
	"testing"

	"session-viewer/internal/principal"
)

// discoverProjectDirsFor must not include the server's own cwd in multi-user
// mode. Solo mode does include it — that is the single-user convenience — and
// the two must not be confused, because the server's cwd is not a user's.
func TestDiscoverProjectDirsForExcludesServerCwd(t *testing.T) {
	home := t.TempDir()
	resolved, err := filepath.EvalSymlinks(home)
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}

	dirs := discoverProjectDirsFor(&principal.Principal{Username: "alice", Home: resolved})
	if !dirs[resolved] {
		t.Fatalf("principal home %q missing from %v", resolved, dirs)
	}
	for dir := range dirs {
		if !(&principal.Principal{Username: "alice", Home: resolved}).Owns(dir) {
			t.Fatalf("discovered dir %q outside principal home %q", dir, resolved)
		}
	}
}

// In multi-user mode the background scanner must never fall back to the solo
// root set.
//
// This is a regression test for a bug that made every real user's sidebar
// empty. discoverProjectDirs passed nil to discoverProjectDirsFor, which takes
// the *solo* branch: the invoking user's home and cwd. The invoking user is
// the service account, so the scanner walked /srv/bonnie/home, cached the
// service's own projects, and never looked at a single user's tree. The
// read-time filter then correctly hid those cached sessions from everyone, so
// the symptom was "no sessions" with a cache that was busily full.
//
// Nothing in the existing suite caught it, because every test ran in solo
// mode, where passing nil is exactly right.
func TestDiscoverProjectDirsNeverUsesSoloRootsInMultiUser(t *testing.T) {
	dir := t.TempDir()
	mapPath := filepath.Join(dir, "users.map")
	// A user that does not exist on this machine: Lookup fails, the loop skips
	// it, and the result is empty. Empty is the correct answer here — what
	// must never happen is silently scanning the service account's home.
	if err := os.WriteFile(mapPath, []byte("nobody@example.com nosuchuser-bonnie-test\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	r, err := principal.NewResolver(true, mapPath)
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	saved := principals
	principals = r
	t.Cleanup(func() { principals = saved })

	got := discoverProjectDirs()

	home, _ := os.UserHomeDir()
	if got[home] {
		t.Fatalf("multi-user scan included the service account's home %q; "+
			"it took the solo branch and no user's sessions will ever be scanned", home)
	}
	if cwd, err := os.Getwd(); err == nil && got[cwd] {
		t.Fatalf("multi-user scan included the service cwd %q (solo branch)", cwd)
	}
}
