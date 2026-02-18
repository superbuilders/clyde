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

**Config Location Strategy** (priority order):
1. **ENV_PATH environment variable** (highest priority override)
2. **`.env` in current directory** (for local development/testing)
3. **`~/.claude-repl/config`** (primary global config location)
4. **`~/.claude-repl`** (legacy fallback - direct file without subdirectory)

**Implementation**:
- Added `findConfigFile()` helper function in `config/config.go`
- Checks all locations in priority order
- Returns first found config file
- Provides helpful error message if no config found

**Error Handling**:
When no config is found, shows:
- Exact commands to create config file
- Links to get API keys (Anthropic, Brave Search)
- Alternative options (project-specific .env)

**Testing**:
- Created `tests/config_test.go` with 9 comprehensive tests
- Tests all config locations (ENV_PATH, .env, ~/.claude-repl/config, ~/.claude-repl)
- Tests priority order (local .env > home config)
- Tests error cases (missing config, missing API key, invalid ENV_PATH)
- Tests default values
- All tests pass with proper environment isolation

**Benefits**:
- ✅ Works after global installation with `go install`
- ✅ No need to copy config files around
- ✅ Run from any directory
- ✅ Project-specific config overrides still work
- ✅ Backward compatible with existing .env files
- ✅ Clear, actionable error messages

**Documentation Updates**:
- `README.md`: Added "Installation" and "Configuration" sections
- Explains all config locations with examples
- Shows setup for global installation
- Documents priority order and use cases

**Results**:
- ✅ All 27 tests pass (9 new config tests)
- ✅ Binary size: 9.0 MB (unchanged)
- ✅ Zero breaking changes
- ✅ Production-ready for global installation
- ✅ Professional UX for CLI tool

**Time Taken**: ~1.5 hours (under estimated 2-3 hours)

**Code Changes**:
- `config/config.go`: Enhanced with multi-location search (+1.6 KB)
- `tests/config_test.go`: New test file (+9.3 KB)
- `README.md`: Installation and config documentation (+1.4 KB)
- `PROGRESS.md`: Feature documentation (+7.4 KB)

**Example Usage**:
```bash
# Install globally
go install github.com/yourusername/claude-repl@latest

# Create config once
mkdir -p ~/.claude-repl
cat > ~/.claude-repl/config << 'EOF'
TS_AGENT_API_KEY=your-key-here
BRAVE_SEARCH_API_KEY=your-brave-key  # Optional
EOF

# Use from anywhere!
cd ~/any/directory
claude-repl  # Just works!
```

---

### 15. 📎 File Input Support (Future)
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

### 16. 🔌 Complete Agent Decoupling (UI-Agnostic Agent)
**Status**: ⏳ **NOT STARTED**

**Purpose**: Make the agent 100% UI-agnostic by removing the single remaining UI coupling

**Current State**:
- ✅ Agent logic is already UI-agnostic (90% done thanks to Priority #10)
- ✅ Tools system is completely decoupled
- ✅ API client is independent
- ✅ Main.go REPL is isolated (only 60 lines)
- ⚠️ **One remaining coupling**: Tool progress messages use `fmt.Println` in agent.go

**The Problem**:
In `agent/agent.go` line ~82:
```go
if reg.Display != nil {
    displayMsg := reg.Display(toolBlock.Input)
    if displayMsg != "" {
        fmt.Println(displayMsg)  // ← ONLY UI COUPLING
    }
}
```

This prevents the agent from being used in:
- HTTP APIs (need to send progress via HTTP/WebSocket)
- GUIs (need to update UI widgets)
- Bots (need to send to Telegram/Discord/Slack)
- Embedded contexts (library usage)

**Proposed Solution**: Add callback interface using Go's options pattern

**Implementation**:

1. **Add callback types to agent.go**:
```go
// ProgressCallback receives progress messages during tool execution
type ProgressCallback func(message string)

// ErrorCallback receives errors during processing (optional, for logging)
type ErrorCallback func(err error)

type Agent struct {
    apiClient        *api.Client
    systemPrompt     string
    history          []api.Message
    progressCallback ProgressCallback  // ← NEW
    errorCallback    ErrorCallback     // ← NEW (optional)
}
```

2. **Add options pattern for flexibility**:
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

3. **Update progress display in HandleMessage()**:
```go
// Replace:
if displayMsg != "" {
    fmt.Println(displayMsg)
}

// With:
if displayMsg != "" && a.progressCallback != nil {
    a.progressCallback(displayMsg)
}
```

4. **Update main.go to use callback**:
```go
agentInstance := agent.NewAgent(
    apiClient, 
    prompts.SystemPrompt,
    agent.WithProgressCallback(func(msg string) {
        fmt.Println(msg)  // REPL prints to stdout
    }),
)
```

**Benefits**:
- ✅ Agent becomes 100% UI-agnostic
- ✅ Zero breaking changes (backward compatible with tests)
- ✅ Enables any frontend: CLI, API, GUI, bot, library
- ✅ Better testability (can capture progress messages in tests)
- ✅ Idiomatic Go (options pattern is standard)

**Testing Updates**:
```go
// In tests, can capture progress messages:
var progressMessages []string
agent := agent.NewAgent(
    apiClient,
    systemPrompt,
    agent.WithProgressCallback(func(msg string) {
        progressMessages = append(progressMessages, msg)
    }),
)
```

**Implementation Tasks**:
1. Add callback types and options pattern to `agent/agent.go` (10 mins)
2. Update progress display to use callback (5 mins)
3. Update `main.go` to use callback (5 mins)
4. Update tests to work with new pattern (10 mins)
5. Add example in README showing callback usage (5 mins)
6. Update progress.md with decoupling completion (5 mins)

**Estimated time**: 1 hour

**Priority**: Medium-High - Unlocks all future interface implementations

---

### 17. 🌐 HTTP REST API Interface
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
