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

1. github_query: For GitHub-related questions (repos, PRs, issues, user profile, etc.)
2. list_files: For listing files and directories in a given path
3. read_file: For reading the contents of a file
4. edit_file: For editing/writing files

IMPORTANT DECIDER: Before responding, determine if you need to use a tool:

GitHub questions - Use github_query for:
- Questions about repositories, PRs, issues, workflows
- Questions about user profile, organizations
- Any "show me", "list", "what are" questions related to GitHub
- Status checks, recent activity, etc.

File system questions - Use list_files for:
- "What files are in X directory?"
- "List files in the current folder"
- "Show me the contents of this directory"

File reading questions - Use read_file for:
- "Show me the contents of X file"
- "What's in X file?"
- "Read X file"

File editing questions - Use edit_file for:
- "Create a file with X content"
- "Edit X file to say Y"
- "Update X file"
- "Write to X file"

For file edits, read the file first if modifying existing content, then edit with the updated content.

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

var githubTool = Tool{
	Name:        "github_query",
	Description: "Execute GitHub CLI (gh) commands to query GitHub information. This tool runs 'gh' bash commands to get information about repositories, pull requests, issues, user profile, and more. The command should be a valid 'gh' command without the 'gh' prefix.",
	InputSchema: map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"command": map[string]interface{}{
				"type":        "string",
				"description": "The gh command to execute (without 'gh' prefix). Examples: 'repo list', 'pr list', 'issue list', 'api user'",
			},
		},
		"required": []string{"command"},
	},
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

var editFileTool = Tool{
	Name:        "edit_file",
	Description: "Edit a file by writing new content to it. This will overwrite the entire file with the provided content.",
	InputSchema: map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "The file path to edit. Can be absolute or relative to the current directory.",
			},
			"content": map[string]interface{}{
				"type":        "string",
				"description": "The new content to write to the file. This will replace the entire file contents.",
			},
		},
		"required": []string{"path", "content"},
	},
}

func executeGitHubCommand(command string) (string, error) {
	cmd := exec.Command("gh", strings.Fields(command)...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("command failed: %s\nOutput: %s", err, string(output))
	}
	return string(output), nil
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

func executeEditFile(path, content string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("file path is required")
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}
	return fmt.Sprintf("Successfully wrote %d bytes to %s", len(content), path), nil
}

func callClaude(apiKey string, messages []Message) (*Response, error) {
	reqBody := Request{
		Model:     modelID,
		MaxTokens: maxTokens,
		System:    systemPrompt,
		Messages:  messages,
		Tools:     []Tool{githubTool, listFilesTool, readFileTool, editFileTool},
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
			case "github_query":
				command := toolBlock.Input["command"].(string)
				displayMessage = "→ Running GitHub query..."
				output, err = executeGitHubCommand(command)

			case "list_files":
				path := ""
				if pathVal, ok := toolBlock.Input["path"]; ok {
					path = pathVal.(string)
				}
				displayMessage = "→ Listing files..."
				output, err = executeListFiles(path)

			case "read_file":
				path := toolBlock.Input["path"].(string)
				displayMessage = "→ Reading file..."
				output, err = executeReadFile(path)

			case "edit_file":
				path := toolBlock.Input["path"].(string)
				content := toolBlock.Input["content"].(string)
				displayMessage = "→ Editing file..."
				output, err = executeEditFile(path, content)

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
