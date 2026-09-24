package main

import (
	"reflect"
	"testing"
)

// The corridor is the set of directories a sharee must be able to walk
// through to reach a grant. Getting it wrong in either direction is a bug
// with teeth: too few and the share silently does not work, too many and we
// have handed out traverse bits nobody asked for.
func TestCorridorFor(t *testing.T) {
	const home = "/srv/bonnie/users/alice"

	tests := []struct {
		name   string
		target string
		want   []string
		err    bool
	}{
		{
			name:   "nested target walks back to the home directory",
			target: home + "/code/scratch",
			want:   []string{home, home + "/code"},
		},
		{
			name:   "a child of home needs only home itself",
			target: home + "/code",
			want:   []string{home},
		},
		{
			name:   "deep target yields every intermediate directory in order",
			target: home + "/a/b/c",
			want:   []string{home, home + "/a", home + "/a/b"},
		},
		{
			// Sharing the home directory itself would give the sharee r-x on
			// it, exposing the names of every project the owner has.
			name:   "the home directory itself is refused",
			target: home,
			err:    true,
		},
		{
			name:   "a target outside the home is refused",
			target: "/srv/bonnie/users/bob/code",
			err:    true,
		},
		{
			// The classic prefix bug: alice-evil must not look like alice.
			name:   "a sibling home with a shared prefix is refused",
			target: "/srv/bonnie/users/alice-evil/code",
			err:    true,
		},
		{
			name:   "traversal that escapes the home after cleaning is refused",
			target: home + "/../bob/code",
			err:    true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := corridorFor(home, tc.target)
			if tc.err {
				if err == nil {
					t.Fatalf("corridorFor(%q) = %v, want error", tc.target, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("corridorFor(%q): %v", tc.target, err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("corridorFor(%q) = %v, want %v", tc.target, got, tc.want)
			}
		})
	}
}

// corridorFor must never emit anything above the home directory. /srv/bonnie
// and /srv/bonnie/users belong to the system, and a bug that walked into them
// would be granting a user traverse rights on shared infrastructure.
func TestCorridorForStopsAtHome(t *testing.T) {
	const home = "/srv/bonnie/users/alice"
	got, err := corridorFor(home, home+"/a/b/c")
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range got {
		if len(d) < len(home) {
			t.Fatalf("corridor escaped above home: %q", d)
		}
	}
	if got[0] != home {
		t.Fatalf("corridor should start at home, got %q", got[0])
	}
}

func TestParseGetfaclRecursive(t *testing.T) {
	// Real getfacl -R output shape, including entries for a user we are not
	// asking about, a default entry, and an effective-permissions comment.
	const out = `# file: /srv/bonnie/users/alice
# owner: alice
# group: alice
user::rwx
user:bob:--x
user:carol:--x
group::r-x
other::---

# file: /srv/bonnie/users/alice/code
# owner: alice
# group: alice
user::rwx
user:bob:--x
mask::r-x
other::---

# file: /srv/bonnie/users/alice/code/scratch
# owner: alice
# group: alice
user::rwx
user:bob:r-x			#effective:r--
default:user:bob:r-x
group::r-x
other::---
`

	got := parseGetfaclRecursive(out, "bob")
	want := []aclEntry{
		{Path: "/srv/bonnie/users/alice", Perms: "--x"},
		{Path: "/srv/bonnie/users/alice/code", Perms: "--x"},
		{Path: "/srv/bonnie/users/alice/code/scratch", Perms: "r-x"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseGetfaclRecursive =\n%v\nwant\n%v", got, want)
	}

	// Carol's entries must not appear in Bob's results — a leak here would
	// make reconcile strip corridor bits belonging to a different sharee.
	for _, e := range got {
		if e.Perms == "" {
			t.Fatalf("empty perms in %v", e)
		}
	}

	// The default: line describes inheritance, not present reachability. If it
	// were parsed as a grant, reconcile would build a corridor for it.
	if n := len(parseGetfaclRecursive(out, "carol")); n != 1 {
		t.Fatalf("carol should have exactly one entry, got %d", n)
	}
}

// isGrant is the whole reason the filesystem can serve as the database: a
// share root is distinguishable from scaffolding by the read bit alone.
func TestACLEntryIsGrant(t *testing.T) {
	if !(aclEntry{Perms: "r-x"}).isGrant() {
		t.Fatal("r-x must be a grant")
	}
	if (aclEntry{Perms: "--x"}).isGrant() {
		t.Fatal("--x must be a corridor bit, not a grant")
	}
}

// The overlap case. Alice shares two directories under the same parent and
// revokes one; the corridor bit on the shared parent is load-bearing for the
// survivor and must not be removed. This is a silent failure if it regresses:
// the remaining share simply stops working for someone who is not running
// these tests.
func TestReconcileCorridorKeepsOverlappingShareAlive(t *testing.T) {
	const home = "/srv/bonnie/users/alice"

	// State immediately after the grant on code/scratch was stripped. The
	// grant on code/notes survives; both corridor bits are still present.
	entries := []aclEntry{
		{Path: home, Perms: "--x"},
		{Path: home + "/code", Perms: "--x"},
		{Path: home + "/code/notes", Perms: "r-x"},
	}

	add, remove, err := reconcileCorridor(home, entries)
	if err != nil {
		t.Fatal(err)
	}
	if len(add) != 0 {
		t.Fatalf("nothing should need adding, got %v", add)
	}
	if len(remove) != 0 {
		t.Fatalf("the surviving share still needs this corridor, but reconcile removed %v", remove)
	}
}

// With the last grant gone, every corridor bit must go too. Leaving them
// behind would publish the fact that Bob once had access to anyone who can
// read the ACL.
func TestReconcileCorridorRemovesEverythingWhenNoGrantsRemain(t *testing.T) {
	const home = "/srv/bonnie/users/alice"
	entries := []aclEntry{
		{Path: home, Perms: "--x"},
		{Path: home + "/code", Perms: "--x"},
	}

	add, remove, err := reconcileCorridor(home, entries)
	if err != nil {
		t.Fatal(err)
	}
	if len(add) != 0 {
		t.Fatalf("nothing to add, got %v", add)
	}
	want := []string{home, home + "/code"}
	if !reflect.DeepEqual(remove, want) {
		t.Fatalf("remove = %v, want %v", remove, want)
	}
}

// Reconcile is a pure function of the tree, so it repairs a corridor that was
// never fully built — the state left by a grant that died half-way through.
func TestReconcileCorridorRepairsMissingBits(t *testing.T) {
	const home = "/srv/bonnie/users/alice"
	entries := []aclEntry{
		{Path: home + "/code/scratch", Perms: "r-x"},
	}

	add, remove, err := reconcileCorridor(home, entries)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{home, home + "/code"}
	if !reflect.DeepEqual(add, want) {
		t.Fatalf("add = %v, want %v", add, want)
	}
	if len(remove) != 0 {
		t.Fatalf("nothing to remove, got %v", remove)
	}
}

// Two shares in unrelated branches: revoking neither, both corridors stand.
func TestReconcileCorridorHandlesDisjointBranches(t *testing.T) {
	const home = "/srv/bonnie/users/alice"
	entries := []aclEntry{
		{Path: home, Perms: "--x"},
		{Path: home + "/a", Perms: "--x"},
		{Path: home + "/a/one", Perms: "r-x"},
		{Path: home + "/b/two", Perms: "r-x"},
	}

	add, remove, err := reconcileCorridor(home, entries)
	if err != nil {
		t.Fatal(err)
	}
	// home/b was never given its traverse bit, so the b/two share is broken
	// and reconcile should repair it.
	if !reflect.DeepEqual(add, []string{home + "/b"}) {
		t.Fatalf("add = %v, want [%s/b]", add, home)
	}
	if len(remove) != 0 {
		t.Fatalf("both shares are live, nothing to remove, got %v", remove)
	}
}

// A grant sitting outside the home is not something we can place a corridor
// for. It means the tree contains an ACL we did not write, and quietly
// ignoring it would let reconcile compute a corridor from incomplete data.
func TestReconcileCorridorRefusesForeignGrant(t *testing.T) {
	const home = "/srv/bonnie/users/alice"
	entries := []aclEntry{
		{Path: "/srv/bonnie/users/bob/code", Perms: "r-x"},
	}
	if _, _, err := reconcileCorridor(home, entries); err == nil {
		t.Fatal("a grant outside the home must be an error, not silently skipped")
	}
}

func TestLinkNameFor(t *testing.T) {
	got := linkNameFor("/srv/bonnie/users/bob", "alice", "/srv/bonnie/users/alice/code/scratch")
	want := "/srv/bonnie/users/bob/shared/alice/scratch"
	if got != want {
		t.Fatalf("linkNameFor = %q, want %q", got, want)
	}

	// Namespacing by owner is what stops two people sharing a directory of the
	// same name from colliding in the sharee's home.
	other := linkNameFor("/srv/bonnie/users/bob", "carol", "/srv/bonnie/users/carol/code/scratch")
	if other == got {
		t.Fatalf("shares from different owners collided at %q", got)
	}
}
