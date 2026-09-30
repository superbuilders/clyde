package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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

// A freshly cloned repository must be discoverable before it has a .clyde.
//
// Discovery keys on .clyde, which only the agent writes — so before M6 a
// project became visible only after a conversation had already happened in
// it. That was self-consistent while code arrived on the box by hand, and
// becomes a trap the moment an agent can clone: the checkout lands in
// ~/code/<repo>, never appears in the sidebar, and the UI can only start
// sessions in projects it lists. The user is told the clone worked and has no
// way to use it.
func TestDiscoverFindsGitRepoWithoutClyde(t *testing.T) {
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	alice := &principal.Principal{Username: "alice", Home: home}

	// A clone: a git checkout, no .clyde anywhere in it.
	cloned := filepath.Join(home, "code", "freshly-cloned")
	if err := os.MkdirAll(filepath.Join(cloned, ".git"), 0o750); err != nil {
		t.Fatal(err)
	}
	// And a conversation-bearing project, to prove the old path still works.
	worked := filepath.Join(home, "code", "worked-in")
	if err := os.MkdirAll(filepath.Join(worked, ".clyde", "sessions"), 0o750); err != nil {
		t.Fatal(err)
	}

	dirs := discoverProjectDirsFor(alice)
	if !dirs[cloned] {
		t.Errorf("a cloned repo with no .clyde was not discovered:\n  want %q\n  got  %v", cloned, dirs)
	}
	if !dirs[worked] {
		t.Errorf("a project with sessions stopped being discovered: %v", dirs)
	}
}

// The repo search must not descend into a checkout and list its innards as
// sibling projects. A vendored dependency or a nested checkout has a .git of
// its own, and at the wrong depth every one of them becomes a project.
func TestDiscoverDoesNotDescendIntoRepos(t *testing.T) {
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	alice := &principal.Principal{Username: "alice", Home: home}

	nested := filepath.Join(home, "code", "outer", "vendor", "inner")
	if err := os.MkdirAll(filepath.Join(nested, ".git"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, "code", "outer", ".git"), 0o750); err != nil {
		t.Fatal(err)
	}

	dirs := discoverProjectDirsFor(alice)
	if !dirs[filepath.Join(home, "code", "outer")] {
		t.Error("the outer checkout was not discovered")
	}
	if dirs[nested] {
		t.Errorf("a nested checkout was listed as its own project: %q", nested)
	}
}

// Solo must stay byte-identical to upstream (PLAN.md §8 item 2). The repo
// search is a Bonnie addition and must not fire there.
func TestDiscoverRepoSearchIsMultiUserOnly(t *testing.T) {
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cloned := filepath.Join(home, "code", "freshly-cloned")
	if err := os.MkdirAll(filepath.Join(cloned, ".git"), 0o750); err != nil {
		t.Fatal(err)
	}

	// Solo resolves its own home from the environment, so point it here.
	t.Setenv("HOME", home)
	if dirs := discoverProjectDirsFor(&principal.Principal{Solo: true, Home: home}); dirs[cloned] {
		t.Errorf("the repo search fired in solo mode, changing upstream behaviour: %v", dirs)
	}
}

// TestProjectInfoReportsHasSessions covers the flag the sidebar depends on.
//
// Discovery alone was not enough to make a freshly cloned repo usable: the
// sidebar groups *sessions*, so a project with none of them renders nowhere
// even though /api/projects lists it. The client needs to be told which
// projects those are, and it cannot work it out from an empty session list —
// a filter can hide every session of a long-used project, and that must not
// make it look freshly cloned.
func TestProjectInfoReportsHasSessions(t *testing.T) {
	home := t.TempDir()

	// A repo that has been worked in.
	worked := filepath.Join(home, "code", "worked-in")
	if err := os.MkdirAll(filepath.Join(worked, ".clyde", "sessions"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(worked, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}

	// A repo that has only been cloned.
	cloned := filepath.Join(home, "code", "freshly-cloned")
	if err := os.MkdirAll(filepath.Join(cloned, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		dir  string
		want bool
	}{
		{worked, true},
		{cloned, false},
	} {
		_, err := os.Stat(filepath.Join(tc.dir, ".clyde", "sessions"))
		got := !os.IsNotExist(err)
		if got != tc.want {
			t.Errorf("%s: hasSessions = %v, want %v", filepath.Base(tc.dir), got, tc.want)
		}
	}

	// The field must be serialised unconditionally. With omitempty a false
	// value vanishes from the JSON, the client reads undefined, and
	// `has_sessions !== false` skips exactly the projects this exists for —
	// so the bug would look like the fix simply not working.
	blob, err := json.Marshal(ProjectInfo{Path: cloned, Name: "freshly-cloned"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(blob), `"has_sessions":false`) {
		t.Errorf("has_sessions:false is missing from the JSON: %s", blob)
	}
}

// TestDirHasEntries pins the distinction that broke the first attempt at this.
//
// A repo the agent has only looked at still gets a .clyde/sessions directory,
// because starting a session anywhere creates it before anything is written
// into it. Treating the directory's existence as proof of a conversation is
// what left a freshly cloned repo invisible in the sidebar even though
// discovery had correctly found it and /api/projects listed it.
func TestDirHasEntries(t *testing.T) {
	root := t.TempDir()

	empty := filepath.Join(root, "empty")
	if err := os.MkdirAll(empty, 0o755); err != nil {
		t.Fatal(err)
	}
	if dirHasEntries(empty) {
		t.Error("an empty directory reported as having entries")
	}

	full := filepath.Join(root, "full")
	if err := os.MkdirAll(filepath.Join(full, "a-session"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !dirHasEntries(full) {
		t.Error("a directory containing a session reported as empty")
	}

	// A path that does not exist is not an error and is not "has entries".
	if dirHasEntries(filepath.Join(root, "absent")) {
		t.Error("a missing directory reported as having entries")
	}
}

// TestProjectHasSessionsIgnoresAnEmptyDirectory is the regression this whole
// change exists for: panopticon was cloned, the agent looked at it, and
// .clyde/sessions appeared with nothing in it. Discovery found the project and
// the API listed it, but the sidebar had no session to group under and the
// project reported itself as already worked-in, so it showed up nowhere.
func TestProjectHasSessionsIgnoresAnEmptyDirectory(t *testing.T) {
	root := t.TempDir()

	cloned := filepath.Join(root, "panopticon")
	if err := os.MkdirAll(filepath.Join(cloned, ".clyde", "sessions"), 0o755); err != nil {
		t.Fatal(err)
	}
	if projectHasSessions(cloned) {
		t.Error("a repo with an empty .clyde/sessions reported as having sessions")
	}

	worked := filepath.Join(root, "scratch")
	if err := os.MkdirAll(filepath.Join(worked, ".clyde", "sessions", "2024-01-01_x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !projectHasSessions(worked) {
		t.Error("a repo with a session reported as having none")
	}

	// No .clyde at all — the freshly-cloned-and-untouched case.
	bare := filepath.Join(root, "bare")
	if err := os.MkdirAll(filepath.Join(bare, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if projectHasSessions(bare) {
		t.Error("a repo with no .clyde reported as having sessions")
	}
}
