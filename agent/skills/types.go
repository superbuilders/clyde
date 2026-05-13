// Package skills implements discovery and cataloging of Agent Skills
// following the open agentskills.io / SKILL.md standard.
//
// Skills are folders containing a SKILL.md file with YAML frontmatter.
// They are discovered from two locations (in priority order):
//   1. Project-local: ./.agents/skills/
//   2. User-global:   ~/.agents/skills/
//
// The harness parses only the YAML frontmatter for the catalog; the model
// loads the full SKILL.md content via the existing read_file tool when needed.
package skills

// SkillMetadata holds the parsed YAML frontmatter from a SKILL.md file.
type SkillMetadata struct {
	// Name is the skill's identifier (from frontmatter or folder name fallback).
	Name string `yaml:"name"`
	// Description is a short summary shown in the catalog.
	Description string `yaml:"description"`
	// Version is the skill version (optional, informational).
	Version string `yaml:"version"`
	// Triggers is an optional list of trigger phrases (reserved for future use).
	Triggers []string `yaml:"triggers"`
	// Path is the filesystem path to the SKILL.md file (not from YAML; set by discovery).
	Path string `yaml:"-"`
}
