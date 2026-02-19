package main

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/this-is-alpha-iota/clyde/tools"
)

// TestExecuteIncludeFile tests the include_file tool execution
func TestExecuteIncludeFile(t *testing.T) {
	// Create a temporary directory for test files
	tmpDir := t.TempDir()

	// Create a small test image (1x1 PNG)
	// This is a valid 1x1 transparent PNG
	pngData := []byte{
		0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00, 0x00, 0x0D,
		0x49, 0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x06, 0x00, 0x00, 0x00, 0x1F, 0x15, 0xC4, 0x89, 0x00, 0x00, 0x00,
		0x0A, 0x49, 0x44, 0x41, 0x54, 0x78, 0x9C, 0x63, 0x00, 0x01, 0x00, 0x00,
		0x05, 0x00, 0x01, 0x0D, 0x0A, 0x2D, 0xB4, 0x00, 0x00, 0x00, 0x00, 0x49,
		0x45, 0x4E, 0x44, 0xAE, 0x42, 0x60, 0x82,
	}

	testImagePath := filepath.Join(tmpDir, "test.png")
	if err := os.WriteFile(testImagePath, pngData, 0644); err != nil {
		t.Fatalf("Failed to create test image: %v", err)
	}

	tests := []struct {
		name        string
		input       map[string]interface{}
		wantErr     bool
		errContains string
		checkOutput func(t *testing.T, output string)
	}{
		{
			name:        "missing path parameter",
			input:       map[string]interface{}{},
			wantErr:     true,
			errContains: "path is required",
		},
		{
			name:        "empty path parameter",
			input:       map[string]interface{}{"path": ""},
			wantErr:     true,
			errContains: "path is required",
		},
		{
			name:        "file not found",
			input:       map[string]interface{}{"path": filepath.Join(tmpDir, "nonexistent.png")},
			wantErr:     true,
			errContains: "not found",
		},
		{
			name:        "unsupported file type",
			input:       map[string]interface{}{"path": filepath.Join(tmpDir, "test.txt")},
			wantErr:     true,
			errContains: "only image files are currently supported",
		},
		{
			name:    "load valid PNG image",
			input:   map[string]interface{}{"path": testImagePath},
			wantErr: false,
			checkOutput: func(t *testing.T, output string) {
				if !strings.HasPrefix(output, "IMAGE_LOADED:") {
					t.Errorf("Expected output to start with IMAGE_LOADED:, got: %s", output[:50])
				}
				parts := strings.SplitN(output, ":", 4)
				if len(parts) != 4 {
					t.Errorf("Expected 4 parts in output, got %d", len(parts))
					return
				}
				if parts[1] != "image/png" {
					t.Errorf("Expected media type image/png, got: %s", parts[1])
				}
				// Verify base64 data is valid
				_, err := base64.StdEncoding.DecodeString(parts[3])
				if err != nil {
					t.Errorf("Invalid base64 data: %v", err)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg, err := tools.GetTool("include_file")
			if err != nil {
				t.Fatalf("Failed to get include_file tool: %v", err)
			}

			output, err := reg.Execute(tt.input, nil, nil)

			if tt.wantErr {
				if err == nil {
					t.Errorf("Expected error, got nil")
				} else if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("Expected error to contain '%s', got: %v", tt.errContains, err)
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error, got: %v", err)
				}
				if tt.checkOutput != nil {
					tt.checkOutput(t, output)
				}
			}
		})
	}
}

// TestDisplayIncludeFile tests the display function
func TestDisplayIncludeFile(t *testing.T) {
	reg, err := tools.GetTool("include_file")
	if err != nil {
		t.Fatalf("Failed to get include_file tool: %v", err)
	}

	tests := []struct {
		name     string
		input    map[string]interface{}
		expected string
	}{
		{
			name:     "local file path",
			input:    map[string]interface{}{"path": "./screenshot.png"},
			expected: "→ Including file: ./screenshot.png",
		},
		{
			name:     "remote URL",
			input:    map[string]interface{}{"path": "https://example.com/image.png"},
			expected: "→ Including file: https://example.com/image.png",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			display := reg.Display(tt.input)
			if display != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, display)
			}
		})
	}
}

// TestIncludeFileIntegration tests the full integration with the agent
func TestIncludeFileIntegration(t *testing.T) {
	envPath := os.Getenv("ENV_PATH")
	if envPath == "" {
		if _, err := os.Stat(".env"); err == nil {
			envPath = ".env"
		} else {
			envPath = "../.env"
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

	// Create test image
	tmpDir := t.TempDir()
	pngData := []byte{
		0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00, 0x00, 0x0D,
		0x49, 0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x06, 0x00, 0x00, 0x00, 0x1F, 0x15, 0xC4, 0x89, 0x00, 0x00, 0x00,
		0x0A, 0x49, 0x44, 0x41, 0x54, 0x78, 0x9C, 0x63, 0x00, 0x01, 0x00, 0x00,
		0x05, 0x00, 0x01, 0x0D, 0x0A, 0x2D, 0xB4, 0x00, 0x00, 0x00, 0x00, 0x49,
		0x45, 0x4E, 0x44, 0xAE, 0x42, 0x60, 0x82,
	}
	testImagePath := filepath.Join(tmpDir, "test_image.png")
	if err := os.WriteFile(testImagePath, pngData, 0644); err != nil {
		t.Fatalf("Failed to create test image: %v", err)
	}

	t.Run("include image in conversation", func(t *testing.T) {
		var history []Message

		response, _ := handleConversation(apiKey,
			fmt.Sprintf("Use the include_file tool to load this image: %s", testImagePath),
			history)

		if response == "" {
			t.Fatal("Expected response but got empty string")
		}

		t.Logf("Response: %s", response)

		// The response should indicate the tool was used successfully
		responseLower := strings.ToLower(response)
		if !strings.Contains(responseLower, "image") && !strings.Contains(responseLower, "loaded") {
			t.Logf("Warning: Response doesn't mention image loading (but test may still pass)")
		}
	})

	t.Run("ask about non-existent image", func(t *testing.T) {
		var history []Message

		response, _ := handleConversation(apiKey,
			"Use include_file to load nonexistent_image.png",
			history)

		if response == "" {
			t.Fatal("Expected response but got empty string")
		}

		t.Logf("Response: %s", response)

		// The agent should mention the file not being found
		responseLower := strings.ToLower(response)
		if !strings.Contains(responseLower, "not found") && 
		   !strings.Contains(responseLower, "doesn't exist") && 
		   !strings.Contains(responseLower, "cannot find") &&
		   !strings.Contains(responseLower, "failed") {
			t.Logf("Warning: Response doesn't clearly indicate file not found")
		}
	})
}
