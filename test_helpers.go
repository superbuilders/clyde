package main

import (
	"claude-repl/api"
	"claude-repl/config"
	"claude-repl/prompts"
	"claude-repl/tools"
	"os"
	"strings"
)

// Test helper functions that wrap the new architecture

var systemPrompt = prompts.SystemPrompt

// Message type alias for tests
type Message = api.Message
type ContentBlock = api.ContentBlock
type Response = api.Response

// Test helpers that call the actual tool implementations
func executeListFiles(path string) (string, error) {
	reg, _ := tools.GetTool("list_files")
	input := map[string]interface{}{"path": path}
	return reg.Execute(input, nil, nil)
}

func executeReadFile(path string) (string, error) {
	reg, _ := tools.GetTool("read_file")
	input := map[string]interface{}{"path": path}
	return reg.Execute(input, nil, nil)
}

func executePatchFile(path, oldText, newText string) (string, error) {
	reg, _ := tools.GetTool("patch_file")
	input := map[string]interface{}{
		"path":     path,
		"old_text": oldText,
		"new_text": newText,
	}
	return reg.Execute(input, nil, nil)
}

func executeRunBash(command string) (string, error) {
	reg, _ := tools.GetTool("run_bash")
	input := map[string]interface{}{"command": command}
	return reg.Execute(input, nil, nil)
}

func executeWriteFile(path, content string) (string, error) {
	reg, _ := tools.GetTool("write_file")
	input := map[string]interface{}{
		"path":    path,
		"content": content,
	}
	return reg.Execute(input, nil, nil)
}

func executeGrep(pattern, path, filePattern string) (string, error) {
	reg, _ := tools.GetTool("grep")
	input := map[string]interface{}{
		"pattern":      pattern,
		"path":         path,
		"file_pattern": filePattern,
	}
	return reg.Execute(input, nil, nil)
}

func executeGlob(pattern, path string) (string, error) {
	reg, _ := tools.GetTool("glob")
	input := map[string]interface{}{
		"pattern": pattern,
		"path":    path,
	}
	return reg.Execute(input, nil, nil)
}

func executeBrowse(urlStr, prompt string, maxLength int, apiKey string, conversationHistory []Message) (string, error) {
	reg, _ := tools.GetTool("browse")
	input := map[string]interface{}{
		"url":        urlStr,
		"prompt":     prompt,
		"max_length": maxLength,
	}
	
	// Create API client for AI processing if needed
	cfg := &config.Config{
		APIKey:    apiKey,
		APIURL:    "https://api.anthropic.com/v1/messages",
		ModelID:   "claude-sonnet-4-5-20250929",
		MaxTokens: 4096,
	}
	apiClient := api.NewClient(cfg.APIKey, cfg.APIURL, cfg.ModelID, cfg.MaxTokens)
	
	return reg.Execute(input, apiClient, conversationHistory)
}

func executeWebSearch(query string, numResults int) (string, error) {
	reg, _ := tools.GetTool("web_search")
	input := map[string]interface{}{
		"query":       query,
		"num_results": float64(numResults),
	}
	return reg.Execute(input, nil, nil)
}

func executeMultiPatch(patches []interface{}) (string, error) {
	reg, _ := tools.GetTool("multi_patch")
	input := map[string]interface{}{
		"patches": patches,
	}
	return reg.Execute(input, nil, nil)
}

func callClaude(apiKey string, messages []Message) (*Response, error) {
	cfg := &config.Config{
		APIKey:    apiKey,
		APIURL:    "https://api.anthropic.com/v1/messages",
		ModelID:   "claude-sonnet-4-5-20250929",
		MaxTokens: 4096,
	}
	client := api.NewClient(cfg.APIKey, cfg.APIURL, cfg.ModelID, cfg.MaxTokens)
	allTools := tools.GetAllTools()
	return client.Call(systemPrompt, messages, allTools)
}

func handleConversation(apiKey string, userInput string, conversationHistory []Message) (string, []Message) {
	// Get Brave API key from environment (may already be set)
	braveAPIKey := os.Getenv("BRAVE_SEARCH_API_KEY")
	
	// If not set, try to load from .env file
	if braveAPIKey == "" {
		envPath := os.Getenv("ENV_PATH")
		if envPath == "" {
			// Try current directory first
			if _, err := os.Stat(".env"); err == nil {
				envPath = ".env"
			} else {
				// Try parent directory
				envPath = "../coding-agent/.env"
			}
		}

		data, err := os.ReadFile(envPath)
		if err == nil {
			// Parse API keys from .env
			lines := strings.Split(string(data), "\n")
			for _, line := range lines {
				if strings.HasPrefix(line, "BRAVE_SEARCH_API_KEY=") {
					braveAPIKey = strings.TrimPrefix(line, "BRAVE_SEARCH_API_KEY=")
					braveAPIKey = strings.TrimSpace(braveAPIKey)
					break
				}
			}

			// Set environment variable for tools that need it
			if braveAPIKey != "" {
				os.Setenv("BRAVE_SEARCH_API_KEY", braveAPIKey)
			}
		}
		// Ignore error if .env not found - not all tests need it
	}

	cfg := &config.Config{
		APIKey:            apiKey,
		BraveSearchAPIKey: braveAPIKey,
		APIURL:            "https://api.anthropic.com/v1/messages",
		ModelID:           "claude-sonnet-4-5-20250929",
		MaxTokens:         4096,
	}

	// Create API client and agent
	apiClient := api.NewClient(cfg.APIKey, cfg.APIURL, cfg.ModelID, cfg.MaxTokens)
	agentInstance := &testAgent{
		apiClient: apiClient,
		history:   conversationHistory,
	}

	response, err := agentInstance.HandleMessage(userInput)
	if err != nil {
		return response, agentInstance.history
	}
	return response, agentInstance.history
}

// testAgent is a wrapper around the actual agent for testing
type testAgent struct {
	apiClient *api.Client
	history   []Message
}

func (a *testAgent) HandleMessage(userInput string) (string, error) {
	// Add user message to history
	a.history = append(a.history, Message{
		Role:    "user",
		Content: userInput,
	})

	// Get all registered tools
	allTools := tools.GetAllTools()

	// Conversation loop - continue until we get a text response
	for {
		resp, err := a.apiClient.Call(systemPrompt, a.history, allTools)
		if err != nil {
			return err.Error(), err
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

		// Add assistant response to history
		a.history = append(a.history, Message{
			Role:    "assistant",
			Content: assistantContent,
		})

		// If no tool use, return text responses
		if len(toolUseBlocks) == 0 {
			return strings.Join(textResponses, "\n"), nil
		}

		// Execute tools
		var toolResults []ContentBlock
		for _, toolBlock := range toolUseBlocks {
			reg, err := tools.GetTool(toolBlock.Name)
			if err != nil {
				// Unknown tool
				toolResults = append(toolResults, ContentBlock{
					Type:      "tool_result",
					ToolUseID: toolBlock.ID,
					Content:   err.Error(),
					IsError:   true,
				})
				continue
			}

			// Execute the tool
			output, err := reg.Execute(toolBlock.Input, a.apiClient, a.history)

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

		// Add tool results to history
		a.history = append(a.history, Message{
			Role:    "user",
			Content: toolResults,
		})
	}
}
