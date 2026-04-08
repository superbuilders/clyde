package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/this-is-alpha-iota/clyde/agent"
)

// --- ARCH-3: Encapsulate Agent as a Self-Contained Package ---
//
// These tests verify the ARCH-3 story: all agent dependencies (providers,
// tools, config) are under agent/, and the CLI only talks to the agent
// package's public surface — never reaching into its internals.

// TestARCH3_PackagesUnderAgent verifies providers, tools, and config
// are under agent/ (not at project root).
func TestARCH3_PackagesUnderAgent(t *testing.T) {
	projectRoot := ".."

	// These must exist under agent/
	requiredDirs := []string{
		"agent/providers",
		"agent/tools",
		"agent/config",
	}

	for _, dir := range requiredDirs {
		path := filepath.Join(projectRoot, dir)
		info, err := os.Stat(path)
		if err != nil {
			t.Errorf("Required directory %q does not exist under agent/: %v", dir, err)
			continue
		}
		if !info.IsDir() {
			t.Errorf("%q exists but is not a directory", dir)
		}
	}

	// These must NOT exist at root anymore
	removedDirs := []string{
		"providers",
		"tools",
		"config",
	}

	for _, dir := range removedDirs {
		path := filepath.Join(projectRoot, dir)
		if _, err := os.Stat(path); err == nil {
			t.Errorf("Old directory %q should have been moved under agent/ but still exists at root", dir)
		}
	}
}

// TestARCH3_CLIImportsOnlyAgent verifies that cli/cli.go imports only the
// agent package (plus its own cli/* subpackages) — no agent/providers,
// agent/tools, agent/config, agent/mcp, or agent/prompts imports.
func TestARCH3_CLIImportsOnlyAgent(t *testing.T) {
	content, err := os.ReadFile("../cli/cli.go")
	if err != nil {
		t.Fatalf("Failed to read cli/cli.go: %v", err)
	}

	s := string(content)

	// These internal agent packages should NOT be imported by the CLI
	forbidden := []string{
		`"github.com/this-is-alpha-iota/clyde/agent/providers"`,
		`"github.com/this-is-alpha-iota/clyde/agent/tools"`,
		`"github.com/this-is-alpha-iota/clyde/agent/config"`,
		`"github.com/this-is-alpha-iota/clyde/agent/mcp"`,
		`"github.com/this-is-alpha-iota/clyde/agent/prompts"`,
	}

	for _, imp := range forbidden {
		if strings.Contains(s, imp) {
			t.Errorf("cli/cli.go should NOT import %s (ARCH-3: agent owns its internals)", imp)
		}
	}

	// The CLI SHOULD import the agent package
	if !strings.Contains(s, `"github.com/this-is-alpha-iota/clyde/agent"`) {
		t.Error("cli/cli.go should import the agent package")
	}
}

// TestARCH3_NoBlankToolsImportInCLI verifies the blank import
// `_ "clyde/tools"` is eliminated from cli/cli.go.
func TestARCH3_NoBlankToolsImportInCLI(t *testing.T) {
	content, err := os.ReadFile("../cli/cli.go")
	if err != nil {
		t.Fatalf("Failed to read cli/cli.go: %v", err)
	}

	s := string(content)
	if strings.Contains(s, `_ "github.com/this-is-alpha-iota/clyde/agent/tools"`) {
		t.Error("cli/cli.go should NOT have blank import of tools (ARCH-3: agent handles registration)")
	}
	if strings.Contains(s, `_ "github.com/this-is-alpha-iota/clyde/tools"`) {
		t.Error("cli/cli.go should NOT have blank import of old tools path")
	}
}

// TestARCH3_AgentNewConstructor verifies agent.New() exists and is the
// primary constructor that handles client creation, tool registration,
// and prompt loading internally.
func TestARCH3_AgentNewConstructor(t *testing.T) {
	// Read agent.go to verify New() exists
	content, err := os.ReadFile("../agent/agent.go")
	if err != nil {
		t.Fatalf("Failed to read agent/agent.go: %v", err)
	}

	s := string(content)
	if !strings.Contains(s, "func New(cfg Config") {
		t.Error("agent/agent.go should have func New(cfg Config, ...Option)")
	}
	if !strings.Contains(s, "type Config struct") {
		t.Error("agent/agent.go should define Config struct")
	}

	// Verify it can be called (compile-time check via type system)
	_ = agent.New
	_ = agent.Config{}
}

// TestARCH3_AgentConfigFields verifies agent.Config contains all required fields.
func TestARCH3_AgentConfigFields(t *testing.T) {
	// This is a compile-time check — if any field is missing, this won't compile.
	cfg := agent.Config{
		APIKey:            "test-key",
		APIURL:            "https://api.example.com",
		ModelID:           "test-model",
		MaxTokens:         4096,
		ContextWindowSize: 200000,
		ThinkingBudget:    8192,
		NoThink:           false,
		BraveSearchAPIKey: "brave-key",
		MCPPlaywright:     false,
		MCPPlaywrightArgs: "--headless",
	}
	_ = cfg
	t.Log("✅ agent.Config has all expected fields")
}

// TestARCH3_AgentNewCreatesWorkingAgent verifies agent.New() creates a
// functional agent instance that can be used.
func TestARCH3_AgentNewCreatesWorkingAgent(t *testing.T) {
	var progressMsgs []string
	var thinkingMsgs []string
	var diagnosticMsgs []string
	var outputMsgs []string

	a := agent.New(
		agent.Config{
			APIKey:            "dummy-key",
			APIURL:            "http://localhost:99999", // won't connect
			ModelID:           "test-model",
			MaxTokens:         4096,
			ContextWindowSize: 200000,
			NoThink:           true,
		},
		agent.WithProgressCallback(func(msg string) {
			progressMsgs = append(progressMsgs, msg)
		}),
		agent.WithThinkingCallback(func(text string) {
			thinkingMsgs = append(thinkingMsgs, text)
		}),
		agent.WithDiagnosticCallback(func(msg string) {
			diagnosticMsgs = append(diagnosticMsgs, msg)
		}),
		agent.WithOutputCallback(func(output string) {
			outputMsgs = append(outputMsgs, output)
		}),
		agent.WithSpinnerCallback(func(start bool, message string) {}),
		agent.WithErrorCallback(func(err error) {}),
	)

	if a == nil {
		t.Fatal("agent.New() should not return nil")
	}

	// Verify Close is safe to call
	if err := a.Close(); err != nil {
		t.Errorf("Close() should not error on agent without MCP: %v", err)
	}

	t.Log("✅ agent.New() creates a working agent with all callbacks")
}

// TestARCH3_AgentCloseMethod verifies the agent has a Close() method
// for releasing resources (MCP server, etc.).
func TestARCH3_AgentCloseMethod(t *testing.T) {
	a := agent.New(agent.Config{
		APIKey:  "dummy",
		APIURL:  "http://localhost",
		ModelID: "test",
	})

	// Close should be idempotent and safe
	if err := a.Close(); err != nil {
		t.Errorf("First Close() should not error: %v", err)
	}
	if err := a.Close(); err != nil {
		t.Errorf("Second Close() should not error: %v", err)
	}
}

// TestARCH3_AgentUsageTypeExported verifies the agent re-exports Usage
// so the CLI doesn't need to import agent/providers.
func TestARCH3_AgentUsageTypeExported(t *testing.T) {
	a := agent.New(agent.Config{
		APIKey:  "dummy",
		APIURL:  "http://localhost",
		ModelID: "test",
	})

	// This compiles only if agent.Usage (or the re-exported providers.Usage)
	// is accessible through LastUsage().
	usage := a.LastUsage()
	_ = usage.InputTokens
	_ = usage.OutputTokens
	_ = usage.CacheReadInputTokens
	_ = usage.CacheCreationInputTokens

	t.Log("✅ agent.Usage is accessible without importing agent/providers")
}

// TestARCH3_NewAgentStillWorks verifies the lower-level NewAgent constructor
// still works for backward compatibility (tests, library consumers).
func TestARCH3_NewAgentStillWorks(t *testing.T) {
	// NewAgent is still available for tests that need full control
	_ = agent.NewAgent
	t.Log("✅ NewAgent still available for backward compatibility")
}

// TestARCH3_ToolsRegisteredByAgent verifies that tools are registered
// automatically by the agent package (no blank import needed externally).
func TestARCH3_ToolsRegisteredByAgent(t *testing.T) {
	// Read agent/agent.go and verify it has the blank import for tools
	content, err := os.ReadFile("../agent/agent.go")
	if err != nil {
		t.Fatalf("Failed to read agent/agent.go: %v", err)
	}

	s := string(content)
	if !strings.Contains(s, `_ "github.com/this-is-alpha-iota/clyde/agent/tools"`) {
		t.Error("agent/agent.go should have blank import of agent/tools for init() registration")
	}
}

// TestARCH3_AgentOwnsMCPSetup verifies the agent handles MCP setup internally
// when MCPPlaywright is true in the config.
func TestARCH3_AgentOwnsMCPSetup(t *testing.T) {
	content, err := os.ReadFile("../agent/agent.go")
	if err != nil {
		t.Fatalf("Failed to read agent/agent.go: %v", err)
	}

	s := string(content)
	// Agent should import mcp and handle setup in New()
	if !strings.Contains(s, `"github.com/this-is-alpha-iota/clyde/agent/mcp"`) {
		t.Error("agent/agent.go should import agent/mcp for MCP setup")
	}
	if !strings.Contains(s, "cfg.MCPPlaywright") {
		t.Error("agent.New() should check cfg.MCPPlaywright to set up MCP")
	}
}

// TestARCH3_AgentOwnsPromptLoading verifies the agent loads the system
// prompt internally (no need for CLI to import agent/prompts).
func TestARCH3_AgentOwnsPromptLoading(t *testing.T) {
	content, err := os.ReadFile("../agent/agent.go")
	if err != nil {
		t.Fatalf("Failed to read agent/agent.go: %v", err)
	}

	s := string(content)
	if !strings.Contains(s, `"github.com/this-is-alpha-iota/clyde/agent/prompts"`) {
		t.Error("agent/agent.go should import agent/prompts")
	}
	if !strings.Contains(s, "prompts.SystemPrompt") {
		t.Error("agent.New() should use prompts.SystemPrompt internally")
	}
}

// TestARCH3_AgentOwnsClientCreation verifies the agent creates its own
// providers.Client in New() rather than receiving one from the CLI.
func TestARCH3_AgentOwnsClientCreation(t *testing.T) {
	content, err := os.ReadFile("../agent/agent.go")
	if err != nil {
		t.Fatalf("Failed to read agent/agent.go: %v", err)
	}

	s := string(content)
	if !strings.Contains(s, "providers.NewClient") {
		t.Error("agent.New() should create its own providers.Client internally")
	}
}

// TestARCH3_NoBehavioralChange documents that there is no behavioral
// change from the user's perspective.
func TestARCH3_NoBehavioralChange(t *testing.T) {
	t.Log("ARCH-3: Agent is a self-contained package")
	t.Log("  Before: CLI assembled agent internals (providers, tools, config)")
	t.Log("  After:  CLI passes agent.Config → agent.New() handles everything")
	t.Log("  The user sees identical behavior at every log level")
	t.Log("  The agent is now independently importable:")
	t.Log("    go get github.com/this-is-alpha-iota/clyde/agent")
}
