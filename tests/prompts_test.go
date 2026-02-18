package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/this-is-alpha-iota/clyde/prompts"
)

func TestSystemPromptLoading(t *testing.T) {
	// Get the embedded prompt
	embeddedPrompt := prompts.SystemPrompt

	// Verify it's not empty
	if embeddedPrompt == "" {
		t.Fatal("System prompt is empty")
	}

	// Verify it contains expected content
	requiredContent := []string{
		"list_files",
		"read_file",
		"patch_file",
		"write_file",
		"run_bash",
		"grep",
		"glob",
		"multi_patch",
		"web_search",
		"browse",
		"IMPORTANT DECIDER",
		"DOCUMENTATION & MEMORY",
	}

	for _, required := range requiredContent {
		if !strings.Contains(embeddedPrompt, required) {
			t.Errorf("System prompt missing required content: %s", required)
		}
	}
}

func TestSystemPromptDevelopmentMode(t *testing.T) {
	// This test verifies that if prompts/system.txt exists,
	// it will be loaded instead of the embedded version.
	// We can't actually test this in the test environment because
	// the file already exists, but we can verify the function works.

	// Verify we can get the prompt
	prompt := prompts.GetSystemPrompt()
	if prompt == "" {
		t.Fatal("GetSystemPrompt returned empty string")
	}

	// Verify it contains key sections
	if !strings.Contains(prompt, "IMPORTANT DECIDER") {
		t.Error("Prompt missing IMPORTANT DECIDER section")
	}
}

func TestSystemPromptProductionMode(t *testing.T) {
	// Save current directory
	originalWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(originalWd)

	// Create a temporary directory without prompts/system.txt
	tempDir := t.TempDir()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatal(err)
	}

	// Get the prompt - should use embedded version
	prompt := prompts.GetSystemPrompt()
	if prompt == "" {
		t.Fatal("GetSystemPrompt returned empty string in production mode")
	}

	// Should still contain all expected content
	if !strings.Contains(prompt, "IMPORTANT DECIDER") {
		t.Error("Embedded prompt missing IMPORTANT DECIDER section")
	}
	if !strings.Contains(prompt, "grep") {
		t.Error("Embedded prompt missing grep tool")
	}
}

func TestSystemPromptFileOverride(t *testing.T) {
	// Save current directory
	originalWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(originalWd)

	// Create a temporary directory with custom system.txt
	tempDir := t.TempDir()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatal(err)
	}

	// Create prompts directory and custom file
	if err := os.MkdirAll("prompts", 0755); err != nil {
		t.Fatal(err)
	}

	customContent := "This is a custom system prompt for testing"
	customPath := filepath.Join("prompts", "system.txt")
	if err := os.WriteFile(customPath, []byte(customContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Get the prompt - should load from file
	prompt := prompts.GetSystemPrompt()
	if prompt != customContent {
		t.Errorf("Expected custom content, got: %s", prompt)
	}
}

func TestSystemPromptNotEmpty(t *testing.T) {
	// Verify the SystemPrompt variable is initialized
	if prompts.SystemPrompt == "" {
		t.Fatal("SystemPrompt variable is empty")
	}

	// Verify minimum length (should be several KB)
	if len(prompts.SystemPrompt) < 1000 {
		t.Errorf("SystemPrompt seems too short: %d bytes", len(prompts.SystemPrompt))
	}
}
