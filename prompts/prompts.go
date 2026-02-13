package prompts

import (
	_ "embed"
	"os"
)

//go:embed system.txt
var embeddedSystemPrompt string

// SystemPrompt returns the system prompt.
// In development mode (when prompts/system.txt exists in current dir),
// it loads from the file to allow iteration without recompilation.
// In production mode (embedded binary), it uses the embedded version.
var SystemPrompt = loadSystemPrompt()

func loadSystemPrompt() string {
	// Try to load from file first (development mode)
	if content, err := os.ReadFile("prompts/system.txt"); err == nil {
		return string(content)
	}
	
	// Fallback to embedded version (production mode)
	return embeddedSystemPrompt
}

// GetSystemPrompt allows reloading the prompt at runtime (useful for testing)
func GetSystemPrompt() string {
	return loadSystemPrompt()
}
