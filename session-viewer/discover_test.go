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

// A root-owned directory anywhere between the home and the sessions directory
// means the agent cannot write its own transcripts. MkdirAll creates
// intermediates owned by the caller, so this is the default failure, not an
// exotic one.
func TestChownTreeStopsAtRoot(t *testing.T) {
	root := t.TempDir()
	deep := filepath.Join(root, "code", "scratch", ".clyde", "sessions")
	if err := os.MkdirAll(deep, 0o750); err != nil {
		t.Fatal(err)
	}

	// Chown to our own uid/gid: a no-op ownership-wise, but it exercises the
	// walk and proves it terminates at root rather than climbing to /.
	uid, gid := os.Getuid(), os.Getgid()
	if err := chownTree(deep, root, uid, gid); err != nil {
		t.Fatalf("chownTree: %v", err)
	}

	// The walk must not have escaped above root.
	parent := filepath.Dir(root)
	if err := chownTree(deep, root, uid, gid); err != nil {
		t.Fatalf("chownTree (second pass): %v", err)
	}
	if _, err := os.Stat(parent); err != nil {
		t.Fatalf("parent of root disturbed: %v", err)
	}
}
