package tools

import (
	"claude-repl/api"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func init() {
	Register(globTool, executeGlob, displayGlob)
}

var globTool = api.Tool{
	Name:        "glob",
	Description: "Find files matching patterns. More flexible than list_files for navigating projects. Returns file paths that match the pattern. Useful for finding specific files in large codebases.",
	InputSchema: map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"pattern": map[string]interface{}{
				"type":        "string",
				"description": "File pattern to match. Examples: '**/*.go' (all Go files), '*_test.go' (test files), '*.md' (markdown files), '**/main.go' (find main.go anywhere)",
			},
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Directory to search. Defaults to current directory if not specified.",
			},
		},
		"required": []string{"pattern"},
	},
}

func executeGlob(input map[string]interface{}, apiClient *api.Client, conversationHistory []api.Message) (string, error) {
	pattern, patternOk := input["pattern"].(string)
	if !patternOk || pattern == "" {
		return "", fmt.Errorf("pattern is required. Example: glob(\"**/*.go\") or glob(\"*_test.go\", \"src\")")
	}

	// Default to current directory if no path specified
	path := ""
	if pathVal, ok := input["path"]; ok && pathVal != nil {
		path, _ = pathVal.(string)
	}
	if path == "" {
		path = "."
	}

	// Check if search path exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return "", fmt.Errorf("directory '%s' does not exist. Use '.' for current directory or provide a valid path", path)
	}

	// Use find command with -name or -path depending on pattern
	// ** patterns need -path, simple patterns need -name
	var args []string
	if strings.Contains(pattern, "**") {
		// Recursive pattern - use -path
		// Convert ** glob pattern to find -path pattern
		// Example: **/*.go -> */*.go (find already recurses by default)
		findPattern := strings.ReplaceAll(pattern, "**/", "")
		if !strings.HasPrefix(findPattern, "*") {
			findPattern = "*/" + findPattern
		}
		args = []string{path, "-path", findPattern, "-type", "f"}
	} else {
		// Simple pattern - use -name
		args = []string{path, "-name", pattern, "-type", "f"}
	}

	cmd := exec.Command("find", args...)
	output, err := cmd.CombinedOutput()

	if err != nil {
		// Check for permission errors
		if strings.Contains(string(output), "Permission denied") {
			return "", fmt.Errorf("permission denied searching in '%s'. Some directories may not be accessible", path)
		}
		return "", fmt.Errorf("find command failed: %s\nOutput: %s", err, string(output))
	}

	// Process results
	if len(output) == 0 || strings.TrimSpace(string(output)) == "" {
		suggestions := []string{
			fmt.Sprintf("No files found matching pattern '%s' in %s", pattern, path),
			"",
			"Suggestions:",
			"  - Check if the pattern is correct",
			"  - Try a broader pattern (e.g., '*.go' instead of 'main.go')",
			"  - Use '**/*.go' to search recursively",
			"  - Verify you're searching in the right directory",
			"",
			"Pattern examples:",
			"  - '*.go' - all Go files in directory",
			"  - '**/*.go' - all Go files recursively",
			"  - '*_test.go' - all test files in directory",
			"  - '**/main.go' - find main.go anywhere",
		}
		return strings.Join(suggestions, "\n"), nil
	}

	// Count files and format output
	files := strings.Split(strings.TrimSpace(string(output)), "\n")
	fileCount := len(files)

	// Build result with summary
	result := fmt.Sprintf("Found %d files matching '%s':\n\n%s", fileCount, pattern, string(output))

	return result, nil
}

func displayGlob(input map[string]interface{}) string {
	pattern, _ := input["pattern"].(string)
	path := ""
	if pathVal, ok := input["path"]; ok && pathVal != nil {
		path, _ = pathVal.(string)
	}
	
	searchPath := path
	if searchPath == "" || searchPath == "." {
		searchPath = "current directory"
	}
	
	return fmt.Sprintf("→ Finding files: '%s' in %s", pattern, searchPath)
}
