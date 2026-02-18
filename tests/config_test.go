package main

import (
	"os"
	"path/filepath"
	"testing"

	"claude-repl/config"
)

// TestConfigLoadFromFile tests loading config from a specific file
func TestConfigLoadFromFile(t *testing.T) {
	// Save and clear environment variables
	oldAPIKey := os.Getenv("TS_AGENT_API_KEY")
	oldBraveKey := os.Getenv("BRAVE_SEARCH_API_KEY")
	defer func() {
		os.Setenv("TS_AGENT_API_KEY", oldAPIKey)
		os.Setenv("BRAVE_SEARCH_API_KEY", oldBraveKey)
	}()
	os.Unsetenv("TS_AGENT_API_KEY")
	os.Unsetenv("BRAVE_SEARCH_API_KEY")

	// Create a temporary config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".env")
	envContent := "TS_AGENT_API_KEY=test-api-key-123\nBRAVE_SEARCH_API_KEY=test-brave-key-456\n"
	if err := os.WriteFile(configPath, []byte(envContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Load config
	cfg, err := config.LoadFromFile(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify
	if cfg.APIKey != "test-api-key-123" {
		t.Errorf("Expected APIKey 'test-api-key-123', got '%s'", cfg.APIKey)
	}
	if cfg.BraveSearchAPIKey != "test-brave-key-456" {
		t.Errorf("Expected BraveSearchAPIKey 'test-brave-key-456', got '%s'", cfg.BraveSearchAPIKey)
	}
}

// TestConfigFileNotFound tests error when config file doesn't exist
func TestConfigFileNotFound(t *testing.T) {
	// Save and clear environment variables
	oldAPIKey := os.Getenv("TS_AGENT_API_KEY")
	oldBraveKey := os.Getenv("BRAVE_SEARCH_API_KEY")
	defer func() {
		os.Setenv("TS_AGENT_API_KEY", oldAPIKey)
		os.Setenv("BRAVE_SEARCH_API_KEY", oldBraveKey)
	}()
	os.Unsetenv("TS_AGENT_API_KEY")
	os.Unsetenv("BRAVE_SEARCH_API_KEY")

	// Try to load non-existent config
	_, err := config.LoadFromFile("/non/existent/config")
	if err == nil {
		t.Fatal("Expected error when config file doesn't exist, got nil")
	}

	// Verify error message mentions the file
	errMsg := err.Error()
	if !contains(errMsg, "not found") {
		t.Error("Error message should mention 'not found'")
	}
}

// TestConfigMissingAPIKey tests error when API key is missing from config
func TestConfigMissingAPIKey(t *testing.T) {
	// Save and clear environment variables
	oldAPIKey := os.Getenv("TS_AGENT_API_KEY")
	oldBraveKey := os.Getenv("BRAVE_SEARCH_API_KEY")
	defer func() {
		os.Setenv("TS_AGENT_API_KEY", oldAPIKey)
		os.Setenv("BRAVE_SEARCH_API_KEY", oldBraveKey)
	}()
	os.Unsetenv("TS_AGENT_API_KEY")
	os.Unsetenv("BRAVE_SEARCH_API_KEY")

	// Create config without TS_AGENT_API_KEY
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".env")
	envContent := "BRAVE_SEARCH_API_KEY=test-brave-key\n"
	if err := os.WriteFile(configPath, []byte(envContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Try to load config
	_, err := config.LoadFromFile(configPath)
	if err == nil {
		t.Fatal("Expected error when TS_AGENT_API_KEY is missing, got nil")
	}

	// Verify error message
	errMsg := err.Error()
	if !contains(errMsg, "TS_AGENT_API_KEY not found") {
		t.Error("Error message should mention missing TS_AGENT_API_KEY")
	}
}

// TestConfigDefaultValues tests that config has proper default values
func TestConfigDefaultValues(t *testing.T) {
	// Save and clear environment variables
	oldAPIKey := os.Getenv("TS_AGENT_API_KEY")
	oldBraveKey := os.Getenv("BRAVE_SEARCH_API_KEY")
	defer func() {
		os.Setenv("TS_AGENT_API_KEY", oldAPIKey)
		os.Setenv("BRAVE_SEARCH_API_KEY", oldBraveKey)
	}()
	os.Unsetenv("TS_AGENT_API_KEY")
	os.Unsetenv("BRAVE_SEARCH_API_KEY")

	// Create minimal config with just API key
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".env")
	envContent := "TS_AGENT_API_KEY=test-key\n"
	if err := os.WriteFile(configPath, []byte(envContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Load config
	cfg, err := config.LoadFromFile(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify default values
	if cfg.APIURL != "https://api.anthropic.com/v1/messages" {
		t.Errorf("Expected default APIURL, got '%s'", cfg.APIURL)
	}
	if cfg.ModelID != "claude-sonnet-4-5-20250929" {
		t.Errorf("Expected default ModelID, got '%s'", cfg.ModelID)
	}
	if cfg.MaxTokens != 4096 {
		t.Errorf("Expected default MaxTokens 4096, got %d", cfg.MaxTokens)
	}
}

// TestConfigOptionalBraveKey tests that Brave API key is optional
func TestConfigOptionalBraveKey(t *testing.T) {
	// Save and clear environment variables
	oldAPIKey := os.Getenv("TS_AGENT_API_KEY")
	oldBraveKey := os.Getenv("BRAVE_SEARCH_API_KEY")
	defer func() {
		os.Setenv("TS_AGENT_API_KEY", oldAPIKey)
		os.Setenv("BRAVE_SEARCH_API_KEY", oldBraveKey)
	}()
	os.Unsetenv("TS_AGENT_API_KEY")
	os.Unsetenv("BRAVE_SEARCH_API_KEY")

	// Create config without Brave API key
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".env")
	envContent := "TS_AGENT_API_KEY=test-key\n"
	if err := os.WriteFile(configPath, []byte(envContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Load config
	cfg, err := config.LoadFromFile(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify Brave key is empty (optional)
	if cfg.BraveSearchAPIKey != "" {
		t.Errorf("Expected empty BraveSearchAPIKey when not provided, got '%s'", cfg.BraveSearchAPIKey)
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) &&
		(s[:len(substr)] == substr || s[len(s)-len(substr):] == substr ||
			len(s) > len(substr)+1 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
