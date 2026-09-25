//go:build unix

package main

import (
	"os"
	"path/filepath"
	"session-viewer/internal/principal"
	"testing"
)

// canAccess widens what a principal may see, so it is exactly where an
// isolation bug would live. ownsPath stays strict and is tested separately in
// principal_test.go; these cover the share-shaped additions.
func TestCanAccess(t *testing.T) {
	root := t.TempDir()
	aliceHome := filepath.Join(root, "alice")
	bobHome := filepath.Join(root, "bob")

	shared := filepath.Join(aliceHome, "code", "shared-proj")
	private := filepath.Join(aliceHome, "code", "private-proj")
	for _, d := range []string{shared, private, bobHome} {
		if err := os.MkdirAll(d, 0o750); err != nil {
			t.Fatal(err)
		}
	}

	alice := &principal.Principal{Username: "alice", Home: aliceHome}
	bob := &principal.Principal{Username: "bob", Home: bobHome}

	// Before any share, Bob sees nothing of Alice's.
	if canAccess(bob, shared) {
		t.Error("bob could reach alice's directory before it was shared")
	}

	// The symlink is what puts it in Bob's search path (A5).
	link := linkNameFor(bobHome, "alice", shared)
	if err := os.MkdirAll(filepath.Dir(link), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(shared, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	t.Run("the shared directory becomes reachable", func(t *testing.T) {
		if !canAccess(bob, shared) {
			t.Error("bob cannot reach the directory shared with him")
		}
	})

	t.Run("reachable by its real path and through the link", func(t *testing.T) {
		if !canAccess(bob, link) {
			t.Error("bob cannot reach the share via the symlink in his own home")
		}
	})

	t.Run("content inside the share is reachable", func(t *testing.T) {
		if !canAccess(bob, filepath.Join(shared, ".clyde", "sessions")) {
			t.Error("sessions inside a shared project must be visible; they are part of the share")
		}
	})

	// The assertion that matters. One share must not become a key to the
	// owner's whole tree — this is the M3 isolation property surviving M4.
	t.Run("a sibling directory stays private", func(t *testing.T) {
		if canAccess(bob, private) {
			t.Error("sharing one directory exposed an unshared sibling")
		}
	})

	t.Run("the owner's home stays private", func(t *testing.T) {
		if canAccess(bob, aliceHome) {
			t.Error("bob could reach alice's home directory")
		}
	})

	t.Run("a prefix sibling of the share is not matched", func(t *testing.T) {
		// shared-proj must not act as a prefix for shared-proj-secret.
		evil := filepath.Join(aliceHome, "code", "shared-proj-secret")
		if err := os.MkdirAll(evil, 0o750); err != nil {
			t.Fatal(err)
		}
		if canAccess(bob, evil) {
			t.Errorf("%s matched as a prefix of the shared path", evil)
		}
	})

	t.Run("the owner keeps access to their own tree", func(t *testing.T) {
		if !canAccess(alice, private) {
			t.Error("alice lost access to her own directory")
		}
	})

	t.Run("sharing is not symmetric", func(t *testing.T) {
		if canAccess(alice, bobHome) {
			t.Error("alice could reach bob's home just because he holds a share of hers")
		}
	})
}

// Solo has no shares and no boundary to enforce; canAccess must be a constant
// true so single-user behaviour is unchanged (PLAN.md §8 item 2).
func TestCanAccessSoloAllowsEverything(t *testing.T) {
	solo := &principal.Principal{Username: "aj", Home: "/home/aj", Solo: true}
	for _, p := range []string{"/home/aj/code", "/etc", "/srv/bonnie/users/someone"} {
		if !canAccess(solo, p) {
			t.Errorf("solo denied %s", p)
		}
	}
}

// A revoked or hand-deleted share leaves a dangling link. It must be skipped,
// not treated as an error that breaks the whole project listing, and above all
// it must not grant anything.
func TestSharedRootsSkipsDanglingLinks(t *testing.T) {
	root := t.TempDir()
	bobHome := filepath.Join(root, "bob")
	bob := &principal.Principal{Username: "bob", Home: bobHome}

	link := linkNameFor(bobHome, "alice", "/nonexistent/gone")
	if err := os.MkdirAll(filepath.Dir(link), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/nonexistent/gone", link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	if got := sharedRoots(bob); len(got) != 0 {
		t.Errorf("sharedRoots = %v, want none: a dangling link grants nothing", got)
	}
	if canAccess(bob, "/nonexistent/gone") {
		t.Error("a dangling share link granted access to its missing target")
	}
}

// A user with no shared/ directory at all is the common case and must not
// error or grant.
func TestSharedRootsWithNoShares(t *testing.T) {
	bob := &principal.Principal{Username: "bob", Home: t.TempDir()}
	if got := sharedRoots(bob); len(got) != 0 {
		t.Errorf("sharedRoots = %v, want none", got)
	}
}
