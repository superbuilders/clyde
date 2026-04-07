package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/this-is-alpha-iota/clyde/agent"
	"github.com/this-is-alpha-iota/clyde/api"
	"github.com/this-is-alpha-iota/clyde/loglevel"
	"github.com/this-is-alpha-iota/clyde/mcp"
	"github.com/this-is-alpha-iota/clyde/prompts"
	"github.com/this-is-alpha-iota/clyde/tools"
)

// --- Story 4: Registration Tests (unit, no MCP server needed) ---

func TestMCPPlaywrightToolRegistration(t *testing.T) {
	// Verify that PlaywrightTools returns correct tools for registration
	apiTools, err := mcp.PlaywrightTools()
	if err != nil {
		t.Fatalf("PlaywrightTools: %v", err)
	}

	if len(apiTools) != 21 {
		t.Fatalf("Expected 21 tools, got %d", len(apiTools))
	}

	// Verify tool format matches what the Anthropic API expects
	for _, tool := range apiTools {
		if tool.Name == "" {
			t.Error("Tool has empty name")
		}
		if !strings.HasPrefix(tool.Name, "mcp_playwright_") {
			t.Errorf("Tool %q missing mcp_playwright_ prefix", tool.Name)
		}
		if tool.Description == "" {
			t.Errorf("Tool %q has empty description", tool.Name)
		}
		if tool.InputSchema == nil {
			t.Errorf("Tool %q has nil InputSchema", tool.Name)
		}

		// InputSchema should be a map (JSON object)
		schemaMap, ok := tool.InputSchema.(map[string]interface{})
		if !ok {
			t.Errorf("Tool %q InputSchema is not a map, got %T", tool.Name, tool.InputSchema)
			continue
		}
		// Should have a "type" field
		if _, hasType := schemaMap["type"]; !hasType {
			t.Errorf("Tool %q InputSchema missing 'type' field", tool.Name)
		}
	}
}

func TestMCPToolsNoCollisionWithBuiltins(t *testing.T) {
	// Verify MCP tool names don't collide with built-in tools
	builtinTools := tools.GetAllTools()
	builtinNames := make(map[string]bool)
	for _, tool := range builtinTools {
		builtinNames[tool.Name] = true
	}

	mcpTools, err := mcp.PlaywrightTools()
	if err != nil {
		t.Fatalf("PlaywrightTools: %v", err)
	}

	for _, tool := range mcpTools {
		if builtinNames[tool.Name] {
			t.Errorf("MCP tool %q collides with built-in tool", tool.Name)
		}
	}
}

func TestMCPToolRegistrationWithServer(t *testing.T) {
	// Test that RegisterPlaywrightTools adds tools to the registry
	server := mcp.NewPlaywrightServer("--headless")
	defer server.Close()

	// Count existing tools
	beforeCount := len(tools.GetAllTools())

	err := mcp.RegisterPlaywrightTools(server)
	if err != nil {
		t.Fatalf("RegisterPlaywrightTools: %v", err)
	}

	afterCount := len(tools.GetAllTools())
	added := afterCount - beforeCount
	if added != 21 {
		t.Errorf("Expected 21 new tools registered, got %d", added)
	}

	// Verify we can look up a registered MCP tool
	reg, err := tools.GetTool("mcp_playwright_browser_navigate")
	if err != nil {
		t.Fatalf("GetTool(browser_navigate): %v", err)
	}
	if reg.Execute == nil {
		t.Error("Execute function is nil")
	}
	if reg.Display == nil {
		t.Error("Display function is nil")
	}

	// Verify display function works
	display := reg.Display(map[string]interface{}{
		"url": "https://example.com",
	})
	if !strings.Contains(display, "Browser:") {
		t.Errorf("Display = %q, expected to contain 'Browser:'", display)
	}
	if !strings.Contains(display, "https://example.com") {
		t.Errorf("Display = %q, expected to contain URL", display)
	}

	// Cleanup: remove MCP tools from registry to not affect other tests
	cleanupMCPTools(t)
}

func TestMCPDisplayMessages(t *testing.T) {
	server := mcp.NewPlaywrightServer("--headless")
	defer server.Close()
	mcp.RegisterPlaywrightTools(server)
	defer cleanupMCPTools(t)

	tests := []struct {
		toolName string
		input    map[string]interface{}
		contains string
	}{
		{
			"mcp_playwright_browser_navigate",
			map[string]interface{}{"url": "https://example.com"},
			"navigate https://example.com",
		},
		{
			"mcp_playwright_browser_click",
			map[string]interface{}{"element": "Submit button"},
			"click Submit button",
		},
		{
			"mcp_playwright_browser_snapshot",
			map[string]interface{}{},
			"snapshot capturing page",
		},
		{
			"mcp_playwright_browser_type",
			map[string]interface{}{"text": "hello"},
			"type \"hello\"",
		},
	}

	for _, tc := range tests {
		t.Run(tc.toolName, func(t *testing.T) {
			reg, err := tools.GetTool(tc.toolName)
			if err != nil {
				t.Fatalf("GetTool: %v", err)
			}
			display := reg.Display(tc.input)
			if !strings.Contains(strings.ToLower(display), strings.ToLower(tc.contains)) {
				t.Errorf("Display = %q, expected to contain %q", display, tc.contains)
			}
		})
	}
}

// --- Story 5: Integration Test ---

func TestMCPPlaywrightIntegration(t *testing.T) {
	// Prerequisites: npx and API key
	if _, err := exec.LookPath("npx"); err != nil {
		t.Skip("npx not available — skipping Playwright MCP integration test")
	}
	apiKey := os.Getenv("TS_AGENT_API_KEY")
	if apiKey == "" {
		t.Skip("TS_AGENT_API_KEY not set — skipping integration test")
	}

	// 1. Start a simple local HTTP server with a known page
	htmlContent := `<!DOCTYPE html>
<html>
<head><title>Test Page for Clyde MCP</title></head>
<body>
<h1>Integration Test Page</h1>
<p>This page contains a unique marker: CLYDE_MCP_TEST_MARKER_42</p>
<a href="/about">About Link</a>
</body>
</html>`

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, htmlContent)
	})

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to start test server: %v", err)
	}

	httpServer := &http.Server{Handler: mux}
	go httpServer.Serve(listener)
	defer httpServer.Close()

	port := listener.Addr().(*net.TCPAddr).Port
	testURL := fmt.Sprintf("http://127.0.0.1:%d", port)

	// Give the server a moment to be ready
	time.Sleep(100 * time.Millisecond)

	// 2. Configure clyde with Playwright MCP
	mcpServer := mcp.NewPlaywrightServer("--headless")
	defer mcpServer.Close()

	if err := mcp.RegisterPlaywrightTools(mcpServer); err != nil {
		t.Fatalf("RegisterPlaywrightTools: %v", err)
	}
	defer cleanupMCPTools(t)

	apiClient := api.NewClient(apiKey, "https://api.anthropic.com/v1/messages", "claude-sonnet-4-5-20250929", 4096)

	var progressMessages []string
	agentInstance := agent.NewAgent(
		apiClient,
		prompts.SystemPrompt,
		agent.WithLogLevel(loglevel.Normal),
		agent.WithProgressCallback(func(lvl loglevel.Level, msg string) {
			progressMessages = append(progressMessages, msg)
			t.Logf("[%s] %s", lvl, truncateStr(msg, 120))
		}),
	)

	// 3. Send a prompt that requires browser navigation
	prompt := fmt.Sprintf(
		"Use the mcp_playwright_browser_navigate tool to go to %s and then use mcp_playwright_browser_snapshot to read the page. Tell me what the unique marker text says. Respond with JUST the marker text.",
		testURL,
	)

	response, err := agentInstance.HandleMessage(prompt)
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}

	t.Logf("Agent response: %s", truncateStr(response, 500))

	// 4. Verify the agent used Playwright tools
	usedBrowserNavigate := false
	usedBrowserSnapshot := false
	for _, msg := range progressMessages {
		if strings.Contains(msg, "Browser:") && strings.Contains(msg, "navigate") {
			usedBrowserNavigate = true
		}
		if strings.Contains(msg, "Browser:") && strings.Contains(msg, "snapshot") {
			usedBrowserSnapshot = true
		}
	}
	if !usedBrowserNavigate {
		t.Error("Expected agent to use mcp_playwright_browser_navigate")
	}
	if !usedBrowserSnapshot {
		t.Error("Expected agent to use mcp_playwright_browser_snapshot")
	}

	// 5. Verify the response contains content from the page
	if !strings.Contains(response, "CLYDE_MCP_TEST_MARKER_42") {
		t.Errorf("Response doesn't contain the test marker. Response: %s",
			truncateStr(response, 300))
	}
}

// TestMCPPlaywrightBrowserStatePersists verifies that browser state (tabs,
// page context) persists across multiple tool calls within a session.
func TestMCPPlaywrightBrowserStatePersists(t *testing.T) {
	if _, err := exec.LookPath("npx"); err != nil {
		t.Skip("npx not available")
	}
	apiKey := os.Getenv("TS_AGENT_API_KEY")
	if apiKey == "" {
		t.Skip("TS_AGENT_API_KEY not set")
	}

	// Start HTTP server with two pages
	mux := http.NewServeMux()
	mux.HandleFunc("/page1", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<html><body><h1>Page One</h1><a href="/page2">Go to page 2</a></body></html>`)
	})
	mux.HandleFunc("/page2", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<html><body><h1>Page Two</h1><p>STATE_PERSISTED_OK</p></body></html>`)
	})

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	httpServer := &http.Server{Handler: mux}
	go httpServer.Serve(listener)
	defer httpServer.Close()

	port := listener.Addr().(*net.TCPAddr).Port

	// Setup MCP
	mcpServer := mcp.NewPlaywrightServer("--headless")
	defer mcpServer.Close()
	mcp.RegisterPlaywrightTools(mcpServer)
	defer cleanupMCPTools(t)

	apiClient := api.NewClient(apiKey, "https://api.anthropic.com/v1/messages", "claude-sonnet-4-5-20250929", 4096)

	agentInstance := agent.NewAgent(
		apiClient,
		prompts.SystemPrompt,
		agent.WithLogLevel(loglevel.Quiet),
		agent.WithProgressCallback(func(lvl loglevel.Level, msg string) {
			t.Logf("[%s] %s", lvl, truncateStr(msg, 120))
		}),
	)

	// Navigate to page1, then navigate to page2, read page2 content
	prompt := fmt.Sprintf(
		"Using Playwright browser tools: 1) Navigate to http://127.0.0.1:%d/page1, "+
			"2) Then navigate to http://127.0.0.1:%d/page2, "+
			"3) Take a snapshot. Tell me what marker text is on page2. Just state the marker.",
		port, port,
	)

	response, err := agentInstance.HandleMessage(prompt)
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}

	t.Logf("Response: %s", truncateStr(response, 300))

	if !strings.Contains(response, "STATE_PERSISTED_OK") {
		t.Errorf("Expected STATE_PERSISTED_OK in response, got: %s", truncateStr(response, 300))
	}
}

// TestMCPPlaywrightDisabledByDefault verifies that without MCP_PLAYWRIGHT=true,
// no MCP tools appear in the registry.
func TestMCPPlaywrightDisabledByDefault(t *testing.T) {
	allTools := tools.GetAllTools()
	for _, tool := range allTools {
		if strings.HasPrefix(tool.Name, "mcp_playwright_") {
			t.Errorf("Found MCP tool %q but MCP should be disabled by default", tool.Name)
		}
	}
}

// TestMCPPlaywrightProcessCleanup verifies the Playwright subprocess is killed on Close.
func TestMCPPlaywrightProcessCleanup(t *testing.T) {
	if _, err := exec.LookPath("npx"); err != nil {
		t.Skip("npx not available")
	}

	server := mcp.NewPlaywrightServer("--headless")

	// Start the server by ensuring it runs
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.EnsureRunning(ctx); err != nil {
		t.Skipf("Failed to start: %v", err)
	}

	// Close the server
	server.Close()

	// After close, calling tools should fail
	_, err := server.CallTool(ctx, "browser_navigate", map[string]interface{}{
		"url": "data:text/html,test",
	})
	if err == nil {
		t.Error("Expected error calling tool after Close")
	}
}

// --- Helpers ---

func cleanupMCPTools(t *testing.T) {
	t.Helper()
	for name := range tools.Registry {
		if strings.HasPrefix(name, "mcp_playwright_") {
			delete(tools.Registry, name)
		}
	}
}

func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
