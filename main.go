package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
)

const (
	apiURL      = "https://api.anthropic.com/v1/messages"
	modelID     = "claude-sonnet-4-5-20250929"
	maxTokens   = 4096
	systemPrompt = `You are a helpful AI assistant with access to several tools:

1. list_files: For listing files and directories in a given path
2. read_file: For reading the contents of a file
3. patch_file: For editing files using find/replace (patch-based approach)
4. write_file: For creating new files or completely replacing file contents
5. run_bash: For executing arbitrary bash commands (including gh, git, etc.)
6. grep: For searching patterns across multiple files with context
7. glob: For finding files matching patterns (fuzzy file finding)

IMPORTANT DECIDER: Before responding, determine if you need to use a tool:

GitHub questions - Use run_bash with gh commands:
- Questions about repositories: run_bash("gh repo list")
- Questions about pull requests: run_bash("gh pr list")
- Questions about issues: run_bash("gh issue list")
- User profile info: run_bash("gh api user")
- Any GitHub queries: run_bash("gh <command>")

File system questions - Use list_files for:
- "What files are in X directory?"
- "List files in the current folder"
- "Show me the contents of this directory"

File reading questions - Use read_file for:
- "Show me the contents of X file"
- "What's in X file?"
- "Read X file"

File editing questions - Use patch_file for:
- "Add X to the file"
- "Change X to Y in the file"
- "Update the function to do Z"
- "Fix the bug by changing X"

File writing questions - Use write_file for:
- "Create a new file with X content"
- "Write X to file Y"
- "Replace the entire contents of file Z"
- Creating new files from scratch

Search questions - Use grep for:
- "Find all references to X"
- "Where is function Y defined?"
- "Search for TODO comments"
- "Find error messages in logs"
- "Locate all files containing X"
- Can filter by file pattern: grep("TODO", ".", "*.go")

File finding questions - Use glob for:
- "Find all test files"
- "Where are all the Go files?"
- "Find all markdown files recursively"
- "Locate main.go anywhere in the project"
- Pattern examples: glob("**/*.go"), glob("*_test.go")

Bash execution - Use run_bash for:
- "Run X command"
- "Execute Y script"
- "Check system information"
- Any shell/command-line operations
- Git operations: run_bash("git status"), run_bash("git commit -m 'message'")
- GitHub CLI: run_bash("gh repo list"), run_bash("gh pr list")
- Package managers, build tools, test runners, etc.

CRITICAL: For patch_file, you MUST:
1. First use read_file to see current content
2. Identify a unique string to replace (include enough surrounding context)
3. Use patch_file with exact old_text and new_text
4. The old_text must be unique in the file (will error if it appears multiple times)

DOCUMENTATION & MEMORY:
When working on tasks, especially complex ones:
1. Read progress.md (if it exists) at the start to understand:
   - Project history and architecture decisions
   - Previous bugs fixed and lessons learned
   - Design patterns and principles being followed
   - Current status and what's been completed

2. Update progress.md when you:
   - Complete a major task or milestone
   - Discover and fix bugs (document the issue, root cause, and fix)
   - Make important design decisions
   - Learn patterns that should be followed consistently
   - Add or modify features

3. ALWAYS update progress.md BEFORE making the final commit:
   - Don't wait to be reminded
   - Treat documentation as part of the task completion
   - Think: "Is this change significant enough to document?" (Usually yes)

4. Keep progress.md structured and curated:
   - Use clear sections (Bugs Fixed, Features Added, Design Decisions, etc.)
   - Write for future readers (including future versions of yourself)
   - Include examples, rationale, and lessons learned
   - Don't dump raw conversation - synthesize and organize

5. Treat progress.md as YOUR memory:
   - It persists across conversations
   - It's more valuable than raw chat history
   - It's what you'll read next time to continue work
   - Maintain it actively as the project evolves

Always use the appropriate tool first, then provide a natural response based on the results.`
)

type Message struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"`
}

type Tool struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema interface{} `json:"input_schema"`
}

type Request struct {
	Model     string    `json:"model"`
	MaxTokens int       `json:"max_tokens"`
	System    string    `json:"system"`
	Messages  []Message `json:"messages"`
	Tools     []Tool    `json:"tools,omitempty"`
}

type ContentBlock struct {
	Type      string                 `json:"type"`
	Text      string                 `json:"text,omitempty"`
	ID        string                 `json:"id,omitempty"`
	Name      string                 `json:"name,omitempty"`
	Input     map[string]interface{} `json:"input,omitempty"`
	Content   interface{}            `json:"content,omitempty"`
	ToolUseID string                 `json:"tool_use_id,omitempty"`
	IsError   bool                   `json:"is_error,omitempty"`
}

type Response struct {
	ID         string         `json:"id"`
	Type       string         `json:"type"`
	Role       string         `json:"role"`
	Content    []ContentBlock `json:"content"`
	Model      string         `json:"model"`
	StopReason string         `json:"stop_reason"`
	Usage      interface{}    `json:"usage,omitempty"`
}

var listFilesTool = Tool{
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

var readFileTool = Tool{
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

var patchFileTool = Tool{
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

var runBashTool = Tool{
	Name:        "run_bash",
	Description: "Execute arbitrary bash commands and return the output. Use this for running shell commands, scripts, or any command-line operations.",
	InputSchema: map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"command": map[string]interface{}{
				"type":        "string",
				"description": "The bash command to execute. Can be any valid bash command or script.",
			},
		},
		"required": []string{"command"},
	},
}

var writeFileTool = Tool{
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

var grepTool = Tool{
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

var globTool = Tool{
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

func executeListFiles(path string) (string, error) {
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

func executeReadFile(path string) (string, error) {
	if path == "" {
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

func executePatchFile(path, oldText, newText string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("file path is required. Example: patch_file(\"main.go\", \"old text\", \"new text\")")
	}
	if oldText == "" {
		return "", fmt.Errorf("old_text is required and cannot be empty. This is the text you want to replace")
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

func executeRunBash(command string) (string, error) {
	if command == "" {
		return "", fmt.Errorf("command is required. Example: run_bash(\"ls -la\")")
	}
	
	cmd := exec.Command("bash", "-c", command)
	output, err := cmd.CombinedOutput()
	
	if err != nil {
		exitErr, ok := err.(*exec.ExitError)
		if ok {
			exitCode := exitErr.ExitCode()
			suggestions := []string{
				fmt.Sprintf("Command failed with exit code %d: %s", exitCode, command),
				"",
				"Output:",
				string(output),
			}
			
			// Add context-specific suggestions
			if exitCode == 127 {
				suggestions = append(suggestions, 
					"",
					"Exit code 127 typically means 'command not found'.",
					"Suggestions:",
					"  - Check if the command is installed",
					"  - Verify the command name is spelled correctly",
					"  - Try which <command> to see if it's in PATH",
				)
			} else if exitCode == 126 {
				suggestions = append(suggestions,
					"",
					"Exit code 126 typically means 'permission denied'.",
					"Suggestions:",
					"  - Check file/script permissions",
					"  - Try: chmod +x <script>",
				)
			} else if exitCode == 1 {
				// Common exit code, try to provide context based on command
				if strings.Contains(command, "test") {
					suggestions = append(suggestions,
						"",
						"This may indicate test failures. Check the output above for details.",
					)
				} else if strings.Contains(command, "git") {
					suggestions = append(suggestions,
						"",
						"Git command failed. Check the output above for details.",
						"Common issues: uncommitted changes, merge conflicts, or invalid references.",
					)
				}
			}
			
			return "", fmt.Errorf("%s", strings.Join(suggestions, "\n"))
		}
		return "", fmt.Errorf("failed to execute command '%s': %w", command, err)
	}
	
	return string(output), nil
}

func executeWriteFile(path, content string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("file path is required. Example: write_file(\"notes.txt\", \"content\")")
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

func executeGlob(pattern, path string) (string, error) {
	if pattern == "" {
		return "", fmt.Errorf("pattern is required. Example: glob(\"**/*.go\") or glob(\"*_test.go\", \"src\")")
	}

	// Default to current directory if no path specified
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

func executeGrep(pattern, path, filePattern string) (string, error) {
	if pattern == "" {
		return "", fmt.Errorf("pattern is required. Example: grep(\"func main\") or grep(\"TODO\", \"src\", \"*.go\")")
	}

	// Default to current directory if no path specified
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

func callClaude(apiKey string, messages []Message) (*Response, error) {
	reqBody := Request{
		Model:     modelID,
		MaxTokens: maxTokens,
		System:    systemPrompt,
		Messages:  messages,
		Tools:     []Tool{listFilesTool, readFileTool, patchFileTool, writeFileTool, runBashTool, grepTool, globTool},
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request to Claude API: %w\nCheck your internet connection", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		// Try to parse error response for better messages
		var errorResp struct {
			Error struct {
				Type    string `json:"type"`
				Message string `json:"message"`
			} `json:"error"`
		}
		
		suggestions := []string{
			fmt.Sprintf("API error (status %d)", resp.StatusCode),
		}
		
		if json.Unmarshal(body, &errorResp) == nil && errorResp.Error.Message != "" {
			suggestions = append(suggestions, fmt.Sprintf("Error: %s", errorResp.Error.Message))
		} else {
			suggestions = append(suggestions, fmt.Sprintf("Response: %s", string(body)))
		}
		
		// Add context-specific help
		switch resp.StatusCode {
		case 401:
			suggestions = append(suggestions,
				"",
				"Authentication failed. Check your API key:",
				"  - Verify TS_AGENT_API_KEY in .env file",
				"  - Ensure the key starts with 'sk-ant-'",
				"  - Try generating a new key at https://console.anthropic.com/",
			)
		case 429:
			suggestions = append(suggestions,
				"",
				"Rate limit exceeded. Suggestions:",
				"  - Wait a moment and try again",
				"  - You may have hit your usage limit",
				"  - Check your plan limits at https://console.anthropic.com/",
			)
		case 400:
			suggestions = append(suggestions,
				"",
				"Bad request. This may indicate:",
				"  - Invalid tool parameters",
				"  - Message format issues",
				"  - Try a simpler request to test",
			)
		case 500, 502, 503, 504:
			suggestions = append(suggestions,
				"",
				"Claude API server error. Suggestions:",
				"  - This is temporary, try again in a moment",
				"  - Check https://status.anthropic.com/ for service status",
			)
		}
		
		return nil, fmt.Errorf("%s", strings.Join(suggestions, "\n"))
	}

	var apiResp Response
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w\nResponse body: %s", err, string(body))
	}

	return &apiResp, nil
}

func handleConversation(apiKey string, userInput string, conversationHistory []Message) (string, []Message) {
	conversationHistory = append(conversationHistory, Message{
		Role:    "user",
		Content: userInput,
	})

	for {
		resp, err := callClaude(apiKey, conversationHistory)
		if err != nil {
			return fmt.Sprintf("Error: %v", err), conversationHistory
		}

		var assistantContent []ContentBlock
		var textResponses []string
		var toolUseBlocks []ContentBlock

		for _, block := range resp.Content {
			assistantContent = append(assistantContent, block)

			if block.Type == "text" && block.Text != "" {
				textResponses = append(textResponses, block.Text)
			} else if block.Type == "tool_use" {
				toolUseBlocks = append(toolUseBlocks, block)
			}
		}

		conversationHistory = append(conversationHistory, Message{
			Role:    "assistant",
			Content: assistantContent,
		})

		if len(toolUseBlocks) == 0 {
			return strings.Join(textResponses, "\n"), conversationHistory
		}

		var toolResults []ContentBlock
		for _, toolBlock := range toolUseBlocks {
			var output string
			var err error
			var displayMessage string

			switch toolBlock.Name {
			case "list_files":
				path := ""
				if pathVal, ok := toolBlock.Input["path"]; ok && pathVal != nil {
					path, _ = pathVal.(string)
				}
				if path == "" || path == "." {
					displayMessage = "→ Listing files: . (current directory)"
				} else {
					displayMessage = fmt.Sprintf("→ Listing files: %s", path)
				}
				output, err = executeListFiles(path)

			case "read_file":
				path, ok := toolBlock.Input["path"].(string)
				if !ok || path == "" {
					err = fmt.Errorf("read_file requires a 'path' parameter. Example: {\"path\": \"main.go\"}")
				} else {
					displayMessage = fmt.Sprintf("→ Reading file: %s", path)
					output, err = executeReadFile(path)
				}

			case "patch_file":
				path, pathOk := toolBlock.Input["path"].(string)
				oldText, oldTextOk := toolBlock.Input["old_text"].(string)
				newText, newTextOk := toolBlock.Input["new_text"].(string)

				if !pathOk || path == "" {
					err = fmt.Errorf("patch_file requires a 'path' parameter. Example: {\"path\": \"main.go\", \"old_text\": \"...\", \"new_text\": \"...\"}")
				} else if !oldTextOk {
					err = fmt.Errorf("patch_file requires an 'old_text' parameter with the exact text to replace")
				} else if !newTextOk {
					err = fmt.Errorf("patch_file requires a 'new_text' parameter with the replacement text")
				} else {
					changeSize := len(newText) - len(oldText)
					if changeSize >= 0 {
						displayMessage = fmt.Sprintf("→ Patching file: %s (+%d bytes)", path, changeSize)
					} else {
						displayMessage = fmt.Sprintf("→ Patching file: %s (%d bytes)", path, changeSize)
					}
					output, err = executePatchFile(path, oldText, newText)
				}

			case "run_bash":
				command, ok := toolBlock.Input["command"].(string)
				if !ok || command == "" {
					err = fmt.Errorf("run_bash requires a 'command' parameter. Example: {\"command\": \"ls -la\"}")
				} else {
					// Truncate long commands for display
					displayCmd := command
					if len(displayCmd) > 60 {
						displayCmd = displayCmd[:57] + "..."
					}
					displayMessage = fmt.Sprintf("→ Running bash: %s", displayCmd)
					output, err = executeRunBash(command)
				}

			case "write_file":
				path, pathOk := toolBlock.Input["path"].(string)
				content, contentOk := toolBlock.Input["content"].(string)

				if !pathOk || path == "" {
					err = fmt.Errorf("write_file requires a 'path' parameter. Example: {\"path\": \"notes.txt\", \"content\": \"...\"}")
				} else if !contentOk {
					err = fmt.Errorf("write_file requires a 'content' parameter with the file contents")
				} else {
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
					displayMessage = fmt.Sprintf("→ Writing file: %s (%s)", path, sizeStr)
					output, err = executeWriteFile(path, content)
				}

			case "grep":
				pattern, patternOk := toolBlock.Input["pattern"].(string)
				path := ""
				filePattern := ""
				
				if pathVal, ok := toolBlock.Input["path"]; ok && pathVal != nil {
					path, _ = pathVal.(string)
				}
				if fpVal, ok := toolBlock.Input["file_pattern"]; ok && fpVal != nil {
					filePattern, _ = fpVal.(string)
				}

				if !patternOk || pattern == "" {
					err = fmt.Errorf("grep requires a 'pattern' parameter. Example: {\"pattern\": \"func main\"}")
				} else {
					searchPath := path
					if searchPath == "" || searchPath == "." {
						searchPath = "current directory"
					}
					if filePattern != "" {
						displayMessage = fmt.Sprintf("→ Searching: '%s' in %s (%s)", pattern, searchPath, filePattern)
					} else {
						displayMessage = fmt.Sprintf("→ Searching: '%s' in %s", pattern, searchPath)
					}
					output, err = executeGrep(pattern, path, filePattern)
				}

			case "glob":
				pattern, patternOk := toolBlock.Input["pattern"].(string)
				path := ""
				
				if pathVal, ok := toolBlock.Input["path"]; ok && pathVal != nil {
					path, _ = pathVal.(string)
				}

				if !patternOk || pattern == "" {
					err = fmt.Errorf("glob requires a 'pattern' parameter. Example: {\"pattern\": \"**/*.go\"}")
				} else {
					searchPath := path
					if searchPath == "" || searchPath == "." {
						searchPath = "current directory"
					}
					displayMessage = fmt.Sprintf("→ Finding files: '%s' in %s", pattern, searchPath)
					output, err = executeGlob(pattern, path)
				}

			default:
				err = fmt.Errorf("unknown tool: %s", toolBlock.Name)
			}

			if displayMessage != "" {
				fmt.Printf("%s\n", displayMessage)
			}

			var resultContent string
			var isError bool
			if err != nil {
				resultContent = err.Error()
				isError = true
			} else {
				resultContent = output
				isError = false
			}

			toolResults = append(toolResults, ContentBlock{
				Type:      "tool_result",
				ToolUseID: toolBlock.ID,
				Content:   resultContent,
				IsError:   isError,
			})
		}

		conversationHistory = append(conversationHistory, Message{
			Role:    "user",
			Content: toolResults,
		})
	}
}


func main() {
	envPath := os.Getenv("ENV_PATH")
	if envPath == "" {
		if _, err := os.Stat(".env"); err == nil {
			envPath = ".env"
		} else {
			envPath = "../coding-agent/.env"
		}
	}

	data, err := os.ReadFile(envPath)
	if err != nil {
		fmt.Printf("Error reading .env file from '%s': %v\n\n", envPath, err)
		fmt.Println("To fix this:")
		fmt.Println("  1. Create a .env file in the current directory, OR")
		fmt.Println("  2. Set ENV_PATH environment variable to your .env file location")
		fmt.Println("  3. Example: export ENV_PATH=/path/to/.env")
		fmt.Println("\nThe .env file should contain:")
		fmt.Println("  TS_AGENT_API_KEY=your-anthropic-api-key-here")
		os.Exit(1)
	}

	var apiKey string
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "TS_AGENT_API_KEY=") {
			apiKey = strings.TrimPrefix(line, "TS_AGENT_API_KEY=")
			apiKey = strings.TrimSpace(apiKey)
			break
		}
	}

	if apiKey == "" {
		fmt.Printf("Error: TS_AGENT_API_KEY not found in '%s'\n\n", envPath)
		fmt.Println("Please add this line to your .env file:")
		fmt.Println("  TS_AGENT_API_KEY=your-anthropic-api-key-here")
		fmt.Println("\nGet your API key from: https://console.anthropic.com/")
		os.Exit(1)
	}

	fmt.Println("Claude REPL - Type 'exit' or 'quit' to exit")
	fmt.Println("============================================")

	reader := bufio.NewReader(os.Stdin)
	var conversationHistory []Message

	for {
		fmt.Print("\nYou: ")
		input, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				fmt.Println("\nGoodbye!")
				break
			}
			fmt.Printf("Error reading input: %v\n", err)
			continue
		}

		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}

		if input == "exit" || input == "quit" {
			fmt.Println("Goodbye!")
			break
		}

		response, updatedHistory := handleConversation(apiKey, input, conversationHistory)
		conversationHistory = updatedHistory

		fmt.Printf("\nClaude: %s\n", response)
	}
}
