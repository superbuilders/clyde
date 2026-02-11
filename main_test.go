package main

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestExecuteGitHubCommand(t *testing.T) {
	tests := []struct {
		name        string
		command     string
		expectError bool
	}{
		{
			name:        "Valid command - check auth status",
			command:     "auth status",
			expectError: false,
		},
		{
			name:        "Valid command - api user",
			command:     "api user",
			expectError: false,
		},
		{
			name:        "Invalid command",
			command:     "invalid-command-xyz",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := executeGitHubCommand(tt.command)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none. Output: %s", output)
				}
			} else {
				if err != nil {
					t.Logf("Command failed (might be due to gh not configured): %v", err)
				} else if output == "" {
					t.Error("Expected output but got empty string")
				}
			}
		})
	}
}

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

	requiredTerms := []string{"github_query", "tool", "GitHub"}
	for _, term := range requiredTerms {
		if !strings.Contains(systemPrompt, term) {
			t.Errorf("System prompt should contain '%s'", term)
		}
	}

	t.Logf("System prompt length: %d characters", len(systemPrompt))
}

func TestGitHubTool(t *testing.T) {
	if githubTool.Name != "github_query" {
		t.Errorf("Expected tool name 'github_query', got '%s'", githubTool.Name)
	}

	if githubTool.Description == "" {
		t.Error("Tool description should not be empty")
	}

	if githubTool.InputSchema == nil {
		t.Fatal("Tool input schema should not be nil")
	}

	schema, ok := githubTool.InputSchema.(map[string]interface{})
	if !ok {
		t.Fatal("Input schema should be a map")
	}

	properties, ok := schema["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("Schema should have properties")
	}

	if _, exists := properties["command"]; !exists {
		t.Error("Schema should have 'command' property")
	}
}

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

	// Check if gh CLI is available
	if _, err := executeGitHubCommand("auth status"); err != nil {
		t.Skipf("Skipping test: gh CLI not configured: %v", err)
	}

	t.Run("Full GitHub tool use round-trip", func(t *testing.T) {
		var history []Message

		// Ask a GitHub-related question that should trigger tool use
		response, updatedHistory := handleConversation(apiKey, "What is my GitHub username? Use gh api to check.", history)

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

		// Look for tool_use in the assistant's messages
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
							if block.Name != "github_query" {
								t.Errorf("Expected tool name 'github_query', got '%s'", block.Name)
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
			t.Error("Expected to find a tool_use block in the conversation history")
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
