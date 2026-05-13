package skills

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// discoverSkillDirs returns the list of directories to scan for skills,
// in priority order: project-local first, then user-global.
func discoverSkillDirs() []string {
	var dirs []string

	// 1. Project-local: ./.agents/skills/
	dirs = append(dirs, filepath.Join(".", ".agents", "skills"))

	// 2. User-global: ~/.agents/skills/
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, ".agents", "skills"))
	}

	return dirs
}

// scanDir scans a single directory for skill folders (each containing SKILL.md).
// Returns discovered skills and any warnings for malformed skills.
func scanDir(dir string) ([]SkillMetadata, []string) {
	var skills []SkillMetadata
	var warnings []string

	entries, err := os.ReadDir(dir)
	if err != nil {
		// Directory doesn't exist or isn't readable — not an error
		return nil, nil
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		skillFile := filepath.Join(dir, entry.Name(), "SKILL.md")
		meta, warning := parseSkillFile(skillFile, entry.Name())
		if meta != nil {
			skills = append(skills, *meta)
		}
		if warning != "" {
			warnings = append(warnings, warning)
		}
	}

	return skills, warnings
}

// parseSkillFile reads and parses a SKILL.md file's YAML frontmatter.
// If frontmatter is missing or invalid, it falls back to a "raw" skill
// using the folder name. Returns nil if the file doesn't exist.
func parseSkillFile(path string, folderName string) (*SkillMetadata, string) {
	f, err := os.Open(path)
	if err != nil {
		return nil, "" // no SKILL.md in this folder — skip silently
	}
	defer f.Close()

	// Read frontmatter (between --- delimiters)
	scanner := bufio.NewScanner(f)
	var frontmatter strings.Builder
	inFrontmatter := false
	foundFrontmatter := false

	for scanner.Scan() {
		line := scanner.Text()
		if !inFrontmatter {
			if strings.TrimSpace(line) == "---" {
				inFrontmatter = true
				continue
			}
			// Non-frontmatter content before opening --- means no frontmatter
			break
		}
		if strings.TrimSpace(line) == "---" {
			foundFrontmatter = true
			break
		}
		frontmatter.WriteString(line)
		frontmatter.WriteString("\n")
	}

	meta := &SkillMetadata{
		Path: path,
	}

	if foundFrontmatter && frontmatter.Len() > 0 {
		if err := yaml.Unmarshal([]byte(frontmatter.String()), meta); err != nil {
			// Malformed YAML — use raw fallback
			meta.Name = folderName
			return meta, fmt.Sprintf("skills: malformed YAML frontmatter in %s: %v", path, err)
		}
	}

	// Fallback: use folder name if name is empty
	if meta.Name == "" {
		meta.Name = folderName
		if !foundFrontmatter {
			return meta, fmt.Sprintf("skills: no YAML frontmatter in %s, using folder name %q", path, folderName)
		}
	}

	return meta, ""
}

// DiscoverAll scans all discovery locations and returns deduplicated skills.
// Project-local skills take priority over global skills with the same name.
func DiscoverAll() ([]SkillMetadata, []string) {
	dirs := discoverSkillDirs()
	seen := make(map[string]bool)
	var allSkills []SkillMetadata
	var allWarnings []string

	for _, dir := range dirs {
		skills, warnings := scanDir(dir)
		allWarnings = append(allWarnings, warnings...)
		for _, s := range skills {
			if !seen[s.Name] {
				seen[s.Name] = true
				allSkills = append(allSkills, s)
			}
		}
	}

	return allSkills, allWarnings
}

// DiscoverFrom scans specific directories for skills (used for testing
// and custom configurations). Same dedup rules as DiscoverAll.
func DiscoverFrom(dirs []string) ([]SkillMetadata, []string) {
	seen := make(map[string]bool)
	var allSkills []SkillMetadata
	var allWarnings []string

	for _, dir := range dirs {
		skills, warnings := scanDir(dir)
		allWarnings = append(allWarnings, warnings...)
		for _, s := range skills {
			if !seen[s.Name] {
				seen[s.Name] = true
				allSkills = append(allSkills, s)
			}
		}
	}

	return allSkills, allWarnings
}
