# Clyde

A modular Go CLI that provides a REPL interface for talking to Claude AI with GitHub integration.

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
- 🖼️ **Vision Support**: Include images for Claude to analyze (multimodal)
- 💾 **Automatic Caching**: Reduces costs by ~80% through intelligent prompt caching
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

You: Look at screenshot.png and tell me what's wrong
→ Including file: screenshot.png
Claude: [Analyzes the image and identifies the error in the screenshot]

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

The system prompt is stored in `agent/prompts/system.txt` and can be customized:

**Development Mode**: If you're running from source, edit `agent/prompts/system.txt` directly. Changes take effect immediately without recompilation.

**Production Mode**: When running the compiled binary in a directory without `agent/prompts/system.txt`, it uses the embedded version from compilation time.

This dual-mode approach allows:
- Fast iteration during development (no rebuild needed)
- Single-binary distribution in production (embedded prompt)

To test prompt changes:
```bash
# Edit the prompt
vim agent/prompts/system.txt

# Run without rebuilding
./clyde

# When satisfied, rebuild to embed the new prompt
go build -o clyde
```

## Automatic Prompt Caching

Clyde automatically uses Claude API's prompt caching feature to reduce costs and improve performance. This is enabled by default and requires no configuration.

### What Gets Cached

The caching system automatically caches:
1. **System prompt** (5.1 KB) - The instructions that guide Claude's behavior
2. **Tool definitions** (11 tools) - The available tools and their schemas
3. **Conversation history** - Previous messages in the conversation

### Benefits

- **Cost Savings**: ~90% reduction in costs for cached content (10x cheaper)
- **Faster Response**: Cached tokens are processed ~10x faster
- **Automatic**: Works transparently without any user action
- **Zero Configuration**: Always enabled, no setup needed

### How It Works

When you see this message during a conversation:
```
💾 Cache hit: 3715 tokens (100% of input)
```

This means Claude reused 3,715 tokens from cache instead of reprocessing them, providing instant cost savings and faster responses.

### Cache Details

- **Cache Lifetime**: 5 minutes (automatically refreshed with each use)
- **Minimum Size**: 1024 tokens (smaller content not cached)
- **Type**: Ephemeral (temporary, per-session)
- **Invalidation**: Any change to cached content breaks the cache

### Example Savings

For a typical 10-turn conversation:
- **Without caching**: ~190 KB processed
- **With caching**: ~41 KB processed (80% reduction!)

The savings increase with longer conversations since the system prompt and tool definitions are cached once and reused for all subsequent turns.

## CLI Mode (Non-Interactive Execution)

In addition to the interactive REPL, Clyde can execute prompts directly and exit. This is useful for automation, scripting, and CI/CD integration.

### Usage

**Direct String Argument**:
```bash
# Execute a prompt and exit
clyde "What files are in the current directory?"

# Multi-word prompts (quotes recommended but not required)
clyde What is 2+2?
```

**From File**:
```bash
# Read prompt from file
clyde -f prompt.txt
```

**From Stdin (Pipe)**:
```bash
# Pipe prompt to clyde
echo "What is the capital of France?" | clyde

# Or from heredoc
cat << EOF | clyde
Review the code in main.go and suggest improvements.
Focus on error handling and readability.
EOF
```

### Output Handling

CLI mode separates output streams for composability:

- **stdout**: Final agent response (for piping/redirection)
- **stderr**: Progress messages (doesn't interfere with output capture)

**Examples**:
```bash
# Capture response only (progress still visible)
clyde "list files" > output.txt

# Capture response, hide progress
clyde "list files" 2>/dev/null > output.txt

# Capture everything (response + progress)
clyde "complex task" > output.txt 2>&1
```

### Exit Codes

- **0**: Success
- **1**: Error (config error, API error, empty prompt, etc.)

### Use Cases

**Quick Queries**:
```bash
# Check Go version
clyde "What version of Go is installed?"

# Count files
clyde "How many Go files are in this project?"
```

**Automation Scripts**:
```bash
#!/bin/bash
# Run tests and generate summary
clyde "Run all tests and create a summary" > test-report.txt

if [ $? -eq 0 ]; then
    echo "Tests passed!"
    cat test-report.txt | mail -s "Test Report" team@example.com
else
    echo "Test analysis failed"
    exit 1
fi
```

**CI/CD Integration**:
```bash
# .github/workflows/code-review.yml
- name: AI Code Review
  run: |
    clyde "Review the latest commit and summarize changes" > review.md
    cat review.md >> $GITHUB_STEP_SUMMARY
```

**Composable with Unix Tools**:
```bash
# Chain with other tools
git log -1 --pretty=%B | clyde "Summarize this commit message" | tee summary.txt

# Process multiple files
for file in *.go; do
    clyde "Count the functions in $file" >> stats.txt
done
```

**File Operations**:
```bash
# Generate documentation
clyde "Create a comprehensive README.md for this project" > README.md

# Refactor code
clyde "Rename all instances of oldFunction to newFunction" && git add -u
```

## Testing

```bash
# Run all tests
go test ./tests/... -v

# Run specific test
go test ./tests/... -v -run TestName
```

## Multiline Input

Clyde supports three ways to compose multi-line prompts in REPL mode:

### 1. Ctrl+J (Universal)
Press **Ctrl+J** to insert a newline without submitting. Works on every terminal, everywhere, unconditionally.

```
main* 12% You: Write a function that  [Ctrl+J]
  > takes two numbers and    [Ctrl+J]
  > returns their sum        [Enter to submit]
```

### 2. Alt+Enter
Press **Alt+Enter** to insert a newline, identical to Ctrl+J.

> **macOS Terminal.app**: Requires "Use Option as Meta Key" to be enabled in terminal preferences.
> **iTerm2**: Works by default (Option acts as Meta).
> **Linux terminals**: Works by default on most terminals.

### 3. Backslash Continuation
End a line with `\` to continue on the next line:

```
main* 12% You: This is a long prompt that \
  > continues on the next line \
  > and finishes here         [Enter to submit]
```

All three methods can be mixed freely within the same input block. **Ctrl+C** while composing a multiline prompt discards the partial input and returns to a fresh prompt. Multiline input is saved to history as a single block.

## Available Tools

The REPL includes twelve integrated tools:

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
11. **include_file**: Include images in conversation for vision analysis
12. **mcp_playwright_***: 21 browser automation tools via Playwright MCP (optional, enable with `MCP_PLAYWRIGHT=true`)

## Background Processes & Subagents

When you need to run background processes (like test servers) or spawn subagents for parallel work, **always use tmux** instead of the shell `&` operator.

### Why Not Use `&`?

The shell `&` operator doesn't work reliably with Clyde's `run_bash` tool:
- Background processes die immediately when the bash command exits
- No way to capture output from backgrounded processes
- Can't check if the process is still running
- Can't cleanly stop the process later

### The Solution: Use TMUX

TMUX provides a reliable way to manage background processes:

**Running Test Servers**:
```bash
# Start server in detached tmux session
tmux new-session -d -s testserver 'npm start'

# Run your tests
npm test

# Stop server when done
tmux kill-session -t testserver
```

**Long-Running Processes**:
```bash
# Start build in background
tmux new-session -d -s build './long-build.sh'

# Check progress later
tmux capture-pane -t build -p
```

**Spawning Subagents** (parallel Clyde instances):
```bash
# Spawn multiple subagents for parallel work
tmux new-session -d -s agent1 './clyde "analyze frontend"'
tmux new-session -d -s agent2 './clyde "analyze backend"'

# Collect results
tmux capture-pane -t agent1 -p > frontend-analysis.txt
tmux capture-pane -t agent2 -p > backend-analysis.txt

# Clean up
tmux kill-session -t agent1
tmux kill-session -t agent2
```

**Parallel Testing**:
```bash
# Start server
tmux new-session -d -s server 'python -m http.server 8000'

# Wait for server to be ready
sleep 2

# Run tests
pytest test_api.py

# Clean up
tmux kill-session -t server
```

### Common TMUX Commands

- **Create detached session**: `tmux new-session -d -s <name> '<command>'`
- **Capture session output**: `tmux capture-pane -t <name> -p`
- **Kill session**: `tmux kill-session -t <name>`
- **List active sessions**: `tmux ls`
- **Send commands to session**: `tmux send-keys -t <name> '<command>' C-m`

### Why TMUX Works

1. **Persistent**: Processes keep running after bash command exits
2. **Observable**: Can capture output anytime with `capture-pane`
3. **Controllable**: Can send signals, check status, kill cleanly
4. **Composable**: Works perfectly with run_bash
5. **Standard**: Widely available and reliable

## Using Clyde as a Library

Clyde's agent is a separate Go module that can be embedded in your own applications. This enables you to build custom interfaces (HTTP APIs, GUIs, bots, etc.) while leveraging the same powerful agent logic — without pulling CLI/TUI dependencies.

```bash
# Install just the agent library (no CLI deps)
go get github.com/this-is-alpha-iota/clyde/agent@latest
```

### Quick Example

```go
import (
    "fmt"
    "github.com/this-is-alpha-iota/clyde/agent"
)

func main() {
    agentInstance := agent.New(agent.Config{
        APIKey:    "your-api-key",
        APIURL:    "https://api.anthropic.com/v1/messages",
        ModelID:   "claude-opus-4-6",
        MaxTokens: 64000,
    },
        agent.WithProgressCallback(func(msg string, toolUseID string) {
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

### Agent Dependencies

The agent module has a minimal dependency tree — it does **not** pull `golang.org/x/sys`, readline, or any TUI libraries:

| Direct | Transitive |
|--------|------------|
| `html-to-markdown` v1.6.0 | `goquery` v1.9.2 |
| `godotenv` v1.5.1 | `cascadia` v1.3.2 |
| | `x/net` v0.25.0 |

For complete documentation including all config fields, callback types, re-exported types, and examples (HTTP API, WebSocket, silent mode), see **[agent/README.md](agent/README.md)**.

## Documentation

See [docs/progress.md](docs/progress.md) for detailed technical documentation.
