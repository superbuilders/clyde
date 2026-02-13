package tools

import (
	"claude-repl/api"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func init() {
	Register(listFilesTool, executeListFiles, displayListFiles)
}

var listFilesTool = api.Tool{
	Name:        "list_files",
	Description: "List files and directories in a specified path. Returns the output of 'ls -la' command.",
	InputSchema: map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "The directory path to list. Use '.' for current directory. Defaults to current directory if not specified.",
			},
		},
		"required": []string{},
	},
}

func executeListFiles(input map[string]interface{}, apiClient *api.Client, conversationHistory []api.Message) (string, error) {
	path := ""
	if pathVal, ok := input["path"]; ok && pathVal != nil {
		path, _ = pathVal.(string)
	}
	if path == "" {
		path = "."
	}
	
	cmd := exec.Command("ls", "-la", path)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Check if directory doesn't exist
		if _, statErr := os.Stat(path); os.IsNotExist(statErr) {
			return "", fmt.Errorf("directory '%s' does not exist. Use '.' for current directory or provide a valid path", path)
		}
		// Check for permission issues
		if strings.Contains(string(output), "Permission denied") {
			return "", fmt.Errorf("permission denied accessing '%s'. Check file permissions or try a different directory", path)
		}
		return "", fmt.Errorf("failed to list files in '%s': %s\nOutput: %s", path, err, string(output))
	}
	return string(output), nil
}

func displayListFiles(input map[string]interface{}) string {
	path := ""
	if pathVal, ok := input["path"]; ok && pathVal != nil {
		path, _ = pathVal.(string)
	}
	if path == "" || path == "." {
		return "→ Listing files: . (current directory)"
	}
	return fmt.Sprintf("→ Listing files: %s", path)
}
