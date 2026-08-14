package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/superbuilders/clyde/agent/providers"
	"github.com/superbuilders/clyde/agent/config"
	"github.com/superbuilders/clyde/agent/prompts"
	"github.com/superbuilders/clyde/agent/tools"
)

// Test helper functions that wrap the new architecture

// testAPIURL returns the endpoint integration tests should call.
//
// It honors TS_AGENT_API_URL so that pointing the harness at a gateway
// (TrueFoundary, Cloudflare AI Gateway, LiteLLM, ...) does not require
// editing hardcoded literals scattered across the suite. Before this
// existed, migrating to a gateway left every live test 401ing against
// api.anthropic.com with a gateway key.
func testAPIURL() string {
	if u := os.Getenv("TS_AGENT_API_URL"); u != "" {
		return u
	}
	return testAPIURL()
}

// testModelID returns the model integration tests should exercise.
// Honors TS_AGENT_MODEL_ID so a model swap cannot silently break, or
// silently keep testing, the wrong model.
func testModelID() string {
	if m := os.Getenv("TS_AGENT_MODEL_ID"); m != "" {
		return m
	}
	return "claude-opus-4-6"
}

// agentEnvOverrides lists every environment variable that can override a
// config default. Tests asserting default values must clear all of them,
// otherwise an ambient gateway/model setting in the developer's shell leaks
// in and the test fails for reasons unrelated to the code under test.
var agentEnvOverrides = []string{
	"TS_AGENT_API_KEY",
	"TS_AGENT_API_URL",
	"TS_AGENT_MODEL_ID",
	"TS_AGENT_THINKING_EFFORT",
	"TS_AGENT_MAX_TOKENS",
	"TS_AGENT_CONTEXT_WINDOW",
	"THINKING_BUDGET_TOKENS",
	"RESERVE_TOKENS",
	"BRAVE_SEARCH_API_KEY",
	"MCP_PLAYWRIGHT",
	"MCP_PLAYWRIGHT_ARGS",
}

// clearAgentEnv unsets every config-override variable for the duration of the
// test, restoring the previous values on cleanup.
func clearAgentEnv(t *testing.T) {
	t.Helper()
	saved := make(map[string]string, len(agentEnvOverrides))
	for _, k := range agentEnvOverrides {
		if v, ok := os.LookupEnv(k); ok {
			saved[k] = v
		}
		os.Unsetenv(k)
	}
	t.Cleanup(func() {
		for _, k := range agentEnvOverrides {
			if v, ok := saved[k]; ok {
				os.Setenv(k, v)
			} else {
				os.Unsetenv(k)
			}
		}
	})
}

var systemPrompt = prompts.SystemPrompt

// Message type alias for tests
type Message = providers.Message
type ContentBlock = providers.ContentBlock
type Response = providers.Response

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
		APIURL:    testAPIURL(),
		ModelID:   testModelID(),
		MaxTokens: 4096,
	}
	apiClient := providers.NewClient(cfg.APIKey, cfg.APIURL, cfg.ModelID, cfg.MaxTokens)
	
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
		APIURL:    testAPIURL(),
		ModelID:   testModelID(),
		MaxTokens: 4096,
	}
	client := providers.NewClient(cfg.APIKey, cfg.APIURL, cfg.ModelID, cfg.MaxTokens)
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
				envPath = "../.env"
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
		APIURL:            testAPIURL(),
		ModelID:           testModelID(),
		MaxTokens:         4096,
	}

	// Create API client and agent
	apiClient := providers.NewClient(cfg.APIKey, cfg.APIURL, cfg.ModelID, cfg.MaxTokens)
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
	apiClient *providers.Client
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

// buildTestCommand creates an exec.Command for the test binary with a proper
// temporary HOME containing a valid Clyde config. This is a helper for tests
// that need to run the compiled binary with custom flags.
func buildTestCommand(t *testing.T, binaryPath string, args ...string) *exec.Cmd {
	t.Helper()

	// Create test config in a temp home directory
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, ".clyde")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("Failed to create config dir: %v", err)
	}

	configPath := filepath.Join(configDir, "config")

	// Read API key from environment
	apiKey := os.Getenv("TS_AGENT_API_KEY")
	if apiKey == "" {
		// Use a dummy key — the test may expect an error anyway
		apiKey = "sk-ant-test-dummy-key"
	}

	configContent := "TS_AGENT_API_KEY=" + apiKey + "\n"
	braveKey := os.Getenv("BRAVE_SEARCH_API_KEY")
	if braveKey != "" {
		configContent += "BRAVE_SEARCH_API_KEY=" + braveKey + "\n"
	}

	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to create test config: %v", err)
	}

	cmd := exec.Command(binaryPath, args...)
	cmd.Env = append(os.Environ(), "HOME="+tmpDir)
	return cmd
}
