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

### ✅ 7. 📦 multi_patch Tool (Coordinated Multi-File Edits) - COMPLETED (2026-02-10)
**Status**: ✅ **COMPLETED**

**Purpose**: Apply patches to multiple files atomically with automatic rollback on failure

**What Was Built**:

**Tool Schema**:
```go
{
  "name": "multi_patch",
  "description": "Apply coordinated changes to multiple files atomically. Uses git for rollback if any patch fails.",
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

**Implementation Details**:
- Parses and validates all patches before applying any
- Checks for git availability and repository status  
- Warns about uncommitted changes (returns early for safety)
- Applies patches sequentially using `executePatchFile`
- On failure: automatically rolls back using `git checkout --`
- On success: provides summary with git commit suggestions
- Comprehensive error handling with helpful messages

**Safety Features**:
1. **Pre-flight validation**: Checks all patch structures first
2. **Git integration**: Detects git availability for rollback
3. **Uncommitted changes warning**: Suggests committing first
4. **Atomic rollback**: Restores all files if any patch fails
5. **Clear guidance**: Next steps after success or failure

**Use Cases**:
- Refactor function name across multiple files
- Update import paths
- Apply consistent formatting changes
- Coordinate breaking changes

**Testing**:
- Unit tests: `TestExecuteMultiPatch` (9 sub-tests)
  - Single & multiple patch success
  - Rollback on failure (verifies restoration)
  - Missing fields validation
  - Uncommitted changes warning
- Integration tests: `TestMultiPatchIntegration` (2 sub-tests)
  - Coordinated multi-file refactor
  - Uncommitted changes handling
- All 20 tests pass (4 skipped)

**System Prompt Updates**:
Added multi_patch decision logic:
```
Multi-file editing - Use multi_patch for:
- "Rename function X to Y across all files"
- "Update all import paths from A to B"
- "Apply consistent changes to multiple files"
- "Refactor code across the codebase"
- Coordinates patches and rolls back on failure
- Best practice: Suggest git commit before multi_patch operations
```

**Progress Message**:
- `→ Applying multi-patch: 3 files`

**Code Changes**:
- Added `multiPatchTool` definition (~40 lines)
- Added `executeMultiPatch()` function (~163 lines)
- Added multi_patch case to switch statement (~7 lines)
- Updated system prompt (+326 bytes)
- Added to tools array in `callClaude()`
- Total: ~5.7 KB added to main.go

**Test Suite**:
- Added `multi_patch_test.go` with 9 unit tests and 2 integration tests
- Total: ~10 KB in separate test file
- Test runtime: +0.17 seconds (unit tests), integration skipped without API key

**Results**:
- ✅ All 20 tests pass (4 skipped)
- ✅ Binary size: 8.0 MB (unchanged)
- ✅ System prompt: 4.2 KB (+326 bytes)
- ✅ Documentation updated (progress.md, readme.md, todos.md)
- ✅ Git-based rollback working perfectly
- ✅ Safety warnings functioning as intended
- ✅ Full integration test coverage

**Time Taken**: ~2 hours (faster than estimated 4 hours!)

**Design Decision - Early Return on Uncommitted Changes**:
The function intentionally returns early with a warning when uncommitted changes are detected, rather than proceeding automatically. This is a safety feature that:
- Prevents accidental loss of work
- Encourages good git hygiene (commit before refactor)
- Gives users conscious control
- Can proceed by re-running after reviewing warning

---

### ✅ 8. 🌐 web_search Tool - COMPLETED (2026-02-10)
**Status**: ✅ **COMPLETED**

**Purpose**: Search the internet for information beyond training data

**Implementation Decision**: **Brave Search API**
- Official API with generous free tier (2,000 queries/month)
- Independent search index with good quality results
- Privacy-focused and ToS-compliant
- Simple REST API with structured JSON responses
- Clear upgrade path ($5/mo for 20K queries)
- Better than Exa AI and DuckDuckGo alternatives

**Tool Schema**:
```go
{
  "name": "web_search",
  "description": "Search the internet using Brave Search API. Returns titles, URLs, and snippets. Use for current info, documentation, error solutions, package versions, and recent news.",
  "input_schema": {
    "type": "object",
    "properties": {
      "query": {
        "type": "string",
        "description": "The search query"
      },
      "num_results": {
        "type": "integer",
        "description": "Number of results to return (1-10, default 5)",
        "default": 5
      }
    },
    "required": ["query"]
  }
}
```

**Configuration**:
```bash
# Add to .env file:
BRAVE_SEARCH_API_KEY=your-brave-api-key-here

# Get API key at: https://brave.com/search/api/
```

**Output Format**:
```
Found 5 results for "golang http client":

1. [Go HTTP Client Documentation] - https://pkg.go.dev/net/http
   The http package provides HTTP client and server implementations...

2. [Making HTTP Requests in Go] - https://example.com
   Learn how to make HTTP requests using Go's standard library...

[... more results ...]
```

**Use Cases**:
- Look up current API documentation: `web_search("golang 1.24 http client")`
- Find solutions to novel errors: `web_search("go error context deadline exceeded")`
- Check latest versions: `web_search("latest stable go version 2026")`
- Research unfamiliar tech: `web_search("what is HTMX")`
- Get recent news/updates: `web_search("anthropic claude api changes")`

**Implementation Details**:
1. Use Brave Search API endpoint: `https://api.search.brave.com/res/v1/web/search`
2. Pass API key in `X-Subscription-Token` header
3. Parse JSON response for `web.results` array
4. Extract `title`, `url`, `description` from each result
5. Format as numbered list with titles, URLs, and snippets
6. Handle rate limits (429) with clear error message
7. Handle missing API key with setup instructions

**Error Handling**:
- Missing API key: "BRAVE_SEARCH_API_KEY not found in .env. Get your free API key at https://brave.com/search/api/"
- Rate limit (429): "Monthly search limit reached (2000/2000). Resets on [date]. Upgrade at brave.com/search/api"
- No results: "No results found for '[query]'. Try different keywords or check spelling."
- API errors: Show status code and error message from Brave

**Testing**:
- Unit tests: `TestExecuteWebSearch` (6 sub-tests)
  - Successful search with results
  - No results found
  - Missing API key error
  - Invalid query handling
  - Rate limit handling
  - API error handling
- Integration tests: `TestWebSearchIntegration` (2 sub-tests)
  - Search for Go documentation
  - Search for specific error message

**System Prompt Addition**:
```
Web search - Use web_search for:
- "Look up the latest [technology/API]"
- "Find documentation for [package/library]"
- "Search for solutions to [error message]"
- "What's the current version of [tool]?"
- "Find recent news about [topic]"
- Returns URLs + snippets from web search
```

**Estimated time**: 3 hours

**Results**:
- ✅ All 22 tests pass (4 skipped: deprecated edit_file tests)
- ✅ Binary size: 8.1 MB (+0.1 MB)
- ✅ System prompt: 4.4 KB (+200 bytes)
- ✅ Documentation updated (progress.md, README.md, todos.md)
- ✅ Integration tests with real Brave API calls working perfectly
- ✅ Comprehensive error handling with helpful setup guidance
- ✅ Privacy-focused solution (ToS-compliant, no scraping)
- ✅ Free tier provides 2,000 searches/month

**Time Taken**: 3 hours (exactly as estimated!)

**Example Output**:
```
→ Searching web: "golang http client tutorial"

Found 4 results for "golang http client tutorial":

1. [Go HTTP Client Documentation] - https://pkg.go.dev/net/http
   The http package provides HTTP client and server implementations...

2. [Making HTTP Requests in Go] - https://www.digitalocean.com/...
   Learn how to make HTTP requests using Go's standard library...

3. [Practical Go Lessons] - https://www.practical-go-lessons.com/...
   Focused on building HTTP clients with the standard library...

4. [Go by Example] - https://gobyexample.com/http-clients
   Simple, practical examples of HTTP client usage...
```

---

### ✅ 9. 🌐 browse Tool (Fetch URL Contents) - COMPLETED (2026-02-10)
**Status**: ✅ **COMPLETED**

**Purpose**: Fetch and read web pages, optionally extracting specific information with AI

**Implementation Decision**: **Go net/http + HTML-to-Markdown library**
- Use Go's standard `net/http` for fetching
- Use `github.com/JohannesKaufmann/html-to-markdown` for HTML conversion
- Optional AI processing for targeted extraction (like Claude Code's WebFetch)
- Breaks zero-dependency principle but provides better UX

**Alternative (Zero Dependencies)**: Use `run_bash` with `curl | pandoc`
- Keeps zero Go dependencies
- Requires pandoc installed on system
- More brittle but aligns with "lean into standard tools" philosophy
- Recommend: Start with library, can switch to bash approach later if desired

**Tool Schema**:
```go
{
  "name": "browse",
  "description": "Fetch a URL and convert HTML to readable markdown. Optionally extract specific information using AI processing.",
  "input_schema": {
    "type": "object",
    "properties": {
      "url": {
        "type": "string",
        "description": "The URL to fetch (HTTP/HTTPS)"
      },
      "prompt": {
        "type": "string",
        "description": "Optional: What to extract/summarize from the page. If not provided, returns the full converted markdown.",
        "default": "Return the full page content as readable markdown"
      },
      "max_length": {
        "type": "integer",
        "description": "Maximum content length in KB (default 500, max 1000)",
        "default": 500
      }
    },
    "required": ["url"]
  }
}
```

**Behavior**:
1. **Fetch**: HTTP GET with 30-second timeout
2. **Follow redirects**: Up to 10 automatic redirects
3. **Size check**: Reject if Content-Length > max_length
4. **Convert HTML → Markdown**: Strip scripts/styles, keep structure
5. **Truncate if needed**: If content > max_length after conversion
6. **AI Processing** (optional): If prompt provided, send markdown + prompt to Claude
7. **Return**: Either full markdown or AI-extracted info

**Output Format** (without prompt):
```markdown
# Page Title

Main content converted to markdown format...

- Lists preserved
- Links: [text](url)
- Code blocks preserved
- Headers maintained
```

**Output Format** (with prompt):
```
AI's response based on the prompt and page content
```

**Use Cases**:
- Read full page: `browse("https://pkg.go.dev/net/http")`
- Extract specific info: `browse("https://go.dev/doc/", "List all tutorial sections")`
- Follow search results: `browse("https://blog.example.com/article")`
- Summarize documentation: `browse("https://docs.example.com", "What are the main features?")`
- Check API reference: `browse("https://api.example.com/docs")`

**Implementation Details**:
1. **HTTP Client Setup**:
   - Set User-Agent: "claude-repl/1.0 (Go HTTP Client)"
   - Set timeout: 30 seconds
   - Follow redirects: http.Client.CheckRedirect (max 10)
   - Set max response size: 1MB (configurable via max_length)

2. **HTML to Markdown**:
   - Use `html-to-markdown` library
   - Strip: `<script>`, `<style>`, `<iframe>`, ads
   - Keep: headings, paragraphs, lists, links, code blocks, tables
   - Clean whitespace and normalize newlines

3. **AI Processing** (if prompt provided):
   - Truncate markdown to fit in Claude context (keep first N chars)
   - Send to Claude API with prompt: "Given this webpage content: [markdown]\n\nUser request: [prompt]"
   - Return Claude's response
   - Cache conversion for repeated queries (optional v2 feature)

4. **Error Handling**:
   - Invalid URL: "Invalid URL format. Must start with http:// or https://"
   - DNS errors: "Could not resolve domain [domain]. Check the URL."
   - 404: "Page not found (404): [url]"
   - 403/401: "Access denied (403). The page may require authentication."
   - Timeout: "Request timed out after 30 seconds. The server may be slow or unreachable."
   - Too large: "Page too large ([size]). Max allowed: [max_length]KB. Increase max_length or try a different page."
   - Network errors: "Network error: [details]. Check your internet connection."

**Configuration**:
No additional API keys needed (uses existing Claude API key for optional AI processing)

**Testing**:
- Unit tests: `TestExecuteBrowse` (8 sub-tests)
  - Fetch and convert valid HTML page
  - Handle 404 errors
  - Handle timeout (mock)
  - Handle too-large content
  - Handle invalid URL
  - Handle redirect following
  - Convert HTML to markdown correctly
  - AI extraction with prompt (integration-like)
- Integration tests: `TestBrowseIntegration` (3 sub-tests)
  - Fetch real documentation page
  - Extract specific info with prompt
  - Handle 404 gracefully

**System Prompt Addition**:
```
Web browsing - Use browse for:
- "Read the page at [URL]"
- "What does [URL] say about [topic]?"
- "Summarize the documentation at [URL]"
- "Extract [specific info] from [URL]"
- Without prompt: returns full page as markdown
- With prompt: AI extracts specific information
```

**Dependencies Added**:
```bash
go get github.com/JohannesKaufmann/html-to-markdown
```

**Estimated time**: 3-4 hours
- 1 hour: Basic fetching + HTML-to-markdown conversion
- 1 hour: Error handling + size limits + redirects
- 1 hour: AI processing with prompt parameter
- 1 hour: Testing (unit + integration)

**Results**:
- ✅ All 25 tests pass (4 skipped: deprecated edit_file tests)
- ✅ Binary size: 8.1 MB (unchanged)
- ✅ System prompt: 4.6 KB (+200 bytes)
- ✅ Documentation updated (progress.md, README.md, todos.md)
- ✅ Integration tests with real web pages (example.com) working perfectly
- ✅ AI extraction with prompts working excellently
- ✅ HTML-to-markdown conversion producing clean, readable output
- ✅ Comprehensive error handling (404, 403, timeouts, etc.)
- ✅ Dependency added: html-to-markdown library

**Time Taken**: ~3.5 hours (on target with 3-4 hour estimate!)

**Example Output** (basic fetch):
```
→ Browsing: https://example.com

# Example Domain

This domain is for use in illustrative examples in documents...
```

**Example Output** (with AI extraction):
```
→ Browsing: https://example.com (extract: "What is the main heading?")

The main heading on the example.com page is **"Example Domain"**. This is
formatted as an H1 heading (the top-level heading) on the page.
```

**Integration with web_search**:
Perfect workflow: use web_search to find pages, then browse to read them:
```
1. web_search("golang http client tutorial") → Get URLs
2. browse("https://pkg.go.dev/net/http") → Read full documentation
3. browse("https://...", "List all key functions") → Extract specific info
```

---

### ✅ 10. 📂 Code Organization & Architecture Separation - COMPLETED (2026-02-13)
**Status**: ✅ **COMPLETED**

**Purpose**: Split single-file architecture into multiple files and packages

**What Was Achieved**: Successfully refactored 1,652-line main.go into organized packages

**Proposed Structure**:
```
claude-repl/
├── main.go                 # CLI entry point (REPL loop, main function)
├── config/
│   └── config.go          # Config struct, env loading, validation
├── api/
│   ├── client.go          # Claude API client
│   └── types.go           # Message, Response, ContentBlock types
├── agent/
│   ├── agent.go           # Core conversation/tool coordination logic
│   └── history.go         # Conversation history management
├── tools/
│   ├── registry.go        # Tool registration and dispatch
│   ├── list_files.go      # list_files tool (definition + execution)
│   ├── read_file.go       # read_file tool
│   ├── patch_file.go      # patch_file tool
│   ├── write_file.go      # write_file tool
│   ├── run_bash.go        # run_bash tool
│   ├── grep.go            # grep tool
│   ├── glob.go            # glob tool
│   ├── multi_patch.go     # multi_patch tool
│   ├── web_search.go      # web_search tool
│   └── browse.go          # browse tool
├── prompts/
│   └── system.txt         # System prompt (external file)
└── errors/
    └── errors.go          # Custom error types and helpers
```

**Benefits**:
- **Maintainability**: Each tool in its own file (~100-200 lines)
- **Separation of concerns**: API, agent, tools, config all separate
- **Testability**: Easier to test individual components
- **Extensibility**: Easy to add new tools (just add new file in tools/)
- **Readability**: Clear structure, easier to find code
- **Reusability**: Agent can be imported as a Go package
- **External prompts**: System prompt in separate file (easier iteration)
- Still compiles to single binary

**Tool Pattern** (each tool file):
```go
package tools

// Tool definition (schema)
var ListFilesTool = Tool{...}

// Execution function
func ExecuteListFiles(params map[string]interface{}) (string, error) {...}

// Display message
func DisplayListFiles(params map[string]interface{}) string {...}

// Register with registry
func init() {
    Register(ListFilesTool, ExecuteListFiles, DisplayListFiles)
}
```

**Migration Strategy**:
1. Create directory structure
2. Extract types.go (Message, Response, etc.)
3. Extract config.go (env loading, validation)
4. Extract api/client.go (callClaude function)
5. Extract each tool to tools/ (one at a time, test after each)
6. Extract agent.go (handleConversation logic)
7. Extract prompts/system.txt (system prompt)
8. Extract errors.go (custom error types from #12 below)
9. Update main.go to orchestrate (import and wire up)
10. Run full test suite after each step

**Estimated time**: 6-8 hours (careful refactoring with testing)

---

### ✅ 11. 🗄️ External System Prompt - COMPLETED (2026-02-13)
**Status**: ✅ **COMPLETED**

**What Was Built**: Dual-mode system prompt loading

**Features Implemented**:
1. ✅ System prompt in external file (`prompts/system.txt`)
2. ✅ Development mode: loads from file (allows iteration without rebuild)
3. ✅ Production mode: uses embedded version (single binary)
4. ✅ Automatic fallback (embedded when file missing)
5. ✅ `GetSystemPrompt()` for runtime reloading in tests

**Implementation**:
```go
//go:embed system.txt
var embeddedSystemPrompt string

var SystemPrompt = loadSystemPrompt()

func loadSystemPrompt() string {
    // Try file first (dev mode)
    if content, err := os.ReadFile("prompts/system.txt"); err == nil {
        return string(content)
    }
    // Fallback to embedded (production)
    return embeddedSystemPrompt
}
```

**Benefits**:
- Fast iteration: edit prompt and restart (no rebuild)
- Single binary: embedded version for distribution
- Zero breaking changes: existing code unchanged
- Better DX: no compilation wait during prompt work

**Testing**:
- 6 new tests in `tests/prompts_test.go`
- Tests both development and production modes
- Tests custom prompt override capability
- All tests pass

**Documentation**:
- Added "Customizing the System Prompt" section to README.md
- Explains development vs production modes
- Shows workflow for testing and finalizing prompts

**Results**:
- ✅ All tests pass (including 6 new prompt tests)
- ✅ Binary size: 8.1 MB (unchanged)
- ✅ Zero breaking changes
- ✅ Significantly improved development experience

**Time Taken**: ~30 minutes (as estimated!)

---

### ✅ 12. 🧰 Consolidated Tool Execution Framework - COMPLETED (2026-02-13)
**Status**: ✅ **COMPLETED** (as part of Priority #10)

**What Was Implemented**: Function-based tool registry pattern

**Implementation** (from `tools/registry.go`):
```go
// ExecutorFunc is a function that executes a tool
type ExecutorFunc func(input map[string]interface{}, apiClient *api.Client, 
                      conversationHistory []api.Message) (string, error)

// DisplayFunc is a function that formats a display message for a tool
type DisplayFunc func(input map[string]interface{}) string

// Registration holds a tool registration
type Registration struct {
    Tool     api.Tool
    Execute  ExecutorFunc
    Display  DisplayFunc
}

// Register registers a tool with its executor and display functions
func Register(tool api.Tool, execute ExecutorFunc, display DisplayFunc) {
    Registry[tool.Name] = &Registration{
        Tool:    tool,
        Execute: execute,
        Display: display,
    }
}
```

**Tool Pattern** (each tool file):
```go
func init() {
    Register(readFileTool, executeReadFile, displayReadFile)
}

var readFileTool = api.Tool{
    Name: "read_file",
    Description: "...",
    InputSchema: {...},
}

func executeReadFile(input map[string]interface{}, apiClient *api.Client, 
                     conversationHistory []api.Message) (string, error) {
    // Validation inline
    path, ok := input["path"].(string)
    if !ok || path == "" {
        return "", fmt.Errorf("file path is required")
    }
    
    // Execution
    content, err := os.ReadFile(path)
    return string(content), err
}

func displayReadFile(input map[string]interface{}) string {
    path, _ := input["path"].(string)
    return fmt.Sprintf("→ Reading file: %s", path)
}
```

**Agent Uses Registry** (from `agent/agent.go`):
```go
// Get tool registration
reg, err := tools.GetTool(toolBlock.Name)
if err != nil {
    // Handle unknown tool
}

// Display progress message
if reg.Display != nil {
    fmt.Println(reg.Display(toolBlock.Input))
}

// Execute the tool
output, err := reg.Execute(toolBlock.Input, a.apiClient, a.history)
```

**Benefits Achieved**:
- ✅ **DRY**: Zero duplication in agent code
- ✅ **Consistency**: All 10 tools follow same pattern
- ✅ **Testability**: Each tool can be tested in isolation
- ✅ **Extensibility**: Add new tools by creating one file with init()
- ✅ **No boilerplate**: Tools self-register via init()
- ✅ **Type-safe**: Function signatures enforced by types

**Why Function-Based vs Interface**:
The implementation uses function-based registration instead of the interface-based approach proposed in the TODO. This is actually **better** because:

1. **More flexible**: Functions are first-class in Go
2. **Less boilerplate**: No need to create struct types for each tool
3. **Easier to test**: Can pass mock functions directly
4. **More idiomatic Go**: Composition over inheritance
5. **Simpler**: Validation inline with execution (fewer moving parts)

**Results**:
- ✅ All 10 tools use consistent pattern
- ✅ Zero tool-specific code in agent
- ✅ Agent.HandleMessage() is generic (40 lines for all tools)
- ✅ Adding new tools requires zero agent changes
- ✅ All tests pass with new architecture

**Completed**: 2026-02-13 (as part of Priority #10 - Code Organization)

**Time Taken**: Included in Priority #10's 2-hour refactor

---

### ❌ 13. 🚨 Custom Error Types - CANCELLED (2026-02-13)
**Status**: ❌ **CANCELLED** - Overengineering

**Original Purpose**: Create structured error types with suggestions and context

**Why Cancelled**:

Priority #4 (Better Error Handling & Messages) already achieved the goal with excellent, actionable error messages using simple string-based errors. Custom error types would add complexity without meaningful benefit.

**Arguments Against Custom Types**:

1. **Current errors are already excellent**:
   ```go
   fmt.Errorf("file '%s' does not exist. Use list_files to see available files", path)
   fmt.Errorf("permission denied reading '%s'. Check file permissions", path)
   ```
   These are clear, actionable, and include suggestions inline.

2. **No need for programmatic error handling**:
   - Errors go directly to Claude AI (needs text, not structure)
   - No recovery logic that would benefit from structured types
   - Agent just passes errors to Claude for natural language explanation

3. **String errors are more flexible**:
   - Easy context inclusion: `fmt.Errorf("failed to read '%s': %w", path, err)`
   - Natural error wrapping with `%w`
   - No struct creation overhead for every error

4. **Testing is already sufficient**:
   - Standard Go error checking works fine
   - Tests verify error conditions without needing type assertions

5. **More code to maintain**:
   - Would add ~300 lines of error type definitions and constructors
   - More complex returns throughout codebase
   - Need to handle both custom and standard errors

6. **Go's philosophy**:
   - Go favors simple errors with good messages over elaborate hierarchies
   - Current approach is more idiomatic

**When Custom Types WOULD Make Sense**:
- Recovery logic based on error type
- External API returning structured errors to clients
- Error categorization for metrics/logging
- Multi-tier system needing error propagation

**But for this REPL**:
- Errors go to Claude (needs human-readable text)
- No recovery logic (just display and continue)
- Simple architecture (not a distributed system)

**Conclusion**: Priority #4 already solved error handling properly. Adding structured types would be overengineering without tangible benefits.

**Decision Date**: 2026-02-13

---

### ✅ 14. 🔑 Config File for Global Installation - COMPLETED (2026-02-18)
**Status**: ✅ **COMPLETED**

**Purpose**: Support running claude-repl from any directory with proper config management

**What Was Built**:

**Clean Architecture** (separation of concerns):
- **CLI Layer (main.go)**: Decides config file location (always `~/.claude-repl/config`)
- **Config Package**: Simple `LoadFromFile(path)` - agnostic to location
- **Tests**: Use `.env` files in their own temp directories
- **Agent**: Receives configuration programmatically, location-agnostic

**Implementation**:

1. **config/config.go**: Pure loading function
```go
func LoadFromFile(path string) (*Config, error) {
    // Load from specified file
    // Validate required fields
    // Return config or error
}
```

2. **main.go**: CLI layer handles file location
```go
func getConfigPath() string {
    homeDir, _ := os.UserHomeDir()
    return filepath.Join(homeDir, ".claude-repl", "config")
}
```

3. **Tests**: Isolated .env files
```go
tmpDir := t.TempDir()
configPath := filepath.Join(tmpDir, ".env")
config.LoadFromFile(configPath)
```

**Benefits**:
- ✅ Clean separation of concerns (CLI vs config vs agent)
- ✅ Works after global installation with `go install`
- ✅ Simple: production always uses `~/.claude-repl/config`
- ✅ Testable: tests use `.env` in temp directories
- ✅ Agent remains configuration-agnostic
- ✅ No complex fallback logic

**Testing**:
- Created `tests/config_test.go` with 5 focused tests
- Tests loading from file, error cases, defaults
- All tests pass with proper environment isolation

**Results**:
- ✅ All 32 tests pass (5 config tests)
- ✅ Binary size: 9.0 MB (unchanged)
- ✅ Clean, maintainable architecture
- ✅ Production-ready for global installation

**Time Taken**: ~2 hours (1.5 initial + 0.5 refactor)

**Architecture Philosophy**:
The config location decision belongs at the CLI layer, not in the config package. This keeps the agent and config package pure and reusable.

---

### ✅ 15. 📷 Image Input Support (Multimodal) - COMPLETED (2026-02-19)
**Status**: ✅ **COMPLETED**

**Purpose**: Send images to Claude for analysis (vision capabilities)

**What Was Built**:

**Tool Implementation** (`tools/include_file.go`):
- Loads images from local filesystem or remote URLs
- Supports: `.jpg`, `.jpeg`, `.png`, `.gif`, `.webp`
- Returns special `IMAGE_LOADED:<media_type>:<size_kb>:<base64_data>` marker
- Agent recognizes marker and adds image content block to conversation
- 5MB size limit per Claude API requirements

**Agent Integration** (`agent/agent.go`):
- Recognizes IMAGE_LOADED marker from tool execution
- Parses media type and base64 data
- Creates image content block with ImageSource
- Appends image to tool results for Claude to analyze
- Provides confirmation message about image loading

**API Types** (`api/types.go`):
- Added `ImageSource` struct with type, media_type, data fields
- Updated `ContentBlock` to support image type with Source field
- Supports both base64 and URL image sources

**System Prompt** (`prompts/system.txt`):
- Added include_file tool description
- Provides guidance on when and how to use the tool
- Explains workflow: verify file exists first, then include
- Notes that images can be analyzed in the same turn

**Testing** (`tests/include_file_test.go`):
- Unit tests: `TestExecuteIncludeFile` (5 sub-tests)
  - Missing/empty path parameter
  - File not found
  - Unsupported file type
  - Load valid PNG image
  - Validate base64 encoding
- Unit tests: `TestDisplayIncludeFile` (2 sub-tests)
  - Local file path display
  - Remote URL display
- Integration tests: `TestIncludeFileIntegration` (2 sub-tests)
  - Include image and Claude analyzes it (vision works!)
  - Handle non-existent file gracefully
- **All tests pass!** ✅

**Results**:
- ✅ All 36 tests pass (130 total test runs including sub-tests)
- ✅ Binary size: 9.0 MB (unchanged)
- ✅ System prompt: 5.1 KB (+500 bytes for include_file section)
- ✅ Claude successfully analyzes images with vision!
- ✅ Agent intelligently searches for files when needed
- ✅ Comprehensive error handling for edge cases
- ✅ Clean tool-based approach (no CLI query-rewriting)

**Use Cases**:
```bash
You: analyze screenshot.png
→ Including file: screenshot.png
Claude: I can see the screenshot shows a "nil pointer dereference" error...

You: what's in that error screenshot?
→ Searching: 'error' in current directory (*.png)
→ Including file: error_screenshot.png
Claude: Looking at error_screenshot.png, I can see...
```

**Time Taken**: ~4 hours (Part 1 agent library complete, Part 2 REPL needs no changes!)

**Design Decision: Agent Tool Approach (Not CLI Auto-Detection)**

**Why The Query-Rewrite Approach Failed**:
The original spec (Haiku query-rewrite to auto-detect image paths) did not work well in practice:
- Query rewrite routinely missed files or didn't include correct paths
- Added latency and cost for every message with image extensions
- Unreliable extraction from natural language
- Created confusion about what the agent "saw"

**New Approach: Agent Tool for File Inclusion**

Instead of the CLI trying to be smart about detecting images, we give the agent a tool that lets it explicitly include local or remote files in the conversation. The agent decides when to use this tool based on the user's request.

**Benefits**:
- ✅ Agent has explicit control over what files to include
- ✅ Agent can verify file existence before including
- ✅ Agent can search for files using existing tools (list_files, grep, glob)
- ✅ Works for images AND other file types (PDFs, text files, etc.)
- ✅ No guessing or query-rewriting needed
- ✅ Clear user feedback about what was included
- ✅ Agent can handle errors gracefully (file not found, wrong type, etc.)

**Now that the agent is a decoupled library, this feature comprises two parts:**

#### Part 1: Agent Library - File Content Tool (4-5 hours)
**Scope**: Add `include_file` tool that loads file content into the conversation

**File Types to Support**:
1. **Images** (vision): `.jpg`, `.jpeg`, `.png`, `.gif`, `.webp`
   - Send as base64-encoded image content blocks
2. **Documents** (future): `.pdf`, `.txt`, `.md`, `.json`, etc.
   - Could be converted to text or sent as document content blocks
3. **Remote URLs**: Both local paths and public URLs

**Tool Definition**:

```go
{
  "name": "include_file",
  "description": "Include a file's contents in the conversation. For images (jpg, png, gif, webp), this sends the image to Claude for vision analysis. Can load from local filesystem or remote URLs. Use this tool when the user asks you to look at, analyze, or work with a specific file.",
  "input_schema": {
    "type": "object",
    "properties": {
      "path": {
        "type": "string",
        "description": "File path (local or URL). Examples: './screenshot.png', '/tmp/diagram.jpg', 'https://example.com/image.png'"
      }
    },
    "required": ["path"]
  }
}
```

**Tool Behavior**:

1. **Pre-flight checks**:
   - Verify file exists (for local paths) or URL is accessible
   - Check file extension/content type
   - Verify file size is within limits (<5MB for images)

2. **Image handling** (jpg, jpeg, png, gif, webp):
   - Read file or fetch from URL
   - Encode as base64
   - Return special response: `"IMAGE_LOADED: <base64_data>"`
   - Agent recognizes this and adds image content block to NEXT turn

3. **Other file types** (future):
   - Read as text and return contents
   - Could support PDF → text conversion
   - Agent includes in conversation as text

**API Changes Needed**:

```go
// api/types.go - Add image source types
type ImageSource struct {
    Type      string `json:"type"`        // "base64" or "url"
    MediaType string `json:"media_type"`  // "image/jpeg", "image/png", etc.
    Data      string `json:"data,omitempty"`  // Base64 data (for type="base64")
    URL       string `json:"url,omitempty"`   // URL (for type="url")
}

// Update ContentBlock to support images
type ContentBlock struct {
    Type      string       `json:"type"`                // "text", "image", "tool_use", "tool_result"
    Text      string       `json:"text,omitempty"`      // For type="text"
    Source    *ImageSource `json:"source,omitempty"`    // For type="image"
    // ... existing tool_use/tool_result fields
}
```

**Tool Implementation** (tools/include_file.go):

```go
package tools

import (
    "encoding/base64"
    "fmt"
    "net/http"
    "os"
    "path/filepath"
    "strings"
    
    "github.com/this-is-alpha-iota/clyde/api"
)

func init() {
    Register(includeFileTool, executeIncludeFile, displayIncludeFile)
}

var includeFileTool = api.Tool{
    Name: "include_file",
    Description: "Include a file's contents in the conversation...",
    InputSchema: map[string]interface{}{
        "type": "object",
        "properties": map[string]interface{}{
            "path": map[string]interface{}{
                "type":        "string",
                "description": "File path (local or URL)",
            },
        },
        "required": []string{"path"},
    },
}

func executeIncludeFile(input map[string]interface{}, apiClient *api.Client, 
                        history []api.Message) (string, error) {
    path, ok := input["path"].(string)
    if !ok || path == "" {
        return "", fmt.Errorf("path is required. Example: include_file(\"./screenshot.png\")")
    }
    
    // Determine if URL or local path
    isURL := strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://")
    
    // Check file type
    ext := strings.ToLower(filepath.Ext(path))
    isImage := ext == ".jpg" || ext == ".jpeg" || ext == ".png" || 
               ext == ".gif" || ext == ".webp"
    
    if isImage {
        return loadImage(path, isURL)
    }
    
    // For non-images, return error for now (future: support text files)
    return "", fmt.Errorf("only image files are currently supported (.jpg, .png, .gif, .webp)")
}

func loadImage(path string, isURL bool) (string, error) {
    var data []byte
    var err error
    var mediaType string
    
    if isURL {
        // Fetch from URL
        resp, err := http.Get(path)
        if err != nil {
            return "", fmt.Errorf("failed to fetch image from URL: %w", err)
        }
        defer resp.Body.Close()
        
        if resp.StatusCode != 200 {
            return "", fmt.Errorf("URL returned status %d", resp.StatusCode)
        }
        
        mediaType = resp.Header.Get("Content-Type")
        data, err = io.ReadAll(resp.Body)
        if err != nil {
            return "", fmt.Errorf("failed to read image data: %w", err)
        }
    } else {
        // Read local file
        data, err = os.ReadFile(path)
        if err != nil {
            if os.IsNotExist(err) {
                return "", fmt.Errorf("file '%s' not found. Use list_files or glob to find files", path)
            }
            return "", fmt.Errorf("failed to read file: %w", err)
        }
        
        // Detect media type from extension
        ext := strings.ToLower(filepath.Ext(path))
        mediaType = detectMediaType(ext)
    }
    
    // Check size (5MB limit)
    if len(data) > 5*1024*1024 {
        return "", fmt.Errorf("image too large (%.1f MB). Maximum is 5MB", 
            float64(len(data))/(1024*1024))
    }
    
    // Encode to base64
    encoded := base64.StdEncoding.EncodeToString(data)
    
    // Return special marker that agent will recognize
    // Format: IMAGE_LOADED:<media_type>:<base64_data>
    return fmt.Sprintf("IMAGE_LOADED:%s:%s", mediaType, encoded), nil
}

func detectMediaType(ext string) string {
    switch ext {
    case ".jpg", ".jpeg":
        return "image/jpeg"
    case ".png":
        return "image/png"
    case ".webp":
        return "image/webp"
    case ".gif":
        return "image/gif"
    default:
        return ""
    }
}

func displayIncludeFile(input map[string]interface{}) string {
    path, _ := input["path"].(string)
    return fmt.Sprintf("→ Including file: %s", path)
}
```

**Agent Integration** (agent/agent.go):

The agent needs to recognize the special `IMAGE_LOADED:` response and convert it to an image content block in the next message to Claude.

```go
// In handleConversation loop, after executing include_file tool:

if strings.HasPrefix(output, "IMAGE_LOADED:") {
    // Parse: IMAGE_LOADED:<media_type>:<base64_data>
    parts := strings.SplitN(output, ":", 3)
    if len(parts) == 3 {
        mediaType := parts[1]
        imageData := parts[2]
        
        // Store image for next turn
        // Add to a pending images slice or immediately include
        pendingImage := api.ContentBlock{
            Type: "image",
            Source: &api.ImageSource{
                Type:      "base64",
                MediaType: mediaType,
                Data:      imageData,
            },
        }
        
        // Include in the tool result or next message
        // Option A: Include immediately in the response to Claude
        // Option B: Store and include in next user message
    }
    
    // Return confirmation to Claude
    output = fmt.Sprintf("Image loaded successfully (%s, %.1f KB)", 
        mediaType, float64(len(imageData))/1024)
}
```

**System Prompt Addition**:
```
File inclusion - Use include_file for:
- "Look at [file]" or "Analyze [image]"
- "What's in screenshot.png?"
- User mentions a specific image file to examine
- Workflow: 1) Verify file exists with list_files/glob, 2) Use include_file
- After including image, you can analyze/describe it in next response
- Tool handles both local paths and URLs
```

**Error Handling**:
- File not found → Suggest using list_files or glob first
- Invalid format → List supported formats
- File too large → Suggest resizing or different file
- URL not accessible → Check URL and network
- Permission denied → Check file permissions

**Testing**:
- Unit tests: file loading, URL loading, base64 encoding
- Integration tests: include image and analyze with Claude
- Error handling: missing file, wrong format, too large
- Multiple images in one conversation

**Estimated time**: 4-5 hours
- 1 hour: Tool implementation (include_file)
- 1 hour: API types and image content block support
- 1 hour: Agent integration (handle IMAGE_LOADED response)
- 1 hour: Testing (unit + integration)
- 0.5 hours: Error handling and edge cases
- 0.5 hours: System prompt and documentation

---

#### Part 2: REPL - No Changes Needed! ✨

**The beauty of the tool approach**: The REPL doesn't need any changes. The agent decides when to use `include_file` based on the user's natural language request.

**User Experience Examples**:

```bash
# Example 1: User asks to look at an image
You: analyze screenshot.png
→ Listing files: . (current directory)
→ Including file: screenshot.png
Claude: I can see the screenshot shows a "nil pointer dereference" error...

# Example 2: User doesn't remember exact filename
You: look at the error screenshot
→ Searching: 'error' in current directory (*.png)
→ Including file: error_screenshot.png
Claude: Looking at error_screenshot.png, I can see...

# Example 3: User references remote URL
You: what's in https://example.com/diagram.png
→ Including file: https://example.com/diagram.png
Claude: This diagram shows a client-server architecture...

# Example 4: File doesn't exist - agent handles gracefully
You: analyze missing.png
→ Listing files: . (current directory)
Claude: I don't see a file called missing.png in the current directory. 
I can see these image files:
  - screenshot.png
  - error1.png
  - error2.png
Would you like me to analyze one of these?

# Example 5: User says "screenshot" without extension
You: look at the screenshot
→ Searching: 'screenshot' in current directory (*.png, *.jpg)
→ Including file: screenshot.png
Claude: I can see in screenshot.png...
```

**How The Agent Decides**:

The agent uses its existing intelligence to determine when to use `include_file`:

1. **Direct mention**: "analyze file.png" → use include_file
2. **Unclear filename**: "look at the error" → use grep/glob first to find it
3. **Doesn't exist**: Agent sees list_files results, tells user file not found
4. **Wrong type**: Agent sees error from tool, explains to user
5. **Multiple files**: "compare error1.png and error2.png" → use include_file twice

**System Prompt Guidance** (already added in Part 1):
```
File inclusion - Use include_file for:
- "Look at [file]" or "Analyze [image]"
- "What's in screenshot.png?"
- User mentions a specific image file to examine
- Workflow: 1) Verify file exists with list_files/glob, 2) Use include_file
- After including image, you can analyze/describe it in next response
```

**Why This Is Better**:
- ✅ No CLI changes needed (cleaner separation)
- ✅ Agent can search for files before including them
- ✅ Agent can verify existence and handle errors intelligently
- ✅ Agent can explain issues to user in natural language
- ✅ Works naturally with existing tools (list_files, grep, glob)
- ✅ No regex pre-filters or query rewrites
- ✅ No extra API calls (no Haiku overhead)
- ✅ Agent fully in control of the workflow

**Testing**: 
Integration tests showing full workflows:
- User asks for image → agent uses include_file → Claude analyzes
- User asks for missing image → agent searches → tells user not found
- User asks for "the screenshot" → agent finds it → includes it
- User asks for multiple images → agent includes all → Claude analyzes

**Estimated time**: 0 hours (no REPL changes needed!)

---

**Total Estimated Time**: 4-5 hours (just Part 1, Part 2 needs no changes!)

**Use Cases**:
- "Debug this error screenshot" → agent uses include_file
- "What's wrong with screenshot.png?" → agent uses include_file
- "Compare error1.png and error2.png" → agent uses include_file twice
- "Analyze the diagram" → agent searches with glob, then includes
- "What's in https://example.com/chart.png" → agent includes URL

**Dependencies**: Claude API supports images (already available in claude-sonnet-4-5)

**Philosophy**: Let the agent use its intelligence to decide when and how to include files. Don't try to outsmart it from the CLI layer. This is cleaner, more reliable, and requires less code.

---

### ✅ 16. 🔌 Complete Agent Decoupling (UI-Agnostic Agent) - COMPLETED (2026-02-18)
**Status**: ✅ **COMPLETED**

**Purpose**: Make the agent 100% UI-agnostic by removing the single remaining UI coupling

**The Problem**:
After Priority #10 (Code Organization), the agent was 90% UI-agnostic. However, there was still one direct coupling in `agent/agent.go` line ~82:

```go
if reg.Display != nil {
    displayMsg := reg.Display(toolBlock.Input)
    if displayMsg != "" {
        fmt.Println(displayMsg)  // ← ONLY UI COUPLING
    }
}
```

This prevented the agent from being used in:
- HTTP APIs (need to send progress via HTTP/WebSocket)
- GUIs (need to update UI widgets)
- Bots (need to send to Telegram/Discord/Slack)
- Embedded contexts (library usage)

**Solution**: Implemented callback-based architecture using Go's options pattern

**Implementation**:

1. **Added callback types to agent.go**:
```go
// ProgressCallback receives progress messages during tool execution
type ProgressCallback func(message string)

// ErrorCallback receives errors during processing (optional, for logging)
type ErrorCallback func(err error)

type Agent struct {
    apiClient        *api.Client
    systemPrompt     string
    history          []api.Message
    progressCallback ProgressCallback  // NEW
    errorCallback    ErrorCallback     // NEW (optional)
}
```

2. **Added options pattern for flexible configuration**:
```go
type AgentOption func(*Agent)

func WithProgressCallback(cb ProgressCallback) AgentOption {
    return func(a *Agent) { a.progressCallback = cb }
}

func WithErrorCallback(cb ErrorCallback) AgentOption {
    return func(a *Agent) { a.errorCallback = cb }
}

func NewAgent(apiClient *api.Client, systemPrompt string, opts ...AgentOption) *Agent {
    agent := &Agent{
        apiClient:    apiClient,
        systemPrompt: systemPrompt,
        history:      []api.Message{},
    }
    
    for _, opt := range opts {
        opt(agent)
    }
    
    return agent
}
```

3. **Updated progress display to use callback**:
```go
// Replaced direct fmt.Println with callback invocation
if displayMsg != "" && a.progressCallback != nil {
    a.progressCallback(displayMsg)
}
```

4. **Updated main.go to use callback**:
```go
agentInstance := agent.NewAgent(
    apiClient,
    prompts.SystemPrompt,
    agent.WithProgressCallback(func(msg string) {
        fmt.Println(msg) // REPL prints to stdout
    }),
)
```

**Benefits Achieved**:

1. **100% UI-Agnostic Agent**:
   - Zero UI dependencies in agent package
   - No direct coupling to any output mechanism
   - Agent doesn't know or care how progress is displayed

2. **Backward Compatible**:
   - Existing tests work without modification
   - No breaking changes to API
   - Optional callbacks (works without them)

3. **Enables Any Frontend**:
   - CLI: Print to stdout (current implementation)
   - HTTP API: Send via WebSocket or collect in buffer
   - GUI: Update status bar or progress widgets
   - Bot: Send to Discord/Telegram/Slack
   - Library: Capture for logging or metrics

4. **Idiomatic Go**:
   - Options pattern is standard in Go
   - Functional options allow flexibility
   - Clean, composable API

**Example Use Cases**:

**CLI (current)**:
```go
agent.NewAgent(apiClient, systemPrompt,
    agent.WithProgressCallback(func(msg string) {
        fmt.Println(msg)
    }),
)
```

**HTTP API**:
```go
var progressBuffer []string
agent.NewAgent(apiClient, systemPrompt,
    agent.WithProgressCallback(func(msg string) {
        progressBuffer = append(progressBuffer, msg)
        websocket.Send(msg) // Real-time updates
    }),
)
```

**GUI**:
```go
agent.NewAgent(apiClient, systemPrompt,
    agent.WithProgressCallback(func(msg string) {
        statusBar.SetText(msg)
        progressList.AddItem(msg)
    }),
)
```

**Testing**:
- All 32 tests pass without modification
- Zero breaking changes to existing tests
- Test helpers continue to work as-is

**Results**:
- ✅ All 32 tests pass
- ✅ Binary size: 9.0 MB (unchanged)
- ✅ Agent is now 100% UI-agnostic
- ✅ Zero breaking changes
- ✅ Ready for any frontend implementation
- ✅ Clean, idiomatic Go code

**Code Changes**:
- `agent/agent.go`: Added callbacks and options pattern (+884 bytes)
- `main.go`: Updated to use callback (+138 bytes)
- `README.md`: Added "Using Clyde as a Library" section (+3.2 KB)
- Total: ~4.2 KB added

**Time Taken**: ~30 minutes (faster than estimated 1 hour)

**Documentation Updates**:
- Added "Using Clyde as a Library" section to README.md
- Shows 7 different callback usage examples
- Explains options pattern and flexibility
- Documents all use cases (CLI, API, GUI, logging, silent)

**Philosophy**:
The agent is now a pure library component that knows nothing about how it's being used. It provides data (responses and progress) via clean callback interfaces, and the caller decides what to do with that data. This is the Unix philosophy applied to Go: do one thing well, compose with others.

**Lesson Learned**:
Complete decoupling requires removing ALL dependencies on specific output mechanisms. A single `fmt.Println` was enough to prevent the agent from being truly reusable. The callback pattern elegantly solves this while maintaining backward compatibility.

---

### ✅ 17. 💾 Automatic Prompt Caching - COMPLETED (2026-02-19)
**Status**: ✅ **COMPLETED**

**Purpose**: Reduce API costs and latency by caching reusable prompt content

**What Is Prompt Caching?**:
Prompt caching allows Claude API to cache and reuse parts of prompts across multiple requests, significantly reducing:
- Processing time for repetitive prompts
- Costs for cached content (10x cheaper: 90% discount on cached tokens)
- Latency for multi-turn conversations

**Key Information from Screenshot**:
- Stores KV cache representations and cryptographic hashes (not raw text)
- ZDR-type data retention compliance
- Two approaches: **Automatic** (recommended) and **Explicit**

**Automatic Caching (Recommended Approach)**:
```json
{
  "model": "claude-sonnet-4",
  "max_tokens": 1024,
  "cache_control": {"type": "ephemeral"},  // ← Single top-level field
  "system": "Your system prompt...",
  "messages": [...]
}
```

**How It Works**:
1. Add single `cache_control` field at top level of request
2. System automatically applies cache breakpoint to last cacheable block
3. Moves cache breakpoint forward as conversations grow
4. Best for multi-turn conversations where growing history should be cached

**What Gets Cached** (in order):
1. Tools (tool definitions array)
2. System prompt
3. Messages (conversation history)

**Cache Behavior**:
- Cache hit: ~90% cost reduction on cached tokens
- Cache lifetime: 5 minutes (default)
- Minimum cacheable size: 1024 tokens (smaller content not cached)
- Cache invalidation: Any change to cached content breaks cache

**Benefits for claude-repl**:
1. **System prompt caching**: Our 5.1 KB system prompt is sent with every request
   - Current cost: Full processing every turn
   - With caching: 90% cheaper after first turn
   - Savings: ~4-5 KB cached per request

2. **Tool definitions caching**: All 10 tool definitions (~3-4 KB) sent every turn
   - Current cost: Full processing every turn  
   - With caching: 90% cheaper, processed once per session
   - Savings: ~3-4 KB cached per request

3. **Conversation history**: Grows with each turn (user + assistant messages)
   - Current cost: Full reprocessing of entire history every turn
   - With caching: Only new messages processed, history cached
   - Savings: Increases linearly with conversation length

**Example Savings** (10-turn conversation):
```
Without caching:
  Turn 1:  10 KB system+tools + 1 KB messages = 11 KB
  Turn 2:  10 KB system+tools + 3 KB messages = 13 KB
  Turn 10: 10 KB system+tools + 25 KB messages = 35 KB
  Total: ~190 KB processed

With automatic caching:
  Turn 1:  11 KB processed (10 KB cached)
  Turn 2:  1 KB system+tools + 2 KB new messages = 3 KB processed (11 KB cached)
  Turn 10: 1 KB system+tools + 2 KB new messages = 3 KB processed (33 KB cached)
  Total: ~41 KB processed (80% reduction!)
```

**Implementation Design**:

**Always-On Automatic Caching** (Simple & Effective)
```go
// api/types.go - Add cache control type
type CacheControl struct {
    Type string `json:"type"` // "ephemeral"
}

// api/types.go - Add to Request struct
type Request struct {
    Model        string           `json:"model"`
    MaxTokens    int              `json:"max_tokens"`
    CacheControl *CacheControl    `json:"cache_control,omitempty"` // NEW
    System       string           `json:"system"`
    Messages     []Message        `json:"messages"`
    Tools        []Tool           `json:"tools,omitempty"`
}

// api/client.go - Enable in every request
reqBody := Request{
    Model:        cfg.Model,
    MaxTokens:    cfg.MaxTokens,
    CacheControl: &CacheControl{Type: "ephemeral"}, // ← Always enabled
    System:       systemPrompt,
    Messages:     messages,
    Tools:        tools,
}
```

**Benefits**:
- ✅ Zero configuration needed
- ✅ Immediate cost savings for all users
- ✅ No breaking changes
- ✅ Optimal for multi-turn conversations (our primary use case)
- ✅ Cache automatically moves forward with conversation
- ✅ No performance penalty (cache miss = normal behavior)

**When Caching Helps Most**:
1. ✅ Multi-turn conversations (our primary use case)
2. ✅ Large system prompts (we have 5.1 KB)
3. ✅ Many tool definitions (we have 10 tools)
4. ✅ Conversations > 1024 tokens (most of ours)
5. ✅ Rapid back-and-forth (within 5-min cache lifetime)

**When Caching Helps Less**:
1. ❌ Single-turn requests (no reuse)
2. ❌ Tiny prompts < 1024 tokens (below minimum)
3. ❌ Infrequent requests (cache expires after 5 min)
4. ❌ Highly variable prompts (cache always breaks)

**For claude-repl**: Caching is a **perfect fit**! Multi-turn conversations with stable system prompt and tools.

**Response Handling**:
Claude API returns cache usage metadata in response:
```json
{
  "usage": {
    "input_tokens": 1500,
    "cache_creation_input_tokens": 1200,  // First time: tokens cached
    "cache_read_input_tokens": 1200,      // Subsequent: tokens from cache
    "output_tokens": 300
  }
}
```

**Display Cache Hits** (for transparency):
```go
// api/types.go - Update Usage struct
type Usage struct {
    InputTokens              int `json:"input_tokens"`
    OutputTokens             int `json:"output_tokens"`
    CacheCreationInputTokens int `json:"cache_creation_input_tokens"` // NEW
    CacheReadInputTokens     int `json:"cache_read_input_tokens"`     // NEW
}

// agent/agent.go - Show cache hits in progress
if response.Usage.CacheReadInputTokens > 0 {
    if a.progressCallback != nil {
        pct := float64(response.Usage.CacheReadInputTokens) / 
               float64(response.Usage.InputTokens) * 100
        a.progressCallback(fmt.Sprintf("💾 Cache hit: %d tokens (%.0f%%)", 
            response.Usage.CacheReadInputTokens, pct))
    }
}
```

**Testing Strategy**:
1. Unit tests: Verify CacheControl field is set correctly
2. Integration tests: Make multiple requests, verify caching works
3. Manual testing: Check API response usage fields
4. Cost analysis: Compare costs before/after over 10+ turn conversation

**Documentation Updates**:
- README.md: Add "Prompt Caching" section explaining benefits
- Show example savings calculation
- Mention cache lifetime and minimum size
- Link to Anthropic docs for details

**Implementation Tasks**:
1. Add CacheControl type to api/types.go (5 mins)
2. Update Request struct to include cache_control field (5 mins)
3. Update Usage struct with cache fields (5 mins)
4. Set CacheControl in api/client.go (5 mins)
5. Add cache hit display in agent.go (15 mins)
6. Write tests (30 mins)
7. Update documentation (20 mins)

**Estimated time**: 1.5 hours

**Priority**: HIGH - Easy win for immediate cost savings with zero downside

**Expected Impact**:
- 50-80% reduction in API costs for typical conversations
- Faster response times (cached tokens processed ~10x faster)
- Zero UX changes (completely transparent to users)
- Especially impactful for power users with long sessions

**Why Automatic (Not Explicit Breakpoints)**:
Automatic caching is perfect for claude-repl because:
- System prompt and tools are stable and should always be cached
- Conversation history grows predictably and should be cached
- No need for fine-grained control over what gets cached
- Simpler implementation (no per-content-block cache_control fields)
- Claude automatically optimizes cache placement as conversation grows

---

### 18. 🌐 HTTP REST API Interface
**Status**: ⏳ **NOT STARTED**
**Depends on**: Priority #16 (Complete Agent Decoupling)

**Purpose**: Provide HTTP REST API for accessing claude-repl agent

**Why This Matters**:
- Access agent from web apps, mobile apps, other services
- Session management (multiple concurrent users)
- Stateless operation with session IDs
- Deploy as service (Docker, Kubernetes, cloud)

**API Design**:

**Endpoints**:
```
POST /api/v1/sessions          # Create new session
POST /api/v1/sessions/:id/messages    # Send message
GET  /api/v1/sessions/:id/history     # Get conversation history
DELETE /api/v1/sessions/:id   # Delete session
GET  /api/v1/health           # Health check
```

**Example Request/Response**:
```bash
# Create session
curl -X POST http://localhost:8080/api/v1/sessions \
  -H "Authorization: Bearer YOUR_API_KEY"

# Response:
{
  "session_id": "sess_abc123",
  "created_at": "2026-02-13T10:00:00Z"
}

# Send message
curl -X POST http://localhost:8080/api/v1/sessions/sess_abc123/messages \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "message": "What files are in the current directory?"
  }'

# Response:
{
  "response": "Here are the files...",
  "progress_messages": [
    "→ Listing files: . (current directory)"
  ]
}
```

**Implementation Structure**:
```
claude-repl/
├── cmd/
│   ├── repl/main.go       # Current CLI REPL
│   └── api/main.go        # NEW: HTTP API server
├── internal/
│   └── server/            # NEW: HTTP server package
│       ├── server.go      # Server setup
│       ├── handlers.go    # HTTP handlers
│       ├── sessions.go    # Session management
│       └── auth.go        # API key authentication
├── agent/                 # Shared agent (decoupled)
├── api/                   # Shared API client
└── tools/                 # Shared tools
```

**Server Implementation**:
```go
// cmd/api/main.go
package main

import (
    "log"
    "net/http"
    
    "claude-repl/config"
    "claude-repl/internal/server"
)

func main() {
    cfg, err := config.Load()
    if err != nil {
        log.Fatal(err)
    }
    
    srv := server.New(cfg)
    
    log.Printf("Starting HTTP API server on :8080")
    if err := http.ListenAndServe(":8080", srv.Handler()); err != nil {
        log.Fatal(err)
    }
}
```

**Session Management**:
```go
// internal/server/sessions.go
package server

import (
    "sync"
    "time"
    
    "claude-repl/agent"
)

type Session struct {
    ID        string
    Agent     *agent.Agent
    CreatedAt time.Time
    LastUsed  time.Time
}

type SessionManager struct {
    sessions map[string]*Session
    mu       sync.RWMutex
}

func (sm *SessionManager) Create(agentInstance *agent.Agent) *Session {
    // Create new session with unique ID
}

func (sm *SessionManager) Get(id string) (*Session, error) {
    // Retrieve session by ID
}

func (sm *SessionManager) Delete(id string) {
    // Remove session
}

func (sm *SessionManager) Cleanup() {
    // Remove old sessions (run periodically)
}
```

**Progress Handling**:
```go
// Capture progress messages during request
type progressCapture struct {
    messages []string
    mu       sync.Mutex
}

func (p *progressCapture) callback(msg string) {
    p.mu.Lock()
    defer p.mu.Unlock()
    p.messages = append(p.messages, msg)
}

// In handler:
progress := &progressCapture{}
session.Agent = agent.NewAgent(
    apiClient,
    prompt,
    agent.WithProgressCallback(progress.callback),
)
```

**Features**:
- ✅ Session-based conversation (multiple users)
- ✅ Progress message streaming
- ✅ API key authentication
- ✅ Rate limiting (optional)
- ✅ CORS support for web clients
- ✅ Health checks
- ✅ Graceful shutdown

**Framework Choice**:
- **Option 1**: Standard library `net/http` (lightweight, zero dependencies)
- **Option 2**: Echo framework (more features, middleware)
- **Option 3**: Gin framework (fast, popular)

**Recommendation**: Start with `net/http` for consistency with project philosophy (minimal dependencies).

**Testing**:
- Unit tests for handlers
- Integration tests with test client
- Session management tests
- Authentication tests

**Documentation**:
- OpenAPI/Swagger spec
- README section on API usage
- Example clients (curl, JavaScript, Python)

**Deployment**:
```dockerfile
# Dockerfile
FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o api-server cmd/api/main.go

FROM alpine:latest
COPY --from=builder /app/api-server /usr/local/bin/
EXPOSE 8080
CMD ["api-server"]
```

**Implementation Tasks**:
1. Create `internal/server` package structure (30 mins)
2. Implement session management (1 hour)
3. Implement HTTP handlers (1 hour)
4. Add authentication (30 mins)
5. Add progress message capture (30 mins)
6. Write tests (1 hour)
7. Create Dockerfile (15 mins)
8. Write API documentation (30 mins)

**Estimated time**: 5-6 hours

**Priority**: Medium - Valuable but not critical for basic usage

**Use Cases**:
- Web frontend for claude-repl
- Mobile app backend
- CI/CD integration (API calls from scripts)
- Multi-user deployment
- Cloud service offering

---

### 19. 🚀 CLI Mode (Non-Interactive Execution)
**Status**: ⏳ **NOT STARTED**

**Purpose**: Execute agent on a prompt without opening the REPL, similar to Claude Code's `-p` flag

**Behavior**:
When a prompt is provided via CLI arguments, clyde should:
1. Execute the agent on that prompt
2. Keep running until the agent completes the task
3. Exit when done (no REPL)
4. Print progress and final response to stdout
5. Exit with code 0 on success, 1 on error

**Use Cases**:
```bash
# Execute a simple task
clyde "What files are in the current directory?"

# Read prompt from file
clyde -f prompt.txt
cat prompt.txt | clyde

# Use in scripts/automation
clyde "Run all tests and create a summary report" > results.txt

# CI/CD integration
clyde "Review the latest commit and summarize changes"

# Quick one-off queries
clyde "What's the latest version of Go installed?"

# File operations
clyde "Create a new file called README.md with project documentation"

# Code generation
clyde "Generate a unit test for the Calculate function in math.go"
```

**CLI Interface Design**:

**Chosen Approach: Positional with `-f` flag for files**
```bash
clyde "your prompt here"     # Direct string argument
clyde -f prompt.txt          # Read from file
cat prompt.txt | clyde       # Read from stdin
```

**Why This Design**:
- Simple and intuitive for the common case (direct string)
- `-f` flag explicit for file input (prevents ambiguity)
- Stdin support for Unix composition
- Consistent with common CLI tool patterns

**Implementation Strategy**:

**1. Update main.go to detect CLI mode**:
```go
func main() {
    // Parse command line arguments
    args := os.Args[1:]
    
    // Determine mode: REPL or CLI
    if len(args) > 0 {
        runCLIMode(args)
    } else {
        runREPLMode()
    }
}

func runCLIMode(args []string) {
    // Determine prompt source
    var prompt string
    var err error
    
    if args[0] == "-f" {
        // Read from file
        if len(args) < 2 {
            fmt.Fprintln(os.Stderr, "Error: -f requires a file path")
            fmt.Fprintln(os.Stderr, "Usage: clyde -f prompt.txt")
            os.Exit(1)
        }
        prompt, err = readPromptFromFile(args[1])
        if err != nil {
            fmt.Fprintf(os.Stderr, "Error reading prompt file: %v\n", err)
            os.Exit(1)
        }
    } else {
        // Check if stdin has input (pipe/redirect)
        stat, _ := os.Stdin.Stat()
        if (stat.Mode() & os.ModeCharDevice) == 0 {
            // stdin is piped/redirected
            prompt, err = readPromptFromStdin()
            if err != nil {
                fmt.Fprintf(os.Stderr, "Error reading from stdin: %v\n", err)
                os.Exit(1)
            }
        } else {
            // Treat all args as the prompt string
            prompt = strings.Join(args, " ")
        }
    }
    
    if strings.TrimSpace(prompt) == "" {
        fmt.Fprintln(os.Stderr, "Error: Empty prompt provided")
        os.Exit(1)
    }
    
    // Load config and create agent
    cfg, err := loadConfig()
    if err != nil {
        fmt.Fprintf(os.Stderr, "Configuration error: %v\n", err)
        os.Exit(1)
    }
    
    apiClient := api.NewClient(cfg.APIKey, cfg.APIURL, cfg.ModelID, cfg.MaxTokens)
    agentInstance := agent.NewAgent(
        apiClient,
        prompts.SystemPrompt,
        agent.WithProgressCallback(func(msg string) {
            fmt.Fprintln(os.Stderr, msg) // Print progress to stderr
        }),
    )
    
    // Execute prompt
    response, err := agentInstance.HandleMessage(prompt)
    if err != nil {
        fmt.Fprintf(os.Stderr, "Error: %v\n", err)
        os.Exit(1)
    }
    
    // Print response to stdout
    fmt.Println(response)
    os.Exit(0)
}

func runREPLMode() {
    // Current REPL implementation
    // ... existing code ...
}

func readPromptFromFile(path string) (string, error) {
    content, err := os.ReadFile(path)
    if err != nil {
        return "", fmt.Errorf("failed to read file '%s': %w", path, err)
    }
    return string(content), nil
}

func readPromptFromStdin() (string, error) {
    // Check if stdin is a pipe/redirect
    stat, err := os.Stdin.Stat()
    if err != nil {
        return "", err
    }
    
    if (stat.Mode() & os.ModeCharDevice) != 0 {
        return "", fmt.Errorf("no input provided on stdin")
    }
    
    content, err := io.ReadAll(os.Stdin)
    if err != nil {
        return "", err
    }
    return string(content), nil
}
```

**2. Extract REPL code into separate function**:
```go
func runREPLMode() {
    // Load config
    configPath := getConfigPath()
    cfg, err := config.LoadFromFile(configPath)
    if err != nil {
        fmt.Println(err)
        os.Exit(1)
    }
    
    // Create API client and agent
    apiClient := api.NewClient(cfg.APIKey, cfg.APIURL, cfg.ModelID, cfg.MaxTokens)
    agentInstance := agent.NewAgent(
        apiClient,
        prompts.SystemPrompt,
        agent.WithProgressCallback(func(msg string) {
            fmt.Println(msg)
        }),
    )
    
    // Run REPL loop
    fmt.Println("Clyde - AI Coding Agent - Type 'exit' or 'quit' to exit")
    fmt.Println("==========================================================")
    
    reader := bufio.NewReader(os.Stdin)
    for {
        fmt.Print("\nYou: ")
        input, err := reader.ReadString('\n')
        if err != nil {
            if err == io.EOF {
                fmt.Println("\nGoodbye!")
                break
            }
            fmt.Printf("Error reading input: %v\n", err)
            continue
        }
        
        input = strings.TrimSpace(input)
        if input == "" {
            continue
        }
        
        if input == "exit" || input == "quit" {
            fmt.Println("Goodbye!")
            break
        }
        
        response, _ := agentInstance.HandleMessage(input)
        fmt.Printf("\nClaude: %s\n", response)
    }
}
```

**Output Handling**:
- **stdout**: Final agent response (for piping/redirection)
- **stderr**: Progress messages (doesn't interfere with output capture)

This allows:
```bash
# Capture response only
clyde "list files" > output.txt

# See progress but capture response
clyde "complex task" > output.txt
# Progress messages still visible on terminal (stderr)

# Capture everything
clyde "task" > output.txt 2>&1

# Silence progress, capture response
clyde "task" 2>/dev/null > output.txt
```

**Error Handling**:
- Exit code 0: Success
- Exit code 1: Error (config error, API error, etc.)
- Print errors to stderr
- Clear error messages for common issues

**Examples**:

**Simple query**:
```bash
$ clyde "What's in the current directory?"
→ Listing files: . (current directory)

Here are the files in the current directory:

total 96
drwxr-xr-x  15 user  staff   480 Feb 19 10:00 .
drwxr-xr-x   8 user  staff   256 Feb 18 15:30 ..
...
```

**From file**:
```bash
$ cat prompt.txt
Review the code in main.go and suggest improvements.
Focus on error handling and readability.

$ clyde -f prompt.txt
→ Reading file: main.go
... (agent analyzes and provides suggestions)
```

**From stdin**:
```bash
$ echo "What version of Go is installed?" | clyde
→ Running bash: go version
The installed Go version is: go1.24.0 darwin/arm64
```

**In scripts**:
```bash
#!/bin/bash
# deploy.sh

echo "Running tests..."
clyde "Run all tests and create a summary" > test-summary.txt

if [ $? -eq 0 ]; then
    echo "Tests passed! Deploying..."
    # deployment steps...
else
    echo "Tests failed. See test-summary.txt"
    exit 1
fi
```

**Benefits**:
- ✅ Automation-friendly (scripts, CI/CD)
- ✅ Quick one-off tasks without REPL
- ✅ Composable with Unix tools (pipes, redirection)
- ✅ Consistent with Claude Code UX
- ✅ Zero breaking changes (REPL still default)

**Testing Strategy**:
1. Unit tests for argument parsing
2. Unit tests for prompt reading (file, stdin, args)
3. Integration tests for CLI mode execution
4. Test error cases (missing file, empty prompt, API errors)
5. Test output redirection scenarios
6. Test exit codes

**Documentation Updates**:
- README.md: Add "CLI Mode" section with examples
- Show automation use cases
- Explain stdout/stderr separation
- Document exit codes

**Implementation Tasks**:
1. Add argument parsing logic (30 mins)
2. Implement prompt reading (file, stdin, args) (30 mins)
3. Split main() into runCLIMode() and runREPLMode() (30 mins)
4. Update output handling (progress to stderr) (15 mins)
5. Add error handling and exit codes (30 mins)
6. Write tests (1 hour)
7. Update documentation (30 mins)

**Estimated time**: 3-4 hours

**Priority**: HIGH - Frequently requested, enables automation workflows

**Comparison with Claude Code**:
Claude Code has `-p` flag for non-interactive mode:
```bash
claude -p "your prompt here"
```

Our approach:
- Simpler: no flag needed for direct string
- `-f` for file input (clear and explicit)
- Stdin support via pipe detection (automatic)
- Same core behavior: execute and exit

**Philosophy**:
CLI mode makes clyde a true Unix citizen. It can be piped, redirected, scripted, and automated. The REPL is great for exploration, but automation needs direct execution.

**Future Enhancements** (not in initial scope):
- `--max-turns` flag to limit conversation length
- `--output` flag for structured output (JSON)
- `--quiet` flag to suppress progress messages
- `--timeout` flag for time limits
- `--continue` flag to resume previous session
