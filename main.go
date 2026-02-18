package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/this-is-alpha-iota/clyde/agent"
	"github.com/this-is-alpha-iota/clyde/api"
	"github.com/this-is-alpha-iota/clyde/config"
	"github.com/this-is-alpha-iota/clyde/prompts"
	_ "github.com/this-is-alpha-iota/clyde/tools" // Import tools to register them
)

func main() {
	// Determine config file location (CLI layer responsibility)
	configPath := getConfigPath()

	// Load configuration from the determined path
	cfg, err := config.LoadFromFile(configPath)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	// Create API client
	apiClient := api.NewClient(cfg.APIKey, cfg.APIURL, cfg.ModelID, cfg.MaxTokens)

	// Create agent with system prompt and progress callback
	agentInstance := agent.NewAgent(
		apiClient,
		prompts.SystemPrompt,
		agent.WithProgressCallback(func(msg string) {
			fmt.Println(msg) // REPL prints progress to stdout
		}),
	)

	// Start REPL
	fmt.Println("Clyde - AI Coding Agent - Type 'exit' or 'quit' to exit")
	fmt.Println("==========================================================")

	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Print("\nYou: ")
		input, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				fmt.Println("\nGoodbye!")
				break
			}
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

		response, _ := agentInstance.HandleMessage(input)
		fmt.Printf("\nClaude: %s\n", response)
	}
}

// getConfigPath determines the config file path for the production app
// Always uses ~/.clyde/config
func getConfigPath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Could not determine home directory: %v\n", err)
		os.Exit(1)
	}

	configPath := filepath.Join(homeDir, ".clyde", "config")

	// Check if config file exists, if not provide helpful error and exit
	if _, err := os.Stat(configPath); err != nil {
		configDir := filepath.Join(homeDir, ".clyde")
		fmt.Printf("Configuration file not found: %s\n\n", configPath)
		fmt.Println("To get started, create a config file:")
		fmt.Println()
		fmt.Printf("  mkdir -p %s\n", configDir)
		fmt.Printf("  cat > %s << 'EOF'\n", configPath)
		fmt.Println("TS_AGENT_API_KEY=your-anthropic-api-key")
		fmt.Println("BRAVE_SEARCH_API_KEY=your-brave-api-key  # Optional")
		fmt.Println("EOF")
		fmt.Println()
		fmt.Println("Get your Anthropic API key at: https://console.anthropic.com/")
		fmt.Println("Get your Brave Search API key at: https://brave.com/search/api/ (optional)")
		os.Exit(1)
	}

	return configPath
}
