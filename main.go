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
	"github.com/this-is-alpha-iota/clyde/loglevel"
	"github.com/this-is-alpha-iota/clyde/prompts"
	"github.com/this-is-alpha-iota/clyde/style"
	_ "github.com/this-is-alpha-iota/clyde/tools" // Import tools to register them
)

func main() {
	// Parse log level flags first, stripping them from args
	level, args := loglevel.ParseFlags(os.Args[1:])

	// Check if stdin has input (pipe/redirect)
	stat, _ := os.Stdin.Stat()
	hasStdinInput := (stat.Mode() & os.ModeCharDevice) == 0

	// Determine mode: CLI or REPL
	// CLI mode if: args provided OR stdin is piped
	// REPL mode if: no args AND stdin is interactive (terminal)
	if len(args) > 0 || hasStdinInput {
		runCLIMode(args, hasStdinInput, level)
	} else {
		runREPLMode(level)
	}
}

// runCLIMode executes the agent on a single prompt and exits
func runCLIMode(args []string, hasStdinInput bool, level loglevel.Level) {
	// Determine prompt source
	var prompt string
	var err error

	if len(args) > 0 && args[0] == "-f" {
		// Read from file
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Error: -f requires a file path")
			fmt.Fprintln(os.Stderr, "Usage: clyde -f prompt.txt")
			os.Exit(1)
		}
		prompt, err = readPromptFromFile(args[1])
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading prompt file: %v\n", err)
			os.Exit(1)
		}
	} else if hasStdinInput {
		// stdin is piped/redirected
		prompt, err = readPromptFromStdin()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading from stdin: %v\n", err)
			os.Exit(1)
		}
	} else {
		// Treat all args as the prompt string
		prompt = strings.Join(args, " ")
	}

	if strings.TrimSpace(prompt) == "" {
		fmt.Fprintln(os.Stderr, "Error: Empty prompt provided")
		os.Exit(1)
	}

	// Load config
	configPath := getConfigPath()
	cfg, err := config.LoadFromFile(configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	// Create API client
	apiClient := api.NewClient(cfg.APIKey, cfg.APIURL, cfg.ModelID, cfg.MaxTokens)

	// Create agent with progress callback (print to stderr so stdout is clean)
	agentInstance := agent.NewAgent(
		apiClient,
		prompts.SystemPrompt,
		agent.WithLogLevel(level),
		agent.WithProgressCallback(func(lvl loglevel.Level, msg string) {
			fmt.Fprintln(os.Stderr, styleMessage(lvl, msg))
		}),
	)

	// Execute prompt
	response, err := agentInstance.HandleMessage(prompt)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Print response to stdout (for piping/redirection)
	fmt.Println(response)
	os.Exit(0)
}

// runREPLMode runs the interactive REPL
func runREPLMode(level loglevel.Level) {
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
		agent.WithLogLevel(level),
		agent.WithProgressCallback(func(lvl loglevel.Level, msg string) {
			fmt.Println(styleMessage(lvl, msg))
		}),
	)

	// Start REPL
	fmt.Println("Clyde - AI Coding Agent - Type 'exit' or 'quit' to exit")
	fmt.Println("==========================================================")

	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Print("\n" + style.FormatUserPrompt())
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
		fmt.Printf("\n%s%s\n", style.FormatAgentPrefix(), response)
	}
}

// styleMessage applies color styling to a progress message based on its log level.
// Messages emitted at different levels carry different semantic meaning:
//   - Quiet:   tool → progress lines (bold yellow tool label)
//   - Normal:  tool output bodies (dim/faint)
//   - Verbose: cache/diagnostic info (default)
//   - Debug:   harness diagnostics (red)
func styleMessage(level loglevel.Level, msg string) string {
	switch level {
	case loglevel.Quiet:
		// Tool progress lines: "→ Reading file: main.go"
		return style.FormatToolProgress(msg)
	case loglevel.Normal:
		// Tool output bodies: secondary content
		return style.FormatDim(msg)
	case loglevel.Debug:
		// Debug diagnostics
		return style.FormatDebug(msg)
	default:
		// Verbose and others: no special styling
		return msg
	}
}

// readPromptFromFile reads a prompt from a file
func readPromptFromFile(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read file '%s': %w", path, err)
	}
	return string(content), nil
}

// readPromptFromStdin reads a prompt from stdin
func readPromptFromStdin() (string, error) {
	content, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", fmt.Errorf("failed to read from stdin: %w", err)
	}
	return string(content), nil
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
