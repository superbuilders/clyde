package tools

import (
	"claude-repl/api"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func init() {
	Register(grepTool, executeGrep, displayGrep)
}

var grepTool = api.Tool{
	Name:        "grep",
	Description: "Search for patterns across multiple files. Returns file paths and matching lines with context. Useful for finding function definitions, variable references, TODO comments, error messages, and configuration values.",
	InputSchema: map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"pattern": map[string]interface{}{
				"type":        "string",
				"description": "The search pattern (text or regex). Example: 'func main', 'TODO', 'error:'",
			},
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Directory to search. Defaults to current directory if not specified.",
			},
			"file_pattern": map[string]interface{}{
				"type":        "string",
				"description": "Optional: filter by file pattern using glob syntax. Example: '*.go', '*.md', 'test_*.py'",
			},
		},
		"required": []string{"pattern"},
	},
}

func executeGrep(input map[string]interface{}, apiClient *api.Client, conversationHistory []api.Message) (string, error) {
	pattern, patternOk := input["pattern"].(string)
	if !patternOk || pattern == "" {
		return "", fmt.Errorf("pattern is required. Example: grep(\"func main\") or grep(\"TODO\", \"src\", \"*.go\")")
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

	// Build the grep command
	// Use -r for recursive, -n for line numbers, -H for file names
	// Use -I to skip binary files
	args := []string{"-rnI", pattern, path}

	// Add file pattern if specified
	filePattern := ""
	if fpVal, ok := input["file_pattern"]; ok && fpVal != nil {
		filePattern, _ = fpVal.(string)
	}
	if filePattern != "" {
		args = append(args, "--include="+filePattern)
	}

	cmd := exec.Command("grep", args...)
	output, err := cmd.CombinedOutput()

	// grep returns exit code 1 if no matches found (not an error for us)
	if err != nil {
		exitErr, ok := err.(*exec.ExitError)
		if ok && exitErr.ExitCode() == 1 {
			// No matches found
			suggestions := []string{
				fmt.Sprintf("No matches found for pattern '%s' in %s", pattern, path),
			}
			if filePattern != "" {
				suggestions = append(suggestions, fmt.Sprintf("(searching files matching '%s')", filePattern))
			}
			suggestions = append(suggestions,
				"",
				"Suggestions:",
				"  - Check if the pattern is spelled correctly",
				"  - Try a simpler or broader search pattern",
				"  - Verify you're searching in the right directory",
			)
			if filePattern != "" {
				suggestions = append(suggestions, "  - Check if the file pattern matches existing files")
			}
			return strings.Join(suggestions, "\n"), nil
		}

		// Check for other grep errors
		if exitErr, ok := err.(*exec.ExitError); ok {
			if exitErr.ExitCode() == 2 {
				// grep syntax error or file not found
				return "", fmt.Errorf("grep error: %s\n\nOutput: %s\n\nCheck your pattern syntax or file paths",
					err, string(output))
			}
		}

		// Permission or other errors
		if strings.Contains(string(output), "Permission denied") {
			return "", fmt.Errorf("permission denied searching in '%s'. Some directories or files may not be accessible", path)
		}

		return "", fmt.Errorf("grep failed: %s\nOutput: %s", err, string(output))
	}

	// Success - format and return results
	if len(output) == 0 {
		return fmt.Sprintf("No matches found for pattern '%s' in %s", pattern, path), nil
	}

	// Count matches and files
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	matchCount := len(lines)

	// Count unique files
	fileSet := make(map[string]bool)
	for _, line := range lines {
		if colonIdx := strings.Index(line, ":"); colonIdx > 0 {
			filename := line[:colonIdx]
			fileSet[filename] = true
		}
	}
	fileCount := len(fileSet)

	// Build result with summary
	result := fmt.Sprintf("Found %d matches in %d files:\n\n%s", matchCount, fileCount, string(output))

	return result, nil
}

func displayGrep(input map[string]interface{}) string {
	pattern, _ := input["pattern"].(string)
	path := ""
	if pathVal, ok := input["path"]; ok && pathVal != nil {
		path, _ = pathVal.(string)
	}
	
	searchPath := path
	if searchPath == "" || searchPath == "." {
		searchPath = "current directory"
	}
	
	filePattern := ""
	if fpVal, ok := input["file_pattern"]; ok && fpVal != nil {
		filePattern, _ = fpVal.(string)
	}
	
	if filePattern != "" {
		return fmt.Sprintf("→ Searching: '%s' in %s (%s)", pattern, searchPath, filePattern)
	}
	return fmt.Sprintf("→ Searching: '%s' in %s", pattern, searchPath)
}
