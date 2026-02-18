package tools

import (
	"github.com/this-is-alpha-iota/clyde/api"
	"fmt"
	"os"
	"strings"
)

func init() {
	Register(writeFileTool, executeWriteFile, displayWriteFile)
}

var writeFileTool = api.Tool{
	Name:        "write_file",
	Description: "Write content to a file. This will create a new file or completely replace the contents of an existing file. Use this for creating new files or when you need to replace the entire file contents. For partial edits, use patch_file instead.",
	InputSchema: map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "The file path to write to. Can be absolute or relative to the current directory.",
			},
			"content": map[string]interface{}{
				"type":        "string",
				"description": "The complete content to write to the file.",
			},
		},
		"required": []string{"path", "content"},
	},
}

func executeWriteFile(input map[string]interface{}, apiClient *api.Client, conversationHistory []api.Message) (string, error) {
	path, pathOk := input["path"].(string)
	content, contentOk := input["content"].(string)

	if !pathOk || path == "" {
		return "", fmt.Errorf("file path is required. Example: write_file(\"notes.txt\", \"content\")")
	}
	if !contentOk {
		return "", fmt.Errorf("content parameter is required")
	}

	// Check if file exists to provide appropriate message
	fileExists := false
	existingSize := int64(0)
	if info, err := os.Stat(path); err == nil {
		fileExists = true
		existingSize = info.Size()

		// Warn if overwriting a large file
		if existingSize > 100*1024 { // 100KB
			suggestions := []string{
				fmt.Sprintf("Warning: You are about to replace the entire contents of '%s' (%d KB).",
					path, existingSize/1024),
				"",
				"If you meant to edit part of the file, use patch_file instead.",
				"write_file will completely replace all existing content.",
			}
			return "", fmt.Errorf("%s", strings.Join(suggestions, "\n"))
		}
	}

	// Check directory exists
	dir := path
	if lastSlash := strings.LastIndex(path, "/"); lastSlash > 0 {
		dir = path[:lastSlash]
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			return "", fmt.Errorf("directory '%s' does not exist. Create it first with: run_bash(\"mkdir -p %s\")", dir, dir)
		}
	}

	// Write the content
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		if os.IsPermission(err) {
			return "", fmt.Errorf("permission denied writing to '%s'. Check directory and file permissions", path)
		}
		return "", fmt.Errorf("failed to write file '%s': %w", path, err)
	}

	if fileExists {
		return fmt.Sprintf("Successfully replaced contents of %s (%d bytes written, was %d bytes)",
			path, len(content), existingSize), nil
	}
	return fmt.Sprintf("Successfully created %s (%d bytes written)", path, len(content)), nil
}

func displayWriteFile(input map[string]interface{}) string {
	path, _ := input["path"].(string)
	content, _ := input["content"].(string)
	
	// Format file size nicely
	size := len(content)
	var sizeStr string
	if size < 1024 {
		sizeStr = fmt.Sprintf("%d bytes", size)
	} else if size < 1024*1024 {
		sizeStr = fmt.Sprintf("%.1f KB", float64(size)/1024)
	} else {
		sizeStr = fmt.Sprintf("%.1f MB", float64(size)/(1024*1024))
	}
	return fmt.Sprintf("→ Writing file: %s (%s)", path, sizeStr)
}
