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

### 10. 📂 Code Organization & Architecture Separation
**Purpose**: Split single-file architecture into multiple files and packages

**Current Issue**: main.go is ~1,200 lines with everything mixed together

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

### 11. 🗄️ External System Prompt
**Purpose**: Move system prompt out of code and into external file

**Current Issue**: 200+ line systemPrompt constant is hardcoded in main.go

**Proposed Solution**:
```
claude-repl/
├── prompts/
│   └── system.txt         # System prompt as plain text file
└── main.go                # Loads prompt at startup
```

**Implementation**:
```go
// Load at startup (in main or init)
systemPrompt, err := os.ReadFile("prompts/system.txt")
if err != nil {
    // Fallback to embedded default
    systemPrompt = defaultSystemPrompt
}
```

**Benefits**:
- **Iteration speed**: Edit prompt without recompiling
- **Version control**: Clear diffs in git for prompt changes
- **Experimentation**: Easy to test prompt variations
- **Separation**: Code vs. instructions clearly separated
- **Readability**: main.go becomes cleaner

**Fallback Strategy**: Embed default prompt using `//go:embed` for single-binary distribution:
```go
//go:embed prompts/system.txt
var defaultSystemPrompt string
```

**This pairs perfectly with #10**: When reorganizing code, move prompt to `prompts/system.txt`

**Estimated time**: 30 minutes (simple extraction)

---

### 12. 🧰 Consolidated Tool Execution Framework
**Purpose**: Eliminate duplication in tool execution pattern

**Current Issue**: Each tool in handleConversation has duplicate code:
- Parameter validation
- Display message formatting
- Error handling boilerplate
- Result formatting

**Proposed Solution**:
```go
type ToolExecutor interface {
    Validate(params map[string]interface{}) error
    Execute(params map[string]interface{}) (string, error)
    DisplayMessage(params map[string]interface{}) string
}

// Generic execution in handleConversation
for _, toolBlock := range toolUseBlocks {
    executor := getExecutor(toolBlock.Name)
    
    if err := executor.Validate(toolBlock.Input); err != nil {
        // Handle validation error
        continue
    }
    
    fmt.Println(executor.DisplayMessage(toolBlock.Input))
    output, err := executor.Execute(toolBlock.Input)
    // Handle result...
}
```

**Example Tool Implementation**:
```go
type ReadFileTool struct{}

func (t *ReadFileTool) Validate(params map[string]interface{}) error {
    path, ok := params["path"].(string)
    if !ok || path == "" {
        return fmt.Errorf("read_file requires 'path' parameter")
    }
    return nil
}

func (t *ReadFileTool) Execute(params map[string]interface{}) (string, error) {
    path := params["path"].(string)
    return executeReadFile(path)
}

func (t *ReadFileTool) DisplayMessage(params map[string]interface{}) string {
    path := params["path"].(string)
    return fmt.Sprintf("→ Reading file: %s", path)
}
```

**Benefits**:
- **DRY**: Eliminate ~200 lines of duplicate validation/formatting
- **Consistency**: All tools follow same pattern
- **Testability**: Easy to test tool behavior in isolation
- **Extensibility**: Add new tools by implementing interface

**This pairs perfectly with #10**: Each tool file implements ToolExecutor interface

**Estimated time**: 3-4 hours (refactor all 10 tools)

---

### 13. 🚨 Better Error Handling (Custom Error Types)
**Purpose**: Create structured error types with suggestions and context

**Current Problem**: Errors are strings with formatting, hard to test or handle programmatically

**Proposed Solution**:
```go
// errors/errors.go
type ToolError struct {
    Tool        string
    Message     string
    Suggestions []string
    Params      map[string]interface{}
}

func (e *ToolError) Error() string {
    var b strings.Builder
    b.WriteString(fmt.Sprintf("Tool '%s' error: %s\n", e.Tool, e.Message))
    if len(e.Suggestions) > 0 {
        b.WriteString("\nSuggestions:\n")
        for _, s := range e.Suggestions {
            b.WriteString(fmt.Sprintf("  - %s\n", s))
        }
    }
    return b.String()
}

type ValidationError struct {
    Field       string
    Value       interface{}
    Expected    string
    Example     string
}

type APIError struct {
    StatusCode  int
    Message     string
    Retry       bool
    Suggestions []string
}
```

**Usage Example**:
```go
// Instead of:
return "", fmt.Errorf("file '%s' does not exist. Use list_files to see available files", path)

// Do:
return "", &ToolError{
    Tool: "read_file",
    Message: fmt.Sprintf("file '%s' does not exist", path),
    Suggestions: []string{
        "Use list_files to see available files",
        fmt.Sprintf("Check if path is correct: %s", path),
    },
    Params: map[string]interface{}{"path": path},
}
```

**Error Helpers**:
```go
func FileNotFoundError(path string) error { ... }
func PermissionDeniedError(path string) error { ... }
func RequiresParameterError(tool, param string) error { ... }
func DirectoryNotFileError(path string) error { ... }
```

**Benefits**:
- **Testability**: Can assert on error types, not string matching
- **Consistency**: Same format across all tools
- **Programmatic handling**: Can inspect error fields
- **Rich context**: Structured data about what went wrong
- **DRY**: Reuse common error helpers

**Combines well with #3 from above**: Error duplication reduction

**Estimated time**: 2-3 hours (define types + refactor error returns)

---

### 14. 📎 File Input Support (Future)
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
