package main

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// TestExecuteGitHubCommand removed - github_query tool deprecated in favor of run_bash

func TestExecuteListFiles(t *testing.T) {
	tests := []struct {
		name        string
		path        string
		expectError bool
	}{
		{
			name:        "List current directory",
			path:        ".",
			expectError: false,
		},
		{
			name:        "List with empty path (defaults to current)",
			path:        "",
			expectError: false,
		},
		{
			name:        "List non-existent directory",
			path:        "/non/existent/path/xyz",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := executeListFiles(tt.path)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none. Output: %s", output)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if output == "" {
					t.Error("Expected output but got empty string")
				}
			}
		})
	}
}

func TestExecuteReadFile(t *testing.T) {
	// Create a test file
	testFile := "test_file.txt"
	testContent := "Test content for file reading"
	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	defer os.Remove(testFile)

	tests := []struct {
		name        string
		path        string
		expectError bool
		checkContent bool
	}{
		{
			name:        "Read existing file",
			path:        testFile,
			expectError: false,
			checkContent: true,
		},
		{
			name:        "Read non-existent file",
			path:        "non_existent_file.txt",
			expectError: true,
			checkContent: false,
		},
		{
			name:        "Empty path",
			path:        "",
			expectError: true,
			checkContent: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := executeReadFile(tt.path)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none. Output: %s", output)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if tt.checkContent && output != testContent {
					t.Errorf("Expected content '%s', got '%s'", testContent, output)
				}
			}
		})
	}
}

func TestExecuteRunBash(t *testing.T) {
	tests := []struct {
		name        string
		command     string
		expectError bool
		checkOutput bool
		expectedOut string
	}{
		{
			name:        "Simple echo command",
			command:     "echo 'Hello, World!'",
			expectError: false,
			checkOutput: true,
			expectedOut: "Hello, World!\n",
		},
		{
			name:        "Command with output",
			command:     "ls -la . | head -1",
			expectError: false,
			checkOutput: false, // Just verify it runs
		},
		{
			name:        "Empty command",
			command:     "",
			expectError: true,
		},
		{
			name:        "Invalid command",
			command:     "nonexistent-command-xyz",
			expectError: true,
		},
		{
			name:        "Command that exits with error",
			command:     "exit 1",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := executeRunBash(tt.command)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none. Output: %s", output)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if tt.checkOutput && output != tt.expectedOut {
					t.Errorf("Expected output '%s', got '%s'", tt.expectedOut, output)
				}
			}
		})
	}
}

func TestExecuteWriteFile(t *testing.T) {
	testFile := "test_write_file.txt"
	defer os.Remove(testFile)

	tests := []struct {
		name        string
		path        string
		content     string
		expectError bool
		setupFile   bool
		setupContent string
	}{
		{
			name:        "Create new file",
			path:        testFile,
			content:     "Hello, World!",
			expectError: false,
			setupFile:   false,
		},
		{
			name:        "Replace existing file",
			path:        testFile,
			content:     "New content",
			expectError: false,
			setupFile:   true,
			setupContent: "Old content",
		},
		{
			name:        "Empty path",
			path:        "",
			content:     "Some content",
			expectError: true,
		},
		{
			name:        "Write empty content",
			path:        testFile,
			content:     "",
			expectError: false,
		},
		{
			name:        "Write multiline content",
			path:        testFile,
			content:     "Line 1\nLine 2\nLine 3",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clean up before each test
			os.Remove(testFile)

			// Setup existing file if needed
			if tt.setupFile {
				if err := os.WriteFile(tt.path, []byte(tt.setupContent), 0644); err != nil {
					t.Fatalf("Failed to setup test file: %v", err)
				}
			}

			output, err := executeWriteFile(tt.path, tt.content)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none. Output: %s", output)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}

				// Verify file content
				if tt.path != "" {
					content, readErr := os.ReadFile(tt.path)
					if readErr != nil {
						t.Errorf("Failed to read written file: %v", readErr)
					} else if string(content) != tt.content {
						t.Errorf("File content mismatch. Expected '%s', got '%s'", tt.content, string(content))
					}
				}

				// Verify output message
				if tt.setupFile {
					if !strings.Contains(output, "replaced") {
						t.Errorf("Expected 'replaced' in output for existing file, got: %s", output)
					}
				} else if tt.path != "" {
					if !strings.Contains(output, "created") {
						t.Errorf("Expected 'created' in output for new file, got: %s", output)
					}
				}
			}
		})
	}
}

func TestExecuteGrep(t *testing.T) {
	// Create test files for grep
	testDir := "test_grep_dir"
	if err := os.MkdirAll(testDir, 0755); err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}
	defer os.RemoveAll(testDir)

	// Create test files with various content
	testFiles := map[string]string{
		"test1.go": `package main
func main() {
	// TODO: implement feature
	fmt.Println("Hello")
}`,
		"test2.go": `package main
func helper() {
	// TODO: fix bug
	return
}`,
		"test.txt": `This is a text file
TODO: write documentation
No code here`,
		"test.md": `# README
This is markdown
func notRealCode()`,
	}

	for filename, content := range testFiles {
		path := testDir + "/" + filename
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to create test file %s: %v", filename, err)
		}
	}

	tests := []struct {
		name        string
		pattern     string
		path        string
		filePattern string
		expectError bool
		checkOutput bool
		shouldMatch bool
		minMatches  int
	}{
		{
			name:        "Search for TODO in all files",
			pattern:     "TODO",
			path:        testDir,
			filePattern: "",
			expectError: false,
			checkOutput: true,
			shouldMatch: true,
			minMatches:  3, // Should find 3 TODOs
		},
		{
			name:        "Search for TODO only in .go files",
			pattern:     "TODO",
			path:        testDir,
			filePattern: "*.go",
			expectError: false,
			checkOutput: true,
			shouldMatch: true,
			minMatches:  2, // Should find 2 TODOs in .go files
		},
		{
			name:        "Search for func in .go files",
			pattern:     "func",
			path:        testDir,
			filePattern: "*.go",
			expectError: false,
			checkOutput: true,
			shouldMatch: true,
			minMatches:  2,
		},
		{
			name:        "Search for non-existent pattern",
			pattern:     "NONEXISTENT_PATTERN_XYZ",
			path:        testDir,
			filePattern: "",
			expectError: false, // grep returns no error for no matches
			checkOutput: true,
			shouldMatch: false,
		},
		{
			name:        "Search in non-existent directory",
			pattern:     "TODO",
			path:        "/nonexistent/path/xyz",
			filePattern: "",
			expectError: true,
		},
		{
			name:        "Empty pattern",
			pattern:     "",
			path:        testDir,
			filePattern: "",
			expectError: true,
		},
		{
			name:        "Search in current directory (empty path)",
			pattern:     "func TestExecuteGrep",
			path:        "",
			filePattern: "*.go",
			expectError: false,
			checkOutput: true,
			shouldMatch: true,
			minMatches:  1, // Should find this function definition
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := executeGrep(tt.pattern, tt.path, tt.filePattern)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none. Output: %s", output)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}

				if tt.checkOutput {
					if tt.shouldMatch {
						// Check if output indicates matches were found
						if !strings.Contains(output, "Found") && !strings.Contains(output, ":") {
							t.Errorf("Expected matches but output suggests none: %s", output)
						}

						// Count matches if specified
						if tt.minMatches > 0 {
							lines := strings.Split(output, "\n")
							matchCount := 0
							for _, line := range lines {
								if strings.Contains(line, ":") && !strings.HasPrefix(line, "Found") {
									matchCount++
								}
							}
							if matchCount < tt.minMatches {
								t.Errorf("Expected at least %d matches, got %d. Output:\n%s", 
									tt.minMatches, matchCount, output)
							}
						}
					} else {
						// Should indicate no matches
						if !strings.Contains(output, "No matches") && !strings.Contains(output, "found") {
							t.Errorf("Expected no matches indication but got: %s", output)
						}
					}
				}
			}
		})
	}
}

func TestExecutePatchFile(t *testing.T) {
	// Create a test file
	testFile := "test_patch.txt"
	initialContent := `Line 1: Hello World
Line 2: This is a test
Line 3: End of file`
	if err := os.WriteFile(testFile, []byte(initialContent), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	defer os.Remove(testFile)

	tests := []struct {
		name        string
		path        string
		oldText     string
		newText     string
		expectError bool
		errorMsg    string
		verify      bool
		expectedContent string
	}{
		{
			name:        "Replace unique text",
			path:        testFile,
			oldText:     "This is a test",
			newText:     "This is MODIFIED",
			expectError: false,
			verify:      true,
			expectedContent: `Line 1: Hello World
Line 2: This is MODIFIED
Line 3: End of file`,
		},
		{
			name:        "Old text not found",
			path:        testFile,
			oldText:     "Nonexistent text",
			newText:     "Something",
			expectError: true,
			errorMsg:    "not found",
		},
		{
			name:        "Non-unique old text",
			path:        testFile,
			oldText:     "Line",
			newText:     "Row",
			expectError: true,
			errorMsg:    "appears",
		},
		{
			name:        "Empty old text",
			path:        testFile,
			oldText:     "",
			newText:     "Something",
			expectError: true,
			errorMsg:    "cannot be empty",
		},
		{
			name:        "Delete text (empty new_text)",
			path:        testFile,
			oldText:     "Line 2: This is a test\n",
			newText:     "",
			expectError: false,
			verify:      true,
			expectedContent: `Line 1: Hello World
Line 3: End of file`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset file content before each test
			if err := os.WriteFile(testFile, []byte(initialContent), 0644); err != nil {
				t.Fatalf("Failed to reset file: %v", err)
			}

			output, err := executePatchFile(tt.path, tt.oldText, tt.newText)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none. Output: %s", output)
				} else if tt.errorMsg != "" && !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("Expected error to contain '%s', got: %v", tt.errorMsg, err)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}

				if tt.verify {
					content, readErr := os.ReadFile(tt.path)
					if readErr != nil {
						t.Errorf("Failed to read patched file: %v", readErr)
					} else if string(content) != tt.expectedContent {
						t.Errorf("File content mismatch.\nExpected:\n%s\n\nGot:\n%s", tt.expectedContent, string(content))
					}
				}
			}
		})
	}
}

func TestExecuteEditFile(t *testing.T) {
	t.Skip("DEPRECATED: edit_file tool replaced with patch_file")
}

func TestCallClaude(t *testing.T) {
	envPath := os.Getenv("ENV_PATH")
	if envPath == "" {
		if _, err := os.Stat(".env"); err == nil {
			envPath = ".env"
		} else {
			envPath = "../coding-agent/.env"
		}
	}

	data, err := os.ReadFile(envPath)
	if err != nil {
		t.Skipf("Skipping test: cannot read .env file: %v", err)
	}

	var apiKey string
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "TS_AGENT_API_KEY=") {
			apiKey = strings.TrimPrefix(line, "TS_AGENT_API_KEY=")
			apiKey = strings.TrimSpace(apiKey)
			break
		}
	}

	if apiKey == "" {
		t.Skip("Skipping test: TS_AGENT_API_KEY not found in .env file")
	}

	tests := []struct {
		name    string
		message string
	}{
		{
			name:    "Simple greeting",
			message: "Say hello in 5 words or less",
		},
		{
			name:    "Math question",
			message: "What is 2+2? Answer with just the number.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			messages := []Message{
				{
					Role:    "user",
					Content: tt.message,
				},
			}

			resp, err := callClaude(apiKey, messages)
			if err != nil {
				t.Fatalf("callClaude failed: %v", err)
			}

			if resp == nil {
				t.Fatal("Expected response but got nil")
			}

			if len(resp.Content) == 0 {
				t.Fatal("Expected content in response but got empty array")
			}

			hasText := false
			for _, block := range resp.Content {
				if block.Type == "text" && block.Text != "" {
					hasText = true
					t.Logf("Response: %s", block.Text)
					break
				}
			}

			if !hasText {
				t.Error("Expected text response but found none")
			}
		})
	}
}

func TestHandleConversation(t *testing.T) {
	envPath := os.Getenv("ENV_PATH")
	if envPath == "" {
		if _, err := os.Stat(".env"); err == nil {
			envPath = ".env"
		} else {
			envPath = "../coding-agent/.env"
		}
	}

	data, err := os.ReadFile(envPath)
	if err != nil {
		t.Skipf("Skipping test: cannot read .env file: %v", err)
	}

	var apiKey string
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "TS_AGENT_API_KEY=") {
			apiKey = strings.TrimPrefix(line, "TS_AGENT_API_KEY=")
			apiKey = strings.TrimSpace(apiKey)
			break
		}
	}

	if apiKey == "" {
		t.Skip("Skipping test: TS_AGENT_API_KEY not found in .env file")
	}

	t.Run("Simple conversation", func(t *testing.T) {
		var history []Message
		response, updatedHistory := handleConversation(apiKey, "Hello! Respond with just 'Hi'", history)

		if response == "" {
			t.Fatal("Expected response but got empty string")
		}

		if len(updatedHistory) == 0 {
			t.Fatal("Expected conversation history to be updated")
		}

		t.Logf("Response: %s", response)
		t.Logf("History length: %d", len(updatedHistory))
	})

	t.Run("Multi-turn conversation", func(t *testing.T) {
		var history []Message

		response1, history := handleConversation(apiKey, "Remember the number 42", history)
		if response1 == "" {
			t.Fatal("Expected first response")
		}
		t.Logf("First response: %s", response1)

		response2, history := handleConversation(apiKey, "What number did I ask you to remember?", history)
		if response2 == "" {
			t.Fatal("Expected second response")
		}
		t.Logf("Second response: %s", response2)

		if !strings.Contains(response2, "42") {
			t.Logf("Warning: Expected '42' in response, but it may have been phrased differently")
		}

		if len(history) < 4 {
			t.Errorf("Expected at least 4 messages in history, got %d", len(history))
		}
	})
}

func TestSystemPromptDecider(t *testing.T) {
	if systemPrompt == "" {
		t.Fatal("System prompt is empty")
	}

	// Check for run_bash with gh CLI instead of deprecated github_query
	requiredTerms := []string{"run_bash", "tool", "gh repo list", "gh pr list"}
	for _, term := range requiredTerms {
		if !strings.Contains(systemPrompt, term) {
			t.Errorf("System prompt should contain '%s'", term)
		}
	}

	// Make sure old github_query tool is NOT present
	if strings.Contains(systemPrompt, "github_query") {
		t.Error("System prompt should NOT contain 'github_query' (deprecated tool)")
	}

	t.Logf("System prompt length: %d characters", len(systemPrompt))
}

// TestGitHubTool removed - github_query tool deprecated in favor of run_bash

func TestListFilesIntegration(t *testing.T) {
	envPath := os.Getenv("ENV_PATH")
	if envPath == "" {
		if _, err := os.Stat(".env"); err == nil {
			envPath = ".env"
		} else {
			envPath = "../coding-agent/.env"
		}
	}

	data, err := os.ReadFile(envPath)
	if err != nil {
		t.Skipf("Skipping test: cannot read .env file: %v", err)
	}

	var apiKey string
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "TS_AGENT_API_KEY=") {
			apiKey = strings.TrimPrefix(line, "TS_AGENT_API_KEY=")
			apiKey = strings.TrimSpace(apiKey)
			break
		}
	}

	if apiKey == "" {
		t.Skip("Skipping test: TS_AGENT_API_KEY not found in .env file")
	}

	t.Run("Full list_files tool use round-trip", func(t *testing.T) {
		var history []Message

		// Ask a question that should trigger the list_files tool
		response, updatedHistory := handleConversation(apiKey, "What files are in the current directory? Use the list_files tool.", history)

		if response == "" {
			t.Fatal("Expected response but got empty string")
		}

		t.Logf("Response: %s", response)
		t.Logf("History length: %d", len(updatedHistory))

		if len(updatedHistory) < 3 {
			t.Errorf("Expected at least 3 messages in history, got %d", len(updatedHistory))
		}

		// Look for tool_use and tool_result in the conversation history
		foundToolUse := false
		foundToolResult := false

		for _, msg := range updatedHistory {
			if msg.Role == "assistant" {
				if contentBlocks, ok := msg.Content.([]ContentBlock); ok {
					for _, block := range contentBlocks {
						if block.Type == "tool_use" && block.Name == "list_files" {
							foundToolUse = true
							if block.ID == "" {
								t.Error("Tool use block should have an ID")
							}
							t.Logf("Found tool_use: %s (ID: %s)", block.Name, block.ID)
						}
					}
				}
			}

			if msg.Role == "user" {
				if contentBlocks, ok := msg.Content.([]ContentBlock); ok {
					for _, block := range contentBlocks {
						if block.Type == "tool_result" {
							foundToolResult = true
							if block.ToolUseID == "" {
								t.Error("Tool result block should have a ToolUseID")
							}
							t.Logf("Found tool_result with ToolUseID: %s", block.ToolUseID)
						}
					}
				}
			}
		}

		if !foundToolUse {
			t.Error("Expected to find a list_files tool_use block in the conversation history")
		}

		if !foundToolResult {
			t.Error("Expected to find a tool_result block in the conversation history")
		}
	})
}

func TestReadFileIntegration(t *testing.T) {
	envPath := os.Getenv("ENV_PATH")
	if envPath == "" {
		if _, err := os.Stat(".env"); err == nil {
			envPath = ".env"
		} else {
			envPath = "../coding-agent/.env"
		}
	}

	data, err := os.ReadFile(envPath)
	if err != nil {
		t.Skipf("Skipping test: cannot read .env file: %v", err)
	}

	var apiKey string
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "TS_AGENT_API_KEY=") {
			apiKey = strings.TrimPrefix(line, "TS_AGENT_API_KEY=")
			apiKey = strings.TrimSpace(apiKey)
			break
		}
	}

	if apiKey == "" {
		t.Skip("Skipping test: TS_AGENT_API_KEY not found in .env file")
	}

	// Create a test file for reading
	testFile := "test_read_file.txt"
	testContent := "Hello, this is a test file for the read_file tool!"
	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	defer os.Remove(testFile)

	t.Run("Full read_file tool use round-trip", func(t *testing.T) {
		var history []Message

		// Ask a question that should trigger the read_file tool
		response, updatedHistory := handleConversation(apiKey, "Read the file test_read_file.txt using the read_file tool.", history)

		if response == "" {
			t.Fatal("Expected response but got empty string")
		}

		t.Logf("Response: %s", response)
		t.Logf("History length: %d", len(updatedHistory))

		if len(updatedHistory) < 3 {
			t.Errorf("Expected at least 3 messages in history, got %d", len(updatedHistory))
		}

		// Look for tool_use and tool_result in the conversation history
		foundToolUse := false
		foundToolResult := false
		var toolResultContent string

		for _, msg := range updatedHistory {
			if msg.Role == "assistant" {
				if contentBlocks, ok := msg.Content.([]ContentBlock); ok {
					for _, block := range contentBlocks {
						if block.Type == "tool_use" && block.Name == "read_file" {
							foundToolUse = true
							if block.ID == "" {
								t.Error("Tool use block should have an ID")
							}
							t.Logf("Found tool_use: %s (ID: %s)", block.Name, block.ID)
						}
					}
				}
			}

			if msg.Role == "user" {
				if contentBlocks, ok := msg.Content.([]ContentBlock); ok {
					for _, block := range contentBlocks {
						if block.Type == "tool_result" {
							foundToolResult = true
							if block.ToolUseID == "" {
								t.Error("Tool result block should have a ToolUseID")
							}
							t.Logf("Found tool_result with ToolUseID: %s", block.ToolUseID)
							if content, ok := block.Content.(string); ok {
								toolResultContent = content
							}
						}
					}
				}
			}
		}

		if !foundToolUse {
			t.Error("Expected to find a read_file tool_use block in the conversation history")
		}

		if !foundToolResult {
			t.Error("Expected to find a tool_result block in the conversation history")
		}

		// Verify the tool result contains the expected file content
		if !strings.Contains(toolResultContent, testContent) {
			t.Errorf("Expected tool result to contain '%s', but got: %s", testContent, toolResultContent)
		}
	})
}

func TestEditFileIntegration(t *testing.T) {
	t.Skip("DEPRECATED: edit_file tool replaced with patch_file")

	envPath := os.Getenv("ENV_PATH")
	if envPath == "" {
		if _, err := os.Stat(".env"); err == nil {
			envPath = ".env"
		} else {
			envPath = "../coding-agent/.env"
		}
	}

	data, err := os.ReadFile(envPath)
	if err != nil {
		t.Skipf("Skipping test: cannot read .env file: %v", err)
	}

	var apiKey string
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "TS_AGENT_API_KEY=") {
			apiKey = strings.TrimPrefix(line, "TS_AGENT_API_KEY=")
			apiKey = strings.TrimSpace(apiKey)
			break
		}
	}

	if apiKey == "" {
		t.Skip("Skipping test: TS_AGENT_API_KEY not found in .env file")
	}

	testFile := "test_edit_integration.txt"
	defer os.Remove(testFile)

	t.Run("Full edit_file tool use round-trip", func(t *testing.T) {
		var history []Message

		expectedContent := "This is test content written by the edit_file tool!"

		// Ask to create/edit a file
		response, updatedHistory := handleConversation(apiKey,
			fmt.Sprintf("Create a file called %s with the content: %s. Use the edit_file tool.", testFile, expectedContent),
			history)

		if response == "" {
			t.Fatal("Expected response but got empty string")
		}

		t.Logf("Response: %s", response)
		t.Logf("History length: %d", len(updatedHistory))

		if len(updatedHistory) < 3 {
			t.Errorf("Expected at least 3 messages in history, got %d", len(updatedHistory))
		}

		// Look for tool_use and tool_result in the conversation history
		foundToolUse := false
		foundToolResult := false

		for _, msg := range updatedHistory {
			if msg.Role == "assistant" {
				if contentBlocks, ok := msg.Content.([]ContentBlock); ok {
					for _, block := range contentBlocks {
						if block.Type == "tool_use" && block.Name == "edit_file" {
							foundToolUse = true
							if block.ID == "" {
								t.Error("Tool use block should have an ID")
							}
							t.Logf("Found tool_use: %s (ID: %s)", block.Name, block.ID)

							// Verify the input parameters
							if path, ok := block.Input["path"].(string); ok {
								if path != testFile {
									t.Errorf("Expected path '%s', got '%s'", testFile, path)
								}
							}
							if content, ok := block.Input["content"].(string); ok {
								if content != expectedContent {
									t.Errorf("Expected content '%s', got '%s'", expectedContent, content)
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
							if block.ToolUseID == "" {
								t.Error("Tool result block should have a ToolUseID")
							}
							t.Logf("Found tool_result with ToolUseID: %s", block.ToolUseID)
						}
					}
				}
			}
		}

		if !foundToolUse {
			t.Error("Expected to find an edit_file tool_use block in the conversation history")
		}

		if !foundToolResult {
			t.Error("Expected to find a tool_result block in the conversation history")
		}

		// Verify the file was actually created with the correct content
		fileContent, err := os.ReadFile(testFile)
		if err != nil {
			t.Errorf("Failed to read created file: %v", err)
		} else if string(fileContent) != expectedContent {
			t.Errorf("File content mismatch. Expected '%s', got '%s'", expectedContent, string(fileContent))
		} else {
			t.Logf("File successfully created with correct content!")
		}
	})
}

func TestEditFileWithLargeContent(t *testing.T) {
	t.Skip("KNOWN ISSUE: API hangs when Claude tries to use edit_file with large content (~14KB)")
	// This test successfully replicates the bug where edit_file doesn't work with large files.
	// The API either:
	// 1. Hangs/timeouts (as seen in this test)
	// 2. Returns tool_use with only [path], omitting content (as seen in REPL)
	// Both indicate the same underlying issue with large tool parameters.


	envPath := os.Getenv("ENV_PATH")
	if envPath == "" {
		if _, err := os.Stat(".env"); err == nil {
			envPath = ".env"
		} else {
			envPath = "../coding-agent/.env"
		}
	}

	data, err := os.ReadFile(envPath)
	if err != nil {
		t.Skipf("Skipping test: cannot read .env file: %v", err)
	}

	var apiKey string
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "TS_AGENT_API_KEY=") {
			apiKey = strings.TrimPrefix(line, "TS_AGENT_API_KEY=")
			apiKey = strings.TrimSpace(apiKey)
			break
		}
	}

	if apiKey == "" {
		t.Skip("Skipping test: TS_AGENT_API_KEY not found in .env file")
	}

	// Read main.go which is ~14KB - large enough to potentially trigger the issue
	mainGoContent, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("Failed to read main.go: %v", err)
	}

	testFile := "test_large_edit.go"
	defer os.Remove(testFile)

	t.Run("Edit file with large content (~14KB)", func(t *testing.T) {
		var history []Message

		// First ask Claude to read main.go, THEN ask it to create a copy
		// This mimics the REPL scenario where Claude reads a file and tries to edit it
		prompt1 := "Read the main.go file using the read_file tool"
		response1, history := handleConversation(apiKey, prompt1, history)

		if response1 == "" {
			t.Fatal("Failed to read main.go")
		}
		t.Logf("Step 1 - Read file response length: %d", len(response1))

		// Now ask Claude to create a new file with that content
		prompt2 := fmt.Sprintf("Now use the edit_file tool to create a file called %s with the EXACT same content you just read from main.go. You must provide the complete file content.", testFile)
		response, updatedHistory := handleConversation(apiKey, prompt2, history)

		if response == "" {
			t.Fatal("Expected response but got empty string")
		}

		t.Logf("Response: %s", response)
		t.Logf("History length: %d", len(updatedHistory))

		// Check if tool was used
		foundToolUse := false
		foundToolResult := false
		var toolUseID string
		var contentInToolUse bool

		for _, msg := range updatedHistory {
			if msg.Role == "assistant" {
				if contentBlocks, ok := msg.Content.([]ContentBlock); ok {
					for _, block := range contentBlocks {
						if block.Type == "tool_use" && block.Name == "edit_file" {
							foundToolUse = true
							toolUseID = block.ID

							// Check if content parameter exists
							if _, hasContent := block.Input["content"]; hasContent {
								contentInToolUse = true
								t.Logf("✓ Tool use has content parameter (ID: %s)", block.ID)
							} else {
								t.Logf("✗ Tool use MISSING content parameter (ID: %s)", block.ID)
								t.Logf("  Input keys: %v", getMapKeysTest(block.Input))
							}
						}
					}
				}
			}

			if msg.Role == "user" {
				if contentBlocks, ok := msg.Content.([]ContentBlock); ok {
					for _, block := range contentBlocks {
						if block.Type == "tool_result" && block.ToolUseID == toolUseID {
							foundToolResult = true

							// Check if it's an error about missing content
							if block.IsError {
								if content, ok := block.Content.(string); ok {
									t.Logf("Tool result error: %s", content)
								}
							}
						}
					}
				}
			}
		}

		if !foundToolUse {
			t.Error("Expected to find an edit_file tool_use block")
		}

		if !foundToolResult {
			t.Error("Expected to find a tool_result block")
		}

		// THIS IS THE KEY TEST: Did Claude provide the content parameter?
		if foundToolUse && !contentInToolUse {
			t.Errorf("REPLICATION SUCCESS: Content parameter was NOT provided in tool_use for large file (~14KB)")
			t.Errorf("This replicates the bug seen in REPL usage")
		} else if foundToolUse && contentInToolUse {
			t.Logf("Content parameter WAS provided - checking if file was created correctly...")

			// Verify file was created with correct content
			if _, err := os.Stat(testFile); err == nil {
				createdContent, err := os.ReadFile(testFile)
				if err != nil {
					t.Errorf("Failed to read created file: %v", err)
				} else if string(createdContent) != string(mainGoContent) {
					t.Errorf("File content mismatch. Expected %d bytes, got %d bytes",
						len(mainGoContent), len(createdContent))
				} else {
					t.Logf("✓ File created successfully with correct content")
				}
			} else {
				t.Logf("File was not created (likely due to validation error)")
			}
		}
	})
}

func getMapKeysTest(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func TestGitHubQueryIntegration(t *testing.T) {
	envPath := os.Getenv("ENV_PATH")
	if envPath == "" {
		if _, err := os.Stat(".env"); err == nil {
			envPath = ".env"
		} else {
			envPath = "../coding-agent/.env"
		}
	}

	data, err := os.ReadFile(envPath)
	if err != nil {
		t.Skipf("Skipping test: cannot read .env file: %v", err)
	}

	var apiKey string
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "TS_AGENT_API_KEY=") {
			apiKey = strings.TrimPrefix(line, "TS_AGENT_API_KEY=")
			apiKey = strings.TrimSpace(apiKey)
			break
		}
	}

	if apiKey == "" {
		t.Skip("Skipping test: TS_AGENT_API_KEY not found in .env file")
	}

	// Check if gh CLI is available using run_bash
	if _, err := executeRunBash("gh auth status"); err != nil {
		t.Skipf("Skipping test: gh CLI not configured: %v", err)
	}

	t.Run("Full GitHub tool use round-trip with run_bash", func(t *testing.T) {
		var history []Message

		// Ask a GitHub-related question that should trigger run_bash with gh command
		response, updatedHistory := handleConversation(apiKey, "What is my GitHub username? Use the bash tool to run 'gh api user'.", history)

		if response == "" {
			t.Fatal("Expected response but got empty string")
		}

		t.Logf("Response: %s", response)
		t.Logf("History length: %d", len(updatedHistory))

		// Verify the conversation history contains the expected message types
		// Should have at least: user message, assistant with tool_use, user with tool_result, assistant with text
		if len(updatedHistory) < 3 {
			t.Errorf("Expected at least 3 messages in history (user, assistant with tool_use, user with tool_result, assistant with text), got %d", len(updatedHistory))
		}

		// Look for run_bash tool_use in the assistant's messages
		foundToolUse := false
		foundToolResult := false

		for _, msg := range updatedHistory {
			if msg.Role == "assistant" {
				if contentBlocks, ok := msg.Content.([]ContentBlock); ok {
					for _, block := range contentBlocks {
						if block.Type == "tool_use" {
							foundToolUse = true
							if block.ID == "" {
								t.Error("Tool use block should have an ID")
							}
							if block.Name != "run_bash" {
								t.Errorf("Expected tool name 'run_bash', got '%s'", block.Name)
							}
							t.Logf("Found tool_use: %s (ID: %s)", block.Name, block.ID)
							
							// Verify the command contains gh
							if command, ok := block.Input["command"].(string); ok {
								if !strings.Contains(command, "gh") {
									t.Errorf("Expected command to contain 'gh', got: %s", command)
								}
								t.Logf("Command: %s", command)
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
							if block.ToolUseID == "" {
								t.Error("Tool result block should have a ToolUseID")
							}
							t.Logf("Found tool_result with ToolUseID: %s", block.ToolUseID)
						}
					}
				}
			}
		}

		if !foundToolUse {
			t.Error("Expected to find a run_bash tool_use block in the conversation history")
		}

		if !foundToolResult {
			t.Error("Expected to find a tool_result block in the conversation history")
		}

		// The response should contain some GitHub-related information
		if !strings.Contains(strings.ToLower(response), "github") &&
		   !strings.Contains(response, "gh") &&
		   len(response) < 3 {
			t.Logf("Warning: Response doesn't seem to contain GitHub information, but this might be okay")
		}
	})
}

func TestRunBashIntegration(t *testing.T) {
	envPath := os.Getenv("ENV_PATH")
	if envPath == "" {
		if _, err := os.Stat(".env"); err == nil {
			envPath = ".env"
		} else {
			envPath = "../coding-agent/.env"
		}
	}

	data, err := os.ReadFile(envPath)
	if err != nil {
		t.Skipf("Skipping test: cannot read .env file: %v", err)
	}

	var apiKey string
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "TS_AGENT_API_KEY=") {
			apiKey = strings.TrimPrefix(line, "TS_AGENT_API_KEY=")
			apiKey = strings.TrimSpace(apiKey)
			break
		}
	}

	if apiKey == "" {
		t.Skip("Skipping test: TS_AGENT_API_KEY not found in .env file")
	}

	t.Run("Full run_bash tool use round-trip", func(t *testing.T) {
		var history []Message

		// Ask a question that should trigger the run_bash tool
		response, updatedHistory := handleConversation(apiKey, "Use the bash tool to find out whoami?", history)

		if response == "" {
			t.Fatal("Expected response but got empty string")
		}

		t.Logf("Response: %s", response)
		t.Logf("History length: %d", len(updatedHistory))

		if len(updatedHistory) < 3 {
			t.Errorf("Expected at least 3 messages in history, got %d", len(updatedHistory))
		}

		// Look for tool_use and tool_result in the conversation history
		foundToolUse := false
		foundToolResult := false
		var toolResultContent string

		for _, msg := range updatedHistory {
			if msg.Role == "assistant" {
				if contentBlocks, ok := msg.Content.([]ContentBlock); ok {
					for _, block := range contentBlocks {
						if block.Type == "tool_use" && block.Name == "run_bash" {
							foundToolUse = true
							if block.ID == "" {
								t.Error("Tool use block should have an ID")
							}
							t.Logf("Found tool_use: %s (ID: %s)", block.Name, block.ID)
							
							// Verify the command parameter
							if command, ok := block.Input["command"].(string); ok {
								if !strings.Contains(command, "whoami") {
									t.Errorf("Expected command to contain 'whoami', got: %s", command)
								}
								t.Logf("Command: %s", command)
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
							if block.ToolUseID == "" {
								t.Error("Tool result block should have a ToolUseID")
							}
							t.Logf("Found tool_result with ToolUseID: %s", block.ToolUseID)
							if content, ok := block.Content.(string); ok {
								toolResultContent = content
								t.Logf("Tool result content: %s", content)
							}
						}
					}
				}
			}
		}

		if !foundToolUse {
			t.Error("Expected to find a run_bash tool_use block in the conversation history")
		}

		if !foundToolResult {
			t.Error("Expected to find a tool_result block in the conversation history")
		}

		// Verify the tool result contains a username (should not be empty)
		if strings.TrimSpace(toolResultContent) == "" {
			t.Error("Expected tool result to contain a username, but got empty string")
		}

		// Verify the response mentions the username
		if !strings.Contains(response, strings.TrimSpace(toolResultContent)) {
			t.Logf("Warning: Response doesn't seem to mention the username from tool result")
		}
	})

	t.Run("Bash tool with echo command", func(t *testing.T) {
		var history []Message

		testString := "Hello from bash test!"
		response, updatedHistory := handleConversation(apiKey, 
			fmt.Sprintf("Use the bash tool to echo '%s'", testString), 
			history)

		if response == "" {
			t.Fatal("Expected response but got empty string")
		}

		t.Logf("Response: %s", response)

		// Look for the echo output in the tool result
		foundExpectedOutput := false
		for _, msg := range updatedHistory {
			if msg.Role == "user" {
				if contentBlocks, ok := msg.Content.([]ContentBlock); ok {
					for _, block := range contentBlocks {
						if block.Type == "tool_result" {
							if content, ok := block.Content.(string); ok {
								if strings.Contains(content, testString) {
									foundExpectedOutput = true
									t.Logf("Found expected output in tool result")
								}
							}
						}
					}
				}
			}
		}

		if !foundExpectedOutput {
			t.Error("Expected to find the echo output in the tool result")
		}
	})

	t.Run("Bash tool with error handling", func(t *testing.T) {
		var history []Message

		// Ask Claude to run a command that will fail
		response, updatedHistory := handleConversation(apiKey, 
			"Use the bash tool to run the command 'exit 1'", 
			history)

		if response == "" {
			t.Fatal("Expected response but got empty string")
		}

		t.Logf("Response: %s", response)

		// Look for error indication in the tool result
		foundError := false
		for _, msg := range updatedHistory {
			if msg.Role == "user" {
				if contentBlocks, ok := msg.Content.([]ContentBlock); ok {
					for _, block := range contentBlocks {
						if block.Type == "tool_result" {
							if block.IsError {
								foundError = true
								t.Logf("Found error in tool result as expected")
							}
						}
					}
				}
			}
		}

		if !foundError {
			t.Logf("Warning: Expected to find IsError flag set in tool result for failing command")
		}
	})
}

func TestWriteFileIntegration(t *testing.T) {
	envPath := os.Getenv("ENV_PATH")
	if envPath == "" {
		if _, err := os.Stat(".env"); err == nil {
			envPath = ".env"
		} else {
			envPath = "../coding-agent/.env"
		}
	}

	data, err := os.ReadFile(envPath)
	if err != nil {
		t.Skipf("Skipping test: cannot read .env file: %v", err)
	}

	var apiKey string
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "TS_AGENT_API_KEY=") {
			apiKey = strings.TrimPrefix(line, "TS_AGENT_API_KEY=")
			apiKey = strings.TrimSpace(apiKey)
			break
		}
	}

	if apiKey == "" {
		t.Skip("Skipping test: TS_AGENT_API_KEY not found in .env file")
	}

	t.Run("Create new file with write_file tool", func(t *testing.T) {
		var history []Message
		testFile := "test_write_integration_new.txt"
		defer os.Remove(testFile)

		expectedContent := "This is a test file created by the write_file tool!"

		response, updatedHistory := handleConversation(apiKey,
			fmt.Sprintf("Use the write_file tool to create a file called %s with this exact content: %s", testFile, expectedContent),
			history)

		if response == "" {
			t.Fatal("Expected response but got empty string")
		}

		t.Logf("Response: %s", response)
		t.Logf("History length: %d", len(updatedHistory))

		// Look for tool_use and tool_result
		foundToolUse := false
		foundToolResult := false

		for _, msg := range updatedHistory {
			if msg.Role == "assistant" {
				if contentBlocks, ok := msg.Content.([]ContentBlock); ok {
					for _, block := range contentBlocks {
						if block.Type == "tool_use" && block.Name == "write_file" {
							foundToolUse = true
							t.Logf("Found tool_use: %s (ID: %s)", block.Name, block.ID)

							// Verify input parameters
							if path, ok := block.Input["path"].(string); ok {
								if path != testFile {
									t.Errorf("Expected path '%s', got '%s'", testFile, path)
								}
							}
							if content, ok := block.Input["content"].(string); ok {
								if content != expectedContent {
									t.Errorf("Expected content '%s', got '%s'", expectedContent, content)
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
							t.Logf("Found tool_result with ToolUseID: %s", block.ToolUseID)
						}
					}
				}
			}
		}

		if !foundToolUse {
			t.Error("Expected to find a write_file tool_use block")
		}

		if !foundToolResult {
			t.Error("Expected to find a tool_result block")
		}

		// Verify file was created with correct content
		if _, err := os.Stat(testFile); err != nil {
			t.Errorf("File was not created: %v", err)
		} else {
			content, err := os.ReadFile(testFile)
			if err != nil {
				t.Errorf("Failed to read created file: %v", err)
			} else if string(content) != expectedContent {
				t.Errorf("File content mismatch. Expected '%s', got '%s'", expectedContent, string(content))
			} else {
				t.Logf("✓ File created successfully with correct content")
			}
		}
	})

	t.Run("Replace existing file with write_file tool", func(t *testing.T) {
		var history []Message
		testFile := "test_write_integration_replace.txt"
		defer os.Remove(testFile)

		// Create initial file
		initialContent := "Old content that will be replaced"
		if err := os.WriteFile(testFile, []byte(initialContent), 0644); err != nil {
			t.Fatalf("Failed to create initial file: %v", err)
		}

		newContent := "New content from write_file tool"

		response, updatedHistory := handleConversation(apiKey,
			fmt.Sprintf("Use the write_file tool to replace the contents of %s with this: %s", testFile, newContent),
			history)

		if response == "" {
			t.Fatal("Expected response but got empty string")
		}

		t.Logf("Response: %s", response)

		// Verify file was replaced
		content, err := os.ReadFile(testFile)
		if err != nil {
			t.Errorf("Failed to read replaced file: %v", err)
		} else if string(content) != newContent {
			t.Errorf("File content mismatch. Expected '%s', got '%s'", newContent, string(content))
		} else {
			t.Logf("✓ File replaced successfully with correct content")
		}

		// Verify tool was used
		foundToolUse := false
		for _, msg := range updatedHistory {
			if msg.Role == "assistant" {
				if contentBlocks, ok := msg.Content.([]ContentBlock); ok {
					for _, block := range contentBlocks {
						if block.Type == "tool_use" && block.Name == "write_file" {
							foundToolUse = true
						}
					}
				}
			}
		}

		if !foundToolUse {
			t.Error("Expected to find a write_file tool_use block")
		}
	})

	t.Run("Write multiline file content", func(t *testing.T) {
		var history []Message
		testFile := "test_write_integration_multiline.txt"
		defer os.Remove(testFile)

		multilineContent := `Line 1: Hello
Line 2: World
Line 3: From write_file tool`

		response, updatedHistory := handleConversation(apiKey,
			fmt.Sprintf("Use the write_file tool to create %s with this multiline content:\n%s", testFile, multilineContent),
			history)

		if response == "" {
			t.Fatal("Expected response but got empty string")
		}

		t.Logf("Response: %s", response)

		// Verify file content
		content, err := os.ReadFile(testFile)
		if err != nil {
			t.Errorf("Failed to read file: %v", err)
		} else {
			// Check if content contains the key lines
			contentStr := string(content)
			if !strings.Contains(contentStr, "Line 1") || 
			   !strings.Contains(contentStr, "Line 2") || 
			   !strings.Contains(contentStr, "Line 3") {
				t.Errorf("File content doesn't contain expected lines. Got: %s", contentStr)
			} else {
				t.Logf("✓ Multiline file created successfully")
			}
		}

		// Verify tool was used
		foundToolUse := false
		for _, msg := range updatedHistory {
			if msg.Role == "assistant" {
				if contentBlocks, ok := msg.Content.([]ContentBlock); ok {
					for _, block := range contentBlocks {
						if block.Type == "tool_use" && block.Name == "write_file" {
							foundToolUse = true
						}
					}
				}
			}
		}

		if !foundToolUse {
			t.Error("Expected to find a write_file tool_use block")
		}
	})
}

func TestGrepIntegration(t *testing.T) {
	envPath := os.Getenv("ENV_PATH")
	if envPath == "" {
		if _, err := os.Stat(".env"); err == nil {
			envPath = ".env"
		} else {
			envPath = "../coding-agent/.env"
		}
	}

	data, err := os.ReadFile(envPath)
	if err != nil {
		t.Skipf("Skipping test: cannot read .env file: %v", err)
	}

	var apiKey string
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "TS_AGENT_API_KEY=") {
			apiKey = strings.TrimPrefix(line, "TS_AGENT_API_KEY=")
			apiKey = strings.TrimSpace(apiKey)
			break
		}
	}

	if apiKey == "" {
		t.Skip("Skipping test: TS_AGENT_API_KEY not found in .env file")
	}

	t.Run("Search for function definitions with grep", func(t *testing.T) {
		var history []Message

		response, updatedHistory := handleConversation(apiKey,
			"Use the grep tool to search for 'func Test' in the current directory, only in .go files",
			history)

		if response == "" {
			t.Fatal("Expected response but got empty string")
		}

		t.Logf("Response: %s", response)
		t.Logf("History length: %d", len(updatedHistory))

		// Look for tool_use and tool_result
		foundToolUse := false
		foundToolResult := false
		var toolResultContent string

		for _, msg := range updatedHistory {
			if msg.Role == "assistant" {
				if contentBlocks, ok := msg.Content.([]ContentBlock); ok {
					for _, block := range contentBlocks {
						if block.Type == "tool_use" && block.Name == "grep" {
							foundToolUse = true
							t.Logf("Found tool_use: %s (ID: %s)", block.Name, block.ID)

							// Verify input parameters
							if pattern, ok := block.Input["pattern"].(string); ok {
								t.Logf("Search pattern: %s", pattern)
							}
							if filePattern, ok := block.Input["file_pattern"].(string); ok {
								t.Logf("File pattern: %s", filePattern)
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
							t.Logf("Found tool_result with ToolUseID: %s", block.ToolUseID)
							if content, ok := block.Content.(string); ok {
								toolResultContent = content
							}
						}
					}
				}
			}
		}

		if !foundToolUse {
			t.Error("Expected to find a grep tool_use block")
		}

		if !foundToolResult {
			t.Error("Expected to find a tool_result block")
		}

		// Verify the tool result contains test function matches
		if toolResultContent != "" {
			// Should find test functions in main_test.go
			if !strings.Contains(toolResultContent, "func Test") {
				t.Logf("Warning: Tool result doesn't seem to contain test function definitions")
				t.Logf("Tool result (first 200 chars): %s", toolResultContent[:min(200, len(toolResultContent))])
			}
		}
	})

	t.Run("Search for TODO comments", func(t *testing.T) {
		var history []Message

		// Create a test file with TODO
		testFile := "test_grep_todos.txt"
		testContent := `Line 1: Some code
TODO: implement feature X
Line 3: More code
TODO: fix bug Y`
		if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}
		defer os.Remove(testFile)

		response, updatedHistory := handleConversation(apiKey,
			"Use the grep tool to find all TODO comments in the current directory",
			history)

		if response == "" {
			t.Fatal("Expected response but got empty string")
		}

		t.Logf("Response: %s", response)

		// Verify grep was used
		foundGrepUse := false
		for _, msg := range updatedHistory {
			if msg.Role == "assistant" {
				if contentBlocks, ok := msg.Content.([]ContentBlock); ok {
					for _, block := range contentBlocks {
						if block.Type == "tool_use" && block.Name == "grep" {
							foundGrepUse = true
							t.Logf("✓ grep tool was used")
						}
					}
				}
			}
		}

		if !foundGrepUse {
			t.Error("Expected grep tool to be used")
		}
	})

	t.Run("Search with no matches", func(t *testing.T) {
		var history []Message

		response, updatedHistory := handleConversation(apiKey,
			"Use grep to search for the pattern 'ZZZNONEXISTENTZZZPATTERN' in the current directory",
			history)

		if response == "" {
			t.Fatal("Expected response but got empty string")
		}

		t.Logf("Response: %s", response)

		// Should handle gracefully with no matches
		foundToolResult := false
		for _, msg := range updatedHistory {
			if msg.Role == "user" {
				if contentBlocks, ok := msg.Content.([]ContentBlock); ok {
					for _, block := range contentBlocks {
						if block.Type == "tool_result" {
							foundToolResult = true
							if content, ok := block.Content.(string); ok {
								// Should mention no matches
								if !strings.Contains(strings.ToLower(content), "no match") &&
								   !strings.Contains(strings.ToLower(content), "found 0") {
									t.Logf("Tool result: %s", content[:min(200, len(content))])
								}
							}
						}
					}
				}
			}
		}

		if !foundToolResult {
			t.Error("Expected to find a tool_result block")
		}
	})
}

func TestExecuteGlob(t *testing.T) {
	// Create test directory structure for glob testing
	testDir := "test_glob_dir"
	if err := os.MkdirAll(testDir+"/subdir", 0755); err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}
	defer os.RemoveAll(testDir)

	// Create test files
	testFiles := []string{
		testDir + "/test1.go",
		testDir + "/test2.go",
		testDir + "/test_helper.go",
		testDir + "/main_test.go",
		testDir + "/README.md",
		testDir + "/subdir/nested.go",
		testDir + "/subdir/doc.md",
	}

	for _, file := range testFiles {
		if err := os.WriteFile(file, []byte("test content"), 0644); err != nil {
			t.Fatalf("Failed to create test file %s: %v", file, err)
		}
	}

	tests := []struct {
		name        string
		pattern     string
		path        string
		expectError bool
		checkOutput bool
		shouldFind  bool
		minFiles    int
	}{
		{
			name:        "Find all Go files (simple pattern)",
			pattern:     "*.go",
			path:        testDir,
			expectError: false,
			checkOutput: true,
			shouldFind:  true,
			minFiles:    4, // 4 Go files in root dir
		},
		{
			name:        "Find all test files",
			pattern:     "*_test.go",
			path:        testDir,
			expectError: false,
			checkOutput: true,
			shouldFind:  true,
			minFiles:    1,
		},
		{
			name:        "Find all Go files recursively",
			pattern:     "**/*.go",
			path:        testDir,
			expectError: false,
			checkOutput: true,
			shouldFind:  true,
			minFiles:    5, // 4 in root + 1 in subdir
		},
		{
			name:        "Find all markdown files recursively",
			pattern:     "**/*.md",
			path:        testDir,
			expectError: false,
			checkOutput: true,
			shouldFind:  true,
			minFiles:    2,
		},
		{
			name:        "Find specific file",
			pattern:     "README.md",
			path:        testDir,
			expectError: false,
			checkOutput: true,
			shouldFind:  true,
			minFiles:    1,
		},
		{
			name:        "Pattern with no matches",
			pattern:     "*.xyz",
			path:        testDir,
			expectError: false, // Not an error, just no matches
			checkOutput: true,
			shouldFind:  false,
		},
		{
			name:        "Search in non-existent directory",
			pattern:     "*.go",
			path:        "/nonexistent/path/xyz",
			expectError: true,
		},
		{
			name:        "Empty pattern",
			pattern:     "",
			path:        testDir,
			expectError: true,
		},
		{
			name:        "Find in current directory (empty path)",
			pattern:     "main_test.go",
			path:        "",
			expectError: false,
			checkOutput: true,
			shouldFind:  true,
			minFiles:    1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := executeGlob(tt.pattern, tt.path)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none. Output: %s", output)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}

				if tt.checkOutput {
					if tt.shouldFind {
						// Check if output indicates files were found
						if !strings.Contains(output, "Found") && !strings.Contains(output, "/") {
							t.Errorf("Expected file matches but output suggests none: %s", output)
						}

						// Count files if specified
						if tt.minFiles > 0 {
							lines := strings.Split(output, "\n")
							fileCount := 0
							for _, line := range lines {
								// Count lines that look like file paths (contain /)
								if strings.Contains(line, "/") && !strings.HasPrefix(line, "Found") {
									fileCount++
								}
							}
							if fileCount < tt.minFiles {
								t.Errorf("Expected at least %d files, got %d. Output:\n%s",
									tt.minFiles, fileCount, output)
							} else {
								t.Logf("✓ Found %d files (expected at least %d)", fileCount, tt.minFiles)
							}
						}
					} else {
						// Should indicate no files found
						if !strings.Contains(output, "No files") && !strings.Contains(output, "found 0") {
							t.Logf("Output for no matches: %s", output[:min(200, len(output))])
						}
					}
				}
			}
		})
	}
}

func TestGlobIntegration(t *testing.T) {
	envPath := os.Getenv("ENV_PATH")
	if envPath == "" {
		if _, err := os.Stat(".env"); err == nil {
			envPath = ".env"
		} else {
			envPath = "../coding-agent/.env"
		}
	}

	data, err := os.ReadFile(envPath)
	if err != nil {
		t.Skipf("Skipping test: cannot read .env file: %v", err)
	}

	var apiKey string
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "TS_AGENT_API_KEY=") {
			apiKey = strings.TrimPrefix(line, "TS_AGENT_API_KEY=")
			apiKey = strings.TrimSpace(apiKey)
			break
		}
	}

	if apiKey == "" {
		t.Skip("Skipping test: TS_AGENT_API_KEY not found in .env file")
	}

	t.Run("Find all test files with glob", func(t *testing.T) {
		var history []Message

		response, updatedHistory := handleConversation(apiKey,
			"Use the glob tool to find all files ending with '_test.go' in the current directory",
			history)

		if response == "" {
			t.Fatal("Expected response but got empty string")
		}

		t.Logf("Response: %s", response)
		t.Logf("History length: %d", len(updatedHistory))

		// Look for tool_use and tool_result
		foundToolUse := false
		foundToolResult := false
		var toolResultContent string

		for _, msg := range updatedHistory {
			if msg.Role == "assistant" {
				if contentBlocks, ok := msg.Content.([]ContentBlock); ok {
					for _, block := range contentBlocks {
						if block.Type == "tool_use" && block.Name == "glob" {
							foundToolUse = true
							t.Logf("Found tool_use: %s (ID: %s)", block.Name, block.ID)

							// Verify input parameters
							if pattern, ok := block.Input["pattern"].(string); ok {
								t.Logf("Pattern: %s", pattern)
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
							t.Logf("Found tool_result with ToolUseID: %s", block.ToolUseID)
							if content, ok := block.Content.(string); ok {
								toolResultContent = content
							}
						}
					}
				}
			}
		}

		if !foundToolUse {
			t.Error("Expected to find a glob tool_use block")
		}

		if !foundToolResult {
			t.Error("Expected to find a tool_result block")
		}

		// Verify the tool result contains test file paths
		if toolResultContent != "" {
			if !strings.Contains(toolResultContent, "_test.go") {
				t.Logf("Warning: Tool result doesn't seem to contain test files")
				t.Logf("Tool result (first 200 chars): %s", toolResultContent[:min(200, len(toolResultContent))])
			} else {
				t.Logf("✓ Found test files in results")
			}
		}
	})

	t.Run("Find all Go files recursively", func(t *testing.T) {
		var history []Message

		response, updatedHistory := handleConversation(apiKey,
			"Use glob to find all Go files recursively using the pattern '**/*.go'",
			history)

		if response == "" {
			t.Fatal("Expected response but got empty string")
		}

		t.Logf("Response: %s", response)

		// Verify glob was used
		foundGlobUse := false
		for _, msg := range updatedHistory {
			if msg.Role == "assistant" {
				if contentBlocks, ok := msg.Content.([]ContentBlock); ok {
					for _, block := range contentBlocks {
						if block.Type == "tool_use" && block.Name == "glob" {
							foundGlobUse = true
							t.Logf("✓ glob tool was used")
						}
					}
				}
			}
		}

		if !foundGlobUse {
			t.Error("Expected glob tool to be used")
		}
	})

	t.Run("Find specific file with glob", func(t *testing.T) {
		var history []Message

		response, updatedHistory := handleConversation(apiKey,
			"Use glob to find README.md in the current directory",
			history)

		if response == "" {
			t.Fatal("Expected response but got empty string")
		}

		t.Logf("Response: %s", response)

		// Verify tool was used
		foundToolResult := false
		for _, msg := range updatedHistory {
			if msg.Role == "user" {
				if contentBlocks, ok := msg.Content.([]ContentBlock); ok {
					for _, block := range contentBlocks {
						if block.Type == "tool_result" {
							foundToolResult = true
							if content, ok := block.Content.(string); ok {
								if strings.Contains(content, "README.md") {
									t.Logf("✓ Found README.md in results")
								}
							}
						}
					}
				}
			}
		}

		if !foundToolResult {
			t.Error("Expected to find a tool_result block")
		}
	})

	t.Run("Handle no matches gracefully", func(t *testing.T) {
		var history []Message

		response, updatedHistory := handleConversation(apiKey,
			"Use glob to find all files matching '*.zzznonexistent' in the current directory",
			history)

		if response == "" {
			t.Fatal("Expected response but got empty string")
		}

		t.Logf("Response: %s", response)

		// Should handle gracefully with no matches
		foundToolResult := false
		for _, msg := range updatedHistory {
			if msg.Role == "user" {
				if contentBlocks, ok := msg.Content.([]ContentBlock); ok {
					for _, block := range contentBlocks {
						if block.Type == "tool_result" {
							foundToolResult = true
							if content, ok := block.Content.(string); ok {
								t.Logf("Tool result (no matches expected): %s", content[:min(200, len(content))])
							}
						}
					}
				}
			}
		}

		if !foundToolResult {
			t.Error("Expected to find a tool_result block")
		}
	})
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
