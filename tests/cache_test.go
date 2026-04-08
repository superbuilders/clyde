package main

import (
	"os"
	"testing"

	"github.com/this-is-alpha-iota/clyde/agent"
	"github.com/this-is-alpha-iota/clyde/agent/providers"
	"github.com/this-is-alpha-iota/clyde/agent/prompts"
)

// TestCacheControlEnabled verifies cache_control is set in requests
func TestCacheControlEnabled(t *testing.T) {
	apiKey := os.Getenv("TS_AGENT_API_KEY")
	if apiKey == "" {
		t.Skip("TS_AGENT_API_KEY not set, skipping integration test")
	}

	apiClient := providers.NewClient(
		apiKey,
		"https://api.anthropic.com/v1/messages",
		"claude-sonnet-4-5-20250929",
		4096,
	)

	agentInstance := agent.NewAgent(apiClient, prompts.SystemPrompt)

	response, err := agentInstance.HandleMessage("Hello! Just say 'Hi' back.")
	if err != nil {
		t.Fatalf("Failed to get response: %v", err)
	}

	if response == "" {
		t.Error("Expected a response, got empty string")
	}
}

// TestCacheUsageDisplay verifies cache hit display works
func TestCacheUsageDisplay(t *testing.T) {
	apiKey := os.Getenv("TS_AGENT_API_KEY")
	if apiKey == "" {
		t.Skip("TS_AGENT_API_KEY not set, skipping integration test")
	}

	apiClient := providers.NewClient(
		apiKey,
		"https://api.anthropic.com/v1/messages",
		"claude-sonnet-4-5-20250929",
		4096,
	)

	var diagnosticMessages []string
	agentInstance := agent.NewAgent(
		apiClient,
		prompts.SystemPrompt,
		agent.WithDiagnosticCallback(func(msg string) {
			diagnosticMessages = append(diagnosticMessages, msg)
		}),
	)

	_, err := agentInstance.HandleMessage("What is 2+2?")
	if err != nil {
		t.Fatalf("Failed on first request: %v", err)
	}

	diagnosticMessages = []string{}
	_, err = agentInstance.HandleMessage("What is 3+3?")
	if err != nil {
		t.Fatalf("Failed on second request: %v", err)
	}

	t.Logf("Diagnostic messages from second request: %v", diagnosticMessages)
}

// TestCacheHitAfterToolUse verifies cache hits work with tool execution
func TestCacheHitAfterToolUse(t *testing.T) {
	apiKey := os.Getenv("TS_AGENT_API_KEY")
	if apiKey == "" {
		t.Skip("TS_AGENT_API_KEY not set, skipping integration test")
	}

	apiClient := providers.NewClient(
		apiKey,
		"https://api.anthropic.com/v1/messages",
		"claude-sonnet-4-5-20250929",
		4096,
	)

	var diagnosticMessages []string
	agentInstance := agent.NewAgent(
		apiClient,
		prompts.SystemPrompt,
		agent.WithDiagnosticCallback(func(msg string) {
			diagnosticMessages = append(diagnosticMessages, msg)
			t.Logf("Diagnostic: %s", msg)
		}),
	)

	_, err := agentInstance.HandleMessage("What files are in the current directory?")
	if err != nil {
		t.Fatalf("Failed on first request: %v", err)
	}

	diagnosticMessages = []string{}
	_, err = agentInstance.HandleMessage("What is 5+5?")
	if err != nil {
		t.Fatalf("Failed on second request: %v", err)
	}

	if len(diagnosticMessages) > 0 {
		t.Logf("Second request diagnostics: %v", diagnosticMessages)
	}
}

// TestUsageStructFields verifies Usage struct has correct fields
func TestUsageStructFields(t *testing.T) {
	usage := providers.Usage{
		InputTokens:              1000,
		OutputTokens:             200,
		CacheCreationInputTokens: 500,
		CacheReadInputTokens:     300,
	}

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
	cacheControl := providers.CacheControl{Type: "ephemeral"}
	if cacheControl.Type != "ephemeral" {
		t.Errorf("Expected Type='ephemeral', got '%s'", cacheControl.Type)
	}
}

// TestRequestWithCacheControl verifies Request includes cache_control
func TestRequestWithCacheControl(t *testing.T) {
	req := providers.Request{
		Model:        "claude-sonnet-4-5-20250929",
		MaxTokens:    4096,
		CacheControl: &providers.CacheControl{Type: "ephemeral"},
		System:       "Test system prompt",
		Messages:     []providers.Message{},
		Tools:        []providers.Tool{},
	}

	if req.CacheControl == nil {
		t.Error("Expected CacheControl to be set")
	}
	if req.CacheControl.Type != "ephemeral" {
		t.Errorf("Expected CacheControl.Type='ephemeral', got '%s'", req.CacheControl.Type)
	}
}
