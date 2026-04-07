// Package cli implements Clyde's CLI and REPL interfaces.
//
// It contains all user-facing I/O orchestration: flag parsing, prompt
// management, spinner control, display filtering, truncation, and mode
// selection (CLI vs REPL). The agent package handles conversation logic;
// this package handles all display concerns.
package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/this-is-alpha-iota/clyde/agent"
	"github.com/this-is-alpha-iota/clyde/agent/mcp"
	"github.com/this-is-alpha-iota/clyde/agent/prompts"
	"github.com/this-is-alpha-iota/clyde/agent/truncate"
	"github.com/this-is-alpha-iota/clyde/cli/input"
	"github.com/this-is-alpha-iota/clyde/cli/prompt"
	"github.com/this-is-alpha-iota/clyde/cli/spinner"
	"github.com/this-is-alpha-iota/clyde/cli/style"
	"github.com/this-is-alpha-iota/clyde/config"
	"github.com/this-is-alpha-iota/clyde/loglevel"
	"github.com/this-is-alpha-iota/clyde/providers"
	_ "github.com/this-is-alpha-iota/clyde/tools" // Import tools to register them
)

// Run is the main entrypoint for the Clyde CLI application.
// It parses flags, determines the execution mode (CLI vs REPL),
// and dispatches accordingly.
func Run() {
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

// setupMCPPlaywright registers Playwright MCP tools if configured.
// Returns the server (for later cleanup) or nil if not enabled.
func setupMCPPlaywright(cfg *config.Config) *mcp.PlaywrightServer {
	if !cfg.MCPPlaywright {
		return nil
	}

	server := mcp.NewPlaywrightServer(cfg.MCPPlaywrightArgs)
	if err := mcp.RegisterPlaywrightTools(server); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Failed to register Playwright MCP tools: %v\n", err)
		return nil
	}

	return server
}

// createAPIClient creates an API client with optional thinking enabled.
// When noThink is false and the model supports it, adaptive thinking is enabled.
func createAPIClient(cfg *config.Config, noThink bool) *providers.Client {
	client := providers.NewClient(cfg.APIKey, cfg.APIURL, cfg.ModelID, cfg.MaxTokens)

	if !noThink {
		// Enable adaptive thinking (recommended for Opus 4.6 and Sonnet 4.6).
		thinking := &providers.ThinkingConfig{
			Type: "adaptive",
		}

		// If a budget is configured, use manual mode instead
		if cfg.ThinkingBudgetTokens > 0 {
			thinking = &providers.ThinkingConfig{
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

	// Setup Playwright MCP if configured
	mcpServer := setupMCPPlaywright(cfg)
	if mcpServer != nil {
		defer mcpServer.Close()
	}

	// Create agent — the CLI layer owns all display filtering.
	// The agent emits everything unconditionally; we filter here.
	agentInstance := agent.NewAgent(
		apiClient,
		prompts.SystemPrompt,
		agent.WithContextWindowSize(cfg.ContextWindowSize),
		agent.WithProgressCallback(func(msg string) {
			if level.ShouldShow(loglevel.Quiet) {
				fmt.Fprintln(os.Stderr, StyleMessage(loglevel.Quiet, msg))
			}
		}),
		agent.WithOutputCallback(func(output string) {
			if level.ShouldShow(loglevel.Normal) {
				displayed := truncateForLevel(output, truncate.ToolOutputLineLimit, level)
				fmt.Fprintln(os.Stderr)
				fmt.Fprintln(os.Stderr, StyleMessage(loglevel.Normal, displayed))
				fmt.Fprintln(os.Stderr)
			}
		}),
		agent.WithThinkingCallback(func(text string) {
			if level.ShouldShow(loglevel.Normal) {
				displayed := truncateForLevel(text, truncate.ThinkingLineLimit, level)
				fmt.Fprintln(os.Stderr, style.FormatThinking(displayed))
			}
		}),
		agent.WithDiagnosticCallback(func(msg string) {
			if strings.HasPrefix(msg, "💾 Cache:") && !strings.Contains(msg, "|") {
				// Verbose cache format
				if level.ShouldShow(loglevel.Verbose) {
					fmt.Fprintln(os.Stderr, msg)
				}
			} else if strings.HasPrefix(msg, "💾 Cache:") && strings.Contains(msg, "|") {
				// Debug cache format
				if level.ShouldShow(loglevel.Debug) {
					fmt.Fprintln(os.Stderr, StyleMessage(loglevel.Debug, msg))
				}
			} else if strings.HasPrefix(msg, "🔍") || strings.HasPrefix(msg, "🔒") {
				// Token diagnostics and redacted thinking
				if level.ShouldShow(loglevel.Debug) {
					fmt.Fprintln(os.Stderr, StyleMessage(loglevel.Debug, msg))
				}
			}
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

	// Setup Playwright MCP if configured
	mcpServer := setupMCPPlaywright(cfg)
	if mcpServer != nil {
		defer mcpServer.Close()
	}

	// Create spinner for animated progress display (REPL mode only).
	sp := spinner.New()

	// lastProgressMsg tracks the most recent tool → progress message so we
	// can print it as a permanent log line when the spinner stops.
	var lastProgressMsg string

	// Create agent — the CLI layer owns all display filtering, truncation,
	// and spinner management. The agent emits everything unconditionally.
	agentInstance := agent.NewAgent(
		apiClient,
		prompts.SystemPrompt,
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
			if !level.ShouldShow(loglevel.Normal) {
				return
			}
			// Stop spinner before printing thinking
			if sp.IsActive() {
				sp.Stop()
			}
			// Flush any pending progress message
			if lastProgressMsg != "" {
				fmt.Println(StyleMessage(loglevel.Quiet, lastProgressMsg))
				lastProgressMsg = ""
			}
			displayed := truncateForLevel(text, truncate.ThinkingLineLimit, level)
			fmt.Println(style.FormatThinking(displayed))
		}),
		agent.WithProgressCallback(func(msg string) {
			if !level.ShouldShow(loglevel.Quiet) {
				return
			}
			// Tool progress line (→ Reading file: main.go).
			// If there's a pending progress message from a previous tool,
			// flush it now before updating.
			if lastProgressMsg != "" {
				if sp.IsActive() {
					sp.Stop()
				}
				fmt.Println(StyleMessage(loglevel.Quiet, lastProgressMsg))
			}
			lastProgressMsg = msg
			if level != loglevel.Silent {
				sp.Start(spinner.FormatSpinnerMessage(msg))
			}
		}),
		agent.WithOutputCallback(func(output string) {
			if !level.ShouldShow(loglevel.Normal) {
				return
			}
			// Tool output body — tool execution is complete.
			// Stop spinner, print permanent progress line, then output
			// body with blank line separation above and below.
			if sp.IsActive() {
				sp.Stop()
			}
			if lastProgressMsg != "" {
				fmt.Println(StyleMessage(loglevel.Quiet, lastProgressMsg))
				lastProgressMsg = ""
			}
			displayed := truncateForLevel(output, truncate.ToolOutputLineLimit, level)
			fmt.Println()
			fmt.Println(StyleMessage(loglevel.Normal, displayed))
			fmt.Println()
		}),
		agent.WithDiagnosticCallback(func(msg string) {
			if strings.HasPrefix(msg, "💾 Cache:") && !strings.Contains(msg, "|") {
				// Verbose cache format
				if !level.ShouldShow(loglevel.Verbose) {
					return
				}
			} else if strings.HasPrefix(msg, "💾 Cache:") && strings.Contains(msg, "|") {
				// Debug cache format
				if !level.ShouldShow(loglevel.Debug) {
					return
				}
			} else {
				// Token diagnostics, redacted thinking
				if !level.ShouldShow(loglevel.Debug) {
					return
				}
			}
			// Stop spinner if active, print directly
			if sp.IsActive() {
				sp.Stop()
				if lastProgressMsg != "" {
					fmt.Println(StyleMessage(loglevel.Quiet, lastProgressMsg))
					lastProgressMsg = ""
				}
			}
			fmt.Println(StyleMessage(loglevel.Debug, msg))
		}),
	)

	// Start REPL
	fmt.Println("Clyde - AI Coding Agent - Type 'exit' or 'quit' to exit")
	fmt.Println("  Multiline: Ctrl+J or Alt+Enter to insert a newline,")
	fmt.Println("             or end a line with \\ to continue")
	fmt.Println("==========================================================")

	// Create rich text input reader (readline-based).
	homeDir, _ := os.UserHomeDir()
	historyFile := ""
	if homeDir != "" {
		historyFile = filepath.Join(homeDir, ".clyde", "history")
	}

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
		if sp.IsActive() {
			sp.Stop()
		}
		if lastProgressMsg != "" {
			fmt.Println(StyleMessage(loglevel.Quiet, lastProgressMsg))
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
func runREPLBasicMode(level loglevel.Level, noThink bool, apiClient *providers.Client, sp *spinner.Spinner, cfg *config.Config) {
	var lastProgressMsg string

	agentInstance := agent.NewAgent(
		apiClient,
		prompts.SystemPrompt,
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
			if !level.ShouldShow(loglevel.Normal) {
				return
			}
			if sp.IsActive() {
				sp.Stop()
			}
			if lastProgressMsg != "" {
				fmt.Println(StyleMessage(loglevel.Quiet, lastProgressMsg))
				lastProgressMsg = ""
			}
			displayed := truncateForLevel(text, truncate.ThinkingLineLimit, level)
			fmt.Println(style.FormatThinking(displayed))
		}),
		agent.WithProgressCallback(func(msg string) {
			if !level.ShouldShow(loglevel.Quiet) {
				return
			}
			if lastProgressMsg != "" {
				if sp.IsActive() {
					sp.Stop()
				}
				fmt.Println(StyleMessage(loglevel.Quiet, lastProgressMsg))
			}
			lastProgressMsg = msg
			if level != loglevel.Silent {
				sp.Start(spinner.FormatSpinnerMessage(msg))
			}
		}),
		agent.WithOutputCallback(func(output string) {
			if !level.ShouldShow(loglevel.Normal) {
				return
			}
			if sp.IsActive() {
				sp.Stop()
			}
			if lastProgressMsg != "" {
				fmt.Println(StyleMessage(loglevel.Quiet, lastProgressMsg))
				lastProgressMsg = ""
			}
			displayed := truncateForLevel(output, truncate.ToolOutputLineLimit, level)
			fmt.Println()
			fmt.Println(StyleMessage(loglevel.Normal, displayed))
			fmt.Println()
		}),
		agent.WithDiagnosticCallback(func(msg string) {
			if strings.HasPrefix(msg, "💾 Cache:") && !strings.Contains(msg, "|") {
				if !level.ShouldShow(loglevel.Verbose) {
					return
				}
			} else if strings.HasPrefix(msg, "💾 Cache:") && strings.Contains(msg, "|") {
				if !level.ShouldShow(loglevel.Debug) {
					return
				}
			} else {
				if !level.ShouldShow(loglevel.Debug) {
					return
				}
			}
			if sp.IsActive() {
				sp.Stop()
				if lastProgressMsg != "" {
					fmt.Println(StyleMessage(loglevel.Quiet, lastProgressMsg))
					lastProgressMsg = ""
				}
			}
			fmt.Println(StyleMessage(loglevel.Debug, msg))
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
			fmt.Println(StyleMessage(loglevel.Quiet, lastProgressMsg))
			lastProgressMsg = ""
		}

		fmt.Printf("\n%s%s\n", style.FormatAgentPrefix(), response)

		usage := agentInstance.LastUsage()
		totalInput := usage.InputTokens + usage.CacheReadInputTokens
		contextPercent = prompt.CalculateContextPercent(totalInput, cfg.ContextWindowSize)
	}
}

// truncateForLevel applies truncation based on the log level.
// At Verbose and Debug, text passes through unmodified.
// At Normal and below, text is truncated to maxLines with character limits.
func truncateForLevel(text string, maxLines int, level loglevel.Level) string {
	if level.ShouldShow(loglevel.Verbose) {
		return text
	}
	return truncate.Text(text, maxLines)
}

// StyleMessage applies color styling to a progress message based on its log level.
// Messages emitted at different levels carry different semantic meaning:
//   - Quiet:   tool → progress lines (bold yellow tool label)
//   - Normal:  tool output bodies (dim/faint)
//   - Verbose: cache/diagnostic info (default)
//   - Debug:   harness diagnostics (red)
func StyleMessage(level loglevel.Level, msg string) string {
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
func getConfigPath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Could not determine home directory: %v\n", err)
		os.Exit(1)
	}

	configPath := filepath.Join(homeDir, ".clyde", "config")

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
