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

// Load loads configuration from environment variables
func Load() (*Config, error) {
	// Determine which .env file to load
	envPath := os.Getenv("ENV_PATH")
	if envPath == "" {
		if _, err := os.Stat(".env"); err == nil {
			envPath = ".env"
		} else {
			envPath = "../coding-agent/.env"
		}
	}

	// Load all environment variables from .env file
	err := godotenv.Load(envPath)
	if err != nil {
		return nil, fmt.Errorf("error loading .env file from '%s': %w\n\nTo fix this:\n  1. Create a .env file in the current directory, OR\n  2. Set ENV_PATH environment variable to your .env file location\n  3. Example: export ENV_PATH=/path/to/.env\n\nThe .env file should contain:\n  TS_AGENT_API_KEY=your-anthropic-api-key-here\n  BRAVE_SEARCH_API_KEY=your-brave-api-key-here  # Optional: for web_search", envPath, err)
	}

	// Verify required API key is present
	apiKey := os.Getenv("TS_AGENT_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("TS_AGENT_API_KEY not found in '%s'\n\nPlease add this line to your .env file:\n  TS_AGENT_API_KEY=your-anthropic-api-key-here\n\nGet your API key from: https://console.anthropic.com/", envPath)
	}

	return &Config{
		APIKey:            apiKey,
		BraveSearchAPIKey: os.Getenv("BRAVE_SEARCH_API_KEY"),
		APIURL:            "https://api.anthropic.com/v1/messages",
		ModelID:           "claude-sonnet-4-5-20250929",
		MaxTokens:         4096,
	}, nil
}
