package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sync"
)

// Client is a JSON-RPC 2.0 stdio client for MCP servers.
// It spawns the server as a subprocess and communicates over stdin/stdout.
// Requests are sequential — the client does not support concurrent in-flight
// requests (no need for Playwright's single-threaded browser model).
type Client struct {
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	scanner *bufio.Scanner
	nextID  int
	mu      sync.Mutex // serialises requests
}

// NewClient spawns the MCP server subprocess and returns a Client.
// The caller must call Close() when done.
func NewClient(command string, args ...string) (*Client, error) {
	cmd := exec.Command(command, args...)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("mcp: stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stdin.Close()
		return nil, fmt.Errorf("mcp: stdout pipe: %w", err)
	}

	// Discard stderr — Playwright logs are noisy
	cmd.Stderr = io.Discard

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("mcp: start %q: %w", command, err)
	}

	scanner := bufio.NewScanner(stdout)
	// MCP messages can be large (tool schemas, page snapshots)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	return &Client{
		cmd:     cmd,
		stdin:   stdin,
		scanner: scanner,
		nextID:  1,
	}, nil
}

// send writes a JSON-RPC request to the server's stdin.
func (c *Client) send(req Request) error {
	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("mcp: marshal request: %w", err)
	}
	data = append(data, '\n')
	_, err = c.stdin.Write(data)
	return err
}

// readResponse reads the next JSON-RPC response from the server's stdout.
// It skips server-sent notifications (messages without an id).
func (c *Client) readResponse(ctx context.Context) (*Response, error) {
	type result struct {
		resp *Response
		err  error
	}
	ch := make(chan result, 1)

	go func() {
		for {
			if !c.scanner.Scan() {
				if err := c.scanner.Err(); err != nil {
					ch <- result{nil, fmt.Errorf("mcp: read: %w", err)}
				} else {
					ch <- result{nil, fmt.Errorf("mcp: server closed stdout")}
				}
				return
			}
			line := c.scanner.Bytes()
			var resp Response
			if err := json.Unmarshal(line, &resp); err != nil {
				continue // skip malformed lines
			}
			// Skip notifications (no id)
			if resp.ID == nil && resp.Method != "" {
				continue
			}
			ch <- result{&resp, nil}
			return
		}
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case r := <-ch:
		return r.resp, r.err
	}
}

// call sends a request and waits for the corresponding response.
func (c *Client) call(ctx context.Context, method string, params interface{}) (json.RawMessage, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	id := c.nextID
	c.nextID++

	req := Request{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}

	if err := c.send(req); err != nil {
		return nil, err
	}

	resp, err := c.readResponse(ctx)
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, resp.Error
	}

	return resp.Result, nil
}

// notify sends a JSON-RPC notification (no response expected).
func (c *Client) notify(method string, params interface{}) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	req := Request{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	}
	return c.send(req)
}

// Initialize performs the MCP initialize handshake.
// It sends "initialize" and then "notifications/initialized".
func (c *Client) Initialize(ctx context.Context) (*InitializeResult, error) {
	params := InitializeParams{
		ProtocolVersion: "2025-03-26",
		Capabilities:    map[string]interface{}{},
		ClientInfo: ClientInfo{
			Name:    "clyde",
			Version: "1.0.0",
		},
	}

	raw, err := c.call(ctx, "initialize", params)
	if err != nil {
		return nil, fmt.Errorf("mcp: initialize: %w", err)
	}

	var result InitializeResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("mcp: unmarshal initialize result: %w", err)
	}

	// Send initialized notification
	if err := c.notify("notifications/initialized", nil); err != nil {
		return nil, fmt.Errorf("mcp: notifications/initialized: %w", err)
	}

	return &result, nil
}

// ListTools calls "tools/list" and returns the server's tool definitions.
func (c *Client) ListTools(ctx context.Context) ([]Tool, error) {
	raw, err := c.call(ctx, "tools/list", nil)
	if err != nil {
		return nil, fmt.Errorf("mcp: tools/list: %w", err)
	}

	var result ToolsListResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("mcp: unmarshal tools list: %w", err)
	}

	return result.Tools, nil
}

// CallTool calls "tools/call" with the given tool name and arguments.
func (c *Client) CallTool(ctx context.Context, name string, args map[string]interface{}) (*CallToolResult, error) {
	params := CallToolParams{
		Name:      name,
		Arguments: args,
	}

	raw, err := c.call(ctx, "tools/call", params)
	if err != nil {
		return nil, fmt.Errorf("mcp: tools/call %q: %w", name, err)
	}

	var result CallToolResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("mcp: unmarshal call result for %q: %w", name, err)
	}

	return &result, nil
}

// Close kills the MCP server subprocess and releases resources.
func (c *Client) Close() error {
	c.stdin.Close()
	if c.cmd.Process != nil {
		c.cmd.Process.Kill()
	}
	return c.cmd.Wait()
}
