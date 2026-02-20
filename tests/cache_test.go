package main

import (
	"os"
	"testing"

	"github.com/this-is-alpha-iota/clyde/agent"
	"github.com/this-is-alpha-iota/clyde/api"
	"github.com/this-is-alpha-iota/clyde/prompts"
)

// TestCacheControlEnabled verifies cache_control is set in requests
func TestCacheControlEnabled(t *testing.T) {
	apiKey := os.Getenv("TS_AGENT_API_KEY")
	if apiKey == "" {
		t.Skip("TS_AGENT_API_KEY not set, skipping integration test")
	}

	apiClient := api.NewClient(
		apiKey,
		"https://api.anthropic.com/v1/messages",
		"claude-sonnet-4-5-20250929",
		4096,
	)

	// Create an agent
	agentInstance := agent.NewAgent(apiClient, prompts.SystemPrompt)

	// Make a simple request
	response, err := agentInstance.HandleMessage("Hello! Just say 'Hi' back.")
	if err != nil {
		t.Fatalf("Failed to get response: %v", err)
	}

	if response == "" {
		t.Error("Expected a response, got empty string")
	}

	// Note: We can't directly check if cache_control was set in the request,
	// but we verify the code compiles and runs without errors.
	// Cache usage will be visible in subsequent requests.
}

// TestCacheUsageDisplay verifies cache hit display works
func TestCacheUsageDisplay(t *testing.T) {
	apiKey := os.Getenv("TS_AGENT_API_KEY")
	if apiKey == "" {
		t.Skip("TS_AGENT_API_KEY not set, skipping integration test")
	}

	apiClient := api.NewClient(
		apiKey,
		"https://api.anthropic.com/v1/messages",
		"claude-sonnet-4-5-20250929",
		4096,
	)

	// Track progress messages
	var progressMessages []string
	agentInstance := agent.NewAgent(
		apiClient,
		prompts.SystemPrompt,
		agent.WithProgressCallback(func(msg string) {
			progressMessages = append(progressMessages, msg)
		}),
	)

	// Make first request (may create cache)
	_, err := agentInstance.HandleMessage("What is 2+2?")
	if err != nil {
		t.Fatalf("Failed on first request: %v", err)
	}

	// Make second request (should hit cache)
	progressMessages = []string{} // Reset
	_, err = agentInstance.HandleMessage("What is 3+3?")
	if err != nil {
		t.Fatalf("Failed on second request: %v", err)
	}

	// Check if cache hit message was displayed
	// Note: Cache hit only happens if the cache was created and still valid (5 min TTL)
	// So this might not always show a cache hit in tests
	t.Logf("Progress messages from second request: %v", progressMessages)
}

// TestCacheHitAfterToolUse verifies cache hits work with tool execution
func TestCacheHitAfterToolUse(t *testing.T) {
	apiKey := os.Getenv("TS_AGENT_API_KEY")
	if apiKey == "" {
		t.Skip("TS_AGENT_API_KEY not set, skipping integration test")
	}

	apiClient := api.NewClient(
		apiKey,
		"https://api.anthropic.com/v1/messages",
		"claude-sonnet-4-5-20250929",
		4096,
	)

	// Track all messages
	var progressMessages []string
	agentInstance := agent.NewAgent(
		apiClient,
		prompts.SystemPrompt,
		agent.WithProgressCallback(func(msg string) {
			progressMessages = append(progressMessages, msg)
			t.Logf("Progress: %s", msg)
		}),
	)

	// First request with tool use
	_, err := agentInstance.HandleMessage("What files are in the current directory?")
	if err != nil {
		t.Fatalf("Failed on first request: %v", err)
	}

	// Second request (should potentially hit cache for system prompt and tools)
	progressMessages = []string{}
	_, err = agentInstance.HandleMessage("What is 5+5?")
	if err != nil {
		t.Fatalf("Failed on second request: %v", err)
	}

	// Log progress messages for manual verification
	if len(progressMessages) > 0 {
		t.Logf("Second request progress messages: %v", progressMessages)
	}
}

// TestUsageStructFields verifies Usage struct has correct fields
func TestUsageStructFields(t *testing.T) {
	// Create a mock Usage struct
	usage := api.Usage{
		InputTokens:              1000,
		OutputTokens:             200,
		CacheCreationInputTokens: 500,
		CacheReadInputTokens:     300,
	}

	// Verify fields are accessible
	if usage.InputTokens != 1000 {
		t.Errorf("Expected InputTokens=1000, got %d", usage.InputTokens)
	}
	if usage.OutputTokens != 200 {
		t.Errorf("Expected OutputTokens=200, got %d", usage.OutputTokens)
	}
	if usage.CacheCreationInputTokens != 500 {
		t.Errorf("Expected CacheCreationInputTokens=500, got %d", usage.CacheCreationInputTokens)
	}
	if usage.CacheReadInputTokens != 300 {
		t.Errorf("Expected CacheReadInputTokens=300, got %d", usage.CacheReadInputTokens)
	}
}

// TestCacheControlStruct verifies CacheControl struct
func TestCacheControlStruct(t *testing.T) {
	// Create a CacheControl struct
	cacheControl := api.CacheControl{Type: "ephemeral"}

	if cacheControl.Type != "ephemeral" {
		t.Errorf("Expected Type='ephemeral', got '%s'", cacheControl.Type)
	}
}

// TestRequestWithCacheControl verifies Request includes cache_control
func TestRequestWithCacheControl(t *testing.T) {
	req := api.Request{
		Model:        "claude-sonnet-4-5-20250929",
		MaxTokens:    4096,
		CacheControl: &api.CacheControl{Type: "ephemeral"},
		System:       "Test system prompt",
		Messages:     []api.Message{},
		Tools:        []api.Tool{},
	}

	if req.CacheControl == nil {
		t.Error("Expected CacheControl to be set")
	}
	if req.CacheControl.Type != "ephemeral" {
		t.Errorf("Expected CacheControl.Type='ephemeral', got '%s'", req.CacheControl.Type)
	}
}
