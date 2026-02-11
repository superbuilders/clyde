# Claude REPL - TODO List

## Priority Order (Do in this sequence)

### ✅ 1. 🗑️ Deprecate GitHub Tool - COMPLETED (2026-02-10)
**Status**: ✅ **COMPLETED**

**Rationale**: Now that we have `run_bash`, the dedicated `github_query` tool is redundant.
- `gh` commands work perfectly via `run_bash`
- Example: `run_bash("gh repo list")` vs `github_query("repo list")`
- Less code to maintain
- Consistent pattern: all external CLI tools go through bash

**Action Items**:
1. ✅ Remove `githubTool` from tools array in `callClaude()`
2. ✅ Remove `executeGitHubCommand()` function
3. ✅ Remove `case "github_query":` from switch statement
4. ✅ Update system prompt to use `run_bash` with `gh` commands instead
5. ✅ Update tests to use bash for GitHub operations
6. ✅ Update documentation (README, progress.md)

**Migration Example**:
```
OLD: github_query("repo list")
NEW: run_bash("gh repo list")
```

**Results**:
- All tests pass (13 passed, 3 skipped)
- Documentation updated
- System prompt updated with clear guidance on using `run_bash` with `gh`

---

### ✅ 2. 📝 System Prompt: Include progress.md Philosophy - COMPLETED (2026-02-10)
**Status**: ✅ **COMPLETED**

**Priority**: ⚠️ **CRITICAL** - Should have prevented needing to be reminded to update progress.md

**Problem**: When completing Priority #1, had to be reminded separately to update progress.md. Documentation should be automatic.

**Action Taken**: Updated system prompt to explicitly instruct Claude to:
- Read `progress.md` at the start of complex tasks
- Update `progress.md` with important learnings, bugs fixed, and design decisions
- Treat `progress.md` as the "memory" rather than raw conversation history
- Keep `progress.md` structured and organized (not a dump of all messages)
- **Always update progress.md before final commit when completing tasks**

**Key Additions to System Prompt**:
```
DOCUMENTATION & MEMORY:
- Read progress.md at start of complex tasks to understand project history
- Update progress.md when you:
  * Complete a major task or milestone
  * Discover and fix bugs
  * Make design decisions
  * Learn important patterns or lessons
- Always update progress.md BEFORE the final commit
- Keep documentation structured and curated (not a message dump)
- progress.md is your memory - maintain it actively
```

**Real Example - Priority #1**: Should have automatically updated docs with code changes.

**Real Example - Priority #2 (this task)**: Updating progress.md AND todos.md BEFORE final commit, not after being reminded!

**Results**:
- ✅ System prompt: 2.1 KB → 2.8 KB (+33%)
- ✅ All tests pass (13 passed, 3 skipped)
- ✅ Binary rebuilt (8.0 MB)
- ✅ Following new documentation pattern

---

### ✅ 3. 📢 Better Tool Progress Messages - COMPLETED (2026-02-10)
**Status**: ✅ **COMPLETED**

**Problem**: Generic progress messages didn't tell users what was happening:
```
→ Reading file...
→ Patching file...
→ Running bash command...
```

**Solution**: Enhanced all tool progress messages to show context:
```
→ Reading file: main.go
→ Patching file: todos.md (+353 bytes)
→ Running bash: go test -v
→ Listing files: . (current directory)
→ Writing file: progress.md (42.5 KB)
```

**Implementation**:
- Updated 5 display message locations in `handleConversation()`
- Added file path display for list_files, read_file
- Added size change display for patch_file (+/- bytes)
- Added command display for run_bash (truncated if > 60 chars)
- Added formatted size display for write_file (bytes/KB/MB)

**Code Changes**:
- Net +921 bytes in main.go
- All display messages now context-aware
- Maintains simplicity while adding clarity

**Results**:
- ✅ All tests pass (13 passed, 3 skipped)
- ✅ Binary rebuilt (8.0 MB, unchanged size)
- ✅ Better UX: users see exactly what's happening
- ✅ Test output shows new messages in action

**Verified in Test Output**:
```
→ Listing files: . (current directory)
→ Reading file: test_read_file.txt
→ Running bash: gh api user
→ Writing file: test_write_integration_new.txt (51 bytes)
```

---

### ✅ 4. 🔧 Better Error Handling & Messages - COMPLETED (2026-02-10)
**Status**: ✅ **COMPLETED**

**Problem**: Error messages were too generic and didn't help users fix problems.

**What Was Done**:
1. ✅ Enhanced all 5 tool execution functions with detailed error messages
2. ✅ Added context-specific guidance for common error scenarios
3. ✅ Improved API error messages with actionable suggestions
4. ✅ Enhanced startup error messages with setup instructions
5. ✅ Added helpful examples to all validation errors

**Error Message Improvements by Tool**:
- **list_files**: Directory not found, permission denied, with suggestions
- **read_file**: File not found, permission denied, directory vs file, large file warnings
- **patch_file**: Multi-line help for text not found and non-unique text errors
- **run_bash**: Exit code explanations (127=command not found, 126=permission denied, etc.)
- **write_file**: Directory doesn't exist with mkdir command, large file warnings
- **API errors**: Context-aware help for 401, 429, 400, 500+ errors
- **Startup errors**: .env file setup instructions with examples

**Examples of Improvements**:
```
BEFORE: "old_text not found in file"
AFTER:  Shows 3 common issues + 3 concrete suggestions

BEFORE: "command failed: exit status 127"
AFTER:  Explains exit code + provides installation/PATH troubleshooting

BEFORE: "failed to read file: no such file"
AFTER:  "file 'X' does not exist. Use list_files to see available files"
```

**Impact**:
- Better user experience (clear, actionable error messages)
- Faster debugging (suggestions save time)
- Educational (users learn best practices)
- Proactive warnings (prevents errors before they happen)
- Net +5.2 KB in main.go
- All tests still pass (13 passed, 3 skipped)

**Results**:
- ✅ All error messages are clear and helpful
- ✅ Context-specific suggestions provided
- ✅ Examples included where appropriate
- ✅ All tests pass with improved error handling
- ✅ Verified with manual testing

**Philosophy**: Error messages should be teachers, not just reporters.

---

### ✅ 5. 🔍 grep Tool (Search Across Files) - COMPLETED (2026-02-10)
**Status**: ✅ **COMPLETED**

**Purpose**: Search for patterns across multiple files with context and line numbers.

**What Was Built**:

**Tool Schema**:
```go
{
  "name": "grep",
  "description": "Search for patterns across multiple files. Returns file paths and matching lines with context.",
  "input_schema": {
    "type": "object",
    "properties": {
      "pattern": {
        "type": "string",
        "description": "The search pattern (can be regex)"
      },
      "path": {
        "type": "string", 
        "description": "Directory to search (defaults to current directory)"
      },
      "file_pattern": {
        "type": "string",
        "description": "Optional: filter by file pattern (e.g., '*.go', '*.md')"
      }
    },
    "required": ["pattern"]
  }
}
```

**Implementation**:
- Uses `grep -rnI` (recursive, line numbers, skip binary files)
- Supports `--include` for file pattern filtering
- Returns formatted results with match count and file count
- Handles "no matches found" gracefully with suggestions
- Context-aware error messages for permission issues

**Use Cases**:
- Find all references to a function/variable: `grep("func main", ".", "*.go")`
- Search for TODO comments: `grep("TODO", ".")`
- Find error messages in logs: `grep("error:", "logs")`
- Locate configuration values: `grep("API_KEY", ".")`

**Testing**:
- Unit tests: `TestExecuteGrep` (7 sub-tests)
  - Search across multiple files
  - File pattern filtering
  - No matches handling
  - Error cases
- Integration tests: `TestGrepIntegration` (3 sub-tests)
  - Search for function definitions
  - Search for TODO comments
  - Handle no matches gracefully
- All 16 tests pass (3 skipped)

**System Prompt Updates**:
Added grep decision logic to system prompt:
```
Search questions - Use grep for:
- "Find all references to X"
- "Where is function Y defined?"
- "Search for TODO comments"
- "Find error messages in logs"
- "Locate all files containing X"
- Can filter by file pattern: grep("TODO", ".", "*.go")
```

**Progress Messages**:
- `→ Searching: 'func main' in current directory (*.go)`
- `→ Searching: 'TODO' in . (all files)`

**Code Changes**:
- Added `grepTool` definition (24 lines)
- Added `executeGrep()` function (94 lines)
- Added grep case to tool execution switch (20 lines)
- Updated system prompt (+314 bytes)
- Added to tools array in `callClaude()`
- Total: ~4.6 KB added to main.go

**Test Suite**:
- Added `TestExecuteGrep()` with 7 sub-tests
- Added `TestGrepIntegration()` with 3 sub-tests
- Total: ~9 KB added to main_test.go
- Test runtime: +22.7 seconds (grep integration tests)

**Results**:
- ✅ All 16 tests pass (3 skipped)
- ✅ Binary size: 8.0 MB (unchanged)
- ✅ System prompt: 3.8 KB (+314 bytes)
- ✅ Documentation updated (progress.md, readme.md, todos.md)
- ✅ Comprehensive error handling and helpful messages
- ✅ Full integration test coverage

**Time Taken**: ~1 hour (as estimated!)

---

### ✅ 6. 🗂️ glob Tool (Fuzzy File Finding) - COMPLETED (2026-02-10)
**Status**: ✅ **COMPLETED**
**Purpose**: Search for patterns across multiple files

**Tool Schema**:
```go
{
  "name": "grep",
  "description": "Search for patterns across multiple files. Returns file paths and matching lines with context.",
  "input_schema": {
    "type": "object",
    "properties": {
      "pattern": {
        "type": "string",
        "description": "The search pattern (can be regex)"
      },
      "path": {
        "type": "string", 
        "description": "Directory to search (defaults to current directory)"
      },
      "file_pattern": {
        "type": "string",
        "description": "Optional: filter by file pattern (e.g., '*.go', '*.md')"
      }
    },
    "required": ["pattern"]
  }
}
```

**Use Cases**:
- Find all references to a function/variable
- Search for TODO comments
- Find error messages in logs
- Locate configuration values

**Implementation**: Use `grep -rn` or `rg` (ripgrep) via bash

**Estimated time**: 2 hours

---

**Purpose**: Find files matching patterns (like `find` or `fd`)

**Results**:
- ✅ All 18 tests pass (3 skipped)
- ✅ Binary size: 8.0 MB (unchanged)
- ✅ System prompt: 3.9 KB (+100 bytes)
- ✅ Documentation updated (progress.md, readme.md, todos.md)
- ✅ Comprehensive error handling and helpful messages
- ✅ Full integration test coverage
- ✅ Complements grep perfectly (grep finds content, glob finds files)

**Implementation**: Uses `find` command with `-name` for simple patterns, `-path` for recursive patterns. Converts `**` glob patterns to find-compatible patterns.

**Time Taken**: ~1 hour (as estimated!)

---

### 7. 📦 multi_patch Tool (Coordinated Multi-File Edits)
**Purpose**: Apply patches to multiple files atomically

**Tool Schema**:
```go
{
  "name": "multi_patch",
  "description": "Apply coordinated changes to multiple files. Uses git for rollback if any patch fails.",
  "input_schema": {
    "type": "object",
    "properties": {
      "patches": {
        "type": "array",
        "items": {
          "type": "object",
          "properties": {
            "path": {"type": "string"},
            "old_text": {"type": "string"},
            "new_text": {"type": "string"}
          }
        },
        "description": "Array of patches to apply"
      }
    },
    "required": ["patches"]
  }
}
```

**Behavior**:
1. Suggest user commit current changes first
2. Apply each patch in sequence
3. If any fails, show which one failed and stop
4. User can `git restore` to undo if needed
5. If all succeed, show summary of changes

**Use Cases**:
- Refactor function name across multiple files
- Update import paths
- Apply consistent formatting changes
- Coordinate breaking changes

**Estimated time**: 4 hours

---

### 8. 🌐 web_search Tool
**Purpose**: Search the internet for information beyond training data

**Tool Schema**:
```go
{
  "name": "web_search",
  "description": "Search the internet and return URLs + snippets. Use for recent info not in training data.",
  "input_schema": {
    "type": "object",
    "properties": {
      "query": {
        "type": "string",
        "description": "The search query"
      },
      "num_results": {
        "type": "integer",
        "description": "Number of results to return (default 5)"
      }
    },
    "required": ["query"]
  }
}
```

**Use Cases**:
- Look up current API documentation
- Find solutions to novel errors
- Check latest versions of dependencies
- Research unfamiliar technologies
- Get recent news/updates

**Implementation**: TBD - need to research web search APIs (meta!)

**Estimated time**: 3 hours

---

### 9. 🌐 browse Tool (Fetch URL Contents)
**Purpose**: Fetch and read web pages

**Tool Schema**:
```go
{
  "name": "browse",
  "description": "Fetch and return the contents of a URL. Like read_file but for web pages.",
  "input_schema": {
    "type": "object",
    "properties": {
      "url": {
        "type": "string",
        "description": "The URL to fetch"
      }
    },
    "required": ["url"]
  }
}
```

**Behavior**:
- Fetch URL contents
- Parse HTML to readable text (strip tags, keep structure)
- Return markdown-like formatted content
- Handle errors (404, timeout, etc.)

**Use Cases**:
- Read documentation pages
- Follow up on web_search results
- Check API reference docs
- Read blog posts/articles

**Implementation**: Use `curl` + HTML parsing (or Go's `net/http` + `goquery`)

**Estimated time**: 2 hours

---

### 10. 📂 Code Organization & Architecture Separation
**Purpose**: Split single-file architecture into multiple files and separate agent from CLI

**File Structure**:
```
claude-repl/
├── main.go       # CLI entry point and REPL loop
├── agent.go      # Core agent logic (API calls, tool execution)
├── tools.go      # Tool definitions and execution functions
├── api.go        # Claude API client code
├── types.go      # Struct definitions
└── ...
```

**Benefits**:
- Easier to navigate and understand
- Better separation of concerns
- Cleaner for contributors
- Maintains single-binary compilation
- **Enables agent reuse in different contexts** (API, GUI, bash, Go package)

**Key Abstraction**:
```go
type Agent interface {
    HandleMessage(input string) (response string, err error)
    RegisterTool(tool Tool) error
    GetHistory() []Message
}
```

**Estimated time**: 3 hours

---

### 11. 📎 File Input Support (Future)
**Purpose**: Accept files as input (text and images)

**Text File Input**:
- Command syntax: `/attach path/to/file.txt`
- Pipe support: `cat error.log | ./claude-repl`
- Useful for: logs, error messages, config files, code snippets

**Image Input** (multimodal):
- Upload and analyze images
- Use Claude's vision capabilities
- Useful for: screenshots, diagrams, UI mockups, charts
- Requires API changes for image content blocks

**Use Cases**:
- "Debug this error screenshot"
- "Convert this diagram to code"
- "What's wrong with this UI?"
- "Analyze this chart and summarize trends"

**Estimated time**: 
- Text file input: 2 hours
- Image input: 4 hours (includes API changes)

---

## Total Estimated Time: ~25.5 hours
