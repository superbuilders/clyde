package main

import (
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
