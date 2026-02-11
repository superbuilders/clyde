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

func executeListFiles(path string) (string, error) {
	if path == "" {
		path = "."
	}
	cmd := exec.Command("ls", "-la", path)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to list files: %s\nOutput: %s", err, string(output))
	}
	return string(output), nil
}

func executeReadFile(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("file path is required")
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}
	return string(content), nil
}

func executePatchFile(path, oldText, newText string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("file path is required")
	}
	if oldText == "" {
		return "", fmt.Errorf("old_text is required (cannot be empty)")
	}

	// Read the current file content
	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	fileContent := string(content)

	// Check if old_text exists in the file
	if !strings.Contains(fileContent, oldText) {
		return "", fmt.Errorf("old_text not found in file. Make sure it matches exactly including whitespace and newlines")
	}

	// Count occurrences to ensure it's unique
	occurrences := strings.Count(fileContent, oldText)
	if occurrences > 1 {
		return "", fmt.Errorf("old_text appears %d times in the file. It must be unique. Add more context to make it unique", occurrences)
	}

	// Replace the text
	newContent := strings.Replace(fileContent, oldText, newText, 1)

	// Write the modified content back
	if err := os.WriteFile(path, []byte(newContent), 0644); err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	changeSize := len(newText) - len(oldText)
	return fmt.Sprintf("Successfully patched %s: replaced %d bytes with %d bytes (change: %+d bytes)",
		path, len(oldText), len(newText), changeSize), nil
}

func executeRunBash(command string) (string, error) {
	if command == "" {
		return "", fmt.Errorf("command is required")
	}
	cmd := exec.Command("bash", "-c", command)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("command failed: %s", err)
	}
	return string(output), nil
}

func executeWriteFile(path, content string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("file path is required")
	}

	// Check if file exists to provide appropriate message
	fileExists := false
	if _, err := os.Stat(path); err == nil {
		fileExists = true
	}

	// Write the content
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	if fileExists {
		return fmt.Sprintf("Successfully replaced contents of %s (%d bytes written)", path, len(content)), nil
	}
	return fmt.Sprintf("Successfully created %s (%d bytes written)", path, len(content)), nil
}

func callClaude(apiKey string, messages []Message) (*Response, error) {
	reqBody := Request{
		Model:     modelID,
		MaxTokens: maxTokens,
		System:    systemPrompt,
		Messages:  messages,
		Tools:     []Tool{listFilesTool, readFileTool, patchFileTool, writeFileTool, runBashTool},
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
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	var apiResp Response
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
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
					err = fmt.Errorf("read_file requires non-empty 'path' parameter")
				} else {
					displayMessage = fmt.Sprintf("→ Reading file: %s", path)
					output, err = executeReadFile(path)
				}

			case "patch_file":
				path, pathOk := toolBlock.Input["path"].(string)
				oldText, oldTextOk := toolBlock.Input["old_text"].(string)
				newText, newTextOk := toolBlock.Input["new_text"].(string)

				if !pathOk || path == "" {
					err = fmt.Errorf("patch_file requires non-empty 'path' parameter")
				} else if !oldTextOk {
					err = fmt.Errorf("patch_file requires 'old_text' parameter")
				} else if !newTextOk {
					err = fmt.Errorf("patch_file requires 'new_text' parameter")
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
					err = fmt.Errorf("run_bash requires non-empty 'command' parameter")
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
					err = fmt.Errorf("write_file requires non-empty 'path' parameter")
				} else if !contentOk {
					err = fmt.Errorf("write_file requires 'content' parameter")
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
		fmt.Printf("Error reading .env file: %v\n", err)
		fmt.Println("Please set ENV_PATH environment variable or ensure ../coding-agent/.env exists")
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
		fmt.Println("Error: TS_AGENT_API_KEY not found in .env file")
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
