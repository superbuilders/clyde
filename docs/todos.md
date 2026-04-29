# Clyde Progress Documentation

## Project Rename (2026-02-18)

**From**: claude-repl / go-coding-agent  
**To**: clyde

**Reason**: Standardize naming across local and remote repositories. The go.mod declared `claude-repl` while GitHub repo was `go-coding-agent`, causing installation conflicts.

**Changes Made**:
1. **go.mod**: Updated module path to `github.com/this-is-alpha-iota/clyde`
2. **All Go files**: Updated imports from `claude-repl/*` to `github.com/this-is-alpha-iota/clyde/*`
3. **Binary name**: Changed from `claude-repl` to `clyde`
4. **Config directory**: Changed from `~/.claude-repl/` to `~/.clyde/`
5. **README.md**: Updated all references to use "clyde"
6. **User-Agent**: Changed browse tool User-Agent to `clyde/1.0`
7. **Startup banner**: Changed from "Claude REPL" to "Clyde - AI Coding Agent"

**Installation Now Works**:
```bash
go install github.com/this-is-alpha-iota/clyde@latest
```

**Config Setup**:
```bash
mkdir -p ~/.clyde
cat > ~/.clyde/config << 'EOF'
TS_AGENT_API_KEY=your-anthropic-api-key
BRAVE_SEARCH_API_KEY=your-brave-api-key  # Optional
EOF
```

**Name Origin**: "Clyde" is a friendly, memorable name that fits the AI coding assistant persona.

## Overview
Built a Go CLI that provides a REPL (Read-Eval-Print Loop) interface for conversing with Claude AI, featuring GitHub integration via the `gh` CLI tool.

**Architecture** (as of 2026-02-13): Modular package-based structure
- `api/` - Claude API client and types
- `config/` - Configuration and .env loading
- `agent/` - Conversation orchestration
- `tools/` - Tool registry and 10 tool implementations
- `prompts/` - System prompt (external file, embedded in binary)
- `main.go` - CLI REPL interface (50 lines)

**Original Architecture** (until 2026-02-13): Single-file monolith
- `main.go` - Everything in one 1,652-line file

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

18. **TestExecuteGrep**: Unit tests for grep execution
    - Tests searching for patterns across multiple files
    - Tests file pattern filtering (*.go, *.md, etc.)
    - Tests handling of no matches found
    - Tests error cases (non-existent directories, empty patterns)
    - Tests searching in current directory with empty path

19. **TestGrepIntegration**: Full end-to-end grep tool test
    - Tests searching for function definitions with file pattern filter
    - Tests searching for TODO comments across all files
    - Tests graceful handling of no matches (with helpful suggestions)
    - Validates tool_use block contains ID and correct input parameters
    - Validates tool_result block contains ToolUseID
    - Verifies grep output includes file paths and line numbers
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

#### Grep Tool
```json
{
  "name": "grep",
  "description": "Search for patterns across multiple files. Returns file paths and matching lines with context.",
  "input_schema": {
    "type": "object",
    "properties": {
      "pattern": {
        "type": "string",
        "description": "The search pattern (text or regex)"
      },
      "path": {
        "type": "string",
        "description": "Directory to search (defaults to current directory)"
      },
      "file_pattern": {
        "type": "string",
        "description": "Optional: filter by file pattern using glob syntax (e.g., '*.go', '*.md')"
      }
    },
    "required": ["pattern"]
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
=== TestExecuteGrep                PASS (0.01s) - NEW ✨
=== TestExecutePatchFile           PASS (0.00s)
=== TestExecuteEditFile            SKIP (deprecated)
=== TestCallClaude                 PASS (2.21s)
=== TestHandleConversation         PASS (8.00s)
=== TestSystemPromptDecider        PASS (0.00s)
=== TestListFilesIntegration       PASS (6.98s)
=== TestReadFileIntegration        PASS (3.57s)
=== TestEditFileIntegration        SKIP (deprecated)
=== TestEditFileWithLargeContent   SKIP (deprecated)
=== TestGitHubQueryIntegration     PASS (3.56s)
=== TestRunBashIntegration         PASS (10.96s)
=== TestWriteFileIntegration       PASS (12.51s)
=== TestGrepIntegration            PASS (22.73s) - NEW ✨

PASS - All tests completed successfully (71.1s total)
16 tests passed, 3 tests skipped
```

## Files Created
1. `main.go` (20.8 KB) - Main application with 6 tools (grep added!)
2. `main_test.go` (50+ KB) - Comprehensive test suite with 16 active tests
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

### Bug #1: max_tokens Too Low Causes Infinite Loop (Fixed 2026-03-09)

**Issue**: When Claude generates large content for tool parameters (like comprehensive documents), it hits the output token limit mid-generation, resulting in incomplete tool_use blocks and infinite retry loops.

**Symptoms**:
```
→ Writing file: /tmp/doc.md (0 bytes)
💾 Cache hit: 4205 tokens (100% of input)
→ Writing file: /tmp/doc.md (0 bytes)
💾 Cache hit: 4318 tokens (100% of input)
→ Writing file: /tmp/doc.md (0 bytes)
[continues indefinitely...]
```

**Root Cause**:
- `MaxTokens` was set to 4,096 (only 6.4% of model capacity)
- When generating large content, Claude hits token limit before completing tool parameters
- API returns `stop_reason: "max_tokens"` with incomplete tool_use block
- The `content` field is completely MISSING (not empty, but absent)
- Tool receives nil content and returns error
- Claude interprets error as retryable and tries again
- Same token limit hit again → infinite loop

**Debug Findings**:
```
[DEBUG API] Stop reason: max_tokens          ← Truncated mid-generation
[DEBUG API] Block 1: content_field=MISSING   ← Field never completed
```

**Fix Applied**:
Changed `config/config.go` line 46:
```go
// Before
MaxTokens: 4096,

// After  
MaxTokens: 64000, // Match industry standard (Aider) - full model capacity
```

**Industry Comparison**:
- **Aider**: 64,000 tokens (100% of model capacity)
- **Claude Code**: 32,000 tokens (50% of capacity)
- **OpenCode**: 32,000 tokens (50% of capacity)
- **Clyde (before)**: 4,096 tokens (6.4% - too low!)
- **Clyde (after)**: 64,000 tokens (100% - matches industry leader)

**Impact**:
- 16x increase in output capacity
- Prevents truncation during large document generation
- No cost increase (only pay for tokens actually generated)
- Matches best-in-class tools (Aider)

**Verification**:
Test with 5-section document generation:
- ✅ File created successfully (17 KB, 436 lines)
- ✅ Single write attempt (no loop)
- ✅ Stop reason: `tool_use` (completed normally)

**Lesson Learned**:
Always match industry standards for critical configuration values. A conservative default (4,096) caused significant usability issues. Research showed all major AI coding tools use much higher limits (32K-64K). When in doubt, use the model's full capacity - there's no cost penalty for setting a higher ceiling.

### Bug #2: Missing `tool_use_id` in Tool Results (Fixed 2026-02-10)

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
1. Read progress.md at the start...
2. Update progress.md when you...
3. Always update progress.md BEFORE the final commit...
4. Keep progress.md structured and curated...
5. Treat progress.md as YOUR memory...
```

**Lesson**: Meta-documentation instructions are just as important as tool instructions. The AI needs to know not just *what* to do, but *when* and *why* to document.

### Better Tool Progress Messages (Added 2026-02-10)

**Priority #3 Completed**: Enhanced all tool progress messages to show context and relevant parameters.

**Problem**: Generic progress messages like "→ Reading file..." didn't tell users which file or what was happening.

**Solution**: Updated each tool's display message to include relevant context:

**Before**:
```
→ Listing files...
→ Reading file...
→ Patching file...
→ Running bash command...
→ Writing file...
```

**After**:
```
→ Listing files: . (current directory)
→ Reading file: main.go
→ Patching file: todos.md (+353 bytes)
→ Running bash: go test -v
→ Writing file: progress.md (42.5 KB)
```

**Implementation Details**:
1. **list_files**: Shows path, with special handling for current directory
2. **read_file**: Shows the file path being read
3. **patch_file**: Shows file path and size change (+/- bytes)
4. **run_bash**: Shows the command (truncated if > 60 chars)
5. **write_file**: Shows file path and formatted size (bytes/KB/MB)

**Code Changes**:
- Updated 5 display message locations in `handleConversation()`
- Added size formatting for write_file (bytes → KB → MB)
- Added command truncation for long bash commands
- Net change: +921 bytes in main.go

**Impact**:
- Users can see exactly what's happening at a glance
- Better transparency without being verbose
- Helps with debugging when operations take time
- All tests still pass (13 passed, 3 skipped)

**Example Output from Tests**:
```
→ Listing files: . (current directory)
→ Reading file: test_read_file.txt
→ Running bash: gh api user
→ Writing file: test_write_integration_new.txt (51 bytes)
→ Writing file: progress.md (42.5 KB)
```

### Better Error Handling & Messages (Added 2026-02-10)

**Priority #4 Completed**: Comprehensive error handling improvements with helpful, context-aware error messages.

**Problem**: Error messages were too generic and didn't help users understand what went wrong or how to fix it:
```
BAD:  "failed to list files: exit status 2"
BAD:  "failed to read file: no such file"
BAD:  "old_text not found in file"
BAD:  "command failed: exit status 127"
```

**Solution**: Enhanced all tool execution functions with detailed, helpful error messages that:
1. Explain what went wrong clearly
2. Provide context about the error
3. Suggest concrete steps to fix the problem
4. Include examples where helpful

**Philosophy**:
Error messages should be **teachers**, not just reporters. Every error is an opportunity to help the user learn and succeed.

### Bug #3: Multi-Line Display Messages Break Session Resume (Fixed 2026-04-16)

**Issue**: Resuming a session that contains a `run_bash` tool call with a multi-line command causes a 400 error from the Claude API:
```
messages.20.content.0: unexpected tool_use_id found in tool_result blocks: toolu_0119dBVnRa6rCiYpbz5Xaa2X.
Each tool_result block must have a corresponding tool_use block in the previous message.
```

**Root Cause — Three contributing factors**:

1. **Writer (`displayRunBash`)**: Returns `"→ Running bash: " + command` with NO truncation. If the command has newlines (common for complex scripts), the display message is multi-line.

2. **Writer (`FormatToolUseID`)**: Appends `[toolu_xxx]` to the END of the display message string. For multi-line messages, this puts the ID on the LAST line instead of line 1.

3. **Reader (`extractToolUseMetadata`)**: Only searches `lines[0]` for the `[toolu_xxx]` regex. For multi-line display messages, the ID is on a later line and is never found.

**Cascade**:
- Tool-use block is SKIPPED (no ID found → reconstruction logs warning, doesn't add to history)
- Corresponding tool-result block still has the explicit ID and IS added
- API receives a `tool_result` referencing a `tool_use_id` that doesn't exist in any preceding assistant message → 400 error

**Evidence**: In the failing session, single-line tools (glob, read_file) had `[toolu_]` on line 1 (3-line files). Multi-line `run_bash` commands had `[toolu_]` on lines 42, 66, and 97 (44–99 line files).

**Fix — Two parts**:

1. **Fix the Reader** (backward-compatible, fixes existing sessions): In `extractToolUseMetadata()`, scan ALL lines for the `[toolu_xxx]` regex instead of only `lines[0]`.

2. **Fix the Writer** (prevents future occurrences): In `FormatToolUseID()`, only use the first line of the progress message when appending the tool use ID, so the ID is always on line 1 regardless of display message content.

**Test gap**: `TestReconstructHistory_MultipleToolCalls` wrote tool-use files with the ID on line 1 and in non-interleaved order. Neither pattern matched real multi-line `run_bash` sessions.

**Tests added**:
- `TestReconstructHistory_MultiLineBashCommand` — end-to-end reconstruction of a multi-line bash tool-use where `[toolu_xxx]` is on the last display line (legacy format). Verifies both the tool_use and tool_result blocks are reconstructed with matching IDs and no warnings.
- `TestExtractToolUseMetadata/multiline_bash_id_on_first_line` — new writer format (ID on line 1)
- `TestExtractToolUseMetadata/multiline_bash_legacy_id_on_last_display_line` — old writer format (ID on last display line)
- `TestSessionToolUseIDsInProgressLines` — two new multi-line cases verifying `FormatToolUseID` places ID on line 1

**Reproduction**: Start a session, ask Claude to run a multi-line bash command (containing `\n`), exit, resume with `--resume`, type any message → 400 error.

## Current Status (2026-02-23)

**Latest Update**: System Prompt Enhancement - TMUX for Background Processes & Subagents ✅

**Active Tools**: 11 ✨
1. `list_files` - Directory listings with helpful error messages
2. `read_file` - Read file contents with size warnings and validation
3. `patch_file` - Find/replace edits with detailed guidance for common issues
4. `write_file` - Create/replace files with safety warnings for large files
5. `run_bash` - Execute any bash command with exit code explanations
6. `grep` - Search for patterns across multiple files with context
7. `glob` - Find files matching patterns (fuzzy file finding)
8. `multi_patch` - Coordinated multi-file edits with automatic rollback
9. `web_search` - Search the internet using Brave Search API
10. `browse` - Fetch and read web pages with optional AI extraction
11. `include_file` - Include images in conversation for vision analysis

**Test Suite**: Clean and comprehensive
- 17 unit tests passing (no API key required)
- 10 integration tests skipped (require API keys for Claude/Brave APIs)
- Total runtime: ~17 seconds (unit tests only)
- Full integration coverage for all tools (when API keys present)
- No flaky tests, no deprecated tests
- Zero build errors or test compilation issues

**Completed Priorities**: 19 / 19 from original todos.md ✨✨✨

**ALL MAIN PRIORITIES COMPLETE!** 🎉🎉🎉

## Design Philosophy & Principles

### Memory Model (Established 2026-02-10)
**Decision**: Do NOT implement traditional message history persistence.

**Philosophy**: Message history compaction is the wrong abstraction for coding agents. Curated documentation > raw chat logs.

### Error Handling Philosophy (Established 2026-02-10)
- Helpful error messages > raw debug output
- Suggest solutions, not just report failures
- Fail fast with clear guidance
- Automatic error recovery (no user confirmation required)

### Tool Design Philosophy (Established 2026-02-10)
- Each tool does one thing well
- Compose tools for complex operations
- Clear feedback for all operations
- **Lean into standard tools**: Use bash for git, gh CLI, etc. rather than custom wrappers

## TUI Spec Written

Created `docs/tui.md` — a comprehensive terminal UI specification covering:

- **5 log levels** (silent → quiet → normal → verbose → debug) controlling display verbosity
- **Color scheme** with theme-aware ANSI colors (bold cyan for user, bold green for agent, bold yellow for tools, dim for secondary content, dim magenta for thinking, red for debug)
- **Thinking traces** — enable Claude API `thinking` parameter by default; display at normal level (truncated) and above
- **Tool output bodies** — shown at normal level and above with newline separation (not just the `→` progress line)
- **Truncation rules** — 25 lines for tool output, 50 for thinking, 2000 chars/line; all removed at verbose
- **Loading spinner** — braille dots (`⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏`), 2 frames/symbol at 60fps (~30 symbols/sec), on second-to-last terminal line
- **Prompt line** — git branch + dirty indicator + context window % + `You:` label
- **Text input** — cursor movement, multiline support, no length limit (replacing raw `bufio.NewReader`)
- **Cache display** — moved to verbose/debug only, shown as fraction; context % on prompt line instead

## Compaction Spec Written

Created `docs/compaction.md` — a pre-spec discussion document covering agentic multi-step compaction, smarter tool-result summarization, git-centric state tracking, preserving the initial user message verbatim, a history search tool, feeding recent context into the summarizer, and automatic trigger strategy.

---

# TODO — User Stories

Stories are ordered by implementation priority. TUI stories come first (they improve the daily experience for every session), followed by compaction stories (which unlock reliable long-running autonomous missions).

---

## TUI Stories

Stories are dependency-ordered: foundations first (1–2), then UI chrome (3–6), then the heaviest feature work last (7–8).

---

### TUI-1: Log Level Infrastructure & CLI Flags  ✅ DONE

**As a** user running Clyde in CLI or TUI mode,
**I want** to control the verbosity of output via `--silent`, `-q`/`--quiet`, `-v`/`--verbose`, and `--debug` flags,
**so that** I see exactly the amount of detail I need for my workflow (scripting, normal use, debugging).

**Depends on**: nothing (foundation)

**Acceptance Criteria**:
- [x] A `LogLevel` type is defined with five values: Silent, Quiet, Normal, Verbose, Debug.
- [x] CLI argument parsing recognizes `--silent`, `-q`/`--quiet`, (no flag = Normal), `-v`/`--verbose`, `--debug`.
- [x] The parsed log level is threaded into the agent via `AgentOption` (e.g., `WithLogLevel`).
- [x] The progress callback receives the log level and can gate output accordingly.
- [x] At Silent level, nothing is printed to stdout or stderr (side-effects only).
- [x] At Quiet level, only `→` tool progress lines and the final agent response are printed.
- [x] At Normal level, tool output bodies and thinking traces are also printed (truncated per TUI-7).
- [x] At Verbose level, all truncation is removed.
- [x] At Debug level, additional harness diagnostics (token counts, latency, request/response sizes) are printed.
- [x] Existing REPL and CLI mode tests still pass.
- [x] New unit tests verify flag parsing for all five levels.
- [x] New unit tests verify that the correct content is emitted (or suppressed) at each level, using a captured output buffer.

---

### TUI-2: Color Scheme & Themed Output  ✅ DONE

**As a** user with either a dark or light terminal theme,
**I want** conversation output to be color-coded (bold cyan for `You:`, bold green for `Claude:`, bold yellow for tool labels, dim for secondary content, dim magenta for thinking, red for debug),
**so that** I can visually scan a long session and immediately distinguish user input, agent responses, tool activity, thinking traces, and debug information.

**Depends on**: nothing (foundation)

**Acceptance Criteria**:
- [x] A `colors` or `style` package is created with helper functions for each semantic style (e.g., `UserLabel()`, `AgentLabel()`, `ToolLabel()`, `Dim()`, `ThinkingStyle()`, `DebugStyle()`).
- [x] Helpers emit ANSI escape codes. They use named ANSI colors (cyan, green, yellow, magenta, red) and the dim/faint attribute — never hardcoded RGB or black/white.
- [x] A `NO_COLOR` or `TERM=dumb` environment variable disables all color output (standard convention).
- [x] The `You:` label is rendered in bold cyan; user input text is default foreground.
- [x] The `Claude:` label is rendered in bold green; agent response text is default foreground.
- [x] Tool `→` progress lines use bold yellow for the tool name portion.
- [x] Tool output bodies are rendered in dim/faint.
- [x] Thinking trace text is rendered in dim magenta, prefixed with `💭`.
- [x] Debug-level lines are rendered in red.
- [x] Body text (user input, agent response) is always default foreground for readability.
- [x] Unit tests verify that styled output contains expected ANSI codes when color is enabled, and contains no ANSI codes when `NO_COLOR` is set.
- [x] Manual visual verification on at least one dark and one light terminal theme (documented in PR description).

---

### TUI-3: Loading Spinner ✅ DONE

**As a** user in TUI/REPL mode,
**I want** a smooth animated spinner on the second-to-last terminal line while the agent is working,
**so that** I have visual feedback that Clyde is processing and I can see what operation is in progress.

**Depends on**: TUI-1 (log level gating)

**Acceptance Criteria**:
- [x] A `spinner` package is created that renders braille dot animation (`⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏`).
- [x] The spinner runs at 1/60s frame delay with 2 frames per symbol (~30 symbols/second).
- [x] The spinner occupies the second-to-last terminal line and is redrawn in place (ANSI cursor control).
- [x] The spinner line shows the current operation text (e.g., `⠹ Patching file: agent.go...`).
- [x] When an operation completes, the spinner clears and the permanent `→` progress line is appended to scrollback above.
- [x] **Persistence rule**: any text shown on the spinner line also appears in the permanent scrollback log — the spinner is a live preview, not a replacement.
- [x] The spinner does not appear in CLI mode (progress goes directly to stderr as permanent lines).
- [x] The spinner does not appear at Silent log level.
- [x] The spinner integrates cleanly with the input line below it (no visual glitches or overwriting).
- [x] Unit tests verify spinner frame sequence and timing.
- [x] Unit tests verify spinner start/stop lifecycle (content appears and clears correctly).
- [x] Manual verification that the spinner animates smoothly and doesn't corrupt the terminal (documented in PR).

---

### TUI-4: Prompt Line (Git Branch, Context %, Input Label) ✅ DONE

**As a** user in REPL mode,
**I want** the input prompt to show the current git branch (with dirty indicator), context window usage percentage, and the `You:` label,
**so that** I always know my repo state and how much context capacity remains without checking manually.

**Depends on**: TUI-2 (bold cyan for `You:` label)

**Acceptance Criteria**:
- [x] The prompt line format is: `<branch><dirty> <context%> You: ` (e.g., `main* 12% You: `).
- [x] Git branch is obtained via `git rev-parse --abbrev-ref HEAD`. If detached HEAD, show short hash. If not a git repo, omit git info entirely.
- [x] Dirty indicator (`*`) is present when `git status --porcelain` returns non-empty output.
- [x] Context window usage is calculated as `(total_input_tokens_last_turn / model_context_window_size) * 100` and displayed as a compact integer percentage (e.g., `12%`).
- [x] The `You:` label is styled in bold cyan per TUI-2.
- [x] Git info is refreshed on each prompt render.
- [x] In CLI mode, there is no prompt line.
- [x] Unit tests verify prompt formatting for: clean repo, dirty repo, detached HEAD, non-git directory, various context percentages (0%, 50%, 99%).
- [x] Unit tests verify git info is omitted when not in a git repo.

---

### TUI-5: Rich Text Input (Cursor Movement, Multiline, History)  ✅ DONE

**As a** user in REPL mode,
**I want** full readline-like input editing with cursor movement, multiline input, and history recall,
**so that** I can efficiently compose and edit prompts without the limitations of raw `bufio.NewReader`.

**Depends on**: TUI-3, TUI-4 (must integrate with spinner line and prompt line)

**Acceptance Criteria**:
- [x] Replace `bufio.NewReader(os.Stdin)` with a Go terminal input library (e.g., `chzyer/readline`, `peterh/liner`, or `charmbracelet/bubbletea`).
- [x] Left/right arrow keys move the cursor within the input line.
- [x] Home/End keys jump to start/end of input.
- [x] Enter submits the input.
- [x] A key combination (Shift+Enter, Alt+Enter, or Ctrl+J) inserts a newline for multiline input.
- [x] Up/down arrow keys recall previous inputs (session-level history).
- [x] There is no artificial length limit on input.
- [x] The input widget integrates with the spinner line above it without visual conflicts.
- [x] The chosen library is documented in `progress.md` with rationale for the selection.
- [x] Unit tests verify that input submission, multiline insertion, and history recall work correctly (may require mock terminal or integration-style tests).
- [x] Existing REPL tests and CLI mode tests still pass (CLI mode does not use the input widget).

---

### TUI-6: Cache Display Rework (Verbose/Debug Only)  ✅ DONE

**As a** user,
**I want** cache hit information to only appear at Verbose and Debug levels (not cluttering Normal output),
**so that** my terminal stays clean during normal use while I can still inspect caching behavior when debugging.

**Depends on**: TUI-1 (log levels), TUI-2 (colors), TUI-4 (context % on prompt line)

**Acceptance Criteria**:
- [x] The current `💾 Cache hit: 3715 tokens (100% of input)` message is suppressed at Silent, Quiet, and Normal levels.
- [x] At Verbose level, cache info is displayed as a token fraction: `💾 Cache: 3715/4102 tokens`.
- [x] At Debug level, cache info includes additional detail: `💾 Cache: 3715/4102 tokens | Creation: 387 tokens | Context: 12% (4102/128000)`.
- [x] The context window percentage is surfaced on the prompt line (TUI-4), replacing the cache message as the primary "how full is my context?" indicator at Normal level.
- [x] Unit tests verify cache display format at Verbose and Debug levels.
- [x] Unit tests verify cache display is suppressed at Normal, Quiet, and Silent levels.
- [x] Existing cache tests continue to pass.

---

### TUI-7: Thinking Traces — API Integration, Truncation & Display  ✅ DONE

**As a** user of Clyde,
**I want** the agent to request and display Claude's thinking traces by default, truncated to reasonable limits at Normal verbosity,
**so that** I can understand *why* the agent is making decisions without my terminal being flooded with huge thinking blocks.

**Depends on**: TUI-1 (log levels), TUI-2 (dim magenta styling)

This story delivers the truncation engine *and* thinking traces together as one user-visible feature. The truncation functions (line limits, character limits, verbose bypass) are built here and reused by TUI-8 (tool output bodies).

**Acceptance Criteria — Truncation Engine**:
- [x] A `truncate` package or set of functions is created with configurable line and character limits.
- [x] Thinking traces are truncated to 50 lines at Normal level, with `... (N more lines)` appended.
- [x] Tool output bodies are truncated to 25 lines at Normal level, with `... (N more lines)` appended. (Exercised in TUI-8, but the function is built and unit-tested here.)
- [x] Any single line exceeding 2000 characters is truncated with `...` appended.
- [x] At Verbose and Debug levels, all truncation is disabled (functions pass through unmodified).
- [x] Single-line bash commands and search queries are **never** truncated at Normal level (the existing 60-char and 50-char truncation in display functions is removed).
- [x] Multi-line bash commands follow the standard 25-line truncation.
- [x] Unit tests verify truncation at exact boundary conditions (24 lines → no truncation, 25 lines → no truncation, 26 lines → truncated to 25 + overflow message).
- [x] Unit tests verify character truncation at 2000 chars.
- [x] Unit tests verify truncation is bypassed at Verbose level.
- [x] Unit tests verify single-line commands are never truncated.

**Acceptance Criteria — Thinking Traces**:
- [x] The `api.Request` struct gains a `Thinking` field (`*ThinkingConfig`) with `Type` ("enabled") and `BudgetTokens` (int).
- [x] The `api.Client.Call()` method includes the `thinking` parameter in every request by default, with a configurable `budget_tokens` (default 8192).
- [x] `budget_tokens` is configurable via `~/.clyde/config` (e.g., `THINKING_BUDGET_TOKENS=8192`).
- [x] A `--no-think` CLI flag disables thinking entirely (omits the parameter from requests).
- [x] The `api.Response` correctly parses `thinking` content blocks from Claude's response.
- [x] The agent extracts thinking blocks and forwards them via a new `ThinkingCallback` (or extends the existing `ProgressCallback`).
- [x] At Normal level: thinking is displayed truncated (50-line limit), in dim magenta, prefixed with `💭`.
- [x] At Verbose/Debug: thinking is displayed in full with no truncation.
- [x] At Silent/Quiet: thinking is suppressed.
- [x] Unit tests verify the `thinking` parameter is included in serialized requests.
- [x] Unit tests verify thinking blocks are correctly parsed from mock API responses.
- [x] Integration test confirms a real API call returns thinking blocks and they are displayed (truncated at Normal, full at Verbose).

---

### TUI-8: Tool Output Bodies Display ✅ DONE

**As a** user of Clyde at Normal verbosity or higher,
**I want** to see the actual output of tool calls (file listings, grep results, bash output, etc.) displayed below the `→` progress line,
**so that** I can follow along with what the agent is seeing without having to re-run commands myself.

**Depends on**: TUI-1 (log levels), TUI-2 (dim styling), TUI-7 (truncation engine)

**Acceptance Criteria**:
- [x] After each tool execution, the tool's output string is forwarded via callback alongside the progress message.
- [x] At Normal level: tool output bodies are displayed in dim text, separated from surrounding content by blank lines above and below, truncated per TUI-7's 25-line limit.
- [x] At Verbose/Debug: tool output is displayed in full with no truncation.
- [x] At Quiet level: only the `→` progress line is shown; tool output body is suppressed.
- [x] At Silent level: nothing is shown.
- [x] The agent's callback interface supports both progress messages and tool output bodies (either via a second callback or by distinguishing message types).
- [x] Unit tests verify tool output is emitted at Normal/Verbose/Debug and suppressed at Quiet/Silent, using a captured output buffer.
- [x] Integration test with a real tool call (e.g., `list_files`) confirms output body appears in the log.

---

### TUI-9: Alt+Enter & Ctrl+J for Multiline Input

**As a** user composing multi-line prompts in REPL mode,
**I want** to press Alt+Enter (or Ctrl+J) to insert a newline without submitting,
**so that** I can write structured, multi-line prompts naturally — without relying solely on backslash continuation.

**Depends on**: TUI-5 (rich text input / chzyer/readline integration)

**Context & Research**:
- Traditional terminals cannot distinguish Shift+Enter from Enter (both send `0x0D`). Shift+Enter requires the Kitty keyboard protocol, which would mean replacing chzyer/readline — a large lift.
- **Alt+Enter** is detectable without Kitty: when Alt/Meta is active, the terminal prefixes the key byte with ESC (`0x1B`), so Alt+Enter sends `0x1B 0x0D` — distinct from plain Enter's `0x0D`. This works on any terminal where Alt acts as Meta (iTerm2 by default, macOS Terminal.app with "Use Option as Meta Key" enabled, most Linux terminals).
- **Ctrl+J** sends `0x0A` (line feed), which is always distinct from Enter's `0x0D`. It works on every terminal, everywhere, unconditionally. The current code comments claim Ctrl+J support but the Listener was never wired up.
- Claude Code and OpenCode both use the Kitty protocol for Shift+Enter but keep Ctrl+J and backslash as universal fallbacks. This story brings Clyde to parity on the fallback layer without the Kitty lift.

**Acceptance Criteria**:

*Ctrl+J (universal):*
- [x] Pressing Ctrl+J (`0x0A`) while typing inserts a newline and enters multiline accumulation mode.
- [x] The prompt changes to the continuation prompt (`  > `) on subsequent lines, matching the existing backslash behavior.
- [x] Pressing plain Enter submits the accumulated multiline input.
- [x] Ctrl+J works in every terminal without configuration (it sends a distinct byte from Enter).

*Alt+Enter (Meta+CR):*
- [x] Pressing Alt+Enter (`0x1B 0x0D`) inserts a newline and enters multiline accumulation mode, identical to Ctrl+J behavior.
- [x] The ESC prefix is correctly disambiguated from a standalone Escape keypress followed by Enter (readline's existing timeout-based disambiguation is sufficient).
- [x] On macOS Terminal.app, this requires "Use Option as Meta Key" to be enabled (documented in README or startup hint).

*Shared behavior:*
- [x] Both methods integrate with the existing backslash continuation — a user can mix `\`-continuation, Ctrl+J, and Alt+Enter freely within the same input.
- [x] Multiline input assembled via Ctrl+J or Alt+Enter is saved to history as a single block (matching backslash behavior).
- [x] Ctrl+C while in multiline mode discards the partial input and returns to a fresh prompt (matching existing behavior).
- [x] The implementation uses chzyer/readline's `FuncFilterInputRune` or `Listener` callback — no library replacement needed.

*Documentation & discoverability:*
- [x] The README documents all three multiline methods (backslash, Ctrl+J, Alt+Enter) with a note about macOS Terminal.app Option-as-Meta.
- [x] On first launch (or via a `/help` hint), the available multiline key combos are mentioned.

*Tests:*
- [x] Unit tests verify Ctrl+J (`0x0A`) triggers multiline mode and accumulates lines correctly.
- [x] Unit tests verify Alt+Enter (`0x1B 0x0D`) triggers multiline mode and accumulates lines correctly.
- [x] Unit tests verify mixed usage (backslash + Ctrl+J + Alt+Enter in the same input block).
- [x] Unit tests verify Ctrl+C discards partial multiline input from Ctrl+J / Alt+Enter mode.
- [x] Unit tests verify history saves the complete assembled block.
- [x] Existing backslash-continuation tests still pass unchanged.

---

## Architecture Stories

Stories are dependency-ordered: ARCH-1 (directory reorg) must land first, then ARCH-2 (agent I/O refactor) can clean up what ARCH-1 revealed.

---

### ARCH-1: Project Directory Reorganization

**As a** developer working on the codebase,
**I want** the directory structure to reflect the logical architecture (CLI layer vs agent layer vs shared),
**so that** it's immediately clear what code belongs where, and import paths communicate intent.

**Depends on**: nothing

**Context & Analysis**:

The current flat structure mixes CLI-only packages (`style`, `spinner`, `prompt`, `input`), agent-only packages (`mcp`, `prompts`, `truncate`), and shared packages (`loglevel`, `config`, `providers`) all at the top level. The `api/` package name is too generic. `main.go` contains ~400 lines of CLI orchestration that obscures the thin-entrypoint pattern.

Current dependency graph (non-test):
```
main.go ──→ agent, api, config, input, loglevel, mcp, prompt, prompts, spinner, style, tools
agent   ──→ api, loglevel, tools, truncate
mcp     ──→ api, tools
tools   ──→ api
truncate──→ loglevel
prompt  ──→ style
```

**Target structure**:
```
.
├── main.go                          # 3-line entrypoint: imports cli.Run()
├── go.mod / go.sum / .gitignore
├── README.md
├── clyde                            # Binary
│
├── cli/                             # All CLI/REPL orchestration + UI
│   ├── cli.go                       # Bulk of current main.go (Run, runCLIMode, runREPLMode, etc.)
│   ├── input/
│   │   └── input.go
│   ├── prompt/
│   │   └── prompt.go
│   ├── spinner/
│   │   └── spinner.go
│   └── style/
│       └── style.go
│
├── agent/                           # Agent loop + agent-only deps
│   ├── agent.go
│   ├── mcp/                         # Playwright MCP integration
│   │   ├── client.go
│   │   ├── register.go
│   │   ├── playwright.go
│   │   ├── snapshot.go
│   │   ├── types.go
│   │   └── playwright_tools.json
│   ├── prompts/                     # System prompt (embedded)
│   │   ├── prompts.go
│   │   └── system.txt
│   └── truncate/
│       └── truncate.go
│
├── providers/                       # Root level — shared API types + client
│   ├── client.go                    # (renamed from api/)
│   └── types.go
│
├── loglevel/                        # Root level — shared (agent + cli both use it today)
│   └── loglevel.go
│
├── config/                          # Root level — shared
│   └── config.go
│
├── tools/                           # Root level — separate from agent core
│   └── *.go
│
├── audit/                           # Separate binary (stays)
├── tests/                           # Flat, all package main (stays)
│   └── *.go
│
└── docs/                            # All docs except README.md
    ├── compaction.md
    ├── playwright-mcp.md
    ├── tui.md
    ├── progress.md
    ├── todos.md
    └── whitepaper.md
```

**Import path mapping**:

| Old import path           | New import path                |
|---------------------------|--------------------------------|
| `clyde/api`               | `clyde/providers`              |
| `clyde/style`             | `clyde/cli/style`              |
| `clyde/spinner`           | `clyde/cli/spinner`            |
| `clyde/prompt`            | `clyde/cli/prompt`             |
| `clyde/input`             | `clyde/cli/input`              |
| `clyde/mcp`               | `clyde/agent/mcp`              |
| `clyde/prompts`           | `clyde/agent/prompts`          |
| `clyde/truncate`          | `clyde/agent/truncate`         |

All type/function references must also be updated (e.g. `api.Client` → `providers.Client`, `api.Message` → `providers.Message`, etc.).

**Execution plan (one atomic commit)**:

1. Safety commit: `git add -A && git commit -m "checkpoint before reorg"`
2. Delete `errors/` (empty directory)
3. Move docs: `git mv progress.md docs/`, `git mv todos.md docs/`, `git mv whitepaper.md docs/`
4. Rename `api/` → `providers/`: `git mv api/ providers/`, change `package api` → `package providers` in both files
5. Create `cli/` and move UI packages under it: `mkdir -p cli && git mv style/ cli/style/ && git mv spinner/ cli/spinner/ && git mv prompt/ cli/prompt/ && git mv input/ cli/input/`
6. Move agent-only packages: `git mv mcp/ agent/mcp/ && git mv prompts/ agent/prompts/ && git mv truncate/ agent/truncate/`
7. Extract `main.go` body → `cli/cli.go` (new file, `package cli`, exports `Run()`); slim `main.go` to thin wrapper
8. Update `agent/prompts/prompts.go` dev-mode path: `os.ReadFile("prompts/system.txt")` → `os.ReadFile("agent/prompts/system.txt")`
9. Bulk rewrite all import paths + type references across every `.go` file (source + tests)
10. Verify: `go build ./...` and `cd tests && go test ./...`
11. Commit

**Notes on shared packages**:
- `loglevel/` stays at root because the agent currently imports it (see ARCH-2 for cleanup).
- `config/` stays at root — used by both CLI (to load config) and potentially agent in the future.
- `providers/` (née `api/`) stays at root — used by `agent/`, `tools/`, `agent/mcp/`, and `cli/`.
- `tools/` stays at root and separate from `agent/` — the agent calls into tools, but tools are independently registered and testable.
- Tests stay flat in `tests/` — all files are `package main` sharing `test_helpers.go`; splitting into subdirs would require separate packages and break the shared helpers.

**Acceptance Criteria**:
- [x] Directory structure matches the target layout above.
- [x] `main.go` is ≤10 lines: imports `cli` and calls `cli.Run()`.
- [x] `cli/cli.go` contains all former `main.go` logic with exported `Run()` entrypoint.
- [x] All import paths updated per the mapping table.
- [x] All type references updated (`api.X` → `providers.X` everywhere).
- [x] `package` declarations updated (`package api` → `package providers`).
- [x] `errors/` directory deleted.
- [x] `progress.md`, `todos.md`, `whitepaper.md` moved to `docs/`.
- [x] `go build ./...` succeeds with zero errors.
- [x] `cd tests && go test ./...` — all tests pass (same count as before).
- [x] No circular imports.
- [x] The `//go:embed system.txt` in `agent/prompts/prompts.go` still works (relative to file).
- [x] The dev-mode `os.ReadFile(...)` fallback path is updated.

---

### ARCH-2: Remove I/O Concerns from the Agent (loglevel decoupling)  ✅ DONE

**As a** developer maintaining the agent package,
**I want** the agent to have zero display/filtering logic and return all information to callers,
**so that** the agent is purely a conversation-and-tool-execution engine with no UI coupling.

**Depends on**: ARCH-1 (directory reorg landed first)

**Context & Analysis**:

Today the agent imports `loglevel` and uses it for three things it shouldn't own:

1. **Gating callbacks** — `emit()` checks `a.logLevel.ShouldShow(threshold)` before calling `progressCallback`. The agent decides what the CLI displays. Instead, the agent should emit everything unconditionally; the CLI callback decides whether to show it.

2. **Truncation** — `emitThinking()` passes `a.logLevel` to `truncate.Thinking(text, a.logLevel)`. Truncation is a display concern. The agent should return full text; the CLI truncates before displaying.

3. **Spinner suppression** — `spinnerStart()` checks `a.logLevel != loglevel.Silent`. The CLI provided the callback; it can make this a no-op itself.

After this refactor:
- `agent/` no longer imports `loglevel/`
- `agent/truncate/` no longer imports `loglevel/` (truncation functions take plain int limits, or move to `cli/`)
- `loglevel/` could potentially move under `cli/` (since only the CLI would use it)
- The agent's callback signatures become simpler (no level parameter needed — the agent just emits, the CLI filters)

**Refactor plan**:

1. **Change `ProgressCallback` signature**: Remove `loglevel.Level` parameter. Instead, use separate callbacks for separate concerns:
   - `ProgressCallback func(message string)` — tool progress lines (the `→` lines)
   - `OutputCallback func(output string)` — tool output bodies (the full text)
   - `DiagnosticCallback func(message string)` — cache info, token counts, etc.
   - Or: keep one callback but tag with a simple string enum (`"progress"`, `"output"`, `"diagnostic"`) instead of importing loglevel.

2. **Remove `WithLogLevel` from agent**: The agent no longer stores or checks a log level. It emits everything.

3. **Move filtering to CLI**: `cli/cli.go` sets up callbacks that check the log level internally:
   ```go
   agent.WithProgressCallback(func(msg string) {
       if level.ShouldShow(loglevel.Quiet) {
           // display it
       }
   })
   ```

4. **Move truncation to CLI**: The CLI applies truncation before displaying, not the agent before emitting. `truncate/` either:
   - Stays under `agent/` but drops its `loglevel` import (takes `maxLines int` instead of `level`), or
   - Moves to `cli/truncate/` since it's now purely a display concern.

5. **Remove `spinnerCallback` from agent entirely**: The agent doesn't know about spinners. The CLI starts/stops the spinner in its own progress callback based on timing.

6. **Update all tests** that construct agents with `WithLogLevel`.

**Acceptance Criteria**:
- [x] `agent/agent.go` has zero imports of `loglevel`.
- [x] `agent/truncate/truncate.go` has zero imports of `loglevel` (functions take plain int params or package moves to `cli/`).
- [x] The agent emits all progress, output, thinking, and diagnostic information unconditionally via callbacks.
- [x] The CLI layer (`cli/cli.go`) is the sole owner of display filtering, truncation, and spinner management.
- [x] `loglevel/` is only imported by packages under `cli/` (and could be moved there in a follow-up).
- [x] All existing tests pass with updated callback wiring.
- [x] No behavioral change from the user's perspective — same output at every log level.

---

---

### ARCH-3: Encapsulate Agent as a Self-Contained Package

**As a** developer who wants to distribute the agent as an independent, importable Go package,
**I want** all agent dependencies (`providers/`, `tools/`, `config/`) moved under `agent/` behind a high-level public API,
**so that** the CLI only talks to the `agent` package's public surface — never reaching into its internals.

**Depends on**: ARCH-2 (agent I/O decoupling — already done)

**Context & Analysis**:

Today the CLI assembles the agent's internals directly:

```go
// cli/cli.go — reaches into agent internals
client := providers.NewClient(apiKey, url, model, maxTokens)   // creates provider client
_ "clyde/tools"                                                 // blank import for init() registration
agent := agent.NewAgent(client, systemPrompt, ...opts)
```

Three root-level packages (`providers/`, `tools/`, `config/`) are shared between `cli/` and `agent/`, but every field in `config.Config` is agent business, every tool is agent business, and `providers` is the agent's API client layer. The CLI imports them only to wire them into the agent.

**Target structure**:
```
.
├── main.go                  # Thin entrypoint → cli.Run()
├── cli/                     # CLI/REPL orchestration + UI
│   ├── cli.go
│   ├── input/
│   ├── loglevel/
│   ├── prompt/
│   ├── spinner/
│   ├── style/
│   └── truncate/
├── agent/                   # Self-contained agent package
│   ├── agent.go             # Public API: New(Config, ...Option) → *Agent
│   ├── config/              # Agent config (moved from root)
│   ├── providers/           # API client + types (moved from root)
│   ├── tools/               # Tool registry + implementations (moved from root)
│   ├── mcp/
│   └── prompts/
└── tests/
```

**Key API change** — the agent owns its own construction:

```go
// Before (CLI assembles internals):
client := providers.NewClient(cfg.APIKey, cfg.APIURL, cfg.ModelID, cfg.MaxTokens)
client = client.WithThinking(thinkingConfig)
agentInstance := agent.NewAgent(client, prompts.SystemPrompt,
    agent.WithContextWindowSize(cfg.ContextWindowSize),
    agent.WithProgressCallback(func(msg string) { ... }),
    ...
)

// After (CLI passes config, agent builds itself):
agentInstance := agent.New(agent.Config{
    APIKey:            cfg.APIKey,
    APIURL:            cfg.APIURL,
    ModelID:           cfg.ModelID,
    MaxTokens:         cfg.MaxTokens,
    BraveSearchAPIKey: cfg.BraveSearchAPIKey,
    ContextWindowSize: cfg.ContextWindowSize,
    ThinkingBudget:    cfg.ThinkingBudgetTokens,
    MCPPlaywright:     cfg.MCPPlaywright,
    MCPPlaywrightArgs: cfg.MCPPlaywrightArgs,
},
    agent.WithProgressCallback(func(msg string) { ... }),
    agent.WithThinkingCallback(func(text string) { ... }),
    ...
)
```

The agent constructor internally:
1. Creates its own `providers.Client`
2. Registers tools (no blank import / init() magic from outside)
3. Starts MCP if configured
4. Loads the system prompt

**Config split**: The CLI reads `~/.clyde/config` into its own struct (which includes CLI-specific fields like the config path itself), then maps the agent-relevant fields into `agent.Config`. The `config/` package under `agent/` defines only what the agent needs.

**Execution plan**:

1. Define `agent.Config` struct with all fields the agent needs
2. Move `config/` → `agent/config/` (or inline into agent if small enough)
3. Move `providers/` → `agent/providers/`
4. Move `tools/` → `agent/tools/`
5. Agent constructor creates its own client + registers tools internally
6. Remove `providers` and `tools` imports from `cli/cli.go`
7. CLI loads config file itself (simple key=value parsing), constructs `agent.Config`
8. Update all import paths across source + tests
9. Verify: `go build .` and `go vet ./...` and tests pass

**Acceptance Criteria**:
- [x] `providers/`, `tools/`, and `config/` are under `agent/` (not at project root).
- [x] `cli/cli.go` imports only `agent` (plus its own `cli/*` subpackages) — no `providers`, `tools`, or `config` imports.
- [x] `agent.New(cfg agent.Config, ...Option)` is the sole public constructor; it handles client creation, tool registration, MCP setup, and prompt loading internally.
- [x] The blank import `_ "clyde/tools"` is eliminated — tool registration is internal to the agent.
- [x] `agent.Config` contains all agent-relevant fields; the CLI maps from its own config to `agent.Config`.
- [x] The agent package is importable and usable by external consumers: `go get github.com/this-is-alpha-iota/clyde/agent`.
- [x] `go build .` succeeds, `go vet ./...` clean, all tests pass.
- [x] No circular imports.
- [x] No behavioral change from the user's perspective.

---

## Session & History Stories

> **Design doc**: `docs/sessions-history.md` — file-based session persistence with one file per message, timestamp-only naming, and Unix-native filtering.
>
> **Design decision — no history search tool (formerly CMP-6)**: Since conversation history is persisted as local `.md` files in a known directory structure, the agent searches its own history using existing tools (`grep`, `read_file`, `glob`, `run_bash`) guided by system prompt instructions. No custom `search_history` tool is needed. This follows the "lean into standard tools" philosophy.
>
> **Design decision — no sequence numbers (ITD-1)**: Message files use timestamps as the sole ordering mechanism. Autoincrementing sequence numbers were eliminated — they require state tracking, impose arbitrary width limits, and are redundant with ISO-8601 timestamps that sort lexicographically. See `docs/sessions-history.md` ITD-1 for full rationale.

Stories are dependency-ordered:

---

### SESS-1: Session History Persistence (One File Per Message) ✅ DONE

**As a** user of Clyde,
**I want** every message in my conversation to be persisted to disk as it happens,
**so that** I have a complete, searchable record of every session that survives crashes and process exits.

**Depends on**: nothing

**Acceptance Criteria**:

*Session infrastructure:*
- [x] On session start (REPL or CLI mode), the session location is determined: `git rev-parse --show-toplevel` → `<repo>/.clyde/sessions/`, else `~/.clyde/sessions/`.
- [x] A session directory is created: `<session-location>/<timestamp>_<username>/` where username comes from `git config user.name` (lowercased, spaces to hyphens), fallback `$USER`.
- [x] If `.clyde/sessions/` is new and inside a git repo, `.clyde/sessions/` is added to `.gitignore` automatically.

*Per-message file writing:*
- [x] After each message or content block, a file is written to the session directory: `<timestamp>_<type>.md` where timestamp is ISO-8601 with milliseconds and hyphens for colons (e.g., `2026-07-14T09-32-05.123`), and type is one of: `user`, `assistant`, `system`, `thinking`, `tool-use`, `tool-result`, `diagnostic`.
- [x] File contents match terminal output at debug level with ANSI codes stripped — role markers (`**You:**`, `**Claude:**`), `💭` for thinking, `→` lines for tool use, fenced output for tool results, `🔍`/`💾`/`🔒` for diagnostics.
- [x] Files are written synchronously after each message (crash safety: at most one incomplete file on crash).
- [x] `cat *.md` in the session directory (sorted by filename) produces a valid, readable conversation transcript.
- [x] A monotonicity guard ensures no timestamp collision: if `time.Now()` ≤ last written timestamp, bump by 1ms.

*Tool use IDs for reconstruction:*
- [x] Every `→` progress line includes the tool_use_id in brackets: `→ Reading file: agent/agent.go [toolu_abc123]`. This appears at all log levels and in the persisted file.

*System prompt additions:*
- [x] The system prompt includes the session path and file naming convention so the agent can search its own history using existing tools (`grep`, `read_file`, `glob`, `run_bash("cat *_user.md")`).

*Session completion:*
- [x] On clean exit, the session path is printed: `Session saved: .clyde/sessions/2026-07-14T09-32-00_aj/`.

*Tests:*
- [x] Unit test: a multi-turn conversation produces correctly named files in the session directory (one per message, timestamps monotonically increasing, types matching content).
- [x] Unit test: `cat *.md` output matches expected transcript (role markers, tool IDs, diagnostics all present and ordered).
- [x] Unit test: crash mid-session leaves all prior messages intact as individual files.
- [x] Unit test: `.gitignore` is updated on first session creation inside a git repo.
- [x] Unit test: session directory naming uses correct username and timestamp format.
- [x] Unit test: tool use IDs appear in `→` lines at all log levels.
- [x] Integration test: a real multi-turn conversation with tool use produces a valid session directory; `cat *.md` is a coherent transcript; `cat *_user.md` shows only user messages; `cat *_tool-result.md` shows only tool output.

---

### SESS-2: Session Resume & Listing

**As a** user who wants to continue a previous conversation,
**I want** to resume a session from where I left off (or from another user's session),
**so that** I can pick up work without losing context — especially after crashes, restarts, or handoffs.

**Depends on**: SESS-1

**Acceptance Criteria**:

*Resume from message files:*
- [x] `clyde --resume` / `clyde -r` loads the most recent session for the current user.
- [x] `clyde --resume <session-id>` loads a specific session by directory name.
- [x] Reconstruction reads message files in sorted order and groups them into API messages using deterministic rules: consecutive `thinking`/`tool-use` files → one assistant message; consecutive `tool-result` files → one user message; `user` and `assistant` files start/flush messages; `diagnostic` and `compaction` files are skipped. (Per `docs/sessions-history.md` §12.)
- [x] The `toolu_id` is extracted from `→ ... [toolu_id]` lines in `tool-use` files and used to populate `tool_use_id` on corresponding `tool_result` blocks.
- [x] If a compaction has occurred (a `*_system.md` file exists), resume loads from the latest `*_system.md` forward. Otherwise loads all files.
- [x] After resume, new messages are written to the same session directory as new timestamped files.
- [x] A malformed last file (from a crash) is skipped or truncated gracefully — the rest of the session loads.

*Cross-user resume (branching):*
- [x] When resuming another user's session, the session directory is copied to a new directory: `<timestamp>_<user>_from_<source-session-id>/`. The copied files are untouched; new messages are appended as new files.
- [x] When resuming your own most recent session, no copy is needed.

*Session listing:*
- [x] `clyde --sessions` lists sessions in reverse chronological order with message count and summary (first user message, truncated).
- [x] All info derived from files on disk — no database or metadata file.

*CLI → REPL transition:*
- [x] A CLI one-shot session (`clyde "do something"`) can be resumed in REPL mode via `clyde --resume`.

*Tests:*
- [x] Unit test: reconstruction from a set of message files produces the correct `a.history` structure (message roles, content block types, tool_use_ids all correct).
- [x] Unit test: resume after compaction loads only from the latest `*_system.md` forward.
- [x] Unit test: resume with a malformed last file loads all prior messages and skips the bad file.
- [x] Unit test: cross-user resume copies the directory and appends the `_from_` provenance.
- [x] Unit test: `--sessions` listing produces correct output from a directory of test sessions.
- [x] Integration test: create a session with tool use, exit, resume, verify history is intact and conversation continues seamlessly.
- [x] Integration test: create a CLI session, resume it in REPL mode, verify continuity.

---

## Compaction Stories

> **Prerequisite**: Session & history persistence (SESS-1, SESS-2) must land before compaction work begins. Compaction writes `compaction` and `system` type files to the session directory per the file-per-message design.
>
> **Design decision — no history search tool (formerly CMP-6)**: Eliminated. The agent searches its own session history using existing tools (`grep`, `read_file`, `glob`) guided by system prompt instructions. This follows the "lean into standard tools" philosophy.
>
> **Folded stories**: CMP-4 (Git-Centric State Tracking), CMP-5 (Preserve Initial User Message), and CMP-7 (Feed Recent Context into Summarizer) were originally separate stories. They are now acceptance criteria on CMP-1 and CMP-2 respectively, since they describe *how* those stories behave rather than independent deliverables.

Stories are dependency-ordered:

---

## Monorepo Stories

> **Goal**: Split the project from a single Go module into a multi-module monorepo so that (a) other teams can `go get` the agent library without pulling CLI/TUI dependencies, (b) the CLI remains installable via `go install`, and (c) a separate private repo can consume the public agent module cleanly.
>
> **Current state (post-ARCH-3)**: Single `go.mod` at root (`module github.com/this-is-alpha-iota/clyde`). The agent is architecturally self-contained under `agent/` with subpackages (`config/`, `mcp/`, `prompts/`, `providers/`, `session/`, `tools/`). The CLI is under `cli/` with its own subpackages. `main.go` is a 3-line wrapper calling `cli.Run()`. Tests are flat under `tests/` importing from both agent and cli.
>
> **Target state**: The `agent/` subtree becomes its own Go module (`github.com/this-is-alpha-iota/clyde/agent`) with its own `go.mod`. The root module remains the CLI binary. A `go.work` file at the root enables seamless local development. External consumers import only the agent module and get minimal dependencies (no TUI, no `x/sys`).
>
> **Design decision — root module is the CLI**: The root module (`github.com/this-is-alpha-iota/clyde`) stays as the CLI binary. This preserves `go install github.com/this-is-alpha-iota/clyde@latest` for the binary while `go get github.com/this-is-alpha-iota/clyde/agent@latest` fetches only the agent's `go.mod` with its minimal deps. No need for a third `cli/` module — the root IS the CLI module. The `tests/` directory stays in the root module and can import from both agent (via `go.work` locally, via published version in CI) and cli (same module).
>
> **Design decision — session stays as an agent subpackage**: `agent/session` is heavily used by the CLI for writing session files. In the multi-module world, it remains a public subpackage of the agent module. External consumers who import `agent/session` get a clean, focused dependency. Moving session to a third module would add complexity without clear benefit.

Stories are dependency-ordered:

---

### MONO-1: Seal the Agent's Public API Surface

**As a** developer preparing the agent for independent consumption,
**I want** all types needed by callers to be accessible via `import "…/agent"` alone,
**so that** consumers never have to import agent subpackages (`agent/providers`, `agent/session`) for core workflows — and those subpackages can eventually become `internal/` if desired.

**Depends on**: ARCH-3 (agent encapsulation — done)

**Context & Analysis**:

Today `cli/cli.go` imports two agent subpackages it shouldn't need:

1. **`agent/providers`** — for the `providers.Message` type used in one function signature:
   ```go
   func runREPLModeWithSession(..., history []providers.Message)
   ```
   The agent already re-exports `Usage` as `type Usage = providers.Usage` but doesn't do the same for `Message`.

2. **`agent/session`** — used heavily (session writing, format helpers, resume, listing). This is a bigger surface: `session.New()`, `session.Session`, `session.WriteMessage()`, `session.FormatToolUseID()`, `session.StripANSI()`, `session.ReconstructHistory()`, `session.ListSessions()`, `session.FindMostRecentSession()`, `session.FindSessionByID()`, `session.CopyForResume()`, `session.SessionOwner()`, `session.GetUsername()`, `session.FindSessionsRoot()`, `session.Open()`, and the `TypeXxx` constants.

Additionally, `agent/config` is dead code within the agent — nothing under `agent/` imports it (the CLI has its own `loadAgentConfig()`), yet tests import it. This should be cleaned up as part of sealing the surface.

**Scope — CLI only, not tests**: This story updates only `cli/cli.go` imports. Test files (`tests/*.go`) currently reference `providers.Message`, `providers.ContentBlock`, `providers.NewClient`, etc. — 164 references across 10 files. These are **intentionally left unchanged**. Tests are internal consumers that test agent internals; importing `agent/providers` directly is correct for that purpose. The type aliases are Go type aliases (`type Message = providers.Message`), so `providers.Message` and `agent.Message` are the *same type* — code using either compiles and interoperates without casts. Migrating tests to `agent.Message` would be a 164-reference mechanical rename with zero behavioral benefit; it can be done later as a style cleanup if desired, but is explicitly not a goal of this story.

**Acceptance Criteria**:

*Type re-exports in `agent/agent.go`:*
- [x] `type Message = providers.Message` is exported from the `agent` package.
- [x] `type ContentBlock = providers.ContentBlock` is exported (or whichever types appear in `Message` fields that callers need to construct).
- [x] The existing `type Usage = providers.Usage` remains.
- [x] `cli/cli.go` is updated: `import "…/agent/providers"` is removed; all `providers.Message` references become `agent.Message`.
- [x] `go vet ./...` passes with no unused imports.

*Session re-exports or facade:*
- [x] The `agent` package exposes session functionality sufficient for the CLI's needs. This can be either:
  - (a) Type aliases and wrapper functions in `agent/` that delegate to `agent/session`, or
  - (b) A documented decision that `agent/session` is a supported public subpackage (acceptable for MONO-1; can revisit in a follow-up).
- [x] If option (b): the decision is documented in `progress.md` with rationale.
- [x] If option (a): `cli/cli.go` no longer imports `agent/session` directly.

*Dead code cleanup:*
- [x] `agent/config/` is evaluated: if nothing under `agent/` imports it, either delete it (move test helpers inline) or mark it clearly as a test utility.
- [x] No new dead code introduced.

*Tests (zero test file changes):*
- [x] `go build ./...` succeeds.
- [x] `cd tests && go test ./...` — all tests pass, same count as before, **no test files modified**.
- [x] Test files continue to import `agent/providers` directly — this is intentional and correct (they test internals; the type alias means `providers.Message` and `agent.Message` are interchangeable at compile time).
- [x] A manual check confirms that a hypothetical external consumer can use the agent with only `import "…/agent"` for the core workflow (create agent, handle message, get history).

---

### MONO-2: Extract agent/ as an Independent Go Module

**As a** developer splitting the monorepo,
**I want** `agent/` to have its own `go.mod` declaring only its own dependencies,
**so that** `go get github.com/this-is-alpha-iota/clyde/agent` pulls only agent-relevant code and deps — not the CLI, TUI, or `x/sys`.

**Depends on**: MONO-1 (clean API surface — minimizes cross-module import churn)

**Context & Analysis**:

Today the single root `go.mod` has three direct deps:
- `github.com/JohannesKaufmann/html-to-markdown` — used only by `agent/tools/browse.go`
- `github.com/joho/godotenv` — used by `agent/config/config.go` AND `cli/cli.go`
- `golang.org/x/sys` — used only by `cli/input/rawmode_{bsd,linux}.go`

After the split:
- `agent/go.mod` declares `html-to-markdown` and `godotenv` (and their transitives: `goquery`, `cascadia`, `x/net`).
- Root `go.mod` declares `godotenv`, `x/sys`, and a `require` on the agent module.
- `x/sys` does NOT appear in `agent/go.mod` — this is the whole point.

**Execution plan (one atomic commit)**:

1. `git add -A && git commit -m "checkpoint before module split"` (safety net)
2. Create `agent/go.mod`:
   ```
   module github.com/this-is-alpha-iota/clyde/agent
   go 1.24
   require (
       github.com/JohannesKaufmann/html-to-markdown v1.6.0
       github.com/joho/godotenv v1.5.1
   )
   ```
3. Run `cd agent && go mod tidy` to generate `agent/go.sum` and resolve transitives.
4. Verify: `cd agent && go build ./...` succeeds (the agent module compiles standalone).
5. Update root `go.mod`: add `require github.com/this-is-alpha-iota/clyde/agent v0.0.0` (pseudo-version, resolved by `go.work`).
6. Update root `go.mod`: `html-to-markdown`, `goquery`, `cascadia`, `x/net` can be removed (they're agent's deps now); keep `godotenv` and `x/sys`.
7. Run `go mod tidy` at root.
8. Rewrite all import paths in root-module files (`cli/`, `tests/`, `main.go`): `"github.com/this-is-alpha-iota/clyde/agent"` stays the same (Go resolves it to the nested module); `"github.com/this-is-alpha-iota/clyde/agent/providers"` etc. also stay the same.
9. Verify: `go build ./...` at root may fail until `go.work` is added (next story) — that's expected. At minimum, `cd agent && go build ./...` must pass.
10. Commit.

**Acceptance Criteria**:
- [x] `agent/go.mod` exists with module path `github.com/this-is-alpha-iota/clyde/agent`.
- [x] `agent/go.sum` exists and is committed.
- [x] `cd agent && go build ./...` succeeds with zero errors (agent compiles standalone).
- [x] `cd agent && go vet ./...` is clean.
- [x] `agent/go.mod` does NOT contain `golang.org/x/sys` (CLI-only dep).
- [x] `agent/go.mod` contains `html-to-markdown` and `godotenv` (agent deps).
- [x] Root `go.mod` contains `golang.org/x/sys` and `godotenv` and a `require` for the agent module.
- [x] Root `go.mod` does NOT contain `html-to-markdown` as a direct dependency (it's the agent's concern now).
- [x] No import paths changed in Go source files (Go's module resolution handles nested modules transparently).
- [x] `.gitignore` is NOT updated to exclude `agent/go.sum` (it must be committed per Go conventions).

---

### MONO-3: Add go.work for Workspace Development

**As a** developer working in the monorepo,
**I want** a `go.work` file at the root that links both modules,
**so that** local changes to `agent/` are immediately reflected when building or testing the CLI — without publishing a version first.

**Depends on**: MONO-2 (agent has its own `go.mod`)

**Context & Analysis**:

Without `go.work`, the root module would resolve the agent dependency from the Go module proxy (or a git tag), not from the local `agent/` directory. Every agent change would require a commit + tag + `go get` cycle — unworkable for development.

`go.work` tells the Go toolchain to use local directories as module sources. This is the Go 1.18+ standard for multi-module monorepo development. The file is committed to the repo (this is a private/internal monorepo pattern; for public libraries, `go.work` is sometimes `.gitignore`d, but since our CLI binary and agent live together, committing it is correct).

**Acceptance Criteria**:
- [x] A `go.work` file exists at the repo root:
  ```
  go 1.24

  use (
      .
      ./agent
  )
  ```
- [x] `go.work.sum` is committed alongside `go.work`.
- [x] From the repo root: `go build ./...` succeeds (builds both the CLI binary and all agent packages).
- [x] From the repo root: `go vet ./...` is clean across both modules.
- [x] From the repo root: `cd tests && go test ./...` — all tests pass (same count as before the split).
- [x] A change to a file in `agent/` (e.g., add a comment to `agent/agent.go`) is immediately reflected when building the CLI at the root — no `go get` or version bump needed.
- [x] `go work sync` runs without error.
- [x] The `clyde` binary still builds and runs correctly: `go build -o clyde . && ./clyde --help`.
- [x] CI pipeline (if any) is updated to use `go work` or explicitly build each module.

---

### MONO-4: Verify External Consumability

**As an** engineer on another team who wants to use Clyde's agent as a library,
**I want** to confirm that `go get github.com/this-is-alpha-iota/clyde/agent@<version>` works correctly and pulls only agent dependencies,
**so that** I can integrate the agent into my own service without inheriting CLI/TUI baggage.

**Depends on**: MONO-3 (workspace development working end-to-end)

**Context & Analysis**:

This story is a verification/documentation story, not a code change. It confirms the module split actually delivers on its promise by simulating the external consumer experience. The test creates a throwaway Go module outside the repo that imports and uses the agent.

**Acceptance Criteria**:

*External consumer smoke test:*
- [ ] A test script (checked in as `scripts/test-external-consume.sh` or similar) does the following:
  1. Creates a temp directory outside the repo.
  2. `go mod init testconsumer`
  3. `go get github.com/this-is-alpha-iota/clyde/agent@<latest-tag-or-commit>`
  4. Writes a minimal `main.go` that imports `"github.com/this-is-alpha-iota/clyde/agent"`, creates an `agent.Config{}`, and calls `agent.New(cfg)`.
  5. `go build .` succeeds.
  6. Inspects `go.sum` and confirms `golang.org/x/sys` is NOT present (proves CLI deps weren't pulled).
  7. Cleans up the temp directory.
- [ ] The script passes on a clean machine (no local replace directives or workspace magic).

*Dependency audit:*
- [ ] `cd agent && go list -m all` is captured and documented. It should contain only:
  - `github.com/JohannesKaufmann/html-to-markdown`
  - `github.com/joho/godotenv`
  - `github.com/PuerkitoBio/goquery` (transitive)
  - `github.com/andybalholm/cascadia` (transitive)
  - `golang.org/x/net` (transitive)
  - NO `golang.org/x/sys`, NO TUI/readline libraries, NO CLI framework.
- [ ] This list is added to `README.md` or a new `agent/README.md` under a "Dependencies" section.

*Consumer documentation:*
- [ ] `agent/README.md` (or a section in root `README.md`) documents:
  - How to install the agent library: `go get github.com/this-is-alpha-iota/clyde/agent@latest`
  - Minimal usage example (create config, create agent, handle message).
  - How to install the CLI binary: `go install github.com/this-is-alpha-iota/clyde@latest`
  - That these are separate modules with independent dependency trees.

*Tests:*
- [ ] The smoke test script is runnable via `make test-external` or equivalent.
- [ ] The test is documented in `progress.md` as a verification gate for future module changes.

---

### MONO-5: Release Tagging Convention & First Tagged Release

**As a** maintainer publishing releases,
**I want** a documented tagging convention that versions the agent and CLI modules independently (or in lockstep),
**so that** consumers can pin stable versions and `go get` resolves correct code.

**Depends on**: MONO-4 (consumability verified)

**Context & Analysis**:

Go's multi-module versioning uses **prefixed tags**. For a module at path `github.com/org/repo/agent`, Go looks for tags prefixed with `agent/`:
- `agent/v0.1.0` → resolves `github.com/org/repo/agent@v0.1.0`
- `v0.1.0` → resolves `github.com/org/repo@v0.1.0` (root module = CLI)

Both modules can share version numbers (tag `v0.1.0` and `agent/v0.1.0` on the same commit) for simplicity, or version independently if their release cadences diverge.

**Acceptance Criteria**:

*Tagging convention:*
- [ ] The convention is documented in `CONTRIBUTING.md` or `docs/releasing.md`:
  - Agent module: `agent/vX.Y.Z` (e.g., `agent/v0.1.0`)
  - CLI module (root): `vX.Y.Z` (e.g., `v0.1.0`)
  - Both tags are created on the same commit for lockstep releases.
  - Pre-v1: use `v0.x.y` (no compatibility guarantees per Go semver convention).
- [ ] The root `go.mod` pins the agent dependency to the latest tagged version (not a pseudo-version) after the first release.

*First tagged release:*
- [ ] Tags `v0.1.0` and `agent/v0.1.0` are created on a clean, passing commit.
- [ ] `go install github.com/this-is-alpha-iota/clyde@v0.1.0` succeeds from a clean machine.
- [ ] `go get github.com/this-is-alpha-iota/clyde/agent@v0.1.0` succeeds from a clean machine.
- [ ] The Go module proxy (proxy.golang.org) has indexed both modules (may take a few minutes; verified via `GOPROXY=https://proxy.golang.org go list -m github.com/this-is-alpha-iota/clyde/agent@v0.1.0`).

*Release script/Makefile target:*
- [ ] A `make release VERSION=0.1.0` target (or equivalent script) automates:
  1. Verify working tree is clean.
  2. Run `go build ./...` and `cd tests && go test ./...`.
  3. Update root `go.mod` to pin `agent@vX.Y.Z` (remove `go.work` replace).
  4. `git tag agent/vX.Y.Z && git tag vX.Y.Z`
  5. `git push origin agent/vX.Y.Z vX.Y.Z`
  6. Print post-release verification commands.
- [ ] The script refuses to run if tests fail or the tree is dirty.

*Tests:*
- [ ] The release script is tested with a dry-run mode (`make release VERSION=0.1.0 DRY_RUN=1`) that prints what it would do without executing.
- [ ] Post-release, the MONO-4 external consumer smoke test passes with the tagged version.

---

### MONO-6: Migrate Private Audit Branch to Multi-Module Architecture

**As a** team maintaining the proprietary security auditer on the `security-auditer` branch,
**I want** that branch updated to work with the new multi-module structure and consume the agent via `agent.New()`,
**so that** merging from `master` continues to work cleanly and the audit tool benefits from the sealed agent API.

**Depends on**: MONO-5 (tagged releases exist; the agent module's public API is stable)

**Context & Analysis**:

The audit tool already exists as ~11,200 lines of Go across 28 files on the `security-auditer` branch (pushed to `private` remote at `clyde-private.git`). The current setup is a long-lived branch — not a separate repo — where `audit/` is an additional directory that only exists on that branch. Merges from `master` have historically been clean because `audit/` doesn't exist on `master`.

**Current state (on `private/security-auditer`)**:

The audit code uses the **pre-ARCH-3 construction pattern**, reaching into agent subpackages directly:

```go
// audit/pipeline/runner.go — current
import (
    "github.com/this-is-alpha-iota/clyde/agent"
    "github.com/this-is-alpha-iota/clyde/agent/prompts"
    "github.com/this-is-alpha-iota/clyde/agent/providers"
    _ "github.com/this-is-alpha-iota/clyde/agent/tools"
)

apiClient := providers.NewClient(r.apiKey, r.apiURL, r.modelID, r.maxTokens)
return agent.NewAgent(apiClient, prompts.SystemPrompt, opts...)
```

```go
// audit/main.go — current
import "github.com/this-is-alpha-iota/clyde/agent/config"

cfg, err := config.LoadFromFile(configPath)
```

This will break after MONO-1–3 because:
1. `agent.NewAgent()` is the legacy constructor — `agent.New(agent.Config{})` is the current API.
2. `providers.NewClient()` is an internal concern — the agent creates its own client now.
3. `_ "clyde/agent/tools"` blank import for init() registration is no longer needed — `agent.New()` handles tool registration internally.
4. `prompts.SystemPrompt` is no longer accessed directly — `agent.New()` loads it internally.
5. `agent/config.LoadFromFile()` is dead code in the agent module — the CLI has its own config loader.

The branch is also ~20 commits behind `master` (missing the input editor rewrite, CSI parser fix, line-wrap fix, compaction improvements, and now the MONO stories).

**Target state**:

```go
// audit/pipeline/runner.go — after migration
import "github.com/this-is-alpha-iota/clyde/agent"

agentInstance := agent.New(agent.Config{
    APIKey:    r.apiKey,
    APIURL:    r.apiURL,
    ModelID:   r.modelID,
    MaxTokens: r.maxTokens,
}, agent.WithProgressCallback(func(msg string, _ string) {
    progressCallback(msg)
}))
```

```go
// audit/main.go — after migration
// Config loading uses its own godotenv call or a local helper,
// not agent/config.LoadFromFile().
```

No more imports of `agent/providers`, `agent/prompts`, `agent/tools`, or `agent/config`. The audit tool talks only to `agent.New()` and `agent.HandleMessage()` — the same sealed API that external consumers use.

**Execution plan**:

1. Checkout `security-auditer`, merge latest `master` (which now has MONO-1–5 changes).
2. Resolve any merge conflicts (expected: none in `audit/` since it doesn't exist on `master`; possible conflicts in `go.mod`/`go.sum` and shared files that both branches touched).
3. Update `audit/pipeline/runner.go`:
   - Remove imports: `agent/providers`, `agent/prompts`, `agent/tools`.
   - Replace `providers.NewClient()` + `agent.NewAgent()` with `agent.New(agent.Config{...})`.
   - Remove the `_ "clyde/agent/tools"` blank import.
4. Update `audit/main.go`:
   - Replace `agent/config.LoadFromFile()` with a local config loader (or inline `godotenv` + `os.Getenv`, matching what `cli/cli.go` does).
5. If the repo now has `go.work`, add `./audit` to the `use` directive (audit is part of the root module since it's under the same `go.mod`, OR give it its own `go.mod` — see decision below).
6. Run `go build ./audit/...` and `cd audit && go test ./...` — all audit tests pass.
7. Push to `private` remote.

**Decision — audit module structure**:
- [ ] Decide: does `audit/` stay as a package in the root module (same `go.mod` as the CLI), or get its own `go.mod` (third module)?
  - **Same root module (recommended)**: simplest. `audit/` is just another directory in the root module, like `cli/`. The branch adds it; `master` doesn't have it. `go.work` doesn't need to change. Audit imports agent via the same `require` the CLI uses.
  - **Own module**: more isolation, but adds complexity to the branch (three `go.mod` files, `go.work` needs updating). Only worth it if audit has dependencies the CLI shouldn't pull. Evaluate and document decision.

**Acceptance Criteria**:

*Merge & update:*
- [ ] `security-auditer` branch is rebased on or merged with `master` at the MONO-5 tag (or later).
- [ ] All merge conflicts are resolved cleanly.
- [ ] `go.mod` / `go.sum` are consistent after merge (no stale deps, `go mod tidy` is clean).

*Audit code migrated to sealed API:*
- [ ] `audit/pipeline/runner.go` imports only `"github.com/this-is-alpha-iota/clyde/agent"` (no `agent/providers`, `agent/prompts`, `agent/tools`).
- [ ] `audit/main.go` does not import `agent/config` — config loading is self-contained.
- [ ] `agent.New(agent.Config{...})` is the sole agent construction path in audit code.
- [ ] No blank import `_ "clyde/agent/tools"` anywhere in `audit/`.
- [ ] `agent.NewAgent()` is not called anywhere in `audit/` (legacy constructor removed from audit code).

*Build & test:*
- [ ] `go build ./...` succeeds from the repo root on the `security-auditer` branch.
- [ ] All existing audit tests pass (`cd audit && go test ./...` or `go test ./audit/...`).
- [ ] All existing CLI/agent tests pass (`cd tests && go test ./...`).
- [ ] No test files were deleted — same test count as before on both the audit and core sides.

*Private remote:*
- [ ] Updated branch is pushed to `private` remote (`git push private security-auditer`).
- [ ] The branch can be cleanly merged with future `master` changes (verified by a dry-run: `git merge --no-commit --no-ff master` produces no conflicts in `audit/`).

*Documentation:*
- [ ] `progress.md` is updated with the migration details: what changed in audit code, why, and the module structure decision.
- [ ] `docs/private-consumer-pattern.md` documents the branch-based private consumer pattern as actually practiced:
  - How the `security-auditer` branch relates to `master`.
  - The merge workflow: `git checkout security-auditer && git merge master`.
  - Why `audit/` doesn't conflict (it only exists on the branch).
  - How the audit tool consumes the agent's sealed API.
  - The `private` remote setup (`git remote add private <url>`).
  - Trade-offs vs. a separate repo (acknowledged: branch-based is simpler for a single team; separate repo is better if multiple teams or CI isolation is needed).

---

### CMP-1: Conversation Token Counting & Automatic Compaction Trigger

**As a** user running a long autonomous session,
**I want** Clyde to automatically detect when the context window is nearly full and trigger compaction,
**so that** my session continues seamlessly without hitting context limits or crashing.

**Depends on**: SESS-1 (session file writing for compaction marker + system summary files)

**Acceptance Criteria**:

*Token counting & trigger:*
- [ ] A token counting mechanism tracks total input tokens from the most recent API response's `usage.input_tokens` field.
- [ ] A configurable `reserve_tokens` threshold is defined (default ~16,000 tokens), settable via `~/.clyde/config` (`RESERVE_TOKENS=16000`).
- [ ] Before each API call, the agent checks if `total_input_tokens > (context_window_size - reserve_tokens)`.
- [ ] When the threshold is exceeded, compaction is triggered automatically before sending the next message.
- [ ] There is no manual `/compact` command — compaction is always automatic.
- [ ] A `<timestamp>_compaction.md` file containing `🗜️ Compacting conversation history...` is written to the session directory.

*Post-compaction history shape:*
- [ ] After compaction, the conversation history is replaced with: system prompt + original user message (verbatim) + compaction summary + recent kept messages.
- [ ] A `<timestamp>_system.md` file containing the compaction summary as a `**System:**` message is written to the session directory.
- [ ] The agent continues the conversation seamlessly after compaction.

*Preserving the initial user message (formerly CMP-5):*
- [ ] The first user message in any conversation is tagged as `pinned` in the conversation history.
- [ ] During compaction, the first user message is always placed immediately after the system prompt and before any compaction summary — in full, unmodified.
- [ ] The first user message is included verbatim in every summarization pass so the handoff document always references the original ask.
- [ ] The first user message is never truncated, rephrased, or dropped, even under extreme token pressure.
- [ ] In any "full history" or debug view, the first message is visually marked (e.g., `📌 Original Mission`).

*Compaction stub:*
- [ ] Compaction produces a summary (initially a simple single-call implementation — detailed multi-step summarization is CMP-2).

*Tests:*
- [ ] Unit tests verify trigger fires at the correct threshold.
- [ ] Unit tests verify the original user message is preserved verbatim after compaction.
- [ ] Unit tests verify the first message survives 1, 2, and 5 compaction cycles unchanged.
- [ ] Unit tests verify the first message appears before the compaction summary in the post-compaction history.
- [ ] Unit tests verify conversation continues successfully after compaction.
- [ ] Unit tests verify `compaction.md` and `system.md` files are written to the session directory.
- [ ] Integration test with a real (or mocked) multi-turn conversation that hits the threshold, compacts, and resumes (SESS-2 resume loads from the new `system.md`).

---

### CMP-2: Agentic Multi-Step Compaction Workflow

**As a** long-running autonomous agent,
**I want** compaction to be performed as a multi-step agentic workflow (not a single LLM call),
**so that** the resulting handoff document is high-fidelity, structured, and reads like a developer status update — not a lossy summary.

**Depends on**: CMP-1 (trigger + stub compaction to replace)

**Acceptance Criteria**:

*Multi-phase workflow:*
- [ ] Compaction is implemented as an internal multi-phase workflow with distinct steps:
  1. **Goal/constraint extraction**: Identify the original mission, constraints, and acceptance criteria.
  2. **Decision capture**: Extract key decisions made, alternatives considered, and rationale.
  3. **File-state analysis**: Summarize current state of modified/created files, referencing git state (see git-centric criteria below).
  4. **Tool-result synthesis**: Summarize significant tool outputs (per CMP-3, or simple truncation if CMP-3 not yet landed).
  5. **Handoff drafting**: Produce a structured Markdown handoff document.
- [ ] Each phase uses a focused prompt running on the strongest available model with generous token budget.
- [ ] The final handoff document is structured Markdown with clear sections (Goal, Constraints, Progress, Decisions, Current State, Next Steps, Critical Context).
- [ ] The handoff document replaces the summarized portion of conversation history.
- [ ] All intermediate phase outputs are logged internally for debugging (viewable at Debug level).
- [ ] A progress message is displayed for each phase (e.g., `🗜️ Compaction: extracting decisions...`).

*Git-centric state tracking (formerly CMP-4):*
- [ ] At each compaction point, the file-state analysis phase captures: current commit SHA, branch name, short commit message (if any), and a one-line "what changed since last compaction" note.
- [ ] The handoff document's "Current State" section references the commit SHA and lets git handle detailed diffs.
- [ ] No cumulative raw-diff or modified-files lists are carried forward across compactions.
- [ ] A post-compaction hook optionally runs `git status` to verify repo cleanliness, appending warnings if uncommitted changes exist.
- [ ] If the working directory is not a git repo, the file-state phase is skipped gracefully and the handoff notes "not a git repo."

*Recent context fed to summarizer (formerly CMP-7):*
- [ ] During the multi-step workflow, the last 1–2 full kept turns are included as extra context in every summarization phase.
- [ ] The final handoff drafter is explicitly instructed to call out any open threads or decisions that bridge the summary and the kept messages.
- [ ] The extra context is kept small (just enough for continuity) and does not significantly increase summarization token usage.
- [ ] A configurable flag can disable this behavior for maximum token savings (`COMPACT_INCLUDE_RECENT_CONTEXT=false`).

*Tests:*
- [ ] Unit tests verify each phase produces expected output structure from mock inputs.
- [ ] Unit tests verify the final handoff document contains all required sections.
- [ ] Unit tests verify git state capture produces expected format (SHA, branch, message).
- [ ] Unit tests verify graceful handling in a non-git directory.
- [ ] Unit tests verify no raw diffs are accumulated across multiple compaction cycles.
- [ ] Unit tests verify recent context is included in summarizer input.
- [ ] Unit tests verify the `COMPACT_INCLUDE_RECENT_CONTEXT` flag disables inclusion when set to false.
- [ ] Integration test with a real multi-turn conversation produces a coherent, readable handoff.
- [ ] Integration test in a real git repo verifies correct SHA and branch after a compaction.
- [ ] Integration test verifies the handoff document references or bridges content from the kept messages.
- [ ] The handoff quality is manually reviewed and documented in the PR (compare to single-call summarization).

---

### CMP-3: Intelligent Tool-Result Summarization

**As a** long-running agent whose conversation contains large tool outputs,
**I want** oversized tool results to be intelligently summarized (not hard-truncated) during compaction,
**so that** critical details in the tail of tool outputs are preserved rather than chopped at an arbitrary character limit.

**Depends on**: CMP-2 (used within the tool-result synthesis phase)

**Acceptance Criteria**:
- [ ] A configurable size threshold for tool output summarization is defined (default: 2000 characters).
- [ ] During compaction, any tool output exceeding the threshold is passed to a dedicated LLM summarizer call.
- [ ] The summarizer receives the original user prompt + the two most recent kept messages as anchoring context.
- [ ] The summarizer decides what to keep verbatim, what to condense, and what to drop — it does NOT enforce a fixed output length.
- [ ] The summarized output includes a metadata note: `[Summarized: original N chars → M chars]`.
- [ ] Under extreme token pressure, the system falls back to hard truncation (configurable fallback).
- [ ] Tool outputs below the threshold are kept as-is (no unnecessary summarization).
- [ ] Unit tests verify summarization is triggered only for outputs exceeding the threshold.
- [ ] Unit tests verify the metadata note is present in summarized outputs.
- [ ] Unit tests verify fallback to truncation under token pressure.
- [ ] Integration test with a real large tool output (e.g., a big `grep` result) produces a meaningful summary.
