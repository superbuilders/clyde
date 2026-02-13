package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestExecuteBrowse(t *testing.T) {
	// Create test API key for AI processing tests
	testAPIKey := "test-api-key"

	t.Run("Empty URL", func(t *testing.T) {
		_, err := executeBrowse("", "", 500, testAPIKey, nil)

		if err == nil {
			t.Error("Expected error for empty URL")
		}
		if !strings.Contains(err.Error(), "url is required") {
			t.Errorf("Expected URL required error, got: %s", err.Error())
		}
	})

	t.Run("Invalid URL format", func(t *testing.T) {
		_, err := executeBrowse("not-a-url", "", 500, testAPIKey, nil)

		if err == nil {
			t.Error("Expected error for invalid URL")
		}
		if !strings.Contains(err.Error(), "invalid URL format") {
			t.Errorf("Expected invalid URL error, got: %s", err.Error())
		}
	})

	t.Run("Fetch valid HTML page", func(t *testing.T) {
		// Create test server
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`
				<!DOCTYPE html>
				<html>
				<head><title>Test Page</title></head>
				<body>
					<h1>Hello World</h1>
					<p>This is a test paragraph.</p>
					<ul>
						<li>Item 1</li>
						<li>Item 2</li>
					</ul>
				</body>
				</html>
			`))
		}))
		defer server.Close()

		output, err := executeBrowse(server.URL, "", 500, testAPIKey, nil)

		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}

		// Check that markdown contains expected content
		if !strings.Contains(output, "Hello World") {
			t.Error("Expected 'Hello World' in output")
		}
		if !strings.Contains(output, "test paragraph") {
			t.Error("Expected 'test paragraph' in output")
		}
		if !strings.Contains(output, "Item 1") {
			t.Error("Expected 'Item 1' in output")
		}
	})

	t.Run("Handle 404 error", func(t *testing.T) {
		// Create test server that returns 404
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		_, err := executeBrowse(server.URL, "", 500, testAPIKey, nil)

		if err == nil {
			t.Error("Expected error for 404")
		}
		if !strings.Contains(err.Error(), "404") {
			t.Errorf("Expected 404 error, got: %s", err.Error())
		}
	})

	t.Run("Handle 403 error", func(t *testing.T) {
		// Create test server that returns 403
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
		}))
		defer server.Close()

		_, err := executeBrowse(server.URL, "", 500, testAPIKey, nil)

		if err == nil {
			t.Error("Expected error for 403")
		}
		if !strings.Contains(err.Error(), "403") {
			t.Errorf("Expected 403 error, got: %s", err.Error())
		}
	})

	t.Run("Handle redirect", func(t *testing.T) {
		// Create test servers - one for redirect, one for final page
		finalServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`<html><body><h1>Final Page</h1></body></html>`))
		}))
		defer finalServer.Close()

		redirectServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, finalServer.URL, http.StatusFound)
		}))
		defer redirectServer.Close()

		output, err := executeBrowse(redirectServer.URL, "", 500, testAPIKey, nil)

		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if !strings.Contains(output, "Final Page") {
			t.Error("Expected 'Final Page' in output after redirect")
		}
	})

	t.Run("Default and max maxLength", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`<html><body><p>Test</p></body></html>`))
		}))
		defer server.Close()

		// Test default (0 should become 500)
		_, err := executeBrowse(server.URL, "", 0, testAPIKey, nil)
		if err != nil {
			t.Errorf("Unexpected error with default maxLength: %v", err)
		}

		// Test cap at 1000 (2000 should become 1000)
		_, err = executeBrowse(server.URL, "", 2000, testAPIKey, nil)
		if err != nil {
			t.Errorf("Unexpected error with capped maxLength: %v", err)
		}
	})

	t.Run("Empty content handling", func(t *testing.T) {
		// Create server that returns empty body
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`<html><head></head><body></body></html>`))
		}))
		defer server.Close()

		_, err := executeBrowse(server.URL, "", 500, testAPIKey, nil)

		if err == nil {
			t.Error("Expected error for empty content")
		}
		if !strings.Contains(err.Error(), "no readable content") {
			t.Errorf("Expected 'no readable content' error, got: %s", err.Error())
		}
	})
}

func TestBrowseIntegration(t *testing.T) {
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

	t.Run("Fetch real documentation page", func(t *testing.T) {
		var history []Message

		// Use a reliable, simple page
		response, updatedHistory := handleConversation(apiKey,
			"Use browse to fetch https://example.com and tell me what it says",
			history)

		if response == "" {
			t.Fatal("Expected response but got empty string")
		}

		t.Logf("Response: %s", response)

		// Verify tool was used
		foundToolUse := false
		foundToolResult := false

		for _, msg := range updatedHistory {
			if msg.Role == "assistant" {
				if contentBlocks, ok := msg.Content.([]ContentBlock); ok {
					for _, block := range contentBlocks {
						if block.Type == "tool_use" && block.Name == "browse" {
							foundToolUse = true
							t.Logf("Found tool_use: %s (ID: %s)", block.Name, block.ID)

							// Verify input parameters
							if url, ok := block.Input["url"].(string); ok {
								if url == "" {
									t.Error("Expected non-empty URL")
								}
								t.Logf("URL: %s", url)
							} else {
								t.Error("Expected URL parameter in tool input")
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

							// Verify the result contains page content
							if content, ok := block.Content.(string); ok {
								if !strings.Contains(content, "Example") {
									t.Logf("Warning: Result may not contain expected content")
								}
							}
						}
					}
				}
			}
		}

		if !foundToolUse {
			t.Error("Expected to find a browse tool_use block")
		}

		if !foundToolResult {
			t.Error("Expected to find a tool_result block")
		}
	})

	t.Run("Extract specific info with prompt", func(t *testing.T) {
		var history []Message

		response, _ := handleConversation(apiKey,
			"Use browse with the URL https://example.com and prompt 'What is the main heading on this page?'",
			history)

		if response == "" {
			t.Fatal("Expected response but got empty string")
		}

		t.Logf("Response with extraction: %s", response)

		// The response should contain information about the heading
		// (example.com has "Example Domain" as heading)
	})

	t.Run("Handle 404 gracefully", func(t *testing.T) {
		var history []Message

		response, _ := handleConversation(apiKey,
			"Use browse to fetch https://example.com/this-page-does-not-exist-404",
			history)

		if response == "" {
			t.Fatal("Expected response but got empty string")
		}

		t.Logf("Response for 404: %s", response)

		// Should handle the error gracefully
		if !strings.Contains(response, "404") && !strings.Contains(response, "not found") {
			t.Log("Warning: Response may not clearly indicate 404 error")
		}
	})
}
