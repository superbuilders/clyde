package main

import (
	"os"
	"path/filepath"
	"testing"

	"claude-repl/config"
)

// TestConfigLoadFromCurrentDirectory tests loading .env from current directory
func TestConfigLoadFromCurrentDirectory(t *testing.T) {
	// Save and clear environment variables
	oldAPIKey := os.Getenv("TS_AGENT_API_KEY")
	oldBraveKey := os.Getenv("BRAVE_SEARCH_API_KEY")
	defer func() {
		os.Setenv("TS_AGENT_API_KEY", oldAPIKey)
		os.Setenv("BRAVE_SEARCH_API_KEY", oldBraveKey)
	}()
	os.Unsetenv("TS_AGENT_API_KEY")
	os.Unsetenv("BRAVE_SEARCH_API_KEY")

	// Create a temporary .env file
	tmpDir := t.TempDir()
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWd)

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}

	envContent := "TS_AGENT_API_KEY=test-api-key-123\nBRAVE_SEARCH_API_KEY=test-brave-key-456\n"
	if err := os.WriteFile(".env", []byte(envContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Load config
	cfg, err := config.Load()
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

// TestConfigLoadFromHomeDirectory tests loading from ~/.claude-repl/config
func TestConfigLoadFromHomeDirectory(t *testing.T) {
	// Save and clear environment variables
	oldAPIKey := os.Getenv("TS_AGENT_API_KEY")
	oldBraveKey := os.Getenv("BRAVE_SEARCH_API_KEY")
	defer func() {
		os.Setenv("TS_AGENT_API_KEY", oldAPIKey)
		os.Setenv("BRAVE_SEARCH_API_KEY", oldBraveKey)
	}()
	os.Unsetenv("TS_AGENT_API_KEY")
	os.Unsetenv("BRAVE_SEARCH_API_KEY")

	// Create a temporary directory to act as home
	tmpHome := t.TempDir()
	oldHome := os.Getenv("HOME")
	defer os.Setenv("HOME", oldHome)
	os.Setenv("HOME", tmpHome)

	// Create ~/.claude-repl/config
	claudeReplDir := filepath.Join(tmpHome, ".claude-repl")
	if err := os.MkdirAll(claudeReplDir, 0755); err != nil {
		t.Fatal(err)
	}

	configPath := filepath.Join(claudeReplDir, "config")
	envContent := "TS_AGENT_API_KEY=home-api-key-789\nBRAVE_SEARCH_API_KEY=home-brave-key-012\n"
	if err := os.WriteFile(configPath, []byte(envContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Change to a different directory (not one with .env)
	tmpDir := t.TempDir()
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWd)
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}

	// Load config
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify
	if cfg.APIKey != "home-api-key-789" {
		t.Errorf("Expected APIKey 'home-api-key-789', got '%s'", cfg.APIKey)
	}
	if cfg.BraveSearchAPIKey != "home-brave-key-012" {
		t.Errorf("Expected BraveSearchAPIKey 'home-brave-key-012', got '%s'", cfg.BraveSearchAPIKey)
	}
}

// TestConfigLoadFromLegacyHomeFile tests loading from ~/.claude-repl (direct file)
func TestConfigLoadFromLegacyHomeFile(t *testing.T) {
	// Save and clear environment variables
	oldAPIKey := os.Getenv("TS_AGENT_API_KEY")
	oldBraveKey := os.Getenv("BRAVE_SEARCH_API_KEY")
	defer func() {
		os.Setenv("TS_AGENT_API_KEY", oldAPIKey)
		os.Setenv("BRAVE_SEARCH_API_KEY", oldBraveKey)
	}()
	os.Unsetenv("TS_AGENT_API_KEY")
	os.Unsetenv("BRAVE_SEARCH_API_KEY")

	// Create a temporary directory to act as home
	tmpHome := t.TempDir()
	oldHome := os.Getenv("HOME")
	defer os.Setenv("HOME", oldHome)
	os.Setenv("HOME", tmpHome)

	// Create ~/.claude-repl as a file (legacy format)
	legacyPath := filepath.Join(tmpHome, ".claude-repl")
	envContent := "TS_AGENT_API_KEY=legacy-api-key-abc\n"
	if err := os.WriteFile(legacyPath, []byte(envContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Change to a different directory
	tmpDir := t.TempDir()
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWd)
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}

	// Load config
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify
	if cfg.APIKey != "legacy-api-key-abc" {
		t.Errorf("Expected APIKey 'legacy-api-key-abc', got '%s'", cfg.APIKey)
	}
}

// TestConfigLoadFromENVPATH tests loading from ENV_PATH override
func TestConfigLoadFromENVPATH(t *testing.T) {
	// Save and clear environment variables
	oldAPIKey := os.Getenv("TS_AGENT_API_KEY")
	oldBraveKey := os.Getenv("BRAVE_SEARCH_API_KEY")
	defer func() {
		os.Setenv("TS_AGENT_API_KEY", oldAPIKey)
		os.Setenv("BRAVE_SEARCH_API_KEY", oldBraveKey)
	}()
	os.Unsetenv("TS_AGENT_API_KEY")
	os.Unsetenv("BRAVE_SEARCH_API_KEY")

	// Create a custom config file
	tmpDir := t.TempDir()
	customConfigPath := filepath.Join(tmpDir, "custom.env")
	envContent := "TS_AGENT_API_KEY=custom-api-key-xyz\n"
	if err := os.WriteFile(customConfigPath, []byte(envContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Set ENV_PATH
	oldEnvPath := os.Getenv("ENV_PATH")
	defer os.Setenv("ENV_PATH", oldEnvPath)
	os.Setenv("ENV_PATH", customConfigPath)

	// Load config
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify
	if cfg.APIKey != "custom-api-key-xyz" {
		t.Errorf("Expected APIKey 'custom-api-key-xyz', got '%s'", cfg.APIKey)
	}
}

// TestConfigPriorityOrder tests that config sources are checked in the correct order
func TestConfigPriorityOrder(t *testing.T) {
	// Save and clear environment variables
	oldAPIKey := os.Getenv("TS_AGENT_API_KEY")
	oldBraveKey := os.Getenv("BRAVE_SEARCH_API_KEY")
	defer func() {
		os.Setenv("TS_AGENT_API_KEY", oldAPIKey)
		os.Setenv("BRAVE_SEARCH_API_KEY", oldBraveKey)
	}()
	os.Unsetenv("TS_AGENT_API_KEY")
	os.Unsetenv("BRAVE_SEARCH_API_KEY")

	// Setup multiple config sources
	tmpHome := t.TempDir()
	oldHome := os.Getenv("HOME")
	defer os.Setenv("HOME", oldHome)
	os.Setenv("HOME", tmpHome)

	// Create ~/.claude-repl/config (lower priority)
	claudeReplDir := filepath.Join(tmpHome, ".claude-repl")
	if err := os.MkdirAll(claudeReplDir, 0755); err != nil {
		t.Fatal(err)
	}
	homeConfigPath := filepath.Join(claudeReplDir, "config")
	if err := os.WriteFile(homeConfigPath, []byte("TS_AGENT_API_KEY=home-key\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create .env in current directory (higher priority)
	tmpDir := t.TempDir()
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWd)
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(".env", []byte("TS_AGENT_API_KEY=local-key\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Load config - should use local .env (higher priority)
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify local .env takes priority
	if cfg.APIKey != "local-key" {
		t.Errorf("Expected local .env to take priority with APIKey 'local-key', got '%s'", cfg.APIKey)
	}
}

// TestConfigNoFileFound tests helpful error message when no config file exists
func TestConfigNoFileFound(t *testing.T) {
	// Create a temporary directory with no config files
	tmpHome := t.TempDir()
	oldHome := os.Getenv("HOME")
	defer os.Setenv("HOME", oldHome)
	os.Setenv("HOME", tmpHome)

	tmpDir := t.TempDir()
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWd)
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}

	// Clear ENV_PATH
	oldEnvPath := os.Getenv("ENV_PATH")
	defer os.Setenv("ENV_PATH", oldEnvPath)
	os.Unsetenv("ENV_PATH")

	// Try to load config
	_, err = config.Load()
	if err == nil {
		t.Fatal("Expected error when no config file exists, got nil")
	}

	// Verify error message contains helpful instructions
	errMsg := err.Error()
	if !contains(errMsg, "No configuration file found") {
		t.Error("Error message should mention 'No configuration file found'")
	}
	if !contains(errMsg, "mkdir -p") {
		t.Error("Error message should include mkdir command")
	}
	if !contains(errMsg, "TS_AGENT_API_KEY") {
		t.Error("Error message should mention TS_AGENT_API_KEY")
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

	// Create .env without TS_AGENT_API_KEY
	tmpDir := t.TempDir()
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWd)
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}

	envContent := "BRAVE_SEARCH_API_KEY=test-brave-key\n"
	if err := os.WriteFile(".env", []byte(envContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Try to load config
	_, err = config.Load()
	if err == nil {
		t.Fatal("Expected error when TS_AGENT_API_KEY is missing, got nil")
	}

	// Verify error message
	errMsg := err.Error()
	if !contains(errMsg, "TS_AGENT_API_KEY not found") {
		t.Error("Error message should mention missing TS_AGENT_API_KEY")
	}
}

// TestConfigInvalidENVPATH tests error when ENV_PATH points to non-existent file
func TestConfigInvalidENVPATH(t *testing.T) {
	// Set ENV_PATH to non-existent file
	oldEnvPath := os.Getenv("ENV_PATH")
	defer os.Setenv("ENV_PATH", oldEnvPath)
	os.Setenv("ENV_PATH", "/non/existent/path/config")

	// Try to load config
	_, err := config.Load()
	if err == nil {
		t.Fatal("Expected error when ENV_PATH points to non-existent file, got nil")
	}

	// Verify error message
	errMsg := err.Error()
	if !contains(errMsg, "ENV_PATH is set") {
		t.Error("Error message should mention ENV_PATH")
	}
	if !contains(errMsg, "does not exist") {
		t.Error("Error message should mention file doesn't exist")
	}
}

// TestConfigDefaultValues tests that config has proper default values
func TestConfigDefaultValues(t *testing.T) {
	// Create minimal config with just API key
	tmpDir := t.TempDir()
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWd)
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}

	envContent := "TS_AGENT_API_KEY=test-key\n"
	if err := os.WriteFile(".env", []byte(envContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Load config
	cfg, err := config.Load()
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
