package main

import (
	"context"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/this-is-alpha-iota/clyde/mcp"
)

// --- Story 1: MCP Client Unit Tests ---

// TestNewClientMockServer spawns a tiny mock MCP server (a Go program that
// reads JSON-RPC from stdin and writes responses to stdout) and verifies
// the full initialize → list → call → close lifecycle.
func TestNewClientMockServer(t *testing.T) {
	// Build the mock server
	mockPath := buildMockServer(t)

	client, err := mcp.NewClient(mockPath)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Initialize
	initResult, err := client.Initialize(ctx)
	if err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if initResult.ServerInfo.Name != "mock-mcp-server" {
		t.Errorf("ServerInfo.Name = %q, want %q", initResult.ServerInfo.Name, "mock-mcp-server")
	}

	// List tools
	tools, err := client.ListTools(ctx)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(tools) != 2 {
		t.Fatalf("ListTools returned %d tools, want 2", len(tools))
	}
	if tools[0].Name != "echo" {
		t.Errorf("tools[0].Name = %q, want %q", tools[0].Name, "echo")
	}
	if tools[1].Name != "fail" {
		t.Errorf("tools[1].Name = %q, want %q", tools[1].Name, "fail")
	}

	// Call tool
	result, err := client.CallTool(ctx, "echo", map[string]interface{}{
		"message": "hello world",
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if len(result.Content) != 1 {
		t.Fatalf("CallTool result has %d content parts, want 1", len(result.Content))
	}
	if result.Content[0].Text != "hello world" {
		t.Errorf("CallTool text = %q, want %q", result.Content[0].Text, "hello world")
	}

	// Call tool that returns isError=true
	result, err = client.CallTool(ctx, "fail", nil)
	if err != nil {
		t.Fatalf("CallTool(fail): %v", err)
	}
	if !result.IsError {
		t.Error("Expected IsError=true for 'fail' tool")
	}

	// Close
	if err := client.Close(); err != nil {
		// Process.Kill returns an error on Wait — that's expected
		t.Logf("Close: %v (expected)", err)
	}
}

func TestClientRPCError(t *testing.T) {
	mockPath := buildMockServer(t)

	client, err := mcp.NewClient(mockPath)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Initialize first
	_, err = client.Initialize(ctx)
	if err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	// Call a non-existent tool — mock server returns RPC error
	_, err = client.CallTool(ctx, "nonexistent", nil)
	if err == nil {
		t.Fatal("Expected error for non-existent tool")
	}
	if !strings.Contains(err.Error(), "unknown tool") {
		t.Errorf("Error = %q, want to contain 'unknown tool'", err.Error())
	}
}

func TestClientContextTimeout(t *testing.T) {
	mockPath := buildMockServer(t)

	client, err := mcp.NewClient(mockPath)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer client.Close()

	// Use an already-cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = client.Initialize(ctx)
	if err == nil {
		t.Fatal("Expected error with cancelled context")
	}
}

// --- Story 2: Snapshot Tests ---

func TestPlaywrightToolsSnapshot(t *testing.T) {
	tools, err := mcp.PlaywrightTools()
	if err != nil {
		t.Fatalf("PlaywrightTools: %v", err)
	}

	if len(tools) != 21 {
		t.Fatalf("PlaywrightTools returned %d tools, want 21", len(tools))
	}

	// Verify all tools have the prefix
	for _, tool := range tools {
		if !strings.HasPrefix(tool.Name, "mcp_playwright_") {
			t.Errorf("Tool %q missing mcp_playwright_ prefix", tool.Name)
		}
		if tool.Description == "" {
			t.Errorf("Tool %q has empty description", tool.Name)
		}
		if tool.InputSchema == nil {
			t.Errorf("Tool %q has nil InputSchema", tool.Name)
		}
	}

	// Verify specific tools exist
	expectedTools := []string{
		"mcp_playwright_browser_navigate",
		"mcp_playwright_browser_click",
		"mcp_playwright_browser_snapshot",
		"mcp_playwright_browser_take_screenshot",
		"mcp_playwright_browser_fill_form",
		"mcp_playwright_browser_type",
		"mcp_playwright_browser_close",
		"mcp_playwright_browser_tabs",
	}

	toolNames := make(map[string]bool)
	for _, tool := range tools {
		toolNames[tool.Name] = true
	}

	for _, expected := range expectedTools {
		if !toolNames[expected] {
			t.Errorf("Expected tool %q not found", expected)
		}
	}
}

func TestStripPrefix(t *testing.T) {
	if got := mcp.StripPrefix("mcp_playwright_browser_navigate"); got != "browser_navigate" {
		t.Errorf("StripPrefix = %q, want %q", got, "browser_navigate")
	}
	if got := mcp.StripPrefix("list_files"); got != "list_files" {
		t.Errorf("StripPrefix should not modify non-prefixed name, got %q", got)
	}
}

func TestHasPrefix(t *testing.T) {
	if !mcp.HasPrefix("mcp_playwright_browser_navigate") {
		t.Error("HasPrefix should return true for prefixed name")
	}
	if mcp.HasPrefix("list_files") {
		t.Error("HasPrefix should return false for non-prefixed name")
	}
}

// --- Story 2: Snapshot Drift Detection ---

func TestPlaywrightToolsMatchLiveServer(t *testing.T) {
	// This test requires npx to be available
	if _, err := exec.LookPath("npx"); err != nil {
		t.Skip("npx not available — skipping live snapshot verification")
	}

	// Spawn a live Playwright MCP server
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := mcp.NewClient("npx", "@playwright/mcp@latest", "--headless")
	if err != nil {
		t.Skipf("Failed to spawn Playwright MCP server: %v", err)
	}
	defer client.Close()

	_, err = client.Initialize(ctx)
	if err != nil {
		t.Skipf("Failed to initialize Playwright MCP: %v", err)
	}

	liveTools, err := client.ListTools(ctx)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}

	// Compare with embedded snapshot
	snapshotTools, err := mcp.PlaywrightTools()
	if err != nil {
		t.Fatalf("PlaywrightTools: %v", err)
	}

	if len(liveTools) != len(snapshotTools) {
		t.Errorf("Live server has %d tools, snapshot has %d",
			len(liveTools), len(snapshotTools))
	}

	// Build a set of live tool names
	liveNames := make(map[string]bool)
	for _, tool := range liveTools {
		liveNames[tool.Name] = true
	}

	// Check all snapshot tools exist in live server
	for _, tool := range snapshotTools {
		originalName := mcp.StripPrefix(tool.Name)
		if !liveNames[originalName] {
			t.Errorf("Snapshot tool %q not found in live server", originalName)
		}
	}

	// Check for new tools in live server not in snapshot
	snapshotNames := make(map[string]bool)
	for _, tool := range snapshotTools {
		snapshotNames[mcp.StripPrefix(tool.Name)] = true
	}
	for _, tool := range liveTools {
		if !snapshotNames[tool.Name] {
			t.Logf("WARNING: Live server has tool %q not in snapshot — snapshot may need updating", tool.Name)
		}
	}
}

// --- Story 3: Server Lifecycle Tests ---

func TestPlaywrightServerLazyStart(t *testing.T) {
	// Verify the server is NOT started until EnsureRunning is called
	server := mcp.NewPlaywrightServer("--headless")
	if server.IsRunning() {
		t.Error("Server should not be running before EnsureRunning")
	}
}

func TestPlaywrightServerCallToolWithoutStart(t *testing.T) {
	server := mcp.NewPlaywrightServer("--headless")
	ctx := context.Background()
	_, err := server.CallTool(ctx, "browser_navigate", nil)
	if err == nil {
		t.Fatal("Expected error calling tool on unstarted server")
	}
	if !strings.Contains(err.Error(), "not running") {
		t.Errorf("Error = %q, want to contain 'not running'", err.Error())
	}
}

func TestPlaywrightServerCloseIdempotent(t *testing.T) {
	server := mcp.NewPlaywrightServer("--headless")
	// Close without starting should be safe
	if err := server.Close(); err != nil {
		t.Errorf("Close on unstarted server: %v", err)
	}
	// Second close should also be safe
	if err := server.Close(); err != nil {
		t.Errorf("Second close: %v", err)
	}
}

func TestPlaywrightServerEnsureRunningWithNpx(t *testing.T) {
	if _, err := exec.LookPath("npx"); err != nil {
		t.Skip("npx not available")
	}

	server := mcp.NewPlaywrightServer("--headless")
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := server.EnsureRunning(ctx)
	if err != nil {
		t.Skipf("Failed to start Playwright MCP server: %v", err)
	}

	// Second call should be a no-op
	err = server.EnsureRunning(ctx)
	if err != nil {
		t.Errorf("Second EnsureRunning failed: %v", err)
	}

	// Call a simple tool
	result, err := server.CallTool(ctx, "browser_navigate", map[string]interface{}{
		"url": "data:text/html,<h1>Hello</h1>",
	})
	if err != nil {
		t.Fatalf("CallTool browser_navigate: %v", err)
	}
	if result == nil {
		t.Fatal("CallTool returned nil result")
	}
}

func TestPlaywrightServerCloseAfterUse(t *testing.T) {
	if _, err := exec.LookPath("npx"); err != nil {
		t.Skip("npx not available")
	}

	server := mcp.NewPlaywrightServer("--headless")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := server.EnsureRunning(ctx)
	if err != nil {
		t.Skipf("Failed to start: %v", err)
	}

	// Close the server
	if err := server.Close(); err != nil {
		t.Logf("Close: %v (may be expected from killed process)", err)
	}

	// Calling after close should error
	_, err = server.CallTool(ctx, "browser_navigate", map[string]interface{}{
		"url": "data:text/html,<h1>test</h1>",
	})
	if err == nil {
		t.Error("Expected error calling tool after Close")
	}
}

// --- Mock Server ---

// buildMockServer compiles a small Go program that acts as a mock MCP server.
func buildMockServer(t *testing.T) string {
	t.Helper()

	srcDir := t.TempDir()
	srcFile := srcDir + "/mock_mcp_server.go"
	binFile := srcDir + "/mock_mcp_server"

	src := `package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
)

type request struct {
	JSONRPC string          ` + "`json:\"jsonrpc\"`" + `
	ID      *int            ` + "`json:\"id,omitempty\"`" + `
	Method  string          ` + "`json:\"method\"`" + `
	Params  json.RawMessage ` + "`json:\"params,omitempty\"`" + `
}

type response struct {
	JSONRPC string      ` + "`json:\"jsonrpc\"`" + `
	ID      *int        ` + "`json:\"id,omitempty\"`" + `
	Result  interface{} ` + "`json:\"result,omitempty\"`" + `
	Error   interface{} ` + "`json:\"error,omitempty\"`" + `
}

func main() {
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	for scanner.Scan() {
		var req request
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			continue
		}

		// Skip notifications (no ID)
		if req.ID == nil {
			continue
		}

		var resp response
		resp.JSONRPC = "2.0"
		resp.ID = req.ID

		switch req.Method {
		case "initialize":
			resp.Result = map[string]interface{}{
				"protocolVersion": "2025-03-26",
				"capabilities":   map[string]interface{}{"tools": map[string]interface{}{}},
				"serverInfo":     map[string]interface{}{"name": "mock-mcp-server", "version": "0.1.0"},
			}

		case "tools/list":
			resp.Result = map[string]interface{}{
				"tools": []interface{}{
					map[string]interface{}{
						"name":        "echo",
						"description": "Echoes the message",
						"inputSchema": map[string]interface{}{
							"type": "object",
							"properties": map[string]interface{}{
								"message": map[string]interface{}{
									"type":        "string",
									"description": "Message to echo",
								},
							},
						},
					},
					map[string]interface{}{
						"name":        "fail",
						"description": "Always returns an error",
						"inputSchema": map[string]interface{}{
							"type":       "object",
							"properties": map[string]interface{}{},
						},
					},
				},
			}

		case "tools/call":
			var params struct {
				Name      string                 ` + "`json:\"name\"`" + `
				Arguments map[string]interface{} ` + "`json:\"arguments\"`" + `
			}
			json.Unmarshal(req.Params, &params)

			switch params.Name {
			case "echo":
				msg, _ := params.Arguments["message"].(string)
				resp.Result = map[string]interface{}{
					"content": []interface{}{
						map[string]interface{}{"type": "text", "text": msg},
					},
				}
			case "fail":
				resp.Result = map[string]interface{}{
					"content": []interface{}{
						map[string]interface{}{"type": "text", "text": "intentional error"},
					},
					"isError": true,
				}
			default:
				resp.Error = map[string]interface{}{
					"code":    -32601,
					"message": fmt.Sprintf("unknown tool: %s", params.Name),
				}
			}

		default:
			resp.Error = map[string]interface{}{
				"code":    -32601,
				"message": fmt.Sprintf("unknown method: %s", req.Method),
			}
		}

		data, _ := json.Marshal(resp)
		fmt.Println(string(data))
	}
}
`

	if err := os.WriteFile(srcFile, []byte(src), 0644); err != nil {
		t.Fatalf("write mock server source: %v", err)
	}

	cmd := exec.Command("go", "build", "-o", binFile, srcFile)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build mock server: %v\n%s", err, out)
	}

	return binFile
}

// --- Type assertions (compile-time) ---

var _ io.Closer = (*mcp.Client)(nil)

// Verify scanner is available on the Client
func TestClientTypes(t *testing.T) {
	// Verify types are correctly defined
	var r mcp.Response
	_ = r.ID
	_ = r.Result
	_ = r.Error

	var tr mcp.CallToolResult
	_ = tr.Content
	_ = tr.IsError
}


