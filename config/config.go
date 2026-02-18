package config

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

// Config holds the application configuration
type Config struct {
	APIKey            string
	BraveSearchAPIKey string
	APIURL            string
	ModelID           string
	MaxTokens         int
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

	return &Config{
		APIKey:            apiKey,
		BraveSearchAPIKey: os.Getenv("BRAVE_SEARCH_API_KEY"),
		APIURL:            "https://api.anthropic.com/v1/messages",
		ModelID:           "claude-sonnet-4-5-20250929",
		MaxTokens:         4096,
	}, nil
}
