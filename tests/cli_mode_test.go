package main

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestCLIMode_DirectString tests CLI mode with direct string argument
func TestCLIMode_DirectString(t *testing.T) {
	// Build the binary for testing
	binaryPath := buildTestBinary(t)
	defer os.Remove(binaryPath)

	// Create test config
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, ".clyde")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("Failed to create config dir: %v", err)
	}

	configPath := filepath.Join(configDir, "config")
	createTestConfig(t, configPath)

	// Run CLI mode with direct string
	cmd := exec.Command(binaryPath, "What is 2+2?")
	cmd.Env = append(os.Environ(), "HOME="+tmpDir)

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("CLI mode failed: %v\nOutput: %s", err, output)
	}

	outputStr := string(output)
	t.Logf("CLI output: %s", outputStr)

	// Verify response contains expected answer
	if !strings.Contains(strings.ToLower(outputStr), "4") {
		t.Errorf("Expected output to contain '4', got: %s", outputStr)
	}
}

// TestCLIMode_FromFile tests CLI mode reading from file
func TestCLIMode_FromFile(t *testing.T) {
	// Build the binary for testing
	binaryPath := buildTestBinary(t)
	defer os.Remove(binaryPath)

	// Create test config
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, ".clyde")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("Failed to create config dir: %v", err)
	}

	configPath := filepath.Join(configDir, "config")
	createTestConfig(t, configPath)

	// Create prompt file
	promptFile := filepath.Join(tmpDir, "prompt.txt")
	promptContent := "What is 5+3?"
	if err := os.WriteFile(promptFile, []byte(promptContent), 0644); err != nil {
		t.Fatalf("Failed to create prompt file: %v", err)
	}

	// Run CLI mode with -f flag
	cmd := exec.Command(binaryPath, "-f", promptFile)
	cmd.Env = append(os.Environ(), "HOME="+tmpDir)

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("CLI mode failed: %v\nOutput: %s", err, output)
	}

	outputStr := string(output)
	t.Logf("CLI output: %s", outputStr)

	// Verify response contains expected answer
	if !strings.Contains(strings.ToLower(outputStr), "8") {
		t.Errorf("Expected output to contain '8', got: %s", outputStr)
	}
}

// TestCLIMode_FromStdin tests CLI mode reading from stdin
func TestCLIMode_FromStdin(t *testing.T) {
	// Build the binary for testing
	binaryPath := buildTestBinary(t)
	defer os.Remove(binaryPath)

	// Create test config
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, ".clyde")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("Failed to create config dir: %v", err)
	}

	configPath := filepath.Join(configDir, "config")
	createTestConfig(t, configPath)

	// Run CLI mode with stdin
	cmd := exec.Command(binaryPath)
	cmd.Env = append(os.Environ(), "HOME="+tmpDir)

	// Write prompt to stdin
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("Failed to create stdin pipe: %v", err)
	}

	go func() {
		defer stdin.Close()
		io.WriteString(stdin, "What is 10-3?")
	}()

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("CLI mode failed: %v\nOutput: %s", err, output)
	}

	outputStr := string(output)
	t.Logf("CLI output: %s", outputStr)

	// Verify response contains expected answer
	if !strings.Contains(strings.ToLower(outputStr), "7") {
		t.Errorf("Expected output to contain '7', got: %s", outputStr)
	}
}

// TestCLIMode_EmptyPrompt tests CLI mode with empty prompt
func TestCLIMode_EmptyPrompt(t *testing.T) {
	// Build the binary for testing
	binaryPath := buildTestBinary(t)
	defer os.Remove(binaryPath)

	// Create test config
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, ".clyde")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("Failed to create config dir: %v", err)
	}

	configPath := filepath.Join(configDir, "config")
	createTestConfig(t, configPath)

	// Run CLI mode with empty string
	cmd := exec.Command(binaryPath, "")
	cmd.Env = append(os.Environ(), "HOME="+tmpDir)

	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Errorf("Expected CLI mode to fail with empty prompt, but it succeeded")
	}

	outputStr := string(output)
	if !strings.Contains(outputStr, "Empty prompt") {
		t.Errorf("Expected error message about empty prompt, got: %s", outputStr)
	}
}

// TestCLIMode_FileNotFound tests CLI mode with non-existent file
func TestCLIMode_FileNotFound(t *testing.T) {
	// Build the binary for testing
	binaryPath := buildTestBinary(t)
	defer os.Remove(binaryPath)

	// Create test config
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, ".clyde")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("Failed to create config dir: %v", err)
	}

	configPath := filepath.Join(configDir, "config")
	createTestConfig(t, configPath)

	// Run CLI mode with non-existent file
	cmd := exec.Command(binaryPath, "-f", "/nonexistent/prompt.txt")
	cmd.Env = append(os.Environ(), "HOME="+tmpDir)

	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Errorf("Expected CLI mode to fail with non-existent file, but it succeeded")
	}

	outputStr := string(output)
	if !strings.Contains(outputStr, "Error reading prompt file") {
		t.Errorf("Expected error message about file not found, got: %s", outputStr)
	}
}

// TestCLIMode_MissingFileArg tests CLI mode with -f but no file path
func TestCLIMode_MissingFileArg(t *testing.T) {
	// Build the binary for testing
	binaryPath := buildTestBinary(t)
	defer os.Remove(binaryPath)

	// Create test config
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, ".clyde")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("Failed to create config dir: %v", err)
	}

	configPath := filepath.Join(configDir, "config")
	createTestConfig(t, configPath)

	// Run CLI mode with -f but no file path
	cmd := exec.Command(binaryPath, "-f")
	cmd.Env = append(os.Environ(), "HOME="+tmpDir)

	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Errorf("Expected CLI mode to fail with missing file argument, but it succeeded")
	}

	outputStr := string(output)
	if !strings.Contains(outputStr, "-f requires a file path") {
		t.Errorf("Expected error message about missing file path, got: %s", outputStr)
	}
}

// TestCLIMode_MultiWordPrompt tests CLI mode with multi-word prompt
func TestCLIMode_MultiWordPrompt(t *testing.T) {
	// Build the binary for testing
	binaryPath := buildTestBinary(t)
	defer os.Remove(binaryPath)

	// Create test config
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, ".clyde")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("Failed to create config dir: %v", err)
	}

	configPath := filepath.Join(configDir, "config")
	createTestConfig(t, configPath)

	// Run CLI mode with multi-word prompt (no quotes needed in exec.Command)
	cmd := exec.Command(binaryPath, "What", "is", "the", "sum", "of", "1", "and", "1?")
	cmd.Env = append(os.Environ(), "HOME="+tmpDir)

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("CLI mode failed: %v\nOutput: %s", err, output)
	}

	outputStr := string(output)
	t.Logf("CLI output: %s", outputStr)

	// Verify response contains expected answer
	if !strings.Contains(strings.ToLower(outputStr), "2") {
		t.Errorf("Expected output to contain '2', got: %s", outputStr)
	}
}

// TestCLIMode_ExitCodes tests CLI mode exit codes
func TestCLIMode_ExitCodes(t *testing.T) {
	// Build the binary for testing
	binaryPath := buildTestBinary(t)
	defer os.Remove(binaryPath)

	// Create test config
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, ".clyde")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("Failed to create config dir: %v", err)
	}

	configPath := filepath.Join(configDir, "config")
	createTestConfig(t, configPath)

	t.Run("success exit code 0", func(t *testing.T) {
		cmd := exec.Command(binaryPath, "Hello!")
		cmd.Env = append(os.Environ(), "HOME="+tmpDir)

		err := cmd.Run()
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				t.Errorf("Expected exit code 0 on success, got %d", exitErr.ExitCode())
			} else {
				t.Errorf("Command failed: %v", err)
			}
		}
	})

	t.Run("error exit code 1", func(t *testing.T) {
		cmd := exec.Command(binaryPath, "")
		cmd.Env = append(os.Environ(), "HOME="+tmpDir)

		err := cmd.Run()
		if err == nil {
			t.Error("Expected non-zero exit code on error, got success")
		} else if exitErr, ok := err.(*exec.ExitError); ok {
			if exitErr.ExitCode() != 1 {
				t.Errorf("Expected exit code 1 on error, got %d", exitErr.ExitCode())
			}
		}
	})
}

// buildTestBinary builds the clyde binary for testing
func buildTestBinary(t *testing.T) string {
	t.Helper()

	binaryPath := filepath.Join(t.TempDir(), "clyde-test")

	// Build the binary
	cmd := exec.Command("go", "build", "-o", binaryPath, "../")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to build binary: %v\nOutput: %s", err, output)
	}

	return binaryPath
}

// createTestConfig creates a test configuration file with API keys
func createTestConfig(t *testing.T, configPath string) {
	t.Helper()

	// Read API keys from environment or use test keys
	apiKey := os.Getenv("TS_AGENT_API_KEY")
	if apiKey == "" {
		t.Skip("Skipping CLI mode tests: TS_AGENT_API_KEY not set")
	}

	braveKey := os.Getenv("BRAVE_SEARCH_API_KEY")

	configContent := "TS_AGENT_API_KEY=" + apiKey + "\n"
	if braveKey != "" {
		configContent += "BRAVE_SEARCH_API_KEY=" + braveKey + "\n"
	}

	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to create test config: %v", err)
	}
}
