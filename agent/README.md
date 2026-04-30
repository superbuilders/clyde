# Clyde Agent — Go Library

The `agent` package provides a self-contained AI coding agent that can be embedded in any Go application. It handles conversation orchestration, tool execution, and Claude API communication — your code just provides the configuration and callbacks.

## Installation

```bash
go get github.com/this-is-alpha-iota/clyde/agent@latest
```

This pulls **only** the agent and its minimal dependencies. It does **not** pull the CLI, TUI, readline, or terminal libraries.

## Quick Start

```go
package main

import (
    "fmt"
    "github.com/this-is-alpha-iota/clyde/agent"
)

func main() {
    agentInstance := agent.New(agent.Config{
        APIKey:    "sk-ant-your-key-here",
        APIURL:    "https://api.anthropic.com/v1/messages",
        ModelID:   "claude-opus-4-6",
        MaxTokens: 64000,
    },
        agent.WithProgressCallback(func(msg string, _ string) {
            fmt.Println(msg)
        }),
    )
    defer agentInstance.Close()

    response, err := agentInstance.HandleMessage("What files are in the current directory?")
    if err != nil {
        fmt.Printf("Error: %v\n", err)
        return
    }
    fmt.Println(response)
}
```

## Config Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `APIKey` | `string` | **Yes** | Anthropic API key |
| `APIURL` | `string` | **Yes** | API endpoint URL |
| `ModelID` | `string` | **Yes** | Claude model identifier (e.g. `"claude-opus-4-6"`) |
| `MaxTokens` | `int` | **Yes** | Maximum output tokens per API call |
| `ContextWindowSize` | `int` | No | Model context window in tokens (for diagnostics + compaction) |
| `ThinkingBudget` | `int` | No | Extended thinking budget. 0 = adaptive (default), >0 = manual |
| `NoThink` | `bool` | No | Disable extended thinking entirely |
| `BraveSearchAPIKey` | `string` | No | Brave Search API key for `web_search` tool |
| `MCPPlaywright` | `bool` | No | Enable Playwright browser automation via MCP |
| `MCPPlaywrightArgs` | `string` | No | Extra args for Playwright MCP server |
| `ReserveTokens` | `int` | No | Tokens to reserve before compaction triggers (default 16000) |
| `CompactIncludeRecentContext` | `*bool` | No | Feed recent messages into compaction (default true) |
| `ToolResultThreshold` | `int` | No | Char threshold for tool-result summarization (default 2000) |

## Callbacks (Functional Options)

All callbacks are optional. The agent emits unconditionally — the caller decides what to display or log.

```go
agent.New(cfg,
    // Tool progress lines (the → lines)
    agent.WithProgressCallback(func(msg string, toolUseID string) { ... }),

    // Tool output bodies (full, untruncated text)
    agent.WithOutputCallback(func(output string, toolUseID string) { ... }),

    // Claude's thinking traces (full text)
    agent.WithThinkingCallback(func(text string, signature string) { ... }),

    // Cache stats, token counts, diagnostics
    agent.WithDiagnosticCallback(func(msg string) { ... }),

    // Spinner start/stop signals
    agent.WithSpinnerCallback(func(start bool, msg string) { ... }),

    // Errors during processing
    agent.WithErrorCallback(func(err error) { ... }),

    // User/assistant message persistence hooks
    agent.WithUserMessageCallback(func(text string) { ... }),
    agent.WithAssistantMessageCallback(func(text string) { ... }),

    // Tool use metadata for session persistence
    agent.WithToolUseCallback(func(displayMsg, toolName, toolUseID string, input map[string]interface{}) { ... }),

    // Context window size for diagnostics
    agent.WithContextWindowSize(200000),

    // Compaction reserve tokens
    agent.WithReserveTokens(16000),
)
```

## Re-exported Types

These types are accessible via `import "…/agent"` — no need to import subpackages:

```go
agent.Message        // Conversation message (role + content)
agent.ContentBlock   // Message content block (text, tool_use, tool_result, etc.)
agent.Usage          // Token usage statistics
```

## Agent Methods

```go
// Send a message and get a response (handles tool execution internally)
response, err := agentInstance.HandleMessage("your prompt here")

// Get conversation history
history := agentInstance.GetHistory()

// Replace conversation history (for session resume)
agentInstance.SetHistory(messages)

// Get token usage from most recent API call
usage := agentInstance.LastUsage()

// Release resources (MCP server, etc.)
agentInstance.Close()
```

## Session Persistence

The agent provides `agent/session` as a supported public subpackage for session management:

```go
import "github.com/this-is-alpha-iota/clyde/agent/session"

// Create a session
sess, err := session.New()

// Write messages
sess.WriteMessage(session.TypeUser, "Hello")
sess.WriteMessage(session.TypeAssistant, "Hi there!")

// Reconstruct history for resume
history, err := session.ReconstructHistory(sessionDir)

// List sessions
sessions, err := session.ListSessions(sessionsRoot)
```

## Built-in Tools

The agent comes with 12 built-in tools (automatically registered):

1. `list_files` — Directory listings
2. `read_file` — Read file contents
3. `patch_file` — Find/replace file edits
4. `write_file` — Create/replace files
5. `run_bash` — Execute shell commands
6. `grep` — Search patterns across files
7. `glob` — Find files by pattern
8. `multi_patch` — Coordinated multi-file edits with git rollback
9. `web_search` — Internet search via Brave API
10. `browse` — Fetch and read web pages
11. `include_file` — Include images for vision analysis
12. `mcp_playwright_*` — 21 browser automation tools via Playwright MCP (optional)

## Examples

### HTTP API Server

```go
type Session struct {
    agent          *agent.Agent
    progressBuffer []string
}

func handleMessage(w http.ResponseWriter, r *http.Request) {
    session := getSession(r)
    session.progressBuffer = nil

    response, err := session.agent.HandleMessage(userInput)

    json.NewEncoder(w).Encode(map[string]interface{}{
        "response": response,
        "progress": session.progressBuffer,
        "error":    err,
    })
}
```

### Silent Agent (No Callbacks)

```go
agentInstance := agent.New(agent.Config{
    APIKey: "your-key", APIURL: "https://api.anthropic.com/v1/messages",
    ModelID: "claude-opus-4-6", MaxTokens: 64000,
})
defer agentInstance.Close()

response, _ := agentInstance.HandleMessage("Hello!")
```

### WebSocket Streaming

```go
agent.New(cfg,
    agent.WithProgressCallback(func(msg string, _ string) {
        websocket.Send(msg)
    }),
    agent.WithThinkingCallback(func(text string, _ string) {
        websocket.Send("💭 " + text)
    }),
)
```

## Dependencies

The agent module has a minimal dependency tree:

**Direct dependencies:**
| Package | Version | Purpose |
|---------|---------|---------|
| `github.com/JohannesKaufmann/html-to-markdown` | v1.6.0 | HTML→Markdown conversion for `browse` tool |
| `github.com/joho/godotenv` | v1.5.1 | Environment variable loading for config |

**Transitive dependencies (pulled automatically):**
| Package | Version | Via |
|---------|---------|-----|
| `github.com/PuerkitoBio/goquery` | v1.9.2 | html-to-markdown |
| `github.com/andybalholm/cascadia` | v1.3.2 | goquery |
| `golang.org/x/net` | v0.25.0 | goquery (HTML parsing) |

**NOT included** (CLI-only dependencies):
- ❌ `golang.org/x/sys` — terminal raw mode (CLI only)
- ❌ `github.com/chzyer/readline` — readline input (CLI only)
- ❌ Any TUI/GUI framework

## Separate Modules

This project uses a multi-module monorepo:

| Module | Path | Purpose |
|--------|------|---------|
| `github.com/this-is-alpha-iota/clyde/agent` | `agent/` | Agent library (this package) |
| `github.com/this-is-alpha-iota/clyde` | root | CLI binary (`go install …/clyde@latest`) |

The two modules have independent dependency trees. Installing the CLI binary pulls `x/sys` and readline; importing the agent library does not.

## Verification

Run the external consumability smoke test:

```bash
# Local verification (uses replace directive)
./scripts/test-external-consume.sh

# Post-release verification (uses tagged version)
./scripts/test-external-consume.sh v0.1.0
```
