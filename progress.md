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

**Improvements by Tool**:

#### 1. list_files
- **Directory doesn't exist**: Suggests using '.' for current directory
- **Permission denied**: Identifies permission issues and suggests checking permissions
- **General errors**: Includes full error details and output

**Example**:
```
directory '/nonexistent' does not exist. Use '.' for current directory or provide a valid path
```

#### 2. read_file
- **Missing path**: Shows example usage
- **File doesn't exist**: Suggests using list_files to see available files
- **Permission denied**: Identifies permission issues
- **Directory instead of file**: Suggests using list_files instead
- **Large files**: Warns about files >1MB and suggests alternatives

**Example**:
```
file 'nonexistent.txt' does not exist. Use list_files to see available files
```

#### 3. patch_file
- **Missing parameters**: Shows example usage with all parameters
- **File doesn't exist**: Suggests using write_file to create new file
- **Old text not found**: Multi-line explanation with 3 common issues + 3 suggestions
- **Non-unique old text**: Explains the problem with occurrence count + detailed fix steps
- **Permission errors**: Clear permission denied messages

**Example (non-unique text)**:
```
The old_text appears 2 times in the file. It must be unique to ensure the right text is replaced.

To fix this:
  1. Include more surrounding context in old_text
  2. Add nearby lines or unique identifiers
  3. Example: Instead of just 'func foo()', use 'func foo() {\n\t// comment\n\treturn nil'

Use read_file to see the full context around each occurrence.
```

**Example (text not found)**:
```
The old_text was not found in the file. Common issues:
  1. Whitespace or newlines don't match exactly
  2. The text has already been changed
  3. There's a typo in old_text

Suggestions:
  - Use read_file first to see the current content
  - Copy the exact text including all whitespace
  - Check for tabs vs spaces, line endings, etc.
```

#### 4. run_bash
- **Missing command**: Shows example usage
- **Exit code 127** (command not found): Explains the error + suggests checking installation and PATH
- **Exit code 126** (permission denied): Explains the error + suggests chmod
- **Exit code 1 with test commands**: Suggests checking output for test failures
- **Exit code 1 with git commands**: Suggests common git issues
- **All errors**: Shows full command output for debugging

**Example (command not found)**:
```
Command failed with exit code 127: nonexistentcommand

Output:
bash: nonexistentcommand: command not found

Exit code 127 typically means 'command not found'.
Suggestions:
  - Check if the command is installed
  - Verify the command name is spelled correctly
  - Try which <command> to see if it's in PATH
```

#### 5. write_file
- **Missing parameters**: Shows example usage
- **Directory doesn't exist**: Shows exact mkdir command needed
- **Permission denied**: Clear permission error messages
- **Large file warning**: Warns before replacing files >100KB, suggests using patch_file

**Example (directory doesn't exist)**:
```
directory '/nonexistent/path' does not exist. Create it first with: run_bash("mkdir -p /nonexistent/path")
```

#### 6. API Errors (callClaude function)
- **401 Unauthorized**: Provides API key setup instructions + console link
- **429 Rate Limit**: Explains rate limits + suggests waiting + console link
- **400 Bad Request**: Lists common causes
- **500/502/503/504 Server Errors**: Explains temporary nature + status page link
- **Network errors**: Suggests checking internet connection
- **All errors**: Parses error response for detailed message when available

**Example (401 error)**:
```
API error (status 401)
Error: Invalid API key

Authentication failed. Check your API key:
  - Verify TS_AGENT_API_KEY in .env file
  - Ensure the key starts with 'sk-ant-'
  - Try generating a new key at https://console.anthropic.com/
```

#### 7. Startup Errors (main function)
- **Missing .env file**: Shows exact location being checked + setup instructions
- **Missing API key**: Shows exact .env format needed + console link
- **EOF on input**: Graceful exit on Ctrl+D

**Example**:
```
Error reading .env file from '.env': no such file or directory

To fix this:
  1. Create a .env file in the current directory, OR
  2. Set ENV_PATH environment variable to your .env file location
  3. Example: export ENV_PATH=/path/to/.env

The .env file should contain:
  TS_AGENT_API_KEY=your-anthropic-api-key-here
```

**Code Changes**:
- Enhanced all 5 tool execution functions (executeListFiles, executeReadFile, executePatchFile, executeRunBash, executeWriteFile)
- Enhanced callClaude API error handling
- Enhanced main function startup error handling
- Enhanced parameter validation messages in handleConversation
- Net change: +5.2 KB in main.go

**Impact**:
- **Better UX**: Users understand what went wrong immediately
- **Faster debugging**: Clear suggestions save time
- **Less frustration**: No more cryptic error codes
- **Educational**: Users learn best practices through error messages
- **Proactive**: Prevents errors (e.g., warns before replacing large files)
- **All tests pass**: 13 passed, 3 skipped (no breaking changes)

**Test Verification**:
Created demo showing all error message improvements:
```bash
# Tests showed:
# ✓ Non-unique text error with detailed fix steps
# ✓ Text not found error with troubleshooting guide
# ✓ File not found error with tool suggestions
# ✓ Command not found error with exit code explanation
# ✓ All messages are clear, helpful, and actionable
```

**Philosophy**:
Error messages should be **teachers**, not just reporters. Every error is an opportunity to help the user learn and succeed.

## Current Status (2026-02-10)

**Active Tools**: 7
1. `list_files` - Directory listings with helpful error messages
2. `read_file` - Read file contents with size warnings and validation
3. `patch_file` - Find/replace edits with detailed guidance for common issues
4. `write_file` - Create/replace files with safety warnings for large files
5. `run_bash` - Execute any bash command with exit code explanations
6. `grep` - Search for patterns across multiple files with context
7. `glob` - Find files matching patterns (fuzzy file finding) (NEW ✨)

**Test Suite**: 18 tests passing, 3 skipped
- Total runtime: ~87 seconds (with new glob tests)
- Full integration coverage for all tools including glob
- No flaky tests
- All tests pass after glob implementation

**Binary**: 8.0 MB compiled binary
- Single-file architecture maintained
- Zero external dependencies
- Fast startup time
- Now includes grep search functionality AND glob file finding

**System Prompt**: 3.9 KB (+100 bytes)
- Includes comprehensive tool decision logic
- Includes grep search patterns and examples
- Includes glob file finding patterns and examples (NEW)
- Includes progress.md philosophy and memory model
- Instructs AI to read and update progress.md proactively

**Tool Progress Messages**: Enhanced
- Show context: file paths, command names, sizes
- Examples: "→ Reading file: main.go", "→ Running bash: go test -v"
- "→ Searching: 'func main' in current directory (*.go)"
- NEW: "→ Finding files: '**/*.go' in current directory" (glob)
- Better user experience and transparency

**Error Handling & Messages**: Enhanced
- Comprehensive error messages with context and suggestions
- Context-specific guidance based on error type
- All tools provide helpful suggestions when operations fail
- All tests still pass (18 passed, 3 skipped)

**Completed Priorities**: 6 / 11 from todos.md
1. ✅ Deprecate GitHub Tool (replaced with run_bash)
2. ✅ System Prompt: progress.md Philosophy
3. ✅ Better Tool Progress Messages
4. ✅ Better Error Handling & Messages
5. ✅ grep Tool (Search Across Files)
6. ✅ glob Tool (Fuzzy File Finding)

**Next Priority**: #7 - multi_patch Tool (Coordinated Multi-File Edits)
- Estimated time: 4 hours

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

**Added grep tool** (2026-02-10) - Priority #5 ✅:
Enables powerful search across multiple files:
- Search for text patterns or regex across directories
- Filter by file patterns (e.g., `*.go`, `*.md`)
- Returns file paths and matching lines with line numbers
- Helpful error messages for no matches or missing directories
- Perfect for finding function definitions, TODO comments, error messages

**Features**:
- Uses `grep -rnI` (recursive, line numbers, skip binary files)
- Supports `--include` for file pattern filtering
- Returns formatted results with match count and file count
- Handles "no matches found" gracefully with suggestions
- Context-aware error messages for permission issues

**Use Cases**:
- Find all references to a function: `grep("func main", ".", "*.go")`
- Search for TODO comments: `grep("TODO", ".")`
- Find error messages: `grep("error:", "logs")`
- Locate configuration values: `grep("API_KEY", ".")`

**Testing Standards Maintained**:
- Unit tests for execution function (`TestExecuteGrep`)
  - Search across multiple files
  - File pattern filtering
  - No matches handling
  - Error cases
- Integration tests with full API round-trips (`TestGrepIntegration`)
  - Search for function definitions
  - Search for TODO comments
  - Handle no matches gracefully
- All 16 tests pass (3 skipped)

**Implementation**:
```go
func executeGrep(pattern, path, filePattern string) (string, error) {
    // Uses grep -rnI with optional --include filter
    // Returns formatted results with match and file counts
    // Handles exit code 1 (no matches) gracefully
    // Provides helpful error messages for common issues
}
```

**Testing Standards Maintained**:
Both run_bash and write_file tools include:
- Unit tests for execution functions (`TestExecuteRunBash`, `TestExecuteWriteFile`)
- Integration tests with full API round-trips (`TestRunBashIntegration`, `TestWriteFileIntegration`)
- Multiple sub-tests covering different scenarios (success, errors, edge cases)
- Validation of tool_use and tool_result blocks
- Explicit checks for ToolUseID to prevent regression bugs

**Added glob tool** (2026-02-10) - Priority #6 ✅:
Enables fuzzy file finding by pattern matching:
- Find files matching patterns (e.g., `*.go`, `**/*.go`, `*_test.go`)
- More flexible than list_files for navigating large projects
- Recursive search support with `**` patterns
- Returns file paths with count summary
- Helpful error messages for no matches or missing directories
- Perfect for locating files in large codebases

**Features**:
- Uses `find` command with `-name` for simple patterns, `-path` for recursive patterns
- Converts `**` glob patterns to find-compatible patterns
- Type filtering (`-type f`) to only find files, not directories
- Returns formatted results with file count summary
- Handles "no files found" gracefully with pattern suggestions
- Context-aware error messages for permission issues

**Use Cases**:
- Find all test files: `glob("*_test.go")`
- Find all Go files recursively: `glob("**/*.go")`
- Locate specific file anywhere: `glob("**/main.go")`
- Find all markdown docs: `glob("**/*.md", "docs")`

**Pattern Support**:
- `*.go` - all Go files in directory (simple pattern)
- `**/*.go` - all Go files recursively (recursive pattern)
- `*_test.go` - all test files in directory
- `**/main.go` - find main.go anywhere in subdirectories
- `*.md` - all markdown files

**Testing Standards Maintained**:
- Unit tests for execution function (`TestExecuteGlob`)
  - Simple patterns (*.go)
  - Test file patterns (*_test.go)
  - Recursive patterns (**/*.go, **/*.md)
  - Specific file search (README.md)
  - No matches handling
  - Error cases (non-existent dir, empty pattern)
  - Default path handling
- Integration tests with full API round-trips (`TestGlobIntegration`)
  - Find test files
  - Find Go files recursively
  - Find specific file (README.md)
  - Handle no matches gracefully
- All 18 tests pass (3 skipped)

**Implementation**:
```go
func executeGlob(pattern, path string) (string, error) {
    // Uses find with -name or -path depending on pattern
    // Converts ** patterns: **/*.go → */*.go (find recurses by default)
    // Returns formatted results with file counts
    // Handles no matches gracefully with suggestions
    // Provides helpful error messages for common issues
}
```

**Comparison: glob vs grep**:
- **glob**: Find files by name pattern
  - Use when: "Find all test files", "Where are the Go files?"
  - Returns: File paths only
  - Example: `glob("*_test.go")` → list of test files
  
- **grep**: Search file contents for patterns
  - Use when: "Find all TODOs", "Where is function X defined?"
  - Returns: File paths + matching lines with context
  - Example: `grep("TODO", ".", "*.go")` → files and lines with TODO

Together, these tools provide comprehensive code navigation: glob finds the files, grep finds the content.

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
