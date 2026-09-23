package main

import (
	"path/filepath"
	"testing"
)

// ownsPath is the application-code half of isolation (PLAN.md §1): the
// webserver can read every user's files, so it must decide what to *show*.
// These cases are the ones that would actually leak.
func TestOwnsPath(t *testing.T) {
	alice := &Principal{Username: "alice", Home: "/srv/bonnie/home/alice"}
	solo := &Principal{Username: "aj", Home: "/Users/aj", Solo: true}

	cases := []struct {
		name string
		p    *Principal
		path string
		want bool
	}{
		{"own home", alice, "/srv/bonnie/home/alice", true},
		{"own subdir", alice, "/srv/bonnie/home/alice/code/notes", true},
		{"unclean own subdir", alice, "/srv/bonnie/home/alice/code/../code/notes", true},
		{"other home", alice, "/srv/bonnie/home/bob", false},
		{"prefix sibling", alice, "/srv/bonnie/home/alice-evil", false},
		{"prefix sibling subdir", alice, "/srv/bonnie/home/alice-evil/code", false},
		{"traversal escape", alice, "/srv/bonnie/home/alice/../bob/secrets", false},
		{"parent dir", alice, "/srv/bonnie/home", false},
		{"root", alice, "/", false},
		{"empty", alice, "", false},
		{"solo sees all", solo, "/srv/bonnie/home/bob", true},
		{"nil principal sees all", nil, "/srv/bonnie/home/bob", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ownsPath(tc.p, tc.path); got != tc.want {
				t.Fatalf("ownsPath(%q, %q) = %v, want %v", tc.p.homeOrSolo(), tc.path, got, tc.want)
			}
		})
	}
}

// homeOrSolo is a test-only helper so failure messages are readable when the
// principal is nil.
func (p *Principal) homeOrSolo() string {
	if p == nil {
		return "<nil>"
	}
	return p.Home
}

// discoverProjectDirsFor must not include the server's own cwd in multi-user
// mode. Solo mode does include it — that is the single-user convenience — and
// the two must not be confused, because the server's cwd is not a user's.
func TestDiscoverProjectDirsForExcludesServerCwd(t *testing.T) {
	home := t.TempDir()
	resolved, err := filepath.EvalSymlinks(home)
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}

	dirs := discoverProjectDirsFor(&Principal{Username: "alice", Home: resolved})
	if !dirs[resolved] {
		t.Fatalf("principal home %q missing from %v", resolved, dirs)
	}
	for dir := range dirs {
		if !ownsPath(&Principal{Username: "alice", Home: resolved}, dir) {
			t.Fatalf("discovered dir %q outside principal home %q", dir, resolved)
		}
	}
}
