//go:build unix

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// closeToOther is the repair half of M4.1: the umask governs new content, this
// governs everything an agent created before the umask changed. If it misses
// anything, M4.2's traverse bits make that thing reachable.
func TestCloseToOther(t *testing.T) {
	root := t.TempDir()

	// A tree with the modes an agent leaves behind under the default umask.
	mustMkdir(t, filepath.Join(root, "code"), 0o755)
	mustMkdir(t, filepath.Join(root, "code", "scratch"), 0o755)
	mustMkdir(t, filepath.Join(root, "code", "scratch", ".clyde"), 0o700)
	mustWrite(t, filepath.Join(root, "code", "scratch", "notes.md"), 0o644)
	mustWrite(t, filepath.Join(root, "code", "scratch", "secret"), 0o600)

	if err := closeToOther(root); err != nil {
		t.Fatalf("closeToOther: %v", err)
	}

	// Nothing anywhere may grant anything to other. This is the whole point,
	// so assert it over the tree rather than on a sample.
	if err := filepath.Walk(root, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if m := fi.Mode().Perm(); m&0o007 != 0 {
			t.Errorf("%s is %04o: other retains %03b", path, m, m&0o007)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	// Owner and group access must survive. Over-tightening would lock the user
	// out of their own tree, which fails closed but is still broken.
	if m := modeOf(t, filepath.Join(root, "code")); m != 0o750 {
		t.Errorf("code = %04o, want 0750 (owner and group preserved)", m)
	}
	if m := modeOf(t, filepath.Join(root, "code", "scratch", "notes.md")); m != 0o640 {
		t.Errorf("notes.md = %04o, want 0640", m)
	}

	// Already-closed entries must be left exactly as they were, not widened to
	// some uniform mode.
	if m := modeOf(t, filepath.Join(root, "code", "scratch", ".clyde")); m != 0o700 {
		t.Errorf(".clyde = %04o, want 0700 unchanged", m)
	}
	if m := modeOf(t, filepath.Join(root, "code", "scratch", "secret")); m != 0o600 {
		t.Errorf("secret = %04o, want 0600 unchanged", m)
	}
}

// Running provision twice is normal, so the repair must be idempotent.
func TestCloseToOtherIsIdempotent(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "a"), 0o755)

	if err := closeToOther(root); err != nil {
		t.Fatal(err)
	}
	first := modeOf(t, filepath.Join(root, "a"))
	if err := closeToOther(root); err != nil {
		t.Fatal(err)
	}
	if second := modeOf(t, filepath.Join(root, "a")); second != first {
		t.Errorf("second run changed mode from %04o to %04o", first, second)
	}
}

// The group bits must be preserved exactly, because on a file carrying a POSIX
// ACL they are the ACL mask. A blanket chmod would clamp every share granted so
// far — silently, since the ACL entries would still be listed by getfacl while
// no longer granting anything.
func TestCloseToOtherPreservesGroupBits(t *testing.T) {
	root := t.TempDir()
	// 0705: no group access at all. A careless implementation that rewrote the
	// mode to a fixed 0750 would *add* group access here.
	mustWrite(t, filepath.Join(root, "f"), 0o705)

	if err := closeToOther(root); err != nil {
		t.Fatal(err)
	}
	if m := modeOf(t, filepath.Join(root, "f")); m != 0o700 {
		t.Errorf("f = %04o, want 0700: only other's bits should have been cleared", m)
	}
}

// A symlink must not be followed. Following one that points into another
// user's tree would rewrite modes outside the home we were asked to close.
func TestCloseToOtherDoesNotFollowSymlinks(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()

	victim := filepath.Join(outside, "victim")
	mustWrite(t, victim, 0o644)
	if err := os.Symlink(victim, filepath.Join(root, "link")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	if err := closeToOther(root); err != nil {
		t.Fatal(err)
	}
	if m := modeOf(t, victim); m != 0o644 {
		t.Errorf("symlink target became %04o: closeToOther escaped the tree it was given", m)
	}
}

func mustMkdir(t *testing.T, path string, mode os.FileMode) {
	t.Helper()
	if err := os.Mkdir(path, mode); err != nil {
		t.Fatal(err)
	}
	// Mkdir applies the test process's umask, so set the mode explicitly.
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

func mustWrite(t *testing.T, path string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte("x"), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

func modeOf(t *testing.T, path string) os.FileMode {
	t.Helper()
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return fi.Mode().Perm()
}
