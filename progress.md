# Claude REPL Progress Documentation

## Overview
Built a single-file Go CLI that provides a REPL (Read-Eval-Print Loop) interface for conversing with Claude AI, featuring GitHub integration via the `gh` CLI tool.

## What Was Built

### Main Application (`main.go`)
A complete Go application that includes:

#### Core Features
1. **REPL Interface**: Interactive command-line interface for natural conversation with Claude
2. **Anthropic API Integration**: Direct HTTP client for Claude API (Sonnet 4.5 model)
3. **Multi-Tool Support**: Five integrated tools with proper feedback:
   - **GitHub Tool**: Executes `gh` CLI commands for GitHub queries
   - **List Files Tool**: Lists files and directories using `ls -la`
   - **Read File Tool**: Reads and displays file contents
   - **Patch File Tool**: Edits files using find/replace (no size limits!)
   - **Run Bash Tool**: Executes arbitrary bash commands
4. **Conversation History**: Maintains context across multiple turns
5. **Tool Use Feedback**: Shows progress messages when using tools:
   - "→ Running GitHub query..."
   - "→ Listing files..."
   - "→ Reading file..."
   - "→ Patching file..."
   - "→ Running bash command..."

#### Architecture Components
- **Message Types**: User and assistant messages with support for text and tool content
- **Tool Definition**: `github_query` tool with JSON schema for gh commands
- **System Prompt with Decider**: Intelligent system prompt that decides when to use the GitHub tool
- **Tool Execution Loop**: Handles tool_use responses and continues conversation until text response

#### System Prompt Decider
The system prompt includes explicit decision logic for all three tools:
```
IMPORTANT DECIDER: Before responding, determine if you need to use a tool:

GitHub questions - Use github_query for:
- Questions about repositories, PRs, issues, workflows
- Questions about user profile, organizations
- Any "show me", "list", "what are" questions related to GitHub
- Status checks, recent activity, etc.

File system questions - Use list_files for:
- "What files are in X directory?"
- "List files in the current folder"
- "Show me the contents of this directory"

File reading questions - Use read_file for:
- "Show me the contents of X file"
- "What's in X file?"
- "Read X file"
```

### Integration Tests (`main_test.go`)
Comprehensive test suite covering:

1. **TestExecuteGitHubCommand**: Tests gh command execution
   - Valid commands (auth status, api user)
   - Invalid commands (error handling)

2. **TestExecuteListFiles**: Tests file listing execution
   - List current directory
   - Empty path (defaults to current)
   - Non-existent directory (error handling)

3. **TestExecuteReadFile**: Tests file reading execution
   - Read existing file
   - Read non-existent file (error handling)
   - Empty path (error handling)

4. **TestExecuteEditFile**: Tests file editing execution
   - Create new file
   - Overwrite existing file
   - Empty path (error handling)
   - Empty content (valid edge case)

5. **TestCallClaude**: Tests direct API calls
   - Simple greeting response
   - Math question response
   - Validates API response structure

6. **TestHandleConversation**: Tests full conversation flow
   - Single-turn conversations
   - Multi-turn conversations with memory
   - History tracking validation

7. **TestSystemPromptDecider**: Validates system prompt content
   - Checks for required terms (tools, GitHub, file operations)

8. **TestGitHubTool**: Validates tool definition structure
   - Tool name, description, and schema validation

9. **TestListFilesIntegration**: Full end-to-end list_files tool test
   - Tests complete file listing flow with actual tool use
   - Validates tool_use block contains ID
   - Validates tool_result block contains ToolUseID
   - Ensures the full round-trip works with the Claude API

10. **TestReadFileIntegration**: Full end-to-end read_file tool test
   - Tests complete file reading flow with actual tool use
   - Creates test file and validates content is read correctly
   - Validates tool_use block contains ID
   - Validates tool_result block contains ToolUseID
   - Ensures the full round-trip works with the Claude API

11. **TestEditFileIntegration**: Full end-to-end edit_file tool test
    - Tests complete file editing flow with actual tool use
    - Creates test file and validates content is written correctly
    - Validates tool_use block contains ID and correct input parameters
    - Validates tool_result block contains ToolUseID
    - Verifies the file was physically created with correct content
    - Ensures the full round-trip works with the Claude API

12. **TestGitHubQueryIntegration**: Full end-to-end GitHub tool test
    - Tests complete GitHub query flow with actual tool use
    - Validates tool_use block contains ID
    - Validates tool_result block contains ToolUseID
    - Ensures the full round-trip works with the Claude API

### Dependencies
- **Minimal external dependencies**: Uses only Go standard library
  - `net/http`: API communication
  - `encoding/json`: JSON marshaling/unmarshaling
  - `os/exec`: Execute gh commands
  - `bufio`: Read user input

### Environment Setup
- Reads API key from `TS_AGENT_API_KEY` in `.env` file
- Supports both local `.env` and `../coding-agent/.env` paths
- Can override with `ENV_PATH` environment variable

## Installation and Setup

### Prerequisites
```bash
# Go 1.24 or later
go version

# GitHub CLI installed and authenticated
gh auth status
```

### Build
```bash
cd claude-repl
go build -o claude-repl
```

### Run Tests
```bash
go test -v
```

### Run the REPL
```bash
./claude-repl
```

## Usage Examples

### Basic Conversation
```
You: Hello!
Claude: Hello! How can I help you today?

You: What's 5 + 3?
Claude: 8
```

### GitHub Queries
```
You: What repositories do I have?
→ Running GitHub query...
Claude: [Lists your repositories using gh repo list]

You: Show me my recent pull requests
→ Running GitHub query...
Claude: [Lists your PRs using gh pr list]
```

### File Operations
```
You: What files are in the current directory?
→ Listing files...
Claude: [Shows list of files with details]

You: Read the README.md file
→ Reading file...
Claude: [Displays the contents of README.md]

You: Create a file called notes.txt with "Meeting at 3pm"
→ Editing file...
Claude: [Confirms file was created successfully]
```

### Exit
```
You: exit
Goodbye!
```

## Technical Details

### API Configuration
- **Endpoint**: `https://api.anthropic.com/v1/messages`
- **Model**: `claude-sonnet-4-5-20250929`
- **Max Tokens**: 4096
- **API Version**: `2023-06-01`

### Tool Schemas

#### GitHub Query Tool
```json
{
  "name": "github_query",
  "description": "Execute GitHub CLI (gh) commands...",
  "input_schema": {
    "type": "object",
    "properties": {
      "command": {
        "type": "string",
        "description": "The gh command to execute (without 'gh' prefix)"
      }
    },
    "required": ["command"]
  }
}
```

#### List Files Tool
```json
{
  "name": "list_files",
  "description": "List files and directories in a specified path...",
  "input_schema": {
    "type": "object",
    "properties": {
      "path": {
        "type": "string",
        "description": "The directory path to list (defaults to current directory)"
      }
    },
    "required": []
  }
}
```

#### Read File Tool
```json
{
  "name": "read_file",
  "description": "Read the contents of a file...",
  "input_schema": {
    "type": "object",
    "properties": {
      "path": {
        "type": "string",
        "description": "The file path to read"
      }
    },
    "required": ["path"]
  }
}
```

#### Edit File Tool
```json
{
  "name": "edit_file",
  "description": "Edit a file by writing new content to it...",
  "input_schema": {
    "type": "object",
    "properties": {
      "path": {
        "type": "string",
        "description": "The file path to edit"
      },
      "content": {
        "type": "string",
        "description": "The new content to write (replaces entire file)"
      }
    },
    "required": ["path", "content"]
  }
}
```

### Conversation Flow
1. User enters message
2. Message added to conversation history
3. API call made with full history and tools
4. If response contains tool_use:
   - Execute gh command
   - Show "→ Running GitHub query..." to user
   - Send tool results back to API
   - Loop until text response received
5. Display text response to user
6. Update conversation history
7. Repeat

## Test Results
```
=== TestExecuteGitHubCommand       PASS (0.74s)
=== TestExecuteListFiles           PASS (0.01s)
=== TestExecuteReadFile            PASS (0.00s)
=== TestExecuteEditFile            PASS (0.00s)
=== TestCallClaude                 PASS (2.84s)
=== TestHandleConversation         PASS (6.61s)
=== TestSystemPromptDecider        PASS (0.00s)
=== TestGitHubTool                 PASS (0.00s)
=== TestListFilesIntegration       PASS (5.74s)
=== TestReadFileIntegration        PASS (3.58s)
=== TestEditFileIntegration        PASS (4.00s)
=== TestGitHubQueryIntegration     PASS (3.56s)

PASS - All 11 tests completed successfully (27.251s total)
```

## Files Created
1. `main.go` (10+ KB) - Main application with 3 tools
2. `main_test.go` (16+ KB) - Comprehensive test suite with 9 tests
3. `go.mod` - Go module definition
4. `claude-repl` - Compiled binary (8.4 MB)
5. `.env` - API key configuration
6. `PROGRESS.md` - This documentation
7. `README.md` - Project readme

## Key Design Decisions

### Single File Architecture
- Both the application logic fits in one file for simplicity
- Easy to understand and modify
- No complex project structure needed

### Minimal Dependencies
- Uses only Go standard library
- No external packages for API calls (native HTTP client)
- Reduces dependency management complexity

### Tool Feedback
- Simple "→ Running GitHub query..." message with ellipses
- Non-intrusive but informative
- Matches the user's requirement for simple progress updates

### GitHub Tool Design
- Accepts gh commands without 'gh' prefix for cleaner syntax
- Uses os/exec to run commands directly
- Returns both stdout and stderr for comprehensive results

### Error Handling
- API errors displayed to user with status codes
- Tool execution errors returned to Claude for natural language explanation
- File reading errors provide helpful guidance

## Bugs Fixed

### Bug #1: Missing `tool_use_id` in Tool Results (Fixed 2026-02-10)

**Issue**: When sending tool results back to the Claude API, the `tool_use_id` field was missing, causing a 400 error:
```
API error (status 400): {"type":"error","error":{"type":"invalid_request_error",
"message":"messages.4.content.0.tool_result.tool_use_id: Field required"}}
```

**Root Cause**:
- The `ContentBlock` struct only had an `ID` field that mapped to `"id"` in JSON
- When creating tool results, the code used `ID: toolBlock.ID` (line 195)
- The Claude API requires `"tool_use_id"` for tool results, not `"id"`

**Fix Applied**:
1. Added `ToolUseID` field to `ContentBlock` struct with JSON tag `"tool_use_id,omitempty"` (line 55)
2. Changed tool result creation to use `ToolUseID: toolBlock.ID` instead of `ID: toolBlock.ID` (line 195)

**Why Tests Didn't Catch This**:
The original test suite had a critical gap: **no test actually triggered the GitHub tool**. All tests either:
- Used non-GitHub queries that didn't trigger tool use
- Tested components in isolation without the full API round-trip
- Only validated static configuration (tool definition, system prompt)

**Test Improvements**:
Added `TestGitHubQueryIntegration` which:
- Asks a GitHub-related question that triggers tool use
- Validates the full round-trip: question → tool_use → tool_result → final response
- Explicitly checks for `tool_use` blocks with IDs
- Explicitly checks for `tool_result` blocks with `ToolUseID`
- Would have caught this bug immediately since the API rejects malformed tool results

**Lesson Learned**:
Integration tests must exercise the actual user workflows, not just individual components. A test suite that passes 100% but never tests the critical path is worse than no tests at all—it creates false confidence. Always ensure your tests cover the "happy path" that users will actually execute.

## Feature Additions

### File System Tools (Added 2026-02-10)

Added three new tools to complement the GitHub tool, following the same integration testing standards:

**1. List Files Tool (`list_files`)**
- Executes `ls -la` to list files and directories
- Optional `path` parameter (defaults to current directory)
- Returns detailed file listings with permissions, sizes, and timestamps

**2. Read File Tool (`read_file`)**
- Reads file contents using `os.ReadFile`
- Required `path` parameter for the file to read
- Proper error handling for missing files and permission issues

**3. Edit File Tool (`edit_file`)**
- Writes content to files using `os.WriteFile`
- Required `path` and `content` parameters
- Creates new files or overwrites existing ones
- Proper error handling and confirmation messages

**Testing Approach**:
Following the lesson learned from the `tool_use_id` bug, all three new tools include:
- **Unit tests** for the execution functions (`TestExecuteListFiles`, `TestExecuteReadFile`, `TestExecuteEditFile`)
- **Integration tests** that trigger actual tool use (`TestListFilesIntegration`, `TestReadFileIntegration`, `TestEditFileIntegration`)
- Validation of the full round-trip including `tool_use` and `tool_result` blocks
- Explicit checks for `ToolUseID` presence to prevent similar bugs

**Implementation Pattern**:
Used a switch statement in `handleConversation` for cleaner tool dispatching:
```go
switch toolBlock.Name {
case "github_query":
    // GitHub handling
case "list_files":
    // File listing handling
case "read_file":
    // File reading handling
case "edit_file":
    // File editing handling
default:
    err = fmt.Errorf("unknown tool: %s", toolBlock.Name)
}
```

This approach makes it easy to add more tools in the future while maintaining consistent error handling and feedback messages.

### Major Tool Improvements (2026-02-10)

**Replaced edit_file with patch_file**:
The original `edit_file` tool used full-file replacement, which hit Claude API size limits (~14KB+) causing:
- API timeouts
- Missing content parameters
- Files being erased

The new `patch_file` tool uses find/replace:
- Only sends the specific text to change (no size limits)
- Validates old_text is unique in the file
- More intuitive for code editing
- Similar to professional editor find/replace

**Added run_bash tool**:
Enables Claude to execute arbitrary bash commands:
- Run system commands
- Execute scripts
- Check system information
- Any shell/command-line operations

## Future Enhancements (Not Implemented)
- Streaming responses for faster feedback
- More tools (file operations, web search, etc.)
- Configuration file for model selection and parameters
- Command history with arrow key navigation
- Syntax highlighting for code in responses
