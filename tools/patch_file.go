package tools

import (
	"claude-repl/api"
	"fmt"
	"os"
	"strings"
)

func init() {
	Register(patchFileTool, executePatchFile, displayPatchFile)
}

var patchFileTool = api.Tool{
	Name:        "patch_file",
	Description: "Edit a file by finding and replacing text. This is a patch-based approach that only requires the specific text to change, not the entire file. To use: (1) use read_file to see current content, (2) identify a unique string to replace, (3) provide the old text and new text. The old_text must match exactly and be unique in the file.",
	InputSchema: map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "The file path to edit. Can be absolute or relative to the current directory.",
			},
			"old_text": map[string]interface{}{
				"type":        "string",
				"description": "The exact text to find and replace. This must be unique in the file. Include enough context to make it unique (e.g., surrounding lines).",
			},
			"new_text": map[string]interface{}{
				"type":        "string",
				"description": "The new text to replace old_text with. Can be empty string to delete the old text.",
			},
		},
		"required": []string{"path", "old_text", "new_text"},
	},
}

func executePatchFile(input map[string]interface{}, apiClient *api.Client, conversationHistory []api.Message) (string, error) {
	path, pathOk := input["path"].(string)
	oldText, oldTextOk := input["old_text"].(string)
	newText, newTextOk := input["new_text"].(string)

	if !pathOk || path == "" {
		return "", fmt.Errorf("file path is required. Example: patch_file(\"main.go\", \"old text\", \"new text\")")
	}
	if !oldTextOk || oldText == "" {
		return "", fmt.Errorf("old_text is required and cannot be empty. This is the text you want to replace")
	}
	if !newTextOk {
		return "", fmt.Errorf("new_text is required (can be empty string to delete)")
	}

	// Check if file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return "", fmt.Errorf("file '%s' does not exist. Use write_file to create a new file, or use list_files to see available files", path)
	}

	// Read the current file content
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsPermission(err) {
			return "", fmt.Errorf("permission denied reading '%s'. Check file permissions", path)
		}
		return "", fmt.Errorf("failed to read file '%s': %w", path, err)
	}

	fileContent := string(content)

	// Check if old_text exists in the file
	if !strings.Contains(fileContent, oldText) {
		// Provide helpful suggestions
		suggestions := []string{
			"The old_text was not found in the file. Common issues:",
			"  1. Whitespace or newlines don't match exactly",
			"  2. The text has already been changed",
			"  3. There's a typo in old_text",
			"",
			"Suggestions:",
			"  - Use read_file first to see the current content",
			"  - Copy the exact text including all whitespace",
			"  - Check for tabs vs spaces, line endings, etc.",
		}
		return "", fmt.Errorf("%s", strings.Join(suggestions, "\n"))
	}

	// Count occurrences to ensure it's unique
	occurrences := strings.Count(fileContent, oldText)
	if occurrences > 1 {
		suggestions := []string{
			fmt.Sprintf("The old_text appears %d times in the file. It must be unique to ensure the right text is replaced.", occurrences),
			"",
			"To fix this:",
			"  1. Include more surrounding context in old_text",
			"  2. Add nearby lines or unique identifiers",
			"  3. Example: Instead of just 'func foo()', use 'func foo() {\\n\\t// comment\\n\\treturn nil'",
			"",
			"Use read_file to see the full context around each occurrence.",
		}
		return "", fmt.Errorf("%s", strings.Join(suggestions, "\n"))
	}

	// Replace the text
	newContent := strings.Replace(fileContent, oldText, newText, 1)

	// Write the modified content back
	if err := os.WriteFile(path, []byte(newContent), 0644); err != nil {
		if os.IsPermission(err) {
			return "", fmt.Errorf("permission denied writing to '%s'. Check file permissions", path)
		}
		return "", fmt.Errorf("failed to write file '%s': %w", path, err)
	}

	changeSize := len(newText) - len(oldText)
	return fmt.Sprintf("Successfully patched %s: replaced %d bytes with %d bytes (change: %+d bytes)",
		path, len(oldText), len(newText), changeSize), nil
}

func displayPatchFile(input map[string]interface{}) string {
	path, _ := input["path"].(string)
	oldText, _ := input["old_text"].(string)
	newText, _ := input["new_text"].(string)
	
	changeSize := len(newText) - len(oldText)
	if changeSize >= 0 {
		return fmt.Sprintf("→ Patching file: %s (+%d bytes)", path, changeSize)
	}
	return fmt.Sprintf("→ Patching file: %s (%d bytes)", path, changeSize)
}
