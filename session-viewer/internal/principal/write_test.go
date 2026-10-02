package principal

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The bug these exist for: the service runs as root with CAP_DAC_READ_SEARCH
// but deliberately without CAP_DAC_OVERRIDE, so a direct os.Create or
// os.Remove inside a user's 0750 tree fails with EACCES. Upload and
// message-deletion both did exactly that and were broken in multi-user mode
// while working fine solo — which is why unit tests alone cannot catch this
// and the e2e gate has to run against the real box.
//
// What is testable here is the contract: solo behaves exactly as the plain
// os call did, and a non-solo principal refuses a path outside its home
// instead of silently writing somewhere it should not.

func TestWriteFromSoloWritesTheBytes(t *testing.T) {
	dir := t.TempDir()
	p := &Principal{Solo: true, Home: dir}
	path := filepath.Join(dir, "upload.png")

	if err := p.WriteFrom(path, strings.NewReader("hello bytes")); err != nil {
		t.Fatalf("WriteFrom: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading back: %v", err)
	}
	if string(got) != "hello bytes" {
		t.Fatalf("content = %q, want %q", got, "hello bytes")
	}
}

func TestWriteFromSoloTruncates(t *testing.T) {
	dir := t.TempDir()
	p := &Principal{Solo: true, Home: dir}
	path := filepath.Join(dir, "f")

	if err := os.WriteFile(path, []byte("a much longer original"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := p.WriteFrom(path, strings.NewReader("short")); err != nil {
		t.Fatalf("WriteFrom: %v", err)
	}
	got, _ := os.ReadFile(path)
	// Truncation, not overlay: a leftover tail would corrupt an uploaded
	// image in a way that only shows up when something tries to decode it.
	if string(got) != "short" {
		t.Fatalf("content = %q, want %q (not truncated?)", got, "short")
	}
}

func TestRemoveAsSoloRemoves(t *testing.T) {
	dir := t.TempDir()
	p := &Principal{Solo: true, Home: dir}
	path := filepath.Join(dir, "msg.md")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := p.RemoveAs(path); err != nil {
		t.Fatalf("RemoveAs: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("file still present after RemoveAs (err=%v)", err)
	}
}

// A non-solo principal must refuse to touch anything outside its own home,
// and the refusal must be a refusal -- not a nil error that silently did
// nothing, and not an attempt that happens to fail on permissions.
func TestWriteFromRefusesOutsideHome(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "home", "alice")
	if err := os.MkdirAll(home, 0o750); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(dir, "home", "bob", "stolen")
	if err := os.MkdirAll(filepath.Dir(outside), 0o750); err != nil {
		t.Fatal(err)
	}

	p := &Principal{Username: "alice", Home: home, UID: 1001, GID: 1001}
	err := p.WriteFrom(outside, strings.NewReader("payload"))
	if err == nil {
		t.Fatal("WriteFrom outside home returned nil error")
	}
	// Assert on the reason. Without this the test passes even if the write
	// was attempted and merely failed for an unrelated reason, which is the
	// failure mode that made the original bug invisible.
	if !strings.Contains(err.Error(), "refusing to write") {
		t.Fatalf("want a refusal, got %v", err)
	}
	if _, serr := os.Stat(outside); !os.IsNotExist(serr) {
		t.Fatal("refused write still created the file")
	}
}

func TestRemoveAsRefusesOutsideHome(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "home", "alice")
	if err := os.MkdirAll(home, 0o750); err != nil {
		t.Fatal(err)
	}
	victim := filepath.Join(dir, "home", "bob", "keepme")
	if err := os.MkdirAll(filepath.Dir(victim), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(victim, []byte("bob's"), 0o644); err != nil {
		t.Fatal(err)
	}

	p := &Principal{Username: "alice", Home: home, UID: 1001, GID: 1001}
	err := p.RemoveAs(victim)
	if err == nil {
		t.Fatal("RemoveAs outside home returned nil error")
	}
	if !strings.Contains(err.Error(), "refusing to remove") {
		t.Fatalf("want a refusal, got %v", err)
	}
	if _, serr := os.Stat(victim); serr != nil {
		t.Fatalf("refused remove deleted another user's file: %v", serr)
	}
}

// Sibling-prefix check: /home/alice-evil must not count as inside
// /home/alice. Owns() has this logic; these assert the write helpers
// actually consult it.
func TestWriteHelpersRejectSiblingPrefix(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "alice")
	evil := filepath.Join(dir, "alice-evil")
	for _, d := range []string{home, evil} {
		if err := os.MkdirAll(d, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	p := &Principal{Username: "alice", Home: home, UID: 1001, GID: 1001}

	if err := p.WriteFrom(filepath.Join(evil, "f"), strings.NewReader("x")); err == nil {
		t.Fatal("WriteFrom accepted a sibling-prefix path")
	}
	if err := p.RemoveAs(filepath.Join(evil, "f")); err == nil {
		t.Fatal("RemoveAs accepted a sibling-prefix path")
	}
}
