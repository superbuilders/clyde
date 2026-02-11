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

The REPL includes five integrated tools:

1. **list_files**: List files and directories in any path
2. **read_file**: Read and display file contents
3. **patch_file**: Edit files using find/replace (patch-based approach)
4. **write_file**: Create new files or completely replace file contents
5. **run_bash**: Execute arbitrary bash commands (including gh, git, etc.)

## Documentation

See [PROGRESS.md](PROGRESS.md) for detailed technical documentation.
