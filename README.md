# Claude REPL

A single-file Go CLI that provides a REPL interface for talking to Claude AI with GitHub integration.

## Quick Start

```bash
# Run the REPL
./claude-repl

# Or build from source
go build -o claude-repl
./claude-repl
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
go install github.com/yourusername/claude-repl@latest
```

After installation, create a config file in your home directory:
```bash
mkdir -p ~/.claude-repl
cat > ~/.claude-repl/config << 'EOF'
TS_AGENT_API_KEY=your-anthropic-api-key
BRAVE_SEARCH_API_KEY=your-brave-api-key  # Optional
EOF
```

Get your API keys:
- **Anthropic API**: https://console.anthropic.com/
- **Brave Search API** (optional): https://brave.com/search/api/ - Free tier: 2,000 searches/month

### Option 2: Build from source (for development)
```bash
git clone https://github.com/yourusername/claude-repl
cd claude-repl
go build -o claude-repl
./claude-repl
```

## Configuration

The application looks for configuration in the following order:

1. **ENV_PATH environment variable** (highest priority override)
   ```bash
   export ENV_PATH=/path/to/custom/config
   claude-repl
   ```

2. **`.env` in current directory** (for project-specific config)
   ```bash
   # Create .env in your project directory
   echo "TS_AGENT_API_KEY=your-key" > .env
   claude-repl
   ```

3. **`~/.claude-repl/config`** (recommended for global installation)
   ```bash
   # Already created during installation (see above)
   claude-repl  # Works from any directory!
   ```

4. **`~/.claude-repl`** (legacy format, direct file without subdirectory)
   ```bash
   # Supported for backward compatibility
   echo "TS_AGENT_API_KEY=your-key" > ~/.claude-repl
   ```

### Configuration File Format
```bash
# Claude REPL Configuration
# Required
TS_AGENT_API_KEY=sk-ant-your-key-here

# Optional (for web_search tool)
BRAVE_SEARCH_API_KEY=BSA-your-key-here
```

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
./claude-repl

# When satisfied, rebuild to embed the new prompt
go build -o claude-repl
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

## Documentation

See [PROGRESS.md](PROGRESS.md) for detailed technical documentation.
