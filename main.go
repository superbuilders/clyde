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
	"github.com/this-is-alpha-iota/clyde/input"
	"github.com/this-is-alpha-iota/clyde/loglevel"
	"github.com/this-is-alpha-iota/clyde/prompt"
	"github.com/this-is-alpha-iota/clyde/prompts"
	"github.com/this-is-alpha-iota/clyde/spinner"
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
	var userPrompt string
	var err error

	if len(args) > 0 && args[0] == "-f" {
		// Read from file
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Error: -f requires a file path")
			fmt.Fprintln(os.Stderr, "Usage: clyde -f prompt.txt")
			os.Exit(1)
		}
		userPrompt, err = readPromptFromFile(args[1])
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading prompt file: %v\n", err)
			os.Exit(1)
		}
	} else if hasStdinInput {
		// stdin is piped/redirected
		userPrompt, err = readPromptFromStdin()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading from stdin: %v\n", err)
			os.Exit(1)
		}
	} else {
		// Treat all args as the prompt string
		userPrompt = strings.Join(args, " ")
	}

	if strings.TrimSpace(userPrompt) == "" {
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
	response, err := agentInstance.HandleMessage(userPrompt)
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

	// Create spinner for animated progress display (REPL mode only).
	// The spinner shows a live preview on the second-to-last terminal line.
	// When a tool completes, the spinner clears and the permanent → line
	// is appended to scrollback.
	sp := spinner.New()

	// lastProgressMsg tracks the most recent tool → progress message so we
	// can print it as a permanent log line when the spinner stops.
	var lastProgressMsg string

	// Create agent with system prompt and spinner-aware progress callback
	agentInstance := agent.NewAgent(
		apiClient,
		prompts.SystemPrompt,
		agent.WithLogLevel(level),
		agent.WithSpinnerCallback(func(start bool, message string) {
			if level == loglevel.Silent {
				return
			}
			if start {
				sp.Start(message)
			} else {
				if sp.IsActive() {
					sp.Stop()
				}
			}
		}),
		agent.WithProgressCallback(func(lvl loglevel.Level, msg string) {
			switch lvl {
			case loglevel.Quiet:
				// Tool progress line (→ Reading file: main.go)
				// Start/update the spinner with this message
				lastProgressMsg = msg
				if level != loglevel.Silent {
					sp.Start(spinner.FormatSpinnerMessage(msg))
				}

			case loglevel.Normal:
				// Tool output body — tool execution is complete.
				// Stop spinner, print permanent progress line, then output body.
				if sp.IsActive() {
					sp.Stop()
				}
				if lastProgressMsg != "" {
					fmt.Println(styleMessage(loglevel.Quiet, lastProgressMsg))
					lastProgressMsg = ""
				}
				fmt.Println(styleMessage(lvl, msg))

			default:
				// Verbose, Debug, etc. — stop spinner if active, print directly.
				if sp.IsActive() {
					sp.Stop()
					if lastProgressMsg != "" {
						fmt.Println(styleMessage(loglevel.Quiet, lastProgressMsg))
						lastProgressMsg = ""
					}
				}
				fmt.Println(styleMessage(lvl, msg))
			}
		}),
	)

	// Start REPL
	fmt.Println("Clyde - AI Coding Agent - Type 'exit' or 'quit' to exit")
	fmt.Println("  Multiline: end a line with \\ to continue on the next line")
	fmt.Println("==========================================================")

	// Create rich text input reader (readline-based).
	// Provides cursor movement, history recall, and multiline input.
	homeDir, _ := os.UserHomeDir()
	historyFile := ""
	if homeDir != "" {
		historyFile = filepath.Join(homeDir, ".clyde", "history")
	}

	// Build the initial prompt
	gitInfo := prompt.GetGitInfo()
	initialPrompt := "\n" + prompt.FormatPrompt(gitInfo, -1)

	reader, err := input.New(input.Config{
		Prompt:      initialPrompt,
		HistoryFile: historyFile,
	})
	if err != nil {
		// Fall back to basic bufio reader if readline fails
		fmt.Fprintf(os.Stderr, "Warning: Rich input unavailable (%v), using basic input\n", err)
		runREPLBasicMode(level, apiClient, sp, cfg)
		return
	}
	defer reader.Close()

	// contextPercent starts at -1 (no data yet) until the first API response
	contextPercent := -1

	for {
		// Refresh git info and update prompt on each iteration
		gitInfo := prompt.GetGitInfo()
		reader.SetPrompt("\n" + prompt.FormatPrompt(gitInfo, contextPercent))

		userInput, err := reader.ReadLine()
		if err != nil {
			if err == io.EOF {
				fmt.Println("\nGoodbye!")
				break
			}
			// ErrInterrupt (Ctrl+C) — just show a new prompt
			continue
		}

		userInput = strings.TrimSpace(userInput)
		if userInput == "" {
			continue
		}

		if userInput == "exit" || userInput == "quit" {
			fmt.Println("Goodbye!")
			break
		}

		response, _ := agentInstance.HandleMessage(userInput)

		// Ensure spinner is stopped before printing the response
		// (handles edge case where tool emits → but no output body)
		if sp.IsActive() {
			sp.Stop()
		}
		if lastProgressMsg != "" {
			fmt.Println(styleMessage(loglevel.Quiet, lastProgressMsg))
			lastProgressMsg = ""
		}

		fmt.Printf("\n%s%s\n", style.FormatAgentPrefix(), response)

		// Update context percentage for next prompt
		usage := agentInstance.LastUsage()
		totalInput := usage.InputTokens + usage.CacheReadInputTokens
		contextPercent = prompt.CalculateContextPercent(totalInput, cfg.ContextWindowSize)
	}
}

// runREPLBasicMode is the fallback REPL when readline is unavailable.
// It uses bufio.NewReader for basic line input without cursor movement
// or history recall. This ensures Clyde can still run on systems where
// readline initialization fails.
func runREPLBasicMode(level loglevel.Level, apiClient *api.Client, sp *spinner.Spinner, cfg *config.Config) {
	var lastProgressMsg string

	agentInstance := agent.NewAgent(
		apiClient,
		prompts.SystemPrompt,
		agent.WithLogLevel(level),
		agent.WithSpinnerCallback(func(start bool, message string) {
			if level == loglevel.Silent {
				return
			}
			if start {
				sp.Start(message)
			} else {
				if sp.IsActive() {
					sp.Stop()
				}
			}
		}),
		agent.WithProgressCallback(func(lvl loglevel.Level, msg string) {
			switch lvl {
			case loglevel.Quiet:
				lastProgressMsg = msg
				if level != loglevel.Silent {
					sp.Start(spinner.FormatSpinnerMessage(msg))
				}
			case loglevel.Normal:
				if sp.IsActive() {
					sp.Stop()
				}
				if lastProgressMsg != "" {
					fmt.Println(styleMessage(loglevel.Quiet, lastProgressMsg))
					lastProgressMsg = ""
				}
				fmt.Println(styleMessage(lvl, msg))
			default:
				if sp.IsActive() {
					sp.Stop()
					if lastProgressMsg != "" {
						fmt.Println(styleMessage(loglevel.Quiet, lastProgressMsg))
						lastProgressMsg = ""
					}
				}
				fmt.Println(styleMessage(lvl, msg))
			}
		}),
	)

	reader := bufio.NewReader(os.Stdin)
	contextPercent := -1

	for {
		gitInfo := prompt.GetGitInfo()
		fmt.Print("\n" + prompt.FormatPrompt(gitInfo, contextPercent))
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				fmt.Println("\nGoodbye!")
				break
			}
			fmt.Printf("Error reading input: %v\n", err)
			continue
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if line == "exit" || line == "quit" {
			fmt.Println("Goodbye!")
			break
		}

		response, _ := agentInstance.HandleMessage(line)

		if sp.IsActive() {
			sp.Stop()
		}
		if lastProgressMsg != "" {
			fmt.Println(styleMessage(loglevel.Quiet, lastProgressMsg))
			lastProgressMsg = ""
		}

		fmt.Printf("\n%s%s\n", style.FormatAgentPrefix(), response)

		usage := agentInstance.LastUsage()
		totalInput := usage.InputTokens + usage.CacheReadInputTokens
		contextPercent = prompt.CalculateContextPercent(totalInput, cfg.ContextWindowSize)
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
