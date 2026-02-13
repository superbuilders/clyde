package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"claude-repl/agent"
	"claude-repl/api"
	"claude-repl/config"
	"claude-repl/prompts"
	_ "claude-repl/tools" // Import tools to register them
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	// Create API client
	apiClient := api.NewClient(cfg.APIKey, cfg.APIURL, cfg.ModelID, cfg.MaxTokens)

	// Create agent with system prompt
	agentInstance := agent.NewAgent(apiClient, prompts.SystemPrompt)

	// Start REPL
	fmt.Println("Claude REPL - Type 'exit' or 'quit' to exit")
	fmt.Println("============================================")

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
