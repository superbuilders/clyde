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
- 🔧 **GitHub Integration**: Ask questions about your GitHub account
- 📁 **File System Tools**: List directories and read files
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

You: Create a file called notes.txt with "Meeting at 3pm"
→ Editing file...
Claude: [Confirms file creation]

You: exit
Goodbye!
```

## Requirements

- Go 1.24+
- GitHub CLI (`gh`) installed and authenticated
- Anthropic API key in `.env` file

## Environment Setup

Create a `.env` file:
```bash
TS_AGENT_API_KEY=your-anthropic-api-key
```

Or set the ENV_PATH variable to point to an existing .env file.

## Testing

```bash
go test -v
```

## Available Tools

The REPL includes four integrated tools:

1. **github_query**: Execute GitHub CLI commands (requires `gh` CLI)
2. **list_files**: List files and directories in any path
3. **read_file**: Read and display file contents
4. **edit_file**: Create or modify files with new content

## Documentation

See [PROGRESS.md](PROGRESS.md) for detailed technical documentation.
