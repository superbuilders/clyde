package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	// Verify all new import paths compile
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
	"github.com/this-is-alpha-iota/clyde/tools"
)

// TestARCH1_DirectoryStructure verifies the target directory layout from the
// ARCH-1 story is correctly in place.
func TestARCH1_DirectoryStructure(t *testing.T) {
	// Find the project root (we're in tests/)
	projectRoot := ".."

	// Required directories that must exist
	requiredDirs := []string{
		"cli",
		"cli/style",
		"cli/spinner",
		"cli/prompt",
		"cli/input",
		"agent",
		"agent/mcp",
		"agent/prompts",
		"agent/truncate",
		"providers",
		"loglevel",
		"config",
		"tools",
		"tests",
		"docs",
	}

	for _, dir := range requiredDirs {
		path := filepath.Join(projectRoot, dir)
		info, err := os.Stat(path)
		if err != nil {
			t.Errorf("Required directory %q does not exist: %v", dir, err)
			continue
		}
		if !info.IsDir() {
			t.Errorf("%q exists but is not a directory", dir)
		}
	}

	// Directories that must NOT exist (old layout remnants)
	removedDirs := []string{
		"errors",  // empty, should be deleted
		"api",     // renamed to providers/
	}

	for _, dir := range removedDirs {
		path := filepath.Join(projectRoot, dir)
		if _, err := os.Stat(path); err == nil {
			t.Errorf("Old directory %q should have been removed but still exists", dir)
		}
	}

	// Files that must have moved to docs/
	docsFiles := []string{
		"docs/progress.md",
		"docs/todos.md",
		"docs/whitepaper.md",
		"docs/compaction.md",
		"docs/tui.md",
		"docs/playwright-mcp.md",
	}

	for _, f := range docsFiles {
		path := filepath.Join(projectRoot, f)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("Expected file %q in docs/: %v", f, err)
		}
	}

	// Files that must NOT exist at root anymore
	movedFiles := []string{
		"progress.md",
		"todos.md",
		"whitepaper.md",
	}

	for _, f := range movedFiles {
		path := filepath.Join(projectRoot, f)
		if _, err := os.Stat(path); err == nil {
			t.Errorf("File %q should have been moved to docs/ but still exists at root", f)
		}
	}
}

// TestARCH1_MainGoThinEntrypoint verifies main.go is a thin wrapper
// that only imports cli and calls cli.Run().
func TestARCH1_MainGoThinEntrypoint(t *testing.T) {
	content, err := os.ReadFile("../main.go")
	if err != nil {
		t.Fatalf("Failed to read main.go: %v", err)
	}

	lines := strings.Split(string(content), "\n")
	// Count non-empty lines
	nonEmpty := 0
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			nonEmpty++
		}
	}

	if nonEmpty > 10 {
		t.Errorf("main.go should be ≤10 non-empty lines, got %d", nonEmpty)
	}

	s := string(content)
	if !strings.Contains(s, `"github.com/this-is-alpha-iota/clyde/cli"`) {
		t.Error("main.go should import the cli package")
	}
	if !strings.Contains(s, "cli.Run()") {
		t.Error("main.go should call cli.Run()")
	}
}

// TestARCH1_CLIPackageExists verifies cli/cli.go contains the Run() function.
func TestARCH1_CLIPackageExists(t *testing.T) {
	content, err := os.ReadFile("../cli/cli.go")
	if err != nil {
		t.Fatalf("Failed to read cli/cli.go: %v", err)
	}

	s := string(content)
	if !strings.Contains(s, "package cli") {
		t.Error("cli/cli.go should declare package cli")
	}
	if !strings.Contains(s, "func Run()") {
		t.Error("cli/cli.go should export Run() function")
	}
}

// TestARCH1_ProvidersPackage verifies the api→providers rename.
func TestARCH1_ProvidersPackage(t *testing.T) {
	// Check package declaration
	for _, f := range []string{"../providers/client.go", "../providers/types.go"} {
		content, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("Failed to read %s: %v", f, err)
		}
		if !strings.HasPrefix(string(content), "package providers") {
			t.Errorf("%s should declare package providers", f)
		}
	}
}

// TestARCH1_NoOldImportPaths verifies no Go file uses the old import paths.
// The old path suffixes are constructed dynamically to avoid false positives
// from this test file's own source code.
func TestARCH1_NoOldImportPaths(t *testing.T) {
	base := "github.com/this-is-alpha-iota/clyde/"
	// Old leaf package names that must NOT appear as direct imports.
	// Each is checked as: "clyde/<leaf>" without any further path component.
	oldLeaves := []string{
		"api",
		"style",
		"spinner",
		"prompt",
		"input",
		"mcp",
		"prompts",
		"truncate",
	}

	err := filepath.Walk("..", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() && (info.Name() == ".git" || info.Name() == ".playwright-mcp") {
			return filepath.SkipDir
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		// Skip this test file itself — it references old paths in test data
		if strings.HasSuffix(path, "arch1_test.go") {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		for _, leaf := range oldLeaves {
			// Build the exact old import string: "github.com/.../clyde/<leaf>"
			// with a closing quote to ensure we match an import, not a substring
			oldImport := `"` + base + leaf + `"`
			if strings.Contains(string(content), oldImport) {
				t.Errorf("File %s still uses old import path %s", path, oldImport)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Walk failed: %v", err)
	}
}

// TestARCH1_EmbeddedPromptWorks verifies the //go:embed in agent/prompts still works.
func TestARCH1_EmbeddedPromptWorks(t *testing.T) {
	p := prompts.SystemPrompt
	if p == "" {
		t.Fatal("SystemPrompt is empty — embed may be broken")
	}
	if !strings.Contains(p, "IMPORTANT DECIDER") {
		t.Error("SystemPrompt missing expected content — embed may be loading wrong file")
	}
}

// TestARCH1_DevModePathUpdated verifies the dev-mode file override path
// was updated from "prompts/system.txt" to "agent/prompts/system.txt".
func TestARCH1_DevModePathUpdated(t *testing.T) {
	content, err := os.ReadFile("../agent/prompts/prompts.go")
	if err != nil {
		t.Fatalf("Failed to read agent/prompts/prompts.go: %v", err)
	}

	s := string(content)
	if strings.Contains(s, `ReadFile("prompts/system.txt")`) {
		t.Error("Dev-mode path still uses old 'prompts/system.txt', should be 'agent/prompts/system.txt'")
	}
	if !strings.Contains(s, `ReadFile("agent/prompts/system.txt")`) {
		t.Error("Dev-mode path not updated to 'agent/prompts/system.txt'")
	}
}

// TestARCH1_ImportPathsCompile is a compile-time verification that all new
// import paths are valid. If any path were wrong, this file wouldn't compile.
// The function uses the imported packages to satisfy the compiler.
func TestARCH1_ImportPathsCompile(t *testing.T) {
	// Use each imported package to prove imports are valid.
	// These are all type/function assertions — zero allocations.

	_ = agent.NewAgent
	_ = mcp.NewPlaywrightServer
	_ = prompts.SystemPrompt
	_ = truncate.ThinkingLineLimit
	_ = input.ContinuationPrompt
	_ = prompt.GetGitInfo
	_ = spinner.Frames
	_ = style.IsColorEnabled
	_ = config.LoadFromFile
	_ = loglevel.Normal
	_ = providers.NewClient
	_ = tools.GetAllTools

	t.Log("All 12 new import paths compile successfully")
}

// TestARCH1_NoCircularImports verifies there are no circular import chains
// by checking that the build succeeds. If there were circular imports, the
// test file itself wouldn't compile, and go vet would fail. This test
// documents the dependency graph for future reference.
func TestARCH1_NoCircularImports(t *testing.T) {
	// The dependency graph (non-test) is:
	//
	//   main.go           → cli
	//   cli                → agent, agent/mcp, agent/prompts, cli/input,
	//                        cli/prompt, cli/spinner, cli/style, config,
	//                        loglevel, providers, tools
	//   agent              → providers, loglevel, tools, agent/truncate
	//   agent/mcp          → providers, tools
	//   agent/truncate     → loglevel
	//   cli/prompt         → cli/style
	//   tools              → providers
	//   loglevel, config, providers, cli/style, cli/spinner, cli/input,
	//   agent/prompts      → (no clyde imports)
	//
	// No circular dependencies exist. If any were introduced, this
	// test file (which imports all packages) would fail to compile.

	t.Log("No circular imports — all packages compile together")
}
