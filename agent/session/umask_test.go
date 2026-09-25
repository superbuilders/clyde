//go:build unix

package session

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// The agent used to hardcode 0755/0644 when creating session directories and
// message files. PLAN.md §7 replaces those with 0777/0666 so that the caller's
// umask decides, which is what lets one binary serve three access regimes.
//
// These tests exist because the failure mode is silent. A hardcoded 0755
// directory looks completely normal; nothing errors, nothing logs. You only
// find out when a second user turns out to be able to read it — and in the
// sharing design (M4.2) a traverse bit makes that reachable, so the difference
// between 0750 and 0755 is the difference between sharing one directory and
// sharing everything below the home.
//
// umask is process-global, so these must not run in parallel with anything.
func TestSessionModesFollowUmask(t *testing.T) {
	tests := []struct {
		name    string
		umask   int
		wantDir os.FileMode
		wantMsg os.FileMode
		why     string
	}{
		{
			name:    "solo is unchanged from the hardcoded behaviour",
			umask:   0o022,
			wantDir: 0o755,
			wantMsg: 0o644,
			why:     "solo mode must be byte-identical to before this change (PLAN.md §8 item 2)",
		},
		{
			name:    "multi-user closes the tree to other",
			umask:   0o027,
			wantDir: 0o750,
			wantMsg: 0o640,
			why:     "if other retains any bit, an ACL traverse bit exposes the whole tree (M4.1)",
		},
		{
			name:    "team mode is group-writable",
			umask:   0o007,
			wantDir: 0o770,
			wantMsg: 0o660,
			why:     "a second user's agent must be able to write into a shared session (M5)",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir, msg := newSessionUnderUmask(t, tc.umask)
			if dir != tc.wantDir {
				t.Errorf("session dir mode = %04o, want %04o\n%s", dir, tc.wantDir, tc.why)
			}
			if msg != tc.wantMsg {
				t.Errorf("message file mode = %04o, want %04o\n%s", msg, tc.wantMsg, tc.why)
			}
		})
	}
}

// The property that actually matters for sharing, stated on its own so a
// regression names itself: under the multi-user umask, nothing an agent
// creates is readable by other.
func TestMultiUserUmaskGrantsNothingToOther(t *testing.T) {
	dir, msg := newSessionUnderUmask(t, 0o027)
	if dir&0o007 != 0 {
		t.Errorf("session dir is %04o: other has %03b, so a traverse bit would expose it", dir, dir&0o007)
	}
	if msg&0o007 != 0 {
		t.Errorf("message file is %04o: other has %03b, so its contents leak", msg, msg&0o007)
	}
}

// newSessionUnderUmask creates a real session under the given umask and
// returns the permission bits of its directory and of a message file in it.
func newSessionUnderUmask(t *testing.T, mask int) (dirMode, msgMode os.FileMode) {
	t.Helper()

	// A home of its own, so findSessionsRoot falls through to ~/.clyde/sessions
	// rather than finding this repo and writing into it.
	home := t.TempDir()
	t.Setenv("HOME", home)

	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	// Leave the repo before creating the session: findSessionsRoot asks git for
	// a toplevel first, and inside the checkout it would get one.
	if err := os.Chdir(home); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(wd) })

	old := syscall.Umask(mask)
	t.Cleanup(func() { syscall.Umask(old) })

	s, err := New()
	if err != nil {
		t.Fatalf("creating session under umask %04o: %v", mask, err)
	}
	if !isUnder(s.Dir, home) {
		t.Fatalf("session escaped the test home: %s not under %s", s.Dir, home)
	}
	if err := s.WriteMessage(TypeUser, "probe"); err != nil {
		t.Fatalf("writing message: %v", err)
	}

	di, err := os.Stat(s.Dir)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(s.Dir)
	if err != nil {
		t.Fatal(err)
	}
	var msg os.FileInfo
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".md" {
			if msg, err = e.Info(); err != nil {
				t.Fatal(err)
			}
			break
		}
	}
	if msg == nil {
		t.Fatalf("no message file written in %s", s.Dir)
	}
	return di.Mode().Perm(), msg.Mode().Perm()
}

func isUnder(path, root string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	// Only a leading ".." means escape. A leading "." is just a dotfile, which
	// is exactly what .clyde/sessions is.
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
