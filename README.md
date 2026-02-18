# Clyde

A single-file Go CLI that provides a REPL interface for talking to Claude AI with GitHub integration.

## Quick Start

```bash
# Run the REPL
./clyde

# Or build from source
go build -o clyde
./clyde
```

## Features

- 💬 **Interactive REPL**: Natural conversation with Claude
- 🔧 **GitHub Integration**: Ask questions about your GitHub account via `gh` CLI
- 📁 **File System Tools**: List directories and read/write files
- ✏️ **Smart Editing**: Patch individual files or coordinate changes across multiple files
- 🔍 **Search Tool**: Find patterns across multiple files with grep
- 🗂️ **File Finding Tool**: Find files matching patterns with glob (fuzzy file finding)
- 🔄 **Conversation Memory**: Maintains context across turns
- ⚡ **Fast & Lightweight**: Single binary, minimal dependencies

## Usage Examples

```
You: Hello!
Claude: Hello! How can I help you today?

You: What repositories do I have?
→ Running GitHub query...
Claude: [Lists your repositories]

You: What files are in the current directory?
→ Listing files...
Claude: [Shows detailed file listing]

You: Read the README.md file
→ Reading file...
Claude: [Displays file contents]

You: Change "Hello" to "Hi" in the file main.go
→ Patching file...
Claude: [Confirms successful patch]

You: Create a new file called test.txt with "Hello World"
→ Writing file...
Claude: [Confirms file creation]

You: Run ls -la to see all files
→ Running bash command...
Claude: [Shows directory listing]

You: Find all TODO comments in Go files
→ Searching for 'TODO' in current directory (*.go)
Claude: [Shows files and lines with TODO comments]

You: Find all test files
→ Finding files: '*_test.go' in current directory
Claude: [Shows all test files in the project]

You: Rename function 'oldName' to 'newName' across all Go files
→ Applying multi-patch: 3 files
Claude: [Coordinates changes across multiple files with rollback on failure]

You: Search for the latest Go HTTP client tutorial
→ Searching web: "golang http client tutorial"
Claude: [Returns search results with titles, URLs, and snippets]

You: Browse https://pkg.go.dev/net/http and tell me about the Client type
→ Browsing: https://pkg.go.dev/net/http
Claude: [Fetches page, converts to markdown, and explains the Client type]

You: exit
Goodbye!
```

## Requirements

- Go 1.24+
- GitHub CLI (`gh`) installed and authenticated
- Anthropic API key (see Configuration below)
- Brave Search API key (optional, for web_search tool)

## Installation

### Option 1: Install globally (recommended for regular use)
```bash
go install github.com/this-is-alpha-iota/clyde@latest
```

After installation, create a config file in your home directory:
```bash
mkdir -p ~/.clyde
cat > ~/.clyde/config << 'EOF'
TS_AGENT_API_KEY=your-anthropic-api-key
BRAVE_SEARCH_API_KEY=your-brave-api-key  # Optional
EOF
```

Get your API keys:
- **Anthropic API**: https://console.anthropic.com/
- **Brave Search API** (optional): https://brave.com/search/api/ - Free tier: 2,000 searches/month

### Option 2: Build from source (for development)
```bash
git clone https://github.com/this-is-alpha-iota/clyde
cd clyde
go build -o clyde
./clyde
```

## Configuration

The application uses a single configuration file at `~/.clyde/config`.

### Configuration File Format
```bash
# Clyde Configuration
# Required
TS_AGENT_API_KEY=sk-ant-your-key-here

# Optional (for web_search tool)
BRAVE_SEARCH_API_KEY=BSA-your-key-here
```

**Why this location?**
- Standard location for user-specific CLI configuration
- Works from any directory after installation
- Clean separation between production and test configurations
- Tests use `.env` files in their own directories

## Customizing the System Prompt

The system prompt is stored in `prompts/system.txt` and can be customized:

**Development Mode**: If you're running from source, edit `prompts/system.txt` directly. Changes take effect immediately without recompilation.

**Production Mode**: When running the compiled binary in a directory without `prompts/system.txt`, it uses the embedded version from compilation time.

This dual-mode approach allows:
- Fast iteration during development (no rebuild needed)
- Single-binary distribution in production (embedded prompt)

To test prompt changes:
```bash
# Edit the prompt
vim prompts/system.txt

# Run without rebuilding
./clyde

# When satisfied, rebuild to embed the new prompt
go build -o clyde
```

## Testing

```bash
# Run all tests
go test ./tests/... -v

# Run specific test
go test ./tests/... -v -run TestName
```

## Available Tools

The REPL includes ten integrated tools:

1. **list_files**: List files and directories in any path
2. **read_file**: Read and display file contents
3. **patch_file**: Edit files using find/replace (patch-based approach)
4. **write_file**: Create new files or completely replace file contents
5. **run_bash**: Execute arbitrary bash commands (including gh, git, etc.)
6. **grep**: Search for patterns across multiple files with context
7. **glob**: Find files matching patterns (fuzzy file finding)
8. **multi_patch**: Apply coordinated changes to multiple files with automatic rollback
9. **web_search**: Search the internet using Brave Search API
10. **browse**: Fetch and read web pages (with optional AI extraction)

## Using Clyde as a Library

Clyde's agent is fully decoupled from the CLI interface and can be embedded in your own Go applications. This enables you to build custom interfaces (HTTP APIs, GUIs, bots, etc.) while leveraging the same powerful agent logic.

### Basic Usage

```go
import (
    "fmt"
    "github.com/this-is-alpha-iota/clyde/agent"
    "github.com/this-is-alpha-iota/clyde/api"
    "github.com/this-is-alpha-iota/clyde/prompts"
    _ "github.com/this-is-alpha-iota/clyde/tools" // Register all tools
)

func main() {
    // Create API client
    apiClient := api.NewClient(
        "your-api-key",
        "https://api.anthropic.com/v1/messages",
        "claude-sonnet-4-5-20250929",
        4096,
    )
    
    // Create agent with progress callback
    agentInstance := agent.NewAgent(
        apiClient,
        prompts.SystemPrompt,
        agent.WithProgressCallback(func(msg string) {
            fmt.Println(msg) // Or send to your UI, log, etc.
        }),
    )
    
    // Send messages
    response, err := agentInstance.HandleMessage("What files are in the current directory?")
    if err != nil {
        fmt.Printf("Error: %v\n", err)
        return
    }
    
    fmt.Printf("Response: %s\n", response)
}
```

### Custom Progress Handling

The agent provides callback hooks for handling progress messages and errors in your application:

```go
// For CLI applications - print to stdout
agent.NewAgent(apiClient, systemPrompt,
    agent.WithProgressCallback(func(msg string) {
        fmt.Println(msg)
    }),
)

// For HTTP APIs - send via WebSocket
agent.NewAgent(apiClient, systemPrompt,
    agent.WithProgressCallback(func(msg string) {
        websocket.Send(msg)
    }),
)

// For GUIs - update UI elements
agent.NewAgent(apiClient, systemPrompt,
    agent.WithProgressCallback(func(msg string) {
        statusBar.SetText(msg)
    }),
)

// For logging - capture all progress
var progressLog []string
agent.NewAgent(apiClient, systemPrompt,
    agent.WithProgressCallback(func(msg string) {
        progressLog = append(progressLog, msg)
    }),
)

// Optional error callback for logging/monitoring
agent.NewAgent(apiClient, systemPrompt,
    agent.WithProgressCallback(progressHandler),
    agent.WithErrorCallback(func(err error) {
        log.Printf("Agent error: %v", err)
        metrics.IncrementErrorCount()
    }),
)
```

### Example: HTTP API Server

```go
type Session struct {
    agent *agent.Agent
    progressBuffer []string
    mu sync.Mutex
}

func handleMessage(w http.ResponseWriter, r *http.Request) {
    session := getSession(r)
    
    // Capture progress messages for this request
    session.progressBuffer = []string{}
    
    response, err := session.agent.HandleMessage(userInput)
    
    // Return response with captured progress
    json.NewEncoder(w).Encode(map[string]interface{}{
        "response": response,
        "progress": session.progressBuffer,
        "error": err,
    })
}
```

### No Callbacks? No Problem!

If you don't provide a progress callback, the agent works silently:

```go
// Silent agent - no progress output
agentInstance := agent.NewAgent(apiClient, prompts.SystemPrompt)
response, _ := agentInstance.HandleMessage("Hello!")
```

## Documentation

See [PROGRESS.md](PROGRESS.md) for detailed technical documentation.
