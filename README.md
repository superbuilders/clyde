# Clyde

A minimal coding agent built with Go and Claude. Available as a [TUI](#tui) or as an [SDK](#agent-sdk).

- [TUI](#tui) — Interactive terminal interface
  - [Installation](#installation)
  - [Configuration](#configuration)
  - [Usage](#usage)
- [Agent SDK](#agent-sdk) — Embed the agent in your own Go apps
  - [Installation](#installation-1)
  - [Quick Start](#quick-start)
  - [Core API](#core-api)
  - [Callbacks](#callbacks)
- [Tools](#tools)
- [Why Minimal?](#why-minimal)
- [Requirements](#requirements)
- [License](#license)

## TUI

### Installation

```bash
go install github.com/superbuilders/clyde@latest
```

### Configuration

Create a config file at `~/.clyde/config`:

```bash
mkdir -p ~/.clyde
cat > ~/.clyde/config << 'EOF'
TS_AGENT_API_KEY=your-anthropic-api-key
BRAVE_SEARCH_API_KEY=your-brave-api-key  # Optional, for web search
EOF
```

### Usage

**Interactive (REPL):**

```bash
clyde
```

**Single prompt (CLI):**

```bash
clyde "What files are in the current directory?"
clyde -f prompt.txt
echo "Summarize this project" | clyde
```

**Session management:**

```bash
clyde --sessions          # List previous sessions
clyde --resume            # Resume most recent session
clyde --resume <id>       # Resume a specific session
```

**Flags:**

| Flag | Description |
|------|-------------|
| `-f <file>` | Read prompt from file |
| `--no-think` | Disable extended thinking |
| `--sessions` | List previous sessions |
| `--resume [id]` | Resume a session |

## Agent SDK

The `agent` package lets you embed the same agent in your own Go applications — HTTP APIs, bots, custom CLIs, etc. It's a separate Go module with no TUI dependencies.

### Installation

```bash
go get github.com/superbuilders/clyde/agent@latest
```

### Quick Start

```go
package main

import (
    "fmt"
    "github.com/superbuilders/clyde/agent"
)

func main() {
    a := agent.New(agent.Config{
        APIKey:    "sk-ant-your-key-here",
        APIURL:    "https://api.anthropic.com/v1/messages",
        ModelID:   "claude-sonnet-4-20250514",
        MaxTokens: 16000,
    },
        agent.WithProgressCallback(func(msg string, _ string) {
            fmt.Println(msg)
        }),
    )
    defer a.Close()

    response, err := a.HandleMessage("What files are in the current directory?")
    if err != nil {
        panic(err)
    }
    fmt.Println(response)
}
```

### Core API

```go
// Send a message, get a response (tool use is handled internally)
response, err := a.HandleMessage("your prompt")

// Conversation history
history := a.GetHistory()
a.SetHistory(messages)

// Token usage from most recent call
usage := a.LastUsage()

// Clean up (stops MCP servers, etc.)
a.Close()
```

### Callbacks

All optional. Wire up only what you need:

```go
agent.New(cfg,
    agent.WithProgressCallback(func(msg, toolUseID string) { ... }),   // Tool progress lines
    agent.WithOutputCallback(func(output, toolUseID string) { ... }),  // Tool output
    agent.WithThinkingCallback(func(text, signature string) { ... }),  // Thinking traces
    agent.WithDiagnosticCallback(func(msg string) { ... }),            // Cache/token stats
    agent.WithSpinnerCallback(func(start bool, msg string) { ... }),   // Spinner signals
    agent.WithErrorCallback(func(err error) { ... }),                  // Errors
    agent.WithUserMessageCallback(func(text string) { ... }),          // Message persistence
    agent.WithAssistantMessageCallback(func(text string) { ... }),     // Message persistence
)
```

Full SDK documentation: **[agent/README.md](agent/README.md)**

## Tools

Both the TUI and the SDK ship with the same 11 built-in tools:

| Tool | Description |
|------|-------------|
| `list_files` | List files and directories (`ls -la`) |
| `read_file` | Read file contents |
| `write_file` | Create or overwrite a file |
| `patch_file` | Find-and-replace edit within a file |
| `multi_patch` | Coordinated edits across multiple files with git rollback on failure |
| `run_bash` | Execute any shell command |
| `grep` | Search for patterns across files with context |
| `glob` | Find files by pattern (e.g. `**/*.go`) |
| `web_search` | Search the internet via Brave Search API |
| `browse` | Fetch a URL and convert to markdown, optionally extract specific info |
| `include_file` | Include an image for vision analysis |

Optionally, Playwright MCP browser automation tools can be enabled with `MCP_PLAYWRIGHT=true`.

## Why Minimal?

Most coding agents are large projects. [Claude Code is 512K lines. OpenCode is 229K. Pi is 98K.](docs/whitepaper.md) Clyde is ~8K.

The difference isn't in the agent itself — across all four projects, the core agent loop (call the LLM, parse tool calls, execute, loop) converges on roughly the same size. The other 85–95% is product infrastructure: TUI frameworks, provider abstraction layers, permission systems, plugin architectures, session databases, server APIs, and configuration management.

Clyde skips most of that:

- **1 provider** instead of an abstraction layer over 11–20. If you use Claude, you don't need the abstraction.
- **No permission system.** The agent runs tools directly. (As one agent author [noted](https://marioslab.io/posts/pi/dissecting-coding-agents/), "everybody is running in YOLO mode anyway.")
- **No plugin architecture.** Tools are compiled in. To add a tool, you write Go and rebuild.
- **No server layer.** The TUI talks to the agent in-process. The SDK embeds directly.
- **3 direct dependencies** for the agent module. No framework, no ORM, no bundler.

The result is a single static binary with no runtime dependencies, fast compilation, and a codebase small enough to read in an afternoon.

For the full analysis, see the [whitepaper](docs/whitepaper.md).

## Requirements

- Go 1.24+
- Anthropic API key
- GitHub CLI (`gh`) — optional, for GitHub tools
- Brave Search API key — optional, for web search

## License

MIT
