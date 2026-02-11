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

### 3. 📢 Better Tool Progress Messages
**Current Behavior**:
```
→ Reading file...
→ Patching file...
→ Running bash command...
```

**Desired Behavior**:
```
→ Reading file: main.go
→ Patching file: main.go (replacing 42 bytes)
→ Running bash command: go test -v
→ Listing files: ./src
→ Writing file: todos.md (4.7 KB)
```

**Implementation**:
- Pass relevant parameters to display messages
- Show file paths, command names, sizes where applicable
- Help users understand what's happening without being verbose

**Estimated time**: 30 minutes

---

### 4. 🔧 Better Error Handling & Messages
**Improvements Needed**:
- More helpful error messages when tools fail
- Suggest what to do when `patch_file` fails (e.g., "old_text not unique - try adding more context")
- Better validation before executing tools
- Clearer feedback on what went wrong and how to fix it
- Improved error recovery strategies (automatic retry with adjustments)

**Examples**:
```
BAD:  "old_text not found in file"
GOOD: "old_text not found in file. Make sure it matches exactly including whitespace. Try reading the file first with read_file."

BAD:  "command failed"
GOOD: "Command 'go test' failed with exit code 1. Output: [error details]. This usually means there are failing tests."
```

**Estimated time**: 2 hours

---

### 5. 🔍 grep Tool (Search Across Files)
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

### 6. 🗂️ glob Tool (Fuzzy File Finding)
**Purpose**: Find files matching patterns (like `find` or `fd`)

**Tool Schema**:
```go
{
  "name": "glob",
  "description": "Find files matching patterns. More flexible than ls for navigating projects.",
  "input_schema": {
    "type": "object",
    "properties": {
      "pattern": {
        "type": "string",
        "description": "File pattern to match (e.g., '**/*.go', '*_test.go', '*.md')"
      },
      "path": {
        "type": "string",
        "description": "Directory to search (defaults to current directory)"
      }
    },
    "required": ["pattern"]
  }
}
```

**Use Cases**:
- Find all test files: `**/*_test.go`
- Find all markdown docs: `**/*.md`
- Find specific file: `**/main.go`
- Navigate large codebases

**Implementation**: Use `find` command or Go's `filepath.Glob`

**Estimated time**: 1 hour

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
