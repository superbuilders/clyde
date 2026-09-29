package provision

import (
	"os"
	"os/user"
	"path/filepath"
	"testing"
)

// TestInstallSkillsCopiesBundledSkills covers the mechanism a user's GitHub
// access depends on. Skill discovery reads ~/.agents/skills and nothing
// machine-wide, so a skill that fails to land is a user who cannot log in to
// GitHub at all — and the symptom is an agent that simply does not know how,
// which looks like a bad model rather than a missing file.
func TestInstallSkillsCopiesBundledSkills(t *testing.T) {
	src := t.TempDir()
	if err := os.MkdirAll(filepath.Join(src, "github-login"), 0o755); err != nil {
		t.Fatal(err)
	}
	body := []byte("---\nname: github-login\n---\nbody\n")
	if err := os.WriteFile(filepath.Join(src, "github-login", "SKILL.md"), body, 0o644); err != nil {
		t.Fatal(err)
	}
	// A stray file beside the skill folders must not be mistaken for one.
	if err := os.WriteFile(filepath.Join(src, "README"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A directory with no SKILL.md must be skipped, not error.
	if err := os.MkdirAll(filepath.Join(src, "notaskill"), 0o755); err != nil {
		t.Fatal(err)
	}

	home := t.TempDir()
	restore := skillsSourceDir
	skillsSourceDir = func() string { return src }
	defer func() { skillsSourceDir = restore }()

	u := &user.User{HomeDir: home}
	if err := installSkills(u, os.Getuid(), os.Getgid()); err != nil {
		t.Fatalf("installSkills: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(home, ".agents", "skills", "github-login", "SKILL.md"))
	if err != nil {
		t.Fatalf("skill was not installed: %v", err)
	}
	if string(got) != string(body) {
		t.Errorf("skill content = %q, want %q", got, body)
	}
	if _, err := os.Stat(filepath.Join(home, ".agents", "skills", "notaskill")); err == nil {
		t.Error("a directory with no SKILL.md was installed as a skill")
	}
	// The home is 0750, but M4.1 normalises the tree closed and this file
	// should not be the exception a later share widens.
	fi, err := os.Stat(filepath.Join(home, ".agents", "skills", "github-login", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm()&0o007 != 0 {
		t.Errorf("SKILL.md is readable by other: %v", fi.Mode().Perm())
	}
}

// A missing source directory is the developer-checkout case and must not fail
// a provision.
func TestInstallSkillsToleratesMissingSource(t *testing.T) {
	restore := skillsSourceDir
	skillsSourceDir = func() string { return filepath.Join(t.TempDir(), "absent") }
	defer func() { skillsSourceDir = restore }()

	u := &user.User{HomeDir: t.TempDir()}
	if err := installSkills(u, os.Getuid(), os.Getgid()); err != nil {
		t.Errorf("installSkills with no bundled skills should succeed, got %v", err)
	}
}
