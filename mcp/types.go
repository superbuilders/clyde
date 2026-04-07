package mcp

import "encoding/json"

// --- JSON-RPC 2.0 framing ---

// Request is a JSON-RPC 2.0 request.
type Request struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      int         `json:"id,omitempty"` // 0 for notifications
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

// Response is a JSON-RPC 2.0 response.
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int            `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
	Method  string          `json:"method,omitempty"` // for server-sent notifications
}

// RPCError is a JSON-RPC 2.0 error object.
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *RPCError) Error() string { return e.Message }

// --- MCP protocol types ---

// InitializeParams are sent in the "initialize" request.
type InitializeParams struct {
	ProtocolVersion string                 `json:"protocolVersion"`
	Capabilities    map[string]interface{} `json:"capabilities"`
	ClientInfo      ClientInfo             `json:"clientInfo"`
}

// ClientInfo identifies the MCP client to the server.
type ClientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// InitializeResult is the server's response to "initialize".
type InitializeResult struct {
	ProtocolVersion string     `json:"protocolVersion"`
	Capabilities    Caps       `json:"capabilities"`
	ServerInfo      ServerInfo `json:"serverInfo"`
}

// Caps represents server capabilities advertised during initialize.
type Caps struct {
	Tools map[string]interface{} `json:"tools,omitempty"`
}

// ServerInfo identifies the MCP server.
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// Tool is an MCP tool definition returned by "tools/list".
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
	Annotations json.RawMessage `json:"annotations,omitempty"`
}

// ToolsListResult is the result of "tools/list".
type ToolsListResult struct {
	Tools []Tool `json:"tools"`
}

// CallToolParams are sent in the "tools/call" request.
type CallToolParams struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments,omitempty"`
}

// CallToolResult is the result of "tools/call".
type CallToolResult struct {
	Content []ContentPart `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

// ContentPart is a single part of a tool call result.
type ContentPart struct {
	Type     string `json:"type"`               // "text" or "image"
	Text     string `json:"text,omitempty"`      // for type="text"
	Data     string `json:"data,omitempty"`      // base64 for type="image"
	MimeType string `json:"mimeType,omitempty"`  // for type="image"
}
