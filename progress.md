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
   - **List Files Tool**: Lists files and directories using `ls -la`
   - **Read File Tool**: Reads and displays file contents
   - **Patch File Tool**: Edits files using find/replace (no size limits!)
   - **Write File Tool**: Creates new files or replaces entire file contents
   - **Run Bash Tool**: Executes arbitrary bash commands (including `gh` for GitHub, `git` for version control, test runners, etc.)
4. **Conversation History**: Maintains context across multiple turns
5. **Tool Use Feedback**: Shows progress messages when using tools:
   - "→ Listing files..."
   - "→ Reading file..."
   - "→ Patching file..."
   - "→ Writing file..."
   - "→ Running bash command..."

#### Architecture Components
- **Message Types**: User and assistant messages with support for text and tool content
- **Tool Definitions**: Five tools with JSON schemas for list_files, read_file, patch_file, write_file, and run_bash
- **System Prompt with Decider**: Intelligent system prompt that decides when to use each tool
- **Tool Execution Loop**: Handles tool_use responses and continues conversation until text response

#### System Prompt Decider
The system prompt includes explicit decision logic for all tools:
```
IMPORTANT DECIDER: Before responding, determine if you need to use a tool:

GitHub questions - Use run_bash with gh commands:
- Questions about repositories: run_bash("gh repo list")
- Questions about pull requests: run_bash("gh pr list")
- Questions about issues: run_bash("gh issue list")
- User profile info: run_bash("gh api user")
- Any GitHub queries: run_bash("gh <command>")

File system questions - Use list_files for:
- "What files are in X directory?"
- "List files in the current folder"
- "Show me the contents of this directory"

File reading questions - Use read_file for:
- "Show me the contents of X file"
- "What's in X file?"
- "Read X file"

File editing questions - Use patch_file for:
- "Add X to the file"
- "Change X to Y in the file"
- "Update the function to do Z"
- "Fix the bug by changing X"

File writing questions - Use write_file for:
- "Create a new file with X content"
- "Write X to file Y"
- "Replace the entire contents of file Z"
- Creating new files from scratch

Bash execution - Use run_bash for:
- "Run X command"
- "Execute Y script"
- "Check system information"
- Any shell/command-line operations
- Git operations: run_bash("git status"), run_bash("git commit -m 'message'")
- GitHub CLI: run_bash("gh repo list"), run_bash("gh pr list")
- Package managers, build tools, test runners, etc.
```

### Integration Tests (`main_test.go`)
Comprehensive test suite covering:

1. ~~**TestExecuteGitHubCommand**~~: REMOVED - github_query tool deprecated in favor of run_bash

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

5. **TestExecuteRunBash**: Tests bash command execution
   - Simple echo command
   - Command with output
   - Empty command (error handling)
   - Invalid command (error handling)
   - Command that exits with error

6. **TestExecuteWriteFile**: Tests file writing execution
   - Create new file
   - Replace existing file
   - Empty path (error handling)
   - Write empty content
   - Write multiline content

7. **TestExecutePatchFile**: Tests patch file execution
   - Replace unique text
   - Old text not found (error handling)
   - Non-unique old text (error handling)
   - Empty old text (error handling)
   - Delete text (empty new_text)

8. **TestCallClaude**: Tests direct API calls
   - Simple greeting response
   - Math question response
   - Validates API response structure

9. **TestHandleConversation**: Tests full conversation flow
   - Single-turn conversations
   - Multi-turn conversations with memory
   - History tracking validation

10. **TestSystemPromptDecider**: Validates system prompt content
   - Checks for required terms (tools, run_bash, gh commands)
   - Verifies old github_query tool is NOT present

11. ~~**TestGitHubTool**~~: REMOVED - github_query tool deprecated

12. **TestListFilesIntegration**: Full end-to-end list_files tool test
   - Tests complete file listing flow with actual tool use
   - Validates tool_use block contains ID
   - Validates tool_result block contains ToolUseID
   - Ensures the full round-trip works with the Claude API

13. **TestReadFileIntegration**: Full end-to-end read_file tool test
   - Tests complete file reading flow with actual tool use
   - Creates test file and validates content is read correctly
   - Validates tool_use block contains ID
   - Validates tool_result block contains ToolUseID
   - Ensures the full round-trip works with the Claude API

14. **TestEditFileIntegration**: Full end-to-end edit_file tool test (DEPRECATED)
    - Tests complete file editing flow with actual tool use
    - Creates test file and validates content is written correctly
    - Validates tool_use block contains ID and correct input parameters
    - Validates tool_result block contains ToolUseID
    - Verifies the file was physically created with correct content
    - Ensures the full round-trip works with the Claude API
    - **NOTE**: Skipped due to edit_file being replaced by patch_file

15. **TestGitHubQueryIntegration**: Full end-to-end GitHub query test using run_bash
   - Tests complete GitHub query flow using run_bash with gh commands
   - Validates tool_use block contains ID and command parameter
   - Validates tool_result block contains ToolUseID
   - Ensures the full round-trip works with the Claude API
   - **UPDATED**: Now uses run_bash instead of deprecated github_query tool

16. **TestRunBashIntegration**: Full end-to-end run_bash tool test
    - Tests complete bash command execution flow
    - Tests whoami command execution
    - Tests echo command with output verification
    - Tests error handling with exit 1 command
    - Validates tool_use block contains ID and command parameter
    - Validates tool_result block contains ToolUseID
    - Ensures the full round-trip works with the Claude API

17. **TestWriteFileIntegration**: Full end-to-end write_file tool test
    - Tests creating new file with write_file tool
    - Tests replacing existing file contents
    - Tests writing multiline file content
    - Validates tool_use block contains ID and correct input parameters
    - Validates tool_result block contains ToolUseID
    - Verifies the file was physically created/replaced with correct content
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

### GitHub Queries (via run_bash)
```
You: What repositories do I have?
→ Running bash command...
Claude: [Lists your repositories using 'gh repo list']

You: Show me my recent pull requests
→ Running bash command...
Claude: [Lists your PRs using 'gh pr list']
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
→ Writing file...
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

#### Patch File Tool
```json
{
  "name": "patch_file",
  "description": "Edit a file by finding and replacing text...",
  "input_schema": {
    "type": "object",
    "properties": {
      "path": {
        "type": "string",
        "description": "The file path to edit"
      },
      "old_text": {
        "type": "string",
        "description": "The exact text to find and replace (must be unique)"
      },
      "new_text": {
        "type": "string",
        "description": "The new text to replace old_text with"
      }
    },
    "required": ["path", "old_text", "new_text"]
  }
}
```

#### Write File Tool
```json
{
  "name": "write_file",
  "description": "Write content to a file. Creates new file or completely replaces existing file...",
  "input_schema": {
    "type": "object",
    "properties": {
      "path": {
        "type": "string",
        "description": "The file path to write to"
      },
      "content": {
        "type": "string",
        "description": "The complete content to write to the file"
      }
    },
    "required": ["path", "content"]
  }
}
```

#### Run Bash Tool
```json
{
  "name": "run_bash",
  "description": "Execute arbitrary bash commands...",
  "input_schema": {
    "type": "object",
    "properties": {
      "command": {
        "type": "string",
        "description": "The bash command to execute"
      }
    },
    "required": ["command"]
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
=== TestExecuteListFiles           PASS (0.01s)
=== TestExecuteReadFile            PASS (0.00s)
=== TestExecuteRunBash             PASS (0.01s)
=== TestExecuteWriteFile           PASS (0.00s)
=== TestExecutePatchFile           PASS (0.00s)
=== TestExecuteEditFile            SKIP (deprecated)
=== TestCallClaude                 PASS (3.33s)
=== TestHandleConversation         PASS (5.61s)
=== TestSystemPromptDecider        PASS (0.00s)
=== TestListFilesIntegration       PASS (6.76s)
=== TestReadFileIntegration        PASS (4.10s)
=== TestEditFileIntegration        SKIP (deprecated)
=== TestEditFileWithLargeContent   SKIP (deprecated)
=== TestGitHubQueryIntegration     PASS (4.31s) - now uses run_bash
=== TestRunBashIntegration         PASS (13.31s)
=== TestWriteFileIntegration       PASS (11.16s)

PASS - All tests completed successfully (47.47s total)
13 tests passed, 3 tests skipped
```

## Files Created
1. `main.go` (16.2 KB) - Main application with 5 tools (down from 6 after github_query deprecation)
2. `main_test.go` (41+ KB) - Comprehensive test suite with 13 active tests
3. `go.mod` - Go module definition
4. `claude-repl` - Compiled binary (8.0 MB)
5. `.env` - API key configuration
6. `PROGRESS.md` - This documentation
7. `README.md` - Project readme
8. `todos.md` - Priority task list

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

## Tool Deprecations

### GitHub Query Tool (Deprecated 2026-02-10)

**Removed**: The dedicated `github_query` tool has been deprecated and removed from the codebase.

**Rationale**: 
- Redundant with `run_bash` tool
- `gh` commands work perfectly via `run_bash`
- Example: `run_bash("gh repo list")` vs `github_query("repo list")`
- Less code to maintain
- Consistent pattern: all external CLI tools go through bash

**Migration**:
```
OLD: github_query("repo list")
NEW: run_bash("gh repo list")

OLD: github_query("pr list")  
NEW: run_bash("gh pr list")

OLD: github_query("api user")
NEW: run_bash("gh api user")
```

**Changes Made**:
1. ✅ Removed `githubTool` from tools array in `callClaude()`
2. ✅ Removed `executeGitHubCommand()` function
3. ✅ Removed `case "github_query":` from switch statement
4. ✅ Updated system prompt to use `run_bash` with `gh` commands
5. ✅ Updated tests to use bash for GitHub operations
6. ✅ Updated documentation (README, progress.md)

**Test Updates**:
- Removed `TestExecuteGitHubCommand` (no longer needed)
- Removed `TestGitHubTool` (tool no longer exists)
- Updated `TestSystemPromptDecider` to check for `run_bash` and `gh` instead of `github_query`
- Updated `TestGitHubQueryIntegration` to expect `run_bash` tool usage with `gh` commands

All tests pass: 13 tests passed, 3 skipped (deprecated edit_file tests), 47.47s total.

**Impact & Results**:
- **Code reduction**: Net -56 lines (197 removed, 141 added)
- **Simplified architecture**: 5 tools instead of 6
- **Improved consistency**: All CLI tools now use run_bash
- **Better flexibility**: Can use any gh command without pre-definition
- **Test coverage maintained**: All 13 active tests pass
- **Binary size**: 8.0 MB (optimized)
- **Zero breaking changes**: Migration path is straightforward

**Commit**: `844ac68` - Deprecate github_query tool in favor of run_bash

### System Prompt Enhancement: progress.md Philosophy (Added 2026-02-10)

**Priority #2 Completed**: Added comprehensive documentation and memory model instructions to system prompt.

**Problem**: The AI had to be reminded to update progress.md after completing Priority #1. This should have been automatic.

**Solution**: Enhanced system prompt with explicit instructions about documentation and memory management:

**Key Additions**:
1. **Read progress.md at start** of complex tasks to understand project history
2. **Update progress.md when**:
   - Completing major tasks/milestones
   - Discovering and fixing bugs
   - Making design decisions
   - Learning important patterns
3. **Always update progress.md BEFORE final commit** - Don't wait to be reminded
4. **Keep documentation structured** - Not a message dump, but curated synthesis
5. **Treat progress.md as memory** - It persists across conversations

**Impact**:
- System prompt expanded from 2.1 KB to 2.8 KB
- AI now has clear guidance on documentation practices
- Should proactively maintain progress.md going forward
- Aligns with Memory Model philosophy established earlier

**Example Instructions Added**:
```
DOCUMENTATION & MEMORY:
When working on tasks, especially complex ones:
1. Read progress.md (if it exists) at the start...
2. Update progress.md when you...
3. ALWAYS update progress.md BEFORE making the final commit...
4. Keep progress.md structured and curated...
5. Treat progress.md as YOUR memory...
```

**Lesson**: Meta-documentation instructions are just as important as tool instructions. The AI needs to know not just *what* to do, but *when* and *why* to document.

## Current Status (2026-02-10)

**Active Tools**: 5
1. `list_files` - Directory listings
2. `read_file` - Read file contents
3. `patch_file` - Find/replace edits
4. `write_file` - Create/replace files
5. `run_bash` - Execute any bash command (including gh, git, npm, go test, etc.)

**Test Suite**: 13 tests passing, 3 skipped
- Total runtime: ~47 seconds (stable)
- Full integration coverage for all tools
- No flaky tests

**Binary**: 8.0 MB compiled binary
- Single-file architecture maintained
- Zero external dependencies
- Fast startup time

**System Prompt**: 2.8 KB (expanded from 2.1 KB)
- Includes comprehensive tool decision logic
- **NEW (Priority #2)**: Includes progress.md philosophy and memory model
- Instructs AI to read and update progress.md proactively
- AI should now document changes automatically

**Next Priority**: #3 - Better Tool Progress Messages
- Show more context in progress messages
- Example: "→ Reading file: main.go" instead of just "→ Reading file..."
- Estimated time: 30 minutes

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

**Added write_file tool** (2026-02-10):
Provides a dedicated tool for creating new files or replacing entire file contents:
- Creates new files with specified content
- Completely replaces existing file contents
- Separate from patch_file for clarity of purpose
- Better for creating new files from scratch
- Returns appropriate messages ("created" vs "replaced")
- Includes comprehensive unit and integration tests

**Testing Standards Maintained**:
Both run_bash and write_file tools include:
- Unit tests for execution functions (`TestExecuteRunBash`, `TestExecuteWriteFile`)
- Integration tests with full API round-trips (`TestRunBashIntegration`, `TestWriteFileIntegration`)
- Multiple sub-tests covering different scenarios (success, errors, edge cases)
- Validation of tool_use and tool_result blocks
- Explicit checks for ToolUseID to prevent regression bugs

## Design Philosophy & Principles

### Memory Model (Established 2026-02-10)
**Decision**: Do NOT implement traditional message history persistence.

**Philosophy**: Message history compaction is the wrong abstraction for coding agents. Curated documentation > raw chat logs.

**Approach**:
- Use `progress.md` as the canonical source of truth for important learnings and project state
- AI reads `progress.md` at the start of complex tasks
- AI updates `progress.md` with important learnings, bugs fixed, and design decisions
- Treat `progress.md` as the "memory" rather than raw conversation history
- Keep `progress.md` structured and organized (not a dump of all messages)
- Human maintains editorial control over what's "remembered"

### Error Handling Philosophy (Established 2026-02-10)
- Helpful error messages > raw debug output
- Suggest solutions, not just report failures
- Fail fast with clear guidance
- Automatic error recovery (no user confirmation required)
- Agent should attempt recovery without asking
- If recovery fails, explain what went wrong and what was tried
- User can always interrupt with Ctrl+C if needed

### Tool Design Philosophy (Established 2026-02-10)
- Each tool does one thing well
- Compose tools for complex operations
- Clear feedback for all operations
- **Lean into standard tools**: Use bash for git, gh CLI, etc. rather than custom wrappers
- Avoid redundant abstractions (e.g., no dedicated git or test wrappers when bash suffices)

### Multi-File Operations Philosophy (Established 2026-02-10)
- Use git for rollback, not custom transaction logic
- Users should commit before risky operations
- Agent can suggest `git commit` before multi-file changes
- Keep it simple: git is good, use git
- Atomic operations where possible
- Search before edit for context
- Coordinate related changes

## Architecture Decisions

### Why No Dedicated Git Tool (Decided 2026-02-10)
**Question**: Should we add a `git` tool for version control operations?

**Answer**: NO. Use `run_bash` for all git operations.
- Git commands are simple enough: `git commit`, `git status`, `git diff`
- No need for a dedicated wrapper
- Consistent with philosophy: lean into standard tools

### Why No Test Wrapper (Decided 2026-02-10)
**Question**: Do we need a `test` tool wrapper, or is `run_bash` sufficient?

**Answer**: `run_bash` is sufficient.
- Tests are just bash commands: `go test -v`, `npm test`, `pytest`
- No abstraction needed
- Keep it simple

### Future: Code Organization & Architecture Separation
**Long-term Vision**: Separate the agent from the CLI so the same agent logic can be called through:
- 💻 CLI (current REPL interface)
- 🌐 HTTP API (REST endpoints)
- 🖥️ GUI (desktop or web interface)
- 🔧 Bash scripts (programmatic access)
- 📦 Go package (import into other projects)

**Key Abstraction**: The agent should be interface-driven:
```go
type Agent interface {
    HandleMessage(input string) (response string, err error)
    RegisterTool(tool Tool) error
    GetHistory() []Message
}
```

This allows different "frontends" (CLI, API, GUI) to use the same agent backend.

## Future Enhancements (Not Implemented)
- Streaming responses for faster feedback
- Configuration file for model selection and parameters
- Command history with arrow key navigation
- Syntax highlighting for code in responses
