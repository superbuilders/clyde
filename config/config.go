package config

import (
	"fmt"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

// Config holds the application configuration
type Config struct {
	APIKey               string
	BraveSearchAPIKey    string
	APIURL               string
	ModelID              string
	MaxTokens            int
	ContextWindowSize    int    // Maximum context window for the model in tokens
	ThinkingBudgetTokens int    // Budget for extended thinking (0 = use default 8192)
	MCPPlaywright        bool   // Enable Playwright MCP browser automation
	MCPPlaywrightArgs    string // Extra args for npx @playwright/mcp (e.g. "--headless")
}

// LoadFromFile loads configuration from a specific file path
func LoadFromFile(path string) (*Config, error) {
	// Check if file exists
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("config file '%s' not found: %w", path, err)
	}

	// Load environment variables from the file
	err := godotenv.Load(path)
	if err != nil {
		return nil, fmt.Errorf("error loading config file from '%s': %w", path, err)
	}

	// Verify required API key is present
	apiKey := os.Getenv("TS_AGENT_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("TS_AGENT_API_KEY not found in '%s'\n\n"+
			"Please add this line to your config file:\n"+
			"  TS_AGENT_API_KEY=your-anthropic-api-key-here\n\n"+
			"Get your API key from: https://console.anthropic.com/", path)
	}

	// Parse optional thinking budget tokens
	thinkingBudget := 0
	if budgetStr := os.Getenv("THINKING_BUDGET_TOKENS"); budgetStr != "" {
		budget, err := strconv.Atoi(budgetStr)
		if err != nil {
			return nil, fmt.Errorf("THINKING_BUDGET_TOKENS must be a number, got %q: %w", budgetStr, err)
		}
		if budget < 1024 {
			return nil, fmt.Errorf("THINKING_BUDGET_TOKENS must be >= 1024, got %d", budget)
		}
		thinkingBudget = budget
	}

	return &Config{
		APIKey:               apiKey,
		BraveSearchAPIKey:    os.Getenv("BRAVE_SEARCH_API_KEY"),
		APIURL:               "https://api.anthropic.com/v1/messages",
		ModelID:              "claude-opus-4-6",
		MaxTokens:            64000,
		ContextWindowSize:    200000, // Claude Opus 4.6 context window
		ThinkingBudgetTokens: thinkingBudget,
		MCPPlaywright:        os.Getenv("MCP_PLAYWRIGHT") == "true",
		MCPPlaywrightArgs:    os.Getenv("MCP_PLAYWRIGHT_ARGS"),
	}, nil
}
