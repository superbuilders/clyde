package config

import (
	"fmt"
	"os"
	"path/filepath"

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

// findConfigFile searches for configuration files in multiple locations
// Priority order:
//  1. ENV_PATH environment variable (if set)
//  2. .env in current directory
//  3. ~/.claude-repl/config
//  4. ~/.claude-repl (legacy fallback)
func findConfigFile() (string, error) {
	// 1. Check ENV_PATH environment variable (highest priority override)
	if envPath := os.Getenv("ENV_PATH"); envPath != "" {
		if _, err := os.Stat(envPath); err == nil {
			return envPath, nil
		}
		return "", fmt.Errorf("ENV_PATH is set to '%s' but file does not exist", envPath)
	}

	// 2. Check .env in current directory (for local development/testing)
	if _, err := os.Stat(".env"); err == nil {
		return ".env", nil
	}

	// 3. Check ~/.claude-repl/config (primary global config location)
	homeDir, err := os.UserHomeDir()
	if err == nil {
		configPath := filepath.Join(homeDir, ".claude-repl", "config")
		if _, err := os.Stat(configPath); err == nil {
			return configPath, nil
		}

		// 4. Check ~/.claude-repl (legacy fallback - direct file without subdirectory)
		legacyPath := filepath.Join(homeDir, ".claude-repl")
		if info, err := os.Stat(legacyPath); err == nil && !info.IsDir() {
			return legacyPath, nil
		}
	}

	// No config file found
	return "", nil
}

// Load loads configuration from environment variables
func Load() (*Config, error) {
	// Find config file
	configPath, err := findConfigFile()
	if err != nil {
		return nil, err
	}

	// If no config file found, provide helpful error message
	if configPath == "" {
		homeDir, _ := os.UserHomeDir()
		configDir := filepath.Join(homeDir, ".claude-repl")
		configFile := filepath.Join(configDir, "config")

		return nil, fmt.Errorf("No configuration file found\n\n" +
			"To get started, create a config file:\n\n" +
			"  mkdir -p %s\n" +
			"  cat > %s << 'EOF'\n" +
			"TS_AGENT_API_KEY=your-anthropic-api-key\n" +
			"BRAVE_SEARCH_API_KEY=your-brave-api-key  # Optional\n" +
			"EOF\n\n" +
			"Get your Anthropic API key at: https://console.anthropic.com/\n" +
			"Get your Brave Search API key at: https://brave.com/search/api/ (optional)\n\n" +
			"Alternatively, create a .env file in your project directory for project-specific config.",
			configDir, configFile)
	}

	// Load all environment variables from config file
	err = godotenv.Load(configPath)
	if err != nil {
		return nil, fmt.Errorf("error loading config file from '%s': %w", configPath, err)
	}

	// Verify required API key is present
	apiKey := os.Getenv("TS_AGENT_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("TS_AGENT_API_KEY not found in '%s'\n\n"+
			"Please add this line to your config file:\n"+
			"  TS_AGENT_API_KEY=your-anthropic-api-key-here\n\n"+
			"Get your API key from: https://console.anthropic.com/", configPath)
	}

	return &Config{
		APIKey:            apiKey,
		BraveSearchAPIKey: os.Getenv("BRAVE_SEARCH_API_KEY"),
		APIURL:            "https://api.anthropic.com/v1/messages",
		ModelID:           "claude-sonnet-4-5-20250929",
		MaxTokens:         4096,
	}, nil
}
