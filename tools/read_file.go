package tools

import (
	"github.com/this-is-alpha-iota/clyde/api"
	"fmt"
	"os"
)

func init() {
	Register(readFileTool, executeReadFile, displayReadFile)
}

var readFileTool = api.Tool{
	Name:        "read_file",
	Description: "Read the contents of a file at the specified path.",
	InputSchema: map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "The file path to read. Can be absolute or relative to the current directory.",
			},
		},
		"required": []string{"path"},
	},
}

func executeReadFile(input map[string]interface{}, apiClient *api.Client, conversationHistory []api.Message) (string, error) {
	path, ok := input["path"].(string)
	if !ok || path == "" {
		return "", fmt.Errorf("file path is required. Example: read_file(\"main.go\")")
	}

	// Check if file exists first
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("file '%s' does not exist. Use list_files to see available files", path)
		}
		if os.IsPermission(err) {
			return "", fmt.Errorf("permission denied reading '%s'. Check file permissions", path)
		}
		return "", fmt.Errorf("cannot access '%s': %w", path, err)
	}

	// Check if it's a directory
	if info.IsDir() {
		return "", fmt.Errorf("'%s' is a directory. Use list_files to list its contents instead", path)
	}

	// Check file size to warn about large files
	if info.Size() > 1024*1024 { // 1MB
		return "", fmt.Errorf("file '%s' is very large (%d MB). Consider reading a smaller section or using a different approach",
			path, info.Size()/(1024*1024))
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read file '%s': %w", path, err)
	}
	return string(content), nil
}

func displayReadFile(input map[string]interface{}) string {
	path, _ := input["path"].(string)
	return fmt.Sprintf("→ Reading file: %s", path)
}
