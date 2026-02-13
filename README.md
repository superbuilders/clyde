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
- Anthropic API key in `.env` file
- Brave Search API key in `.env` file (optional, for web_search tool)

## Environment Setup

Create a `.env` file:
```bash
TS_AGENT_API_KEY=your-anthropic-api-key
BRAVE_SEARCH_API_KEY=your-brave-search-api-key  # Optional: for web_search
# Get free API key at: https://brave.com/search/api/
# Free tier: 2,000 searches/month
```

Or set the ENV_PATH variable to point to an existing .env file.

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
