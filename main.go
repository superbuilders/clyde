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
	// Parse log level and extended flags, stripping them from args
	flags := loglevel.ParseFlagsExt(os.Args[1:])

	// Check if stdin has input (pipe/redirect)
	stat, _ := os.Stdin.Stat()
	hasStdinInput := (stat.Mode() & os.ModeCharDevice) == 0

	// Determine mode: CLI or REPL
	// CLI mode if: args provided OR stdin is piped
	// REPL mode if: no args AND stdin is interactive (terminal)
	if len(flags.Args) > 0 || hasStdinInput {
		runCLIMode(flags.Args, hasStdinInput, flags.Level, flags.NoThink)
	} else {
		runREPLMode(flags.Level, flags.NoThink)
	}
}

// createAPIClient creates an API client with optional thinking enabled.
// When noThink is false and the model supports it, adaptive thinking is enabled.
func createAPIClient(cfg *config.Config, noThink bool) *api.Client {
	client := api.NewClient(cfg.APIKey, cfg.APIURL, cfg.ModelID, cfg.MaxTokens)

	if !noThink {
		// Enable adaptive thinking (recommended for Opus 4.6 and Sonnet 4.6).
		// Adaptive thinking lets Claude decide when and how much to think.
		// For older models, budget_tokens would be needed instead.
		thinking := &api.ThinkingConfig{
			Type: "adaptive",
		}

		// If a budget is configured, use manual mode instead
		if cfg.ThinkingBudgetTokens > 0 {
			thinking = &api.ThinkingConfig{
				Type:         "enabled",
				BudgetTokens: cfg.ThinkingBudgetTokens,
			}
		}

		client = client.WithThinking(thinking)
	}

	return client
}

// runCLIMode executes the agent on a single prompt and exits
func runCLIMode(args []string, hasStdinInput bool, level loglevel.Level, noThink bool) {
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

	// Create API client with thinking
	apiClient := createAPIClient(cfg, noThink)

	// Create agent with progress callback (print to stderr so stdout is clean)
	agentInstance := agent.NewAgent(
		apiClient,
		prompts.SystemPrompt,
		agent.WithLogLevel(level),
		agent.WithContextWindowSize(cfg.ContextWindowSize),
		agent.WithProgressCallback(func(lvl loglevel.Level, msg string) {
			if lvl == loglevel.Normal {
				// Tool output body — add blank line separation above and below
				fmt.Fprintln(os.Stderr)
				fmt.Fprintln(os.Stderr, styleMessage(lvl, msg))
				fmt.Fprintln(os.Stderr)
			} else {
				fmt.Fprintln(os.Stderr, styleMessage(lvl, msg))
			}
		}),
		agent.WithThinkingCallback(func(text string) {
			fmt.Fprintln(os.Stderr, style.FormatThinking(text))
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
func runREPLMode(level loglevel.Level, noThink bool) {
	// Determine config file location (CLI layer responsibility)
	configPath := getConfigPath()

	// Load configuration from the determined path
	cfg, err := config.LoadFromFile(configPath)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	// Create API client with thinking
	apiClient := createAPIClient(cfg, noThink)

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
		agent.WithContextWindowSize(cfg.ContextWindowSize),
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
		agent.WithThinkingCallback(func(text string) {
			// Stop spinner before printing thinking (thinking comes before tool calls)
			if sp.IsActive() {
				sp.Stop()
			}
			// Flush any pending progress message
			if lastProgressMsg != "" {
				fmt.Println(styleMessage(loglevel.Quiet, lastProgressMsg))
				lastProgressMsg = ""
			}
			fmt.Println(style.FormatThinking(text))
		}),
		agent.WithProgressCallback(func(lvl loglevel.Level, msg string) {
			switch lvl {
			case loglevel.Quiet:
				// Tool progress line (→ Reading file: main.go).
				// If there's a pending progress message from a previous tool,
				// flush it now before updating. This ensures → lines are
				// persisted at Quiet level where the Normal handler never fires.
				if lastProgressMsg != "" {
					if sp.IsActive() {
						sp.Stop()
					}
					fmt.Println(styleMessage(loglevel.Quiet, lastProgressMsg))
				}
				lastProgressMsg = msg
				if level != loglevel.Silent {
					sp.Start(spinner.FormatSpinnerMessage(msg))
				}

			case loglevel.Normal:
				// Tool output body — tool execution is complete.
				// Stop spinner, print permanent progress line, then output
				// body with blank line separation above and below.
				if sp.IsActive() {
					sp.Stop()
				}
				if lastProgressMsg != "" {
					fmt.Println(styleMessage(loglevel.Quiet, lastProgressMsg))
					lastProgressMsg = ""
				}
				fmt.Println()                        // blank line above output body
				fmt.Println(styleMessage(lvl, msg))
				fmt.Println()                        // blank line below output body

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
	fmt.Println("  Multiline: Ctrl+J or Alt+Enter to insert a newline,")
	fmt.Println("             or end a line with \\ to continue")
	fmt.Println("==========================================================")

	// Create rich text input reader (readline-based).
	// Provides cursor movement, history recall, and multiline input.
	homeDir, _ := os.UserHomeDir()
	historyFile := ""
	if homeDir != "" {
		historyFile = filepath.Join(homeDir, ".clyde", "history")
	}

	// Build the initial prompt.
	// NOTE: The "\n" separator is NOT part of the prompt string because
	// chzyer/readline redraws the prompt on every keystroke. A newline
	// embedded in the prompt would be emitted on every redraw, scrolling
	// content upward. Instead we print the newline once before ReadLine().
	gitInfo := prompt.GetGitInfo()
	initialPrompt := prompt.FormatPrompt(gitInfo, -1)

	reader, err := input.New(input.Config{
		Prompt:      initialPrompt,
		HistoryFile: historyFile,
	})
	if err != nil {
		// Fall back to basic bufio reader if readline fails
		fmt.Fprintf(os.Stderr, "Warning: Rich input unavailable (%v), using basic input\n", err)
		runREPLBasicMode(level, noThink, apiClient, sp, cfg)
		return
	}
	defer reader.Close()

	// contextPercent starts at -1 (no data yet) until the first API response
	contextPercent := -1

	for {
		// Refresh git info and update prompt on each iteration.
		// Print the blank-line separator here (not in the prompt string)
		// so it is emitted once instead of on every keystroke redraw.
		gitInfo := prompt.GetGitInfo()
		fmt.Println()
		reader.SetPrompt(prompt.FormatPrompt(gitInfo, contextPercent))

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
func runREPLBasicMode(level loglevel.Level, noThink bool, apiClient *api.Client, sp *spinner.Spinner, cfg *config.Config) {
	var lastProgressMsg string

	agentInstance := agent.NewAgent(
		apiClient,
		prompts.SystemPrompt,
		agent.WithLogLevel(level),
		agent.WithContextWindowSize(cfg.ContextWindowSize),
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
		agent.WithThinkingCallback(func(text string) {
			if sp.IsActive() {
				sp.Stop()
			}
			if lastProgressMsg != "" {
				fmt.Println(styleMessage(loglevel.Quiet, lastProgressMsg))
				lastProgressMsg = ""
			}
			fmt.Println(style.FormatThinking(text))
		}),
		agent.WithProgressCallback(func(lvl loglevel.Level, msg string) {
			switch lvl {
			case loglevel.Quiet:
				if lastProgressMsg != "" {
					if sp.IsActive() {
						sp.Stop()
					}
					fmt.Println(styleMessage(loglevel.Quiet, lastProgressMsg))
				}
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
				fmt.Println()                        // blank line above output body
				fmt.Println(styleMessage(lvl, msg))
				fmt.Println()                        // blank line below output body
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
