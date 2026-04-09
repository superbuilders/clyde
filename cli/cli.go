// Package cli implements Clyde's CLI and REPL interfaces.
//
// It contains all user-facing I/O orchestration: flag parsing, prompt
// management, spinner control, display filtering, truncation, and mode
// selection (CLI vs REPL). The agent package handles conversation logic;
// this package handles all display concerns.
//
// The CLI imports only the agent package (plus its own cli/* subpackages).
// It never reaches into agent/providers, agent/tools, or agent/config
// directly — all agent construction goes through agent.New().
package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/joho/godotenv"

	"github.com/this-is-alpha-iota/clyde/agent"
	"github.com/this-is-alpha-iota/clyde/agent/providers"
	"github.com/this-is-alpha-iota/clyde/agent/session"
	"github.com/this-is-alpha-iota/clyde/cli/input"
	"github.com/this-is-alpha-iota/clyde/cli/loglevel"
	"github.com/this-is-alpha-iota/clyde/cli/prompt"
	"github.com/this-is-alpha-iota/clyde/cli/spinner"
	"github.com/this-is-alpha-iota/clyde/cli/style"
	"github.com/this-is-alpha-iota/clyde/cli/truncate"
)

// Run is the main entrypoint for the Clyde CLI application.
// It parses flags, determines the execution mode (CLI vs REPL),
// and dispatches accordingly.
func Run() {
	// Parse log level and extended flags, stripping them from args
	flags := loglevel.ParseFlagsExt(os.Args[1:])

	// Handle --sessions mode (list and exit)
	if flags.Sessions {
		runSessionsMode()
		return
	}

	// Handle --resume mode
	if flags.Resume {
		runResumeMode(flags.ResumeTarget, flags.Level, flags.NoThink)
		return
	}

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

// loadAgentConfig reads the config file and returns an agent.Config.
// The CLI owns config file discovery and parsing; it maps the result
// into the agent's Config struct.
func loadAgentConfig(configPath string, noThink bool) (agent.Config, error) {
	// Check if file exists
	if _, err := os.Stat(configPath); err != nil {
		return agent.Config{}, fmt.Errorf("config file '%s' not found: %w", configPath, err)
	}

	// Load environment variables from the file
	if err := godotenv.Load(configPath); err != nil {
		return agent.Config{}, fmt.Errorf("error loading config file from '%s': %w", configPath, err)
	}

	// Verify required API key is present
	apiKey := os.Getenv("TS_AGENT_API_KEY")
	if apiKey == "" {
		return agent.Config{}, fmt.Errorf("TS_AGENT_API_KEY not found in '%s'\n\n"+
			"Please add this line to your config file:\n"+
			"  TS_AGENT_API_KEY=your-anthropic-api-key-here\n\n"+
			"Get your API key from: https://console.anthropic.com/", configPath)
	}

	// Parse optional thinking budget tokens
	thinkingBudget := 0
	if budgetStr := os.Getenv("THINKING_BUDGET_TOKENS"); budgetStr != "" {
		budget, err := strconv.Atoi(budgetStr)
		if err != nil {
			return agent.Config{}, fmt.Errorf("THINKING_BUDGET_TOKENS must be a number, got %q: %w", budgetStr, err)
		}
		if budget < 1024 {
			return agent.Config{}, fmt.Errorf("THINKING_BUDGET_TOKENS must be >= 1024, got %d", budget)
		}
		thinkingBudget = budget
	}

	return agent.Config{
		APIKey:            apiKey,
		APIURL:            "https://api.anthropic.com/v1/messages",
		ModelID:           "claude-opus-4-6",
		MaxTokens:         64000,
		ContextWindowSize: 200000, // Claude Opus 4.6 context window
		ThinkingBudget:    thinkingBudget,
		NoThink:           noThink,
		BraveSearchAPIKey: os.Getenv("BRAVE_SEARCH_API_KEY"),
		MCPPlaywright:     os.Getenv("MCP_PLAYWRIGHT") == "true",
		MCPPlaywrightArgs: os.Getenv("MCP_PLAYWRIGHT_ARGS"),
	}, nil
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

	// Load config and build agent.Config
	configPath := getConfigPath()
	cfg, err := loadAgentConfig(configPath, noThink)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	// Create session for persistence
	sess, err := session.New()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: session creation failed: %v\n", err)
		// Continue without session — non-fatal
	}

	// Create agent — the CLI layer owns all display filtering.
	// The agent emits everything unconditionally; we filter here.
	agentInstance := agent.New(cfg,
		agent.WithProgressCallback(func(msg string, toolUseID string) {
			// Append tool use ID for display
			msgWithID := session.FormatToolUseID(msg, toolUseID)
			if level.ShouldShow(loglevel.Quiet) {
				fmt.Fprintln(os.Stderr, StyleMessage(loglevel.Quiet, msgWithID))
			}
		}),
		agent.WithToolUseCallback(func(displayMsg, toolName, toolUseID string, toolInput map[string]interface{}) {
			// Persist enriched tool-use metadata for session reconstruction
			if sess != nil {
				msgWithID := session.FormatToolUseID(displayMsg, toolUseID)
				inputJSON, _ := json.Marshal(toolInput)
				content := fmt.Sprintf("%s\nname: %s\ninput: %s\n",
					session.StripANSI(msgWithID), toolName, string(inputJSON))
				sess.WriteMessage(session.TypeToolUse, content)
			}
		}),
		agent.WithOutputCallback(func(output string, toolUseID string) {
			if level.ShouldShow(loglevel.Normal) {
				displayed := truncateForLevel(output, truncate.ToolOutputLineLimit, level)
				fmt.Fprintln(os.Stderr)
				fmt.Fprintln(os.Stderr, StyleMessage(loglevel.Normal, displayed))
				fmt.Fprintln(os.Stderr)
			}
			// Persist full (untruncated) tool output with tool_use_id
			if sess != nil {
				content := fmt.Sprintf("[%s]\n```\n%s\n```\n", toolUseID, output)
				sess.WriteMessage(session.TypeToolResult, content)
			}
		}),
		agent.WithThinkingCallback(func(text string) {
			if level.ShouldShow(loglevel.Normal) {
				displayed := truncateForLevel(text, truncate.ThinkingLineLimit, level)
				fmt.Fprintln(os.Stderr, style.FormatThinking(displayed))
			}
			// Persist full thinking to session
			if sess != nil {
				sess.WriteMessage(session.TypeThinking, "💭 "+text+"\n")
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
			// Persist all diagnostics to session (always at debug level)
			if sess != nil {
				sess.WriteMessage(session.TypeDiagnostic, msg+"\n")
			}
		}),
		agent.WithUserMessageCallback(func(text string) {
			if sess != nil {
				sess.WriteMessage(session.TypeUser, "**You:**\n\n"+text+"\n")
			}
		}),
		agent.WithAssistantMessageCallback(func(text string) {
			if sess != nil {
				sess.WriteMessage(session.TypeAssistant, "**Claude:**\n\n"+text+"\n")
			}
		}),
		agent.WithErrorCallback(func(err error) {
			fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
		}),
	)
	defer agentInstance.Close()

	// Execute prompt
	response, err := agentInstance.HandleMessage(userPrompt)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Print response to stdout (for piping/redirection)
	fmt.Println(response)

	// Print session path on exit
	if sess != nil {
		fmt.Fprintf(os.Stderr, "Session saved: %s\n", sess.RelativeDir())
	}

	os.Exit(0)
}

// runREPLMode runs the interactive REPL
func runREPLMode(level loglevel.Level, noThink bool) {
	// Load config and build agent.Config
	configPath := getConfigPath()
	cfg, err := loadAgentConfig(configPath, noThink)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	// Create session for persistence
	sess, err := session.New()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: session creation failed: %v\n", err)
		// Continue without session — non-fatal
	}

	// Create spinner for animated progress display (REPL mode only).
	sp := spinner.New()

	// lastProgressMsg tracks the most recent tool → progress message so we
	// can print it as a permanent log line when the spinner stops.
	var lastProgressMsg string

	// Create agent — the CLI layer owns all display filtering, truncation,
	// and spinner management. The agent emits everything unconditionally.
	agentInstance := agent.New(cfg,
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
			// Persist full thinking to session (always, regardless of display level)
			if sess != nil {
				sess.WriteMessage(session.TypeThinking, "💭 "+text+"\n")
			}
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
		agent.WithProgressCallback(func(msg string, toolUseID string) {
			// Append tool use ID for display
			msgWithID := session.FormatToolUseID(msg, toolUseID)
			if !level.ShouldShow(loglevel.Quiet) {
				return
			}
			// Tool progress line (→ Reading file: main.go [toolu_abc123]).
			// If there's a pending progress message from a previous tool,
			// flush it now before updating.
			if lastProgressMsg != "" {
				if sp.IsActive() {
					sp.Stop()
				}
				fmt.Println(StyleMessage(loglevel.Quiet, lastProgressMsg))
			}
			lastProgressMsg = msgWithID
			if level != loglevel.Silent {
				sp.Start(spinner.FormatSpinnerMessage(msgWithID))
			}
		}),
		agent.WithToolUseCallback(func(displayMsg, toolName, toolUseID string, toolInput map[string]interface{}) {
			// Persist enriched tool-use metadata for session reconstruction
			if sess != nil {
				msgWithID := session.FormatToolUseID(displayMsg, toolUseID)
				inputJSON, _ := json.Marshal(toolInput)
				content := fmt.Sprintf("%s\nname: %s\ninput: %s\n",
					session.StripANSI(msgWithID), toolName, string(inputJSON))
				sess.WriteMessage(session.TypeToolUse, content)
			}
		}),
		agent.WithOutputCallback(func(output string, toolUseID string) {
			// Persist full (untruncated) tool output with tool_use_id
			if sess != nil {
				content := fmt.Sprintf("[%s]\n```\n%s\n```\n", toolUseID, output)
				sess.WriteMessage(session.TypeToolResult, content)
			}
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
			// Persist all diagnostics to session (always at debug level)
			if sess != nil {
				sess.WriteMessage(session.TypeDiagnostic, msg+"\n")
			}
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
		agent.WithUserMessageCallback(func(text string) {
			if sess != nil {
				sess.WriteMessage(session.TypeUser, "**You:**\n\n"+text+"\n")
			}
		}),
		agent.WithAssistantMessageCallback(func(text string) {
			if sess != nil {
				sess.WriteMessage(session.TypeAssistant, "**Claude:**\n\n"+text+"\n")
			}
		}),
		agent.WithErrorCallback(func(err error) {
			fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
		}),
	)
	defer agentInstance.Close()

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
		runREPLBasicMode(level, agentInstance, sp, cfg.ContextWindowSize, sess)
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
				printGoodbye(sess)
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
			printGoodbye(sess)
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
func runREPLBasicMode(level loglevel.Level, agentInstance *agent.Agent, sp *spinner.Spinner, contextWindowSize int, sess *session.Session) {
	var lastProgressMsg string

	// The agent is already created by the caller. We just need to set up
	// the basic input loop. The callbacks were already configured when the
	// agent was created in runREPLMode.

	reader := bufio.NewReader(os.Stdin)
	contextPercent := -1

	for {
		gitInfo := prompt.GetGitInfo()
		fmt.Print("\n" + prompt.FormatPrompt(gitInfo, contextPercent))
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				printGoodbye(sess)
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
			printGoodbye(sess)
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
		contextPercent = prompt.CalculateContextPercent(totalInput, contextWindowSize)
	}
}

// printGoodbye prints the goodbye message and session path.
func printGoodbye(sess *session.Session) {
	fmt.Println("Goodbye!")
	if sess != nil {
		fmt.Printf("Session saved: %s\n", sess.RelativeDir())
	}
}

// runSessionsMode lists all sessions and exits.
func runSessionsMode() {
	sessionsRoot, _ := session.FindSessionsRoot()
	sessions, err := session.ListSessions(sessionsRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing sessions: %v\n", err)
		os.Exit(1)
	}

	if len(sessions) == 0 {
		fmt.Println("No sessions found.")
		fmt.Printf("Sessions directory: %s\n", sessionsRoot)
		return
	}

	fmt.Printf("Sessions in %s:\n\n", sessionsRoot)
	for _, s := range sessions {
		summary := s.Summary
		if summary == "" {
			summary = "(no user messages)"
		}
		fmt.Printf("  %-40s  %3d messages  %q\n", s.DirName, s.MessageCount, summary)
	}
	fmt.Println()
	fmt.Println("Use --resume <session-id> to resume, or --resume for your most recent.")
}

// runResumeMode loads a previous session and starts the REPL.
func runResumeMode(target string, level loglevel.Level, noThink bool) {
	sessionsRoot, _ := session.FindSessionsRoot()
	currentUser := session.GetUsername()

	var sessionDir string
	var err error

	if target == "" {
		// --resume with no argument: find most recent session for current user
		sessionDir, err = session.FindMostRecentSession(sessionsRoot, currentUser)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			fmt.Fprintln(os.Stderr, "Use --sessions to list available sessions.")
			os.Exit(1)
		}
	} else {
		// --resume <session-id>: find specific session
		sessionDir, err = session.FindSessionByID(sessionsRoot, target)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			fmt.Fprintln(os.Stderr, "Use --sessions to list available sessions.")
			os.Exit(1)
		}
	}

	// Check if cross-user resume is needed
	sessionOwner := session.SessionOwner(filepath.Base(sessionDir))
	if sessionOwner != currentUser {
		// Cross-user resume: copy directory
		fmt.Fprintf(os.Stderr, "Branching from %s's session...\n", sessionOwner)
		sessionDir, err = session.CopyForResume(sessionDir, sessionsRoot, currentUser)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error branching session: %v\n", err)
			os.Exit(1)
		}
	}

	// Reconstruct history
	history, warnings, err := session.ReconstructHistory(sessionDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reconstructing session: %v\n", err)
		os.Exit(1)
	}
	for _, w := range warnings {
		fmt.Fprintf(os.Stderr, "Warning: %s\n", w)
	}

	// Open session for continued writing
	sess, err := session.Open(sessionDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening session: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "Resuming session: %s (%d messages loaded)\n", filepath.Base(sessionDir), len(history))

	// Load config
	configPath := getConfigPath()
	cfg, err := loadAgentConfig(configPath, noThink)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	// Start REPL with restored history
	runREPLModeWithSession(level, noThink, cfg, sess, history)
}

// runREPLModeWithSession runs the REPL with a pre-existing session and history.
// Used by both resume mode and (potentially) regular REPL mode.
func runREPLModeWithSession(level loglevel.Level, noThink bool, cfg agent.Config, sess *session.Session, history []providers.Message) {
	// Create spinner for animated progress display (REPL mode only).
	sp := spinner.New()

	// lastProgressMsg tracks the most recent tool → progress message
	var lastProgressMsg string

	// Create agent
	agentInstance := agent.New(cfg,
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
			if sess != nil {
				sess.WriteMessage(session.TypeThinking, "💭 "+text+"\n")
			}
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
		agent.WithProgressCallback(func(msg string, toolUseID string) {
			msgWithID := session.FormatToolUseID(msg, toolUseID)
			if !level.ShouldShow(loglevel.Quiet) {
				return
			}
			if lastProgressMsg != "" {
				if sp.IsActive() {
					sp.Stop()
				}
				fmt.Println(StyleMessage(loglevel.Quiet, lastProgressMsg))
			}
			lastProgressMsg = msgWithID
			if level != loglevel.Silent {
				sp.Start(spinner.FormatSpinnerMessage(msgWithID))
			}
		}),
		agent.WithToolUseCallback(func(displayMsg, toolName, toolUseID string, toolInput map[string]interface{}) {
			if sess != nil {
				msgWithID := session.FormatToolUseID(displayMsg, toolUseID)
				inputJSON, _ := json.Marshal(toolInput)
				content := fmt.Sprintf("%s\nname: %s\ninput: %s\n",
					session.StripANSI(msgWithID), toolName, string(inputJSON))
				sess.WriteMessage(session.TypeToolUse, content)
			}
		}),
		agent.WithOutputCallback(func(output string, toolUseID string) {
			if sess != nil {
				content := fmt.Sprintf("[%s]\n```\n%s\n```\n", toolUseID, output)
				sess.WriteMessage(session.TypeToolResult, content)
			}
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
			if sess != nil {
				sess.WriteMessage(session.TypeDiagnostic, msg+"\n")
			}
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
		agent.WithUserMessageCallback(func(text string) {
			if sess != nil {
				sess.WriteMessage(session.TypeUser, "**You:**\n\n"+text+"\n")
			}
		}),
		agent.WithAssistantMessageCallback(func(text string) {
			if sess != nil {
				sess.WriteMessage(session.TypeAssistant, "**Claude:**\n\n"+text+"\n")
			}
		}),
		agent.WithErrorCallback(func(err error) {
			fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
		}),
	)
	defer agentInstance.Close()

	// Restore history
	if len(history) > 0 {
		agentInstance.SetHistory(history)
	}

	// Start REPL
	sessionName := ""
	if sess != nil {
		sessionName = filepath.Base(sess.Dir)
	}
	if len(history) > 0 {
		fmt.Printf("Clyde - AI Coding Agent - Resumed session: %s\n", sessionName)
	} else {
		fmt.Println("Clyde - AI Coding Agent - Type 'exit' or 'quit' to exit")
	}
	fmt.Println("  Multiline: Ctrl+J or Alt+Enter to insert a newline,")
	fmt.Println("             or end a line with \\ to continue")
	fmt.Println("==========================================================")

	// Create rich text input reader
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
		fmt.Fprintf(os.Stderr, "Warning: Rich input unavailable (%v), using basic input\n", err)
		runREPLBasicMode(level, agentInstance, sp, cfg.ContextWindowSize, sess)
		return
	}
	defer reader.Close()

	contextPercent := -1

	for {
		gitInfo := prompt.GetGitInfo()
		fmt.Println()
		reader.SetPrompt(prompt.FormatPrompt(gitInfo, contextPercent))

		userInput, err := reader.ReadLine()
		if err != nil {
			if err == io.EOF {
				printGoodbye(sess)
				break
			}
			continue
		}

		userInput = strings.TrimSpace(userInput)
		if userInput == "" {
			continue
		}

		if userInput == "exit" || userInput == "quit" {
			printGoodbye(sess)
			break
		}

		response, _ := agentInstance.HandleMessage(userInput)

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
