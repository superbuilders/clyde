package skills

import (
	"fmt"
	"strings"
)

// Registry holds discovered skills and builds the catalog block
// for injection into the system prompt.
type Registry struct {
	skills   []SkillMetadata
	warnings []string
}

// NewRegistry creates an empty Registry.
func NewRegistry() *Registry {
	return &Registry{}
}

// Load discovers skills from the standard locations and populates
// the registry. It replaces any previously loaded skills.
func (r *Registry) Load() {
	r.skills, r.warnings = DiscoverAll()
}

// LoadFrom discovers skills from specific directories (for testing).
func (r *Registry) LoadFrom(dirs []string) {
	r.skills, r.warnings = DiscoverFrom(dirs)
}

// Skills returns all discovered skills.
func (r *Registry) Skills() []SkillMetadata {
	return r.skills
}

// Warnings returns any warnings generated during discovery/parsing.
func (r *Registry) Warnings() []string {
	return r.warnings
}

// BuildCatalogBlock returns the Markdown block to inject into the system prompt.
// Returns an empty string if no skills are discovered (zero overhead).
func (r *Registry) BuildCatalogBlock() string {
	if len(r.skills) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("\nYou have access to specialized Agent Skills.\n\nAvailable skills:\n")

	for _, s := range r.skills {
		if s.Description != "" {
			b.WriteString(fmt.Sprintf("• %s – %s (path: %s)\n", s.Name, s.Description, s.Path))
		} else {
			b.WriteString(fmt.Sprintf("• %s (path: %s)\n", s.Name, s.Path))
		}
	}

	b.WriteString("\nWhen a user request matches a skill's description, call the `read_file` tool on the skill's path to load its full instructions. Then follow those instructions exactly.\n")

	return b.String()
}
