package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ── helpers ──────────────────────────────────────────────────────────────

// createSkill creates a skill folder with a SKILL.md file in the given base dir.
func createSkill(t *testing.T, baseDir, folderName, content string) string {
	t.Helper()
	dir := filepath.Join(baseDir, folderName)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "SKILL.md")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

// ── US-1: Discovery & Catalog ────────────────────────────────────────────

func TestDiscoverFrom_ValidSkill(t *testing.T) {
	tmp := t.TempDir()
	createSkill(t, tmp, "tdd-cycle", `---
name: tdd-cycle
description: Follow strict red-green-refactor workflow
version: "1.0"
---
# TDD Cycle
Step 1: Write a failing test.
`)

	skills, warnings := DiscoverFrom([]string{tmp})
	if len(warnings) != 0 {
		t.Errorf("unexpected warnings: %v", warnings)
	}
	if len(skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(skills))
	}
	s := skills[0]
	if s.Name != "tdd-cycle" {
		t.Errorf("name = %q, want %q", s.Name, "tdd-cycle")
	}
	if s.Description != "Follow strict red-green-refactor workflow" {
		t.Errorf("description = %q, want %q", s.Description, "Follow strict red-green-refactor workflow")
	}
	if s.Version != "1.0" {
		t.Errorf("version = %q, want %q", s.Version, "1.0")
	}
	if s.Path == "" {
		t.Error("path should be set")
	}
}

func TestDiscoverFrom_MultipleSkills(t *testing.T) {
	tmp := t.TempDir()
	createSkill(t, tmp, "frontend-design", `---
name: frontend-design
description: Apply modern, accessible, responsive UI/UX best practices
---
# Frontend Design
`)
	createSkill(t, tmp, "security-scan", `---
name: security-scan
description: Check for OWASP Top 10 issues
---
# Security Scan
`)

	skills, _ := DiscoverFrom([]string{tmp})
	if len(skills) != 2 {
		t.Fatalf("expected 2 skills, got %d", len(skills))
	}

	names := map[string]bool{}
	for _, s := range skills {
		names[s.Name] = true
	}
	if !names["frontend-design"] || !names["security-scan"] {
		t.Errorf("expected frontend-design and security-scan, got %v", names)
	}
}

func TestDiscoverFrom_NoFrontmatter_FallbackToFolderName(t *testing.T) {
	tmp := t.TempDir()
	createSkill(t, tmp, "my-raw-skill", `# My Raw Skill
No frontmatter here, just instructions.
`)

	skills, warnings := DiscoverFrom([]string{tmp})
	if len(skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(skills))
	}
	if skills[0].Name != "my-raw-skill" {
		t.Errorf("name = %q, want folder name %q", skills[0].Name, "my-raw-skill")
	}
	if len(warnings) == 0 {
		t.Error("expected a warning about missing frontmatter")
	}
}

func TestDiscoverFrom_MalformedYAML_FallbackToFolderName(t *testing.T) {
	tmp := t.TempDir()
	createSkill(t, tmp, "broken-skill", `---
name: [this is invalid yaml
description: broken
---
# Broken
`)

	skills, warnings := DiscoverFrom([]string{tmp})
	if len(skills) != 1 {
		t.Fatalf("expected 1 skill (raw fallback), got %d", len(skills))
	}
	if skills[0].Name != "broken-skill" {
		t.Errorf("name = %q, want folder fallback %q", skills[0].Name, "broken-skill")
	}
	if len(warnings) == 0 {
		t.Error("expected a warning about malformed YAML")
	}
	if !strings.Contains(warnings[0], "malformed") {
		t.Errorf("warning = %q, expected it to mention 'malformed'", warnings[0])
	}
}

func TestDiscoverFrom_EmptyDir_NoSkills(t *testing.T) {
	tmp := t.TempDir()
	skills, warnings := DiscoverFrom([]string{tmp})
	if len(skills) != 0 {
		t.Errorf("expected 0 skills from empty dir, got %d", len(skills))
	}
	if len(warnings) != 0 {
		t.Errorf("expected 0 warnings, got %v", warnings)
	}
}

func TestDiscoverFrom_NonexistentDir_NoError(t *testing.T) {
	skills, warnings := DiscoverFrom([]string{"/nonexistent/path/12345"})
	if len(skills) != 0 {
		t.Errorf("expected 0 skills, got %d", len(skills))
	}
	if len(warnings) != 0 {
		t.Errorf("expected 0 warnings, got %v", warnings)
	}
}

func TestDiscoverFrom_FolderWithoutSkillMD_Ignored(t *testing.T) {
	tmp := t.TempDir()
	// Create a folder that does NOT contain SKILL.md
	dir := filepath.Join(tmp, "not-a-skill")
	os.MkdirAll(dir, 0755)
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Not a skill"), 0644)

	skills, _ := DiscoverFrom([]string{tmp})
	if len(skills) != 0 {
		t.Errorf("expected 0 skills, got %d", len(skills))
	}
}

func TestDiscoverFrom_Dedup_ProjectLocalWins(t *testing.T) {
	localDir := t.TempDir()
	globalDir := t.TempDir()

	createSkill(t, localDir, "tdd", `---
name: tdd
description: Local TDD
---
`)
	createSkill(t, globalDir, "tdd", `---
name: tdd
description: Global TDD
---
`)

	skills, _ := DiscoverFrom([]string{localDir, globalDir})
	if len(skills) != 1 {
		t.Fatalf("expected 1 skill (deduped), got %d", len(skills))
	}
	if skills[0].Description != "Local TDD" {
		t.Errorf("expected local skill to win, got description %q", skills[0].Description)
	}
}

func TestDiscoverFrom_FileInDir_NotFolder_Ignored(t *testing.T) {
	tmp := t.TempDir()
	// Create a regular file (not a directory) in the skills dir
	os.WriteFile(filepath.Join(tmp, "stray-file.md"), []byte("random"), 0644)

	skills, _ := DiscoverFrom([]string{tmp})
	if len(skills) != 0 {
		t.Errorf("expected 0 skills, got %d", len(skills))
	}
}

func TestDiscoverFrom_EmptyName_FallbackToFolder(t *testing.T) {
	tmp := t.TempDir()
	createSkill(t, tmp, "my-skill", `---
description: Has description but no name
---
`)

	skills, _ := DiscoverFrom([]string{tmp})
	if len(skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(skills))
	}
	if skills[0].Name != "my-skill" {
		t.Errorf("name = %q, want folder fallback %q", skills[0].Name, "my-skill")
	}
	if skills[0].Description != "Has description but no name" {
		t.Errorf("description not parsed correctly: %q", skills[0].Description)
	}
}

func TestDiscoverFrom_TriggersField(t *testing.T) {
	tmp := t.TempDir()
	createSkill(t, tmp, "with-triggers", `---
name: with-triggers
description: A skill with triggers
triggers:
  - design
  - ui
---
`)

	skills, _ := DiscoverFrom([]string{tmp})
	if len(skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(skills))
	}
	if len(skills[0].Triggers) != 2 {
		t.Errorf("expected 2 triggers, got %d: %v", len(skills[0].Triggers), skills[0].Triggers)
	}
}

// ── Registry & Catalog Block ─────────────────────────────────────────────

func TestRegistry_BuildCatalogBlock_NoSkills(t *testing.T) {
	reg := NewRegistry()
	block := reg.BuildCatalogBlock()
	if block != "" {
		t.Errorf("expected empty catalog block with no skills, got %q", block)
	}
}

func TestRegistry_BuildCatalogBlock_WithSkills(t *testing.T) {
	tmp := t.TempDir()
	createSkill(t, tmp, "tdd-cycle", `---
name: tdd-cycle
description: Follow strict red-green-refactor workflow
---
`)
	createSkill(t, tmp, "security-scan", `---
name: security-scan
description: Check for OWASP Top 10 issues
---
`)

	reg := NewRegistry()
	reg.LoadFrom([]string{tmp})

	block := reg.BuildCatalogBlock()
	if block == "" {
		t.Fatal("expected non-empty catalog block")
	}

	// Must contain the catalog header
	if !strings.Contains(block, "You have access to specialized Agent Skills.") {
		t.Error("catalog missing header")
	}

	// Must contain both skill entries
	if !strings.Contains(block, "tdd-cycle") {
		t.Error("catalog missing tdd-cycle")
	}
	if !strings.Contains(block, "security-scan") {
		t.Error("catalog missing security-scan")
	}

	// Must contain descriptions
	if !strings.Contains(block, "Follow strict red-green-refactor workflow") {
		t.Error("catalog missing tdd-cycle description")
	}

	// Must contain paths
	if !strings.Contains(block, "SKILL.md") {
		t.Error("catalog missing skill paths")
	}

	// Must contain the instruction to use read_file
	if !strings.Contains(block, "read_file") {
		t.Error("catalog missing read_file instruction")
	}
}

func TestRegistry_BuildCatalogBlock_SkillWithoutDescription(t *testing.T) {
	tmp := t.TempDir()
	createSkill(t, tmp, "minimal", `---
name: minimal
---
# Minimal Skill
`)

	reg := NewRegistry()
	reg.LoadFrom([]string{tmp})

	block := reg.BuildCatalogBlock()
	if !strings.Contains(block, "• minimal") {
		t.Error("catalog missing minimal skill entry")
	}
	// Should NOT have a dash after name (no description)
	if strings.Contains(block, "minimal –") {
		t.Error("catalog should not have dash for skill without description")
	}
}

func TestRegistry_LoadFrom_Reload(t *testing.T) {
	tmp := t.TempDir()
	createSkill(t, tmp, "v1-skill", `---
name: v1-skill
description: Version 1
---
`)

	reg := NewRegistry()
	reg.LoadFrom([]string{tmp})

	if len(reg.Skills()) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(reg.Skills()))
	}

	// Now add another skill and reload
	createSkill(t, tmp, "v2-skill", `---
name: v2-skill
description: Version 2
---
`)

	reg.LoadFrom([]string{tmp})
	if len(reg.Skills()) != 2 {
		t.Fatalf("expected 2 skills after reload, got %d", len(reg.Skills()))
	}
}

// ── US-2: Model can load skill via read_file ─────────────────────────────

func TestSkillPath_ReadableByReadFile(t *testing.T) {
	// This test verifies that the paths stored in SkillMetadata are valid
	// filesystem paths that can be read (simulating what read_file would do).
	tmp := t.TempDir()
	skillContent := `---
name: test-skill
description: A test skill
---
# Test Skill

## Instructions
1. Do this
2. Do that
`
	createSkill(t, tmp, "test-skill", skillContent)

	skills, _ := DiscoverFrom([]string{tmp})
	if len(skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(skills))
	}

	// Simulate read_file: read the path from the skill metadata
	content, err := os.ReadFile(skills[0].Path)
	if err != nil {
		t.Fatalf("failed to read skill at path %q: %v", skills[0].Path, err)
	}

	// Full content (including frontmatter) should be returned
	if !strings.Contains(string(content), "# Test Skill") {
		t.Error("read content missing skill body")
	}
	if !strings.Contains(string(content), "Do this") {
		t.Error("read content missing instructions")
	}
}

func TestCatalogBlock_ContainsPaths(t *testing.T) {
	// The catalog block must contain the path so the model knows what to read_file
	tmp := t.TempDir()
	createSkill(t, tmp, "design", `---
name: design
description: Apply design patterns
---
`)

	reg := NewRegistry()
	reg.LoadFrom([]string{tmp})

	block := reg.BuildCatalogBlock()
	skills := reg.Skills()
	for _, s := range skills {
		if !strings.Contains(block, s.Path) {
			t.Errorf("catalog block does not contain path %q for skill %q", s.Path, s.Name)
		}
	}
}

// ── Edge Cases ───────────────────────────────────────────────────────────

func TestParseSkillFile_EmptyFrontmatter(t *testing.T) {
	tmp := t.TempDir()
	createSkill(t, tmp, "empty-fm", `---
---
# Empty frontmatter
`)

	skills, _ := DiscoverFrom([]string{tmp})
	if len(skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(skills))
	}
	// Should fall back to folder name
	if skills[0].Name != "empty-fm" {
		t.Errorf("name = %q, want %q", skills[0].Name, "empty-fm")
	}
}

func TestParseSkillFile_FrontmatterOnly(t *testing.T) {
	tmp := t.TempDir()
	createSkill(t, tmp, "fm-only", `---
name: fm-only
description: Just frontmatter, no body
---`)

	skills, _ := DiscoverFrom([]string{tmp})
	if len(skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(skills))
	}
	if skills[0].Name != "fm-only" {
		t.Errorf("name = %q, want %q", skills[0].Name, "fm-only")
	}
}

func TestParseSkillFile_ExtraFieldsIgnored(t *testing.T) {
	tmp := t.TempDir()
	createSkill(t, tmp, "extras", `---
name: extras
description: Has extra fields
author: Someone
license: MIT
custom_field: value
---
# Extras
`)

	skills, warnings := DiscoverFrom([]string{tmp})
	if len(warnings) != 0 {
		t.Errorf("unexpected warnings for extra YAML fields: %v", warnings)
	}
	if len(skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(skills))
	}
	if skills[0].Name != "extras" {
		t.Errorf("name = %q, want %q", skills[0].Name, "extras")
	}
}
