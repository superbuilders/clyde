package mcp

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// PlaywrightServer manages the lifecycle of a Playwright MCP server subprocess.
// It starts lazily on first use and stays alive for the session.
type PlaywrightServer struct {
	extraArgs string  // extra CLI args for npx (e.g. "--headless")
	client    *Client // nil until EnsureRunning
	once      sync.Once
	startErr  error
	mu        sync.Mutex // guards Close
	closed    bool
}

// NewPlaywrightServer creates a new server manager.
// The server is NOT started until EnsureRunning is called.
func NewPlaywrightServer(extraArgs string) *PlaywrightServer {
	return &PlaywrightServer{
		extraArgs: extraArgs,
	}
}

// EnsureRunning starts the Playwright MCP server if it hasn't been started yet.
// It is safe to call from multiple goroutines — only the first call starts the
// server. If the server was already started (or failed to start), this returns
// immediately.
func (s *PlaywrightServer) EnsureRunning(ctx context.Context) error {
	s.once.Do(func() {
		s.startErr = s.start(ctx)
	})
	return s.startErr
}

// start launches the npx subprocess, performs the MCP initialize handshake,
// and verifies the server responds with tools.
func (s *PlaywrightServer) start(ctx context.Context) error {
	args := []string{"@playwright/mcp@latest"}
	if s.extraArgs != "" {
		// Split extra args (simple space-based split)
		for _, a := range splitArgs(s.extraArgs) {
			args = append(args, a)
		}
	}
	// Always add --headless if not already present
	hasHeadless := false
	hasIsolated := false
	for _, a := range args {
		if a == "--headless" {
			hasHeadless = true
		}
		if a == "--isolated" {
			hasIsolated = true
		}
	}
	if !hasHeadless {
		args = append(args, "--headless")
	}
	// Always use --isolated so each clyde instance gets its own browser
	// profile (temp directory). Without this, Playwright locks the shared
	// profile and subsequent instances fail with "Browser is already in use".
	if !hasIsolated {
		args = append(args, "--isolated")
	}

	// Use a timeout for the startup handshake
	startCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	client, err := NewClient("npx", args...)
	if err != nil {
		return fmt.Errorf("mcp playwright: failed to spawn server: %w", err)
	}

	_, err = client.Initialize(startCtx)
	if err != nil {
		client.Close()
		return fmt.Errorf("mcp playwright: initialize failed: %w", err)
	}

	s.client = client
	return nil
}

// CallTool forwards a tool call to the running Playwright MCP server.
// The name should be the original MCP tool name (without the "mcp_playwright_"
// prefix — the caller should strip it before calling this method).
func (s *PlaywrightServer) CallTool(ctx context.Context, name string, args map[string]interface{}) (*CallToolResult, error) {
	if s.client == nil {
		return nil, fmt.Errorf("mcp playwright: server not running")
	}
	if s.closed {
		return nil, fmt.Errorf("mcp playwright: server has been closed")
	}

	return s.client.CallTool(ctx, name, args)
}

// Close kills the Playwright subprocess and releases resources.
// It is safe to call multiple times.
func (s *PlaywrightServer) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}
	s.closed = true

	if s.client != nil {
		return s.client.Close()
	}
	return nil
}

// splitArgs does a simple whitespace split of extra CLI arguments.
func splitArgs(s string) []string {
	var args []string
	current := ""
	for _, c := range s {
		if c == ' ' || c == '\t' {
			if current != "" {
				args = append(args, current)
				current = ""
			}
		} else {
			current += string(c)
		}
	}
	if current != "" {
		args = append(args, current)
	}
	return args
}
