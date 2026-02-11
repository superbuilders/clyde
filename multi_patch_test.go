package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// Unit tests for executeMultiPatch
func TestExecuteMultiPatch(t *testing.T) {
	// Create a temporary test directory with git repo
	tmpDir := t.TempDir()
	oldDir, _ := os.Getwd()
	defer os.Chdir(oldDir)
	os.Chdir(tmpDir)

	// Initialize git repo
	exec.Command("git", "init").Run()
	exec.Command("git", "config", "user.email", "test@example.com").Run()
	exec.Command("git", "config", "user.name", "Test User").Run()

	t.Run("Apply single patch successfully", func(t *testing.T) {
		// Create test file
		content := "line 1\nline 2\nline 3\n"
		os.WriteFile("test1.txt", []byte(content), 0644)
		exec.Command("git", "add", ".").Run()
		exec.Command("git", "commit", "-m", "initial").Run()

		patches := []interface{}{
			map[string]interface{}{
				"path":     "test1.txt",
				"old_text": "line 2",
				"new_text": "LINE TWO",
			},
		}

		result, err := executeMultiPatch(patches)
		if err != nil {
			t.Fatalf("Expected success, got error: %v", err)
		}

		if !strings.Contains(result, "Successfully applied all 1 patches") {
			t.Errorf("Expected success message, got: %s", result)
		}

		// Verify file was changed
		newContent, _ := os.ReadFile("test1.txt")
		if !strings.Contains(string(newContent), "LINE TWO") {
			t.Errorf("Expected file to contain 'LINE TWO', got: %s", string(newContent))
		}
	})

	t.Run("Apply multiple patches successfully", func(t *testing.T) {
		// Create test files
		os.WriteFile("file1.txt", []byte("foo bar baz"), 0644)
		os.WriteFile("file2.txt", []byte("hello world"), 0644)
		exec.Command("git", "add", ".").Run()
		exec.Command("git", "commit", "-m", "multi files").Run()

		patches := []interface{}{
			map[string]interface{}{
				"path":     "file1.txt",
				"old_text": "bar",
				"new_text": "BAR",
			},
			map[string]interface{}{
				"path":     "file2.txt",
				"old_text": "world",
				"new_text": "WORLD",
			},
		}

		result, err := executeMultiPatch(patches)
		if err != nil {
			t.Fatalf("Expected success, got error: %v", err)
		}

		if !strings.Contains(result, "Successfully applied all 2 patches") {
			t.Errorf("Expected success message for 2 patches, got: %s", result)
		}

		// Verify both files were changed
		content1, _ := os.ReadFile("file1.txt")
		content2, _ := os.ReadFile("file2.txt")

		if !strings.Contains(string(content1), "BAR") {
			t.Errorf("Expected file1 to contain 'BAR', got: %s", string(content1))
		}
		if !strings.Contains(string(content2), "WORLD") {
			t.Errorf("Expected file2 to contain 'WORLD', got: %s", string(content2))
		}
	})

	t.Run("Rollback on failure", func(t *testing.T) {
		// Create test files
		os.WriteFile("rollback1.txt", []byte("alpha beta gamma"), 0644)
		os.WriteFile("rollback2.txt", []byte("one two three"), 0644)
		exec.Command("git", "add", ".").Run()
		exec.Command("git", "commit", "-m", "rollback test").Run()

		patches := []interface{}{
			map[string]interface{}{
				"path":     "rollback1.txt",
				"old_text": "beta",
				"new_text": "BETA",
			},
			map[string]interface{}{
				"path":     "rollback2.txt",
				"old_text": "NONEXISTENT", // This will fail
				"new_text": "something",
			},
		}

		_, err := executeMultiPatch(patches)
		if err == nil {
			t.Fatal("Expected error due to second patch failing")
		}

		errMsg := err.Error()
		if !strings.Contains(errMsg, "FAILED") {
			t.Errorf("Expected failure message, got: %s", errMsg)
		}

		if !strings.Contains(errMsg, "rollback") || !strings.Contains(errMsg, "Rolling back") {
			t.Errorf("Expected rollback message, got: %s", errMsg)
		}

		// Verify first file was rolled back
		content, _ := os.ReadFile("rollback1.txt")
		if strings.Contains(string(content), "BETA") {
			t.Errorf("Expected file to be rolled back to 'beta', but found 'BETA': %s", string(content))
		}
		if !strings.Contains(string(content), "beta") {
			t.Errorf("Expected original 'beta' after rollback, got: %s", string(content))
		}
	})

	t.Run("Empty patches array", func(t *testing.T) {
		patches := []interface{}{}

		_, err := executeMultiPatch(patches)
		if err == nil {
			t.Fatal("Expected error for empty patches array")
		}

		if !strings.Contains(err.Error(), "at least one patch") {
			t.Errorf("Expected 'at least one patch' error, got: %v", err)
		}
	})

	t.Run("Missing required fields", func(t *testing.T) {
		tests := []struct {
			name    string
			patch   map[string]interface{}
			errText string
		}{
			{
				name:    "missing path",
				patch:   map[string]interface{}{"old_text": "x", "new_text": "y"},
				errText: "missing 'path'",
			},
			{
				name:    "missing old_text",
				patch:   map[string]interface{}{"path": "file.txt", "new_text": "y"},
				errText: "missing 'old_text'",
			},
			{
				name:    "missing new_text",
				patch:   map[string]interface{}{"path": "file.txt", "old_text": "x"},
				errText: "missing 'new_text'",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				patches := []interface{}{tt.patch}
				_, err := executeMultiPatch(patches)
				if err == nil {
					t.Fatal("Expected error for missing field")
				}
				if !strings.Contains(err.Error(), tt.errText) {
					t.Errorf("Expected error to contain '%s', got: %v", tt.errText, err)
				}
			})
		}
	})

	t.Run("Warn about uncommitted changes", func(t *testing.T) {
		// Make uncommitted changes
		os.WriteFile("dirty.txt", []byte("uncommitted content"), 0644)

		patches := []interface{}{
			map[string]interface{}{
				"path":     "dirty.txt",
				"old_text": "uncommitted",
				"new_text": "UNCOMMITTED",
			},
		}

		result, err := executeMultiPatch(patches)
		// Should not error, but should return warning
		if err != nil {
			t.Fatalf("Expected warning, not error: %v", err)
		}

		t.Logf("Result: %s", result)

		if !strings.Contains(result, "uncommitted changes") && !strings.Contains(result, "commit") {
			t.Errorf("Expected warning about uncommitted changes, got: %s", result)
		}

		// Clean up
		exec.Command("git", "checkout", "--", "dirty.txt").Run()
		os.Remove("dirty.txt")
	})
}

// Integration test for multi_patch tool
func TestMultiPatchIntegration(t *testing.T) {
	apiKey := os.Getenv("TS_AGENT_API_KEY")
	if apiKey == "" {
		t.Skip("TS_AGENT_API_KEY not set")
	}

	// Create a temporary test directory with git repo
	tmpDir := t.TempDir()
	oldDir, _ := os.Getwd()
	defer os.Chdir(oldDir)
	os.Chdir(tmpDir)

	// Initialize git repo
	exec.Command("git", "init").Run()
	exec.Command("git", "config", "user.email", "test@example.com").Run()
	exec.Command("git", "config", "user.name", "Test User").Run()

	// Create test files
	os.WriteFile("func.go", []byte("func oldName() {\n\treturn\n}"), 0644)
	os.WriteFile("caller.go", []byte("result := oldName()"), 0644)
	exec.Command("git", "add", ".").Run()
	exec.Command("git", "commit", "-m", "initial").Run()

	t.Run("Coordinated multi-file refactor", func(t *testing.T) {
		var history []Message

		// Ask to rename function across files
		response, updatedHistory := handleConversation(apiKey,
			"Use multi_patch to rename 'oldName' to 'newName' in both func.go and caller.go",
			history)

		if response == "" {
			t.Fatal("Expected response but got empty string")
		}

		t.Logf("Response: %s", response)

		// Verify multi_patch was used
		foundMultiPatch := false
		foundToolResult := false
		var toolResultContent string

		for _, msg := range updatedHistory {
			if msg.Role == "assistant" {
				if contentBlocks, ok := msg.Content.([]ContentBlock); ok {
					for _, block := range contentBlocks {
						if block.Type == "tool_use" && block.Name == "multi_patch" {
							foundMultiPatch = true
							t.Logf("✓ multi_patch tool was used")

							// Verify patches structure
							if patches, ok := block.Input["patches"].([]interface{}); ok {
								t.Logf("✓ Found %d patches in input", len(patches))
								if len(patches) >= 2 {
									t.Logf("✓ Multiple patches provided")
								}
							}
						}
					}
				}
			}
			if msg.Role == "user" {
				if contentBlocks, ok := msg.Content.([]ContentBlock); ok {
					for _, block := range contentBlocks {
						if block.Type == "tool_result" {
							foundToolResult = true
							if content, ok := block.Content.(string); ok {
								toolResultContent = content
								t.Logf("Tool result preview: %s", content[:min(200, len(content))])
							}
						}
					}
				}
			}
		}

		if !foundMultiPatch {
			t.Error("Expected multi_patch tool to be used")
		}

		if !foundToolResult {
			t.Error("Expected to find a tool_result block")
		}

		// Check if files were actually modified (if patches succeeded)
		if strings.Contains(toolResultContent, "Successfully applied") {
			funcContent, _ := os.ReadFile("func.go")
			callerContent, _ := os.ReadFile("caller.go")

			if strings.Contains(string(funcContent), "newName") {
				t.Logf("✓ func.go was updated with newName")
			}
			if strings.Contains(string(callerContent), "newName") {
				t.Logf("✓ caller.go was updated with newName")
			}
		}
	})

	t.Run("Handle uncommitted changes warning", func(t *testing.T) {
		// Make uncommitted changes
		os.WriteFile("uncommitted.txt", []byte("test content"), 0644)

		var history []Message

		response, updatedHistory := handleConversation(apiKey,
			"Use multi_patch to change 'test' to 'TEST' in uncommitted.txt",
			history)

		if response == "" {
			t.Fatal("Expected response but got empty string")
		}

		t.Logf("Response: %s", response)

		// Look for warning about uncommitted changes in tool result
		for _, msg := range updatedHistory {
			if msg.Role == "user" {
				if contentBlocks, ok := msg.Content.([]ContentBlock); ok {
					for _, block := range contentBlocks {
						if block.Type == "tool_result" {
							if content, ok := block.Content.(string); ok {
								if strings.Contains(content, "uncommitted") || strings.Contains(content, "commit") {
									t.Logf("✓ Warning about uncommitted changes detected")
								}
							}
						}
					}
				}
			}
		}

		// Clean up
		os.Remove("uncommitted.txt")
	})
}
