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

// ── per-conversation sharing ────────────────────────────────────────────────

// splitSessionDir is the inverse of sessionDir, and getShares depends on that
// being exactly true: a grant it cannot decompose is silently dropped from the
// owner's list of who can see what, which is the worst possible failure for a
// sharing UI — access that exists and is not reported.
func TestSplitSessionDir(t *testing.T) {
	t.Run("round-trips with sessionDir", func(t *testing.T) {
		const cwd, id = "/srv/bonnie/users/alice/code/api", "abc-123"
		gotCWD, gotID, ok := splitSessionDir(sessionDir(cwd, id))
		if !ok {
			t.Fatal("a path built by sessionDir must be recognised by splitSessionDir")
		}
		if gotCWD != cwd || gotID != id {
			t.Errorf("round-trip = (%q, %q), want (%q, %q)", gotCWD, gotID, cwd, id)
		}
	})

	// Anything else is not a conversation and must not be reported as one.
	for _, p := range []string{
		"/srv/bonnie/users/alice/code/api",                     // a project
		"/srv/bonnie/users/alice/code/api/.clyde",              // the agent dir
		"/srv/bonnie/users/alice/code/api/.clyde/sessions",     // the container
		"/srv/bonnie/users/alice/code/api/.other/sessions/abc", // wrong parent
		"/srv/bonnie/users/alice/code/api/.clyde/history/abc",  // wrong container
	} {
		if _, _, ok := splitSessionDir(p); ok {
			t.Errorf("splitSessionDir(%q) accepted a path that is not a session directory", p)
		}
	}
}

// A conversation is shared without its project. This is the whole point of
// M4.2: the sharee reads one transcript and learns nothing else about the
// owner's tree, including that the project's other conversations exist.
func TestCanAccessSessionIsNotProjectAccess(t *testing.T) {
	root := t.TempDir()
	aliceHome := filepath.Join(root, "alice")
	bobHome := filepath.Join(root, "bob")

	const cwd = "code/api"
	project := filepath.Join(aliceHome, cwd)
	sharedSess := sessionDir(project, "shared-one")
	otherSess := sessionDir(project, "other-one")
	for _, d := range []string{sharedSess, otherSess, bobHome} {
		if err := os.MkdirAll(d, 0o750); err != nil {
			t.Fatal(err)
		}
	}

	alice := &principal.Principal{Username: "alice", Home: aliceHome}
	bob := &principal.Principal{Username: "bob", Home: bobHome}

	link := linkNameFor(bobHome, "alice", sharedSess)
	if err := os.MkdirAll(filepath.Dir(link), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(sharedSess, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	t.Run("the shared conversation is readable", func(t *testing.T) {
		if !canAccessSession(bob, project, "shared-one") {
			t.Error("bob cannot read the conversation shared with him")
		}
	})

	t.Run("a sibling conversation is not", func(t *testing.T) {
		if canAccessSession(bob, project, "other-one") {
			t.Error("sharing one conversation exposed another in the same project")
		}
		_ = otherSess
	})

	t.Run("the project itself is not", func(t *testing.T) {
		if canAccess(bob, project) {
			t.Error("sharing a conversation exposed the project directory")
		}
	})

	t.Run("the owner still reaches everything", func(t *testing.T) {
		if !canAccessSession(alice, project, "other-one") {
			t.Error("alice lost access to her own conversation")
		}
	})
}

// linkNameFor gained a project component for conversations. Since revoke
// recomputes the name to delete the link, a change here that is not a pure
// function of the target would strand links in the sharee's home forever.
func TestLinkNameForConversation(t *testing.T) {
	target := sessionDir("/srv/bonnie/users/alice/code/api", "abc-123")
	got := linkNameFor("/srv/bonnie/users/bob", "alice", target)
	want := "/srv/bonnie/users/bob/shared/alice/api/abc-123"
	if got != want {
		t.Fatalf("linkNameFor = %q, want %q", got, want)
	}
	// The uuid alone says nothing; the project name is what makes the link
	// legible to the sharee and to their agent.
	if filepath.Base(filepath.Dir(got)) != "api" {
		t.Error("the link must be namespaced by the owner's project name")
	}
}
