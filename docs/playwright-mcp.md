# Playwright MCP Integration for Clyde

**Date:** July 2025
**Status:** Design document (pre-implementation)

## Table of Contents

1. [Motivation](#1-motivation)
2. [Research: How Others Do MCP](#2-research-how-others-do-mcp)
3. [Important Technical Decisions](#3-important-technical-decisions)
4. [Architecture](#4-architecture)
5. [User Stories](#5-user-stories)

---

## 1. Motivation

### Philosophy: MCP Is Not Needed — Except When It Is

We are ideologically aligned with Mario Zechner's position that **MCP is not needed for coding agents.** The protocol's core pattern — dumping large tool catalogs into context permanently — is wasteful, and for most tool integrations a simple CLI script invoked via bash is cheaper, more composable, and easier to maintain. Pi proves this convincingly.

Where we diverge from Pi is on what constitutes a complete coding agent. In our view, **a coding agent is fundamentally incomplete without web search and browser use.** Code does not exist in a vacuum — developers constantly look up documentation, read error threads, check deployed UIs, reference API specs, and verify behavior in a browser. An agent that cannot do these things is missing a core capability, not a nice-to-have. This is why clyde already ships with `web_search` and `browse` as built-in tools, where Pi treats these as optional external scripts.

Browser automation specifically — navigating pages, clicking elements, filling forms, reading DOM state — goes beyond what our existing `browse` tool (which fetches and converts static HTML) can do. We need a real browser with persistent state: tabs, cookies, JavaScript execution, accessibility snapshots. **This is where the pragmatic case for MCP wins.** Microsoft's Playwright MCP server is well-maintained, actively developed, and handles all of this complexity. Writing and maintaining our own browser automation layer in Go would be a large, ongoing burden for no differentiated value. The MCP protocol is the simplest possible interface to the Playwright team's work — we spawn their process, send JSON, read JSON.

### The Concrete Trade-Off

We pay **~3,900 tokens of context overhead** (1.9% of a 200k window) to get a battle-tested, 21-tool browser automation layer maintained by someone else. We write ~620 lines of Go glue code with zero new dependencies. We don't adopt MCP as a general-purpose extension system — this is a scoped, pragmatic integration for a single server that provides a capability we consider essential.

We want the same vanilla experience a Claude Code user gets when they install Playwright MCP — no interference with tool definitions, no curation, no filtering. Minimal Go code on our side.

---

## 2. Research: How Others Do MCP

We examined four approaches to understand the design space before choosing ours.

### 2.1 Pi Coding Agent (Mario Zechner) — No MCP

Pi does not support MCP and never will. Mario's position is that MCP servers are overkill for coding agents and waste context tokens. His alternative: simple CLI tools (shell scripts) with README files. The agent reads the README on-demand via bash ("progressive disclosure"), paying the token cost only when the tool is actually needed.

His token overhead comparison for browser automation:

| Approach | Tools | Token cost | When paid |
|----------|-------|------------|-----------|
| Playwright MCP | 21 default | ~13,700 (his measurement, older version) | Every API call |
| CLI + README | 4–6 scripts | ~225 | Only when agent reads README |

His benchmarks ("MCP vs CLI", August 2025) found that **MCP vs CLI is largely a wash in success rate and cost** — what matters is tool design and documentation quality, not the protocol. For users who must use MCP servers, he points to [mcporter](https://github.com/steipete/mcporter), a tool that wraps MCP servers as standalone CLI tools.

**Relevance to us:** Mario's token-efficiency argument is real, but the actual cost has dropped. Our live measurement of the current Playwright MCP shows **21 tools at ~3,900 tokens (1.9% of context)**, not the 13,700 he cited. The convenience of a maintained browser server outweighs the token cost for our use case.

### 2.2 oh-my-pi / omp (can1357) — Full Custom MCP

[oh-my-pi](https://github.com/can1357/oh-my-pi) is the most widely used Pi fork and has a massive MCP implementation: **~6,000 lines of TypeScript** across 19 files plus a transports directory. It does NOT use mcporter — it has its own:

- Custom JSON-RPC 2.0 client and transport layer (stdio + HTTP)
- Full OAuth discovery and authentication flows
- Smithery registry integration for discovering MCP servers
- Tool caching with change notifications
- A `tool-bridge.ts` that converts MCP tools to Pi's `CustomTool` interface
- Reconnection logic with retriable error detection
- Deferred connection resolution (lazy connect on first tool call)
- Tool name sanitization and collision avoidance (`mcp_{server}_{tool}`)
- Universal config discovery (reads MCP configs from 8 different AI tools)

| File | Lines | Purpose |
|------|-------|---------|
| manager.ts | 1,152 | Server lifecycle, connect/disconnect, config watching |
| client.ts | 482 | JSON-RPC transport, initialize, list, call |
| smithery-registry.ts | 477 | Smithery marketplace integration |
| tool-bridge.ts | 416 | MCP tool → CustomTool adapter |
| types.ts | 423 | Type definitions |
| config.ts | 365 | Config loading, validation, merging |
| oauth-flow.ts | 387 | OAuth authentication |
| oauth-discovery.ts | 349 | OAuth metadata discovery |
| transports/http.ts | 475 | HTTP/SSE transport |
| transports/stdio.ts | 325 | Stdio transport |
| *(7 more files)* | ~1,160 | Config writer, render, loader, cache, etc. |
| **Total** | **~6,000** | |

**Relevance to us:** This is the heavyweight end of the spectrum. We want roughly 1/20th of this — just enough to talk to one stdio server.

### 2.3 OpenCode (Dax Raad / sst) — SDK-Based MCP

There are two projects called "OpenCode." The one by Dax Raad / anomalyco ([sst/opencode](https://github.com/sst/opencode)) is a TypeScript coding agent using the official `@modelcontextprotocol/sdk` package. Its MCP module is **~1,500 lines** across 4 files (921-line `index.ts` + OAuth files). It supports stdio, StreamableHTTP, and SSE transports with full OAuth.

The other ([opencode-ai/opencode](https://github.com/opencode-ai/opencode), now archived, moved to Crush by charmbracelet) was a Go coding agent that used the `mcp-go` library. Its MCP integration was only **~200 lines** in a single file `mcp-tools.go`, but had a critical flaw: it **restarted the MCP server subprocess on every tool call** (`defer c.Close()` in the `runTool` function). For Playwright this would kill the browser between every `navigate` → `click` → `snapshot` sequence — completely unusable.

**Relevance to us:** The Go OpenCode shows both the promise (200 lines!) and the pitfall (per-call lifecycle) of a minimal approach. We want the line count without the bug.

### 2.4 Claude Code — Enterprise MCP Ecosystem

Claude Code has the most comprehensive MCP support: three transports (stdio, SSE, HTTP), OAuth 2.0 flows, three config scopes (local/project/user), a plugin system with bundled MCP servers, dynamic tool updates, push messages via channels, MCP Tool Search for scale, managed MCP with allowlists/denylists, and configurable output size warnings. This is an enterprise platform feature — thousands of lines of code for concerns we will never have.

**Relevance to us:** Claude Code is what our users would compare against. When a Claude Code user runs `claude mcp add playwright -- npx @playwright/mcp@latest`, they get 21 tools in their context and can immediately browse the web. We want the same end-user experience with a fraction of the implementation.

### 2.5 Comparison Summary

| Project | Language | MCP Code | Files | External MCP Deps | Transports |
|---------|----------|----------|-------|--------------------|------------|
| Pi | TypeScript | 0 lines | 0 | None (no MCP) | N/A |
| oh-my-pi | TypeScript | ~6,000 | 19+ | None (custom) | stdio, HTTP |
| OpenCode (sst) | TypeScript | ~1,500 | 4 | `@modelcontextprotocol/sdk` | stdio, HTTP, SSE |
| OpenCode (Go) | Go | ~200 | 1 | `mcp-go` | stdio, SSE |
| Claude Code | TypeScript | thousands | many | `@modelcontextprotocol/sdk` | stdio, SSE, HTTP |
| **Clyde (target)** | **Go** | **~400** | **3–4** | **None** | **stdio only** |

---

## 3. Important Technical Decisions

### ITD-1: Hand-Rolled Stdio Client (Zero Dependencies)

**Decision:** Write our own MCP stdio client in Go using only the standard library.

**Rationale:** The MCP stdio protocol is JSON-RPC 2.0 over stdin/stdout — each message is a single JSON object per line. We need exactly three RPC methods: `initialize`, `tools/list`, and `tools/call`. The `mcp-go` library (used by the archived Go OpenCode) is well-made but brings 4 transitive dependencies and a large API surface (SSE, HTTP, OAuth, resources, prompts, sampling, elicitation, roots) that we will never use.

A hand-rolled client for our three methods is approximately ~230 lines of Go. It uses `os/exec` to spawn the subprocess, `encoding/json` for marshaling, and `bufio.Scanner` for line-delimited reading. There is no protocol complexity worth importing a library for.

**What we implement:**

| Component | Approx Lines | What It Does |
|-----------|-------------|--------------|
| JSON-RPC framing | ~60 | Request/response structs, ID tracking, error handling |
| Stdio transport | ~80 | Spawn subprocess, pipe stdin/stdout, line-delimited JSON |
| MCP methods | ~40 | `initialize`, `tools/list`, `tools/call` request builders |
| Type definitions | ~50 | `MCPTool`, `MCPCallResult`, `MCPInitializeResult`, etc. |

**What we don't implement:** SSE transport, HTTP transport, OAuth, resources, prompts, sampling, elicitation, roots, pagination, notifications. None of these are needed for Playwright MCP.

### ITD-2: Server Lifecycle — Lazy Start, Keep Alive, Kill on Exit

**Decision:** The Playwright MCP server process is started lazily on first browser tool invocation, kept alive for the entire clyde session, and killed when clyde exits.

**Rationale:** This isn't really a decision — it's the only correct approach. The Playwright MCP server manages the browser instance inside its process. As long as the process is alive, the browser is alive with all its state (tabs, cookies, DOM). If we killed and restarted the process between tool calls (as the archived Go OpenCode does), the browser would die each time — destroying all state. We just need:

1. `sync.Once` to spawn on first use (most sessions won't need a browser)
2. Hold the `exec.Cmd` and stdin/stdout pipes for the session lifetime
3. `cmd.Process.Kill()` in a cleanup function when clyde exits (or the REPL session ends)

The Playwright MCP server itself handles everything on its side of the boundary — browser lifecycle, tab management, crash recovery, timeouts. We are just the pipe.

### ITD-3: Vanilla Tool Passthrough (No Curation, No Filtering)

**Decision:** Pass through whatever `tools/list` returns from the Playwright MCP server, unmodified. No tool curation, no subsetting, no `--caps` filtering.

**Rationale:** We measured the actual default Playwright MCP output by running `npx @playwright/mcp@latest --headless` and calling `tools/list`:

| Metric | Value |
|--------|-------|
| Default tools | 21 (core automation + tabs) |
| Total JSON | 15,505 chars |
| Approx tokens | ~3,900 |
| % of 200k context | 1.9% |
| Combined with clyde's 12 tools | ~6,300 tokens (3.1%) |

The 58-tool number often cited includes opt-in categories (storage, devtools, vision, PDF, testing, network) that require explicit `--caps` flags. The default is already a curated subset chosen by the Playwright team.

The server sends **no instructions, no prompts, no resources** — just tool definitions. There is nothing on the MCP server's side of the boundary that we need to interfere with or augment:

```json
{
  "capabilities": { "tools": {} },
  "serverInfo": { "name": "Playwright", "version": "1.60.0" }
}
```

**The 21 default tools:**

```
browser_click           browser_close            browser_console_messages
browser_drag            browser_evaluate          browser_file_upload
browser_fill_form       browser_handle_dialog     browser_hover
browser_navigate        browser_navigate_back     browser_network_requests
browser_press_key       browser_resize            browser_run_code
browser_select_option   browser_snapshot          browser_tabs
browser_take_screenshot browser_type              browser_wait_for
```

### ITD-4: Direct Schema Passthrough

**Decision:** Map MCP tool definitions directly to Anthropic tool definitions with no transformation.

**Rationale:** Both MCP and the Anthropic API use JSON Schema for tool parameter definitions. The mapping is trivial:

```
mcp.Tool.Name         → api.Tool.Name (prefixed with "mcp_playwright_")
mcp.Tool.Description  → api.Tool.Description
mcp.Tool.InputSchema  → api.Tool.InputSchema (verbatim)
```

We prefix tool names with `mcp_playwright_` to avoid any collision with clyde's built-in tools (e.g., our `browse` tool vs Playwright's `browser_*` tools). This is the same convention used by oh-my-pi and the archived Go OpenCode.

### ITD-5: Configuration via Env Vars

**Decision:** Use environment variables in `~/.clyde/config`, consistent with existing configuration.

**Rationale:** Clyde's config is a simple env-var file loaded by `godotenv`. Adding MCP config follows the same pattern:

```bash
# ~/.clyde/config
TS_AGENT_API_KEY=sk-ant-...
BRAVE_SEARCH_API_KEY=BSA-...

# Playwright MCP (new)
MCP_PLAYWRIGHT=true
MCP_PLAYWRIGHT_ARGS=--headless    # Optional: extra args for npx
```

When `MCP_PLAYWRIGHT=true`, clyde will launch `npx @playwright/mcp@latest` (plus any extra args from `MCP_PLAYWRIGHT_ARGS`) as a stdio subprocess on first browser tool use.

If we later want to support arbitrary MCP servers, we can evolve to a JSON config file. But for the single Playwright use case, env vars are sufficient and consistent.

### ITD-6: No Approval Gates

**Decision:** MCP tools are treated identically to built-in tools — no permission prompts, no approval flows.

**Rationale:** Clyde is designed as a full coding agent that runs securely on its own box. It already has unrestricted `run_bash`, `write_file`, and `multi_patch`. Adding approval gates for browser actions would be inconsistent and pointless. The security boundary is the box, not per-tool permissions.

---

## 4. Architecture

### 4.1 New Files

```
clyde/
├── mcp/
│   ├── client.go          # JSON-RPC stdio client (~140 lines)
│   ├── types.go           # MCP type definitions (~60 lines)
│   └── playwright.go      # Playwright server lifecycle + tool registration (~200 lines)
├── config/
│   └── config.go          # (modified) Add MCP_PLAYWRIGHT fields
├── agent/
│   └── agent.go           # (modified) Wire MCP tools into tool loop
└── main.go                # (modified) Cleanup on exit
```

### 4.2 Interaction Sequence

```
Session Start (REPL or CLI)
│
├── Load config → MCP_PLAYWRIGHT=true?
│   └── Yes → Record that Playwright is enabled (but don't start it yet)
│
├── ... normal conversation turns ...
│
├── Claude calls "mcp_playwright_browser_navigate"
│   │
│   ├── First call? (sync.Once)
│   │   ├── Spawn: npx @playwright/mcp@latest --headless
│   │   ├── Send: initialize request
│   │   ├── Read: initialize response
│   │   ├── Send: notifications/initialized
│   │   ├── Send: tools/list request
│   │   ├── Read: tools/list response (21 tools)
│   │   └── Register tools in tools.Registry
│   │       (This only happens once — tools are already registered
│   │        for this turn because we pre-registered them at startup
│   │        from a cached/known tool list. See §4.3.)
│   │
│   ├── Send: tools/call { name: "browser_navigate", arguments: {...} }
│   ├── Read: tools/call result
│   └── Return result to agent loop
│
├── Claude calls "mcp_playwright_browser_snapshot"
│   ├── Server already running
│   ├── Send: tools/call { name: "browser_snapshot", arguments: {...} }
│   ├── Read: tools/call result
│   └── Return result to agent loop
│
├── ... more tool calls ...
│
└── Session End (exit/quit/EOF)
    └── Kill Playwright subprocess
```

### 4.3 Tool Registration Timing

There is a chicken-and-egg problem: we want tools registered before the first API call (so Claude knows they exist), but we don't want to start the Playwright server until it's actually needed. Two approaches:

**Option A — Eager server start:** Start the Playwright MCP server at session startup when `MCP_PLAYWRIGHT=true`. Call `tools/list` immediately and register the tools. This adds a few seconds of startup latency to every session that has Playwright enabled, even if the browser is never used.

**Option B — Static tool registration with lazy server start:** Ship a snapshot of Playwright's default tool definitions (the 21 tools and their schemas) embedded in clyde. Register these tools at startup from the snapshot. When one is actually invoked, lazily start the server and begin forwarding `tools/call` requests. On first connect, verify the live `tools/list` matches our snapshot and log a warning if it doesn't.

Option B avoids startup latency and the `npx` install on sessions that never use browser tools, at the cost of maintaining a snapshot that could drift from the live server. Given that Playwright's default tool set changes infrequently (and we'd notice mismatches via the verification check), **Option B is preferred**.

---

## 5. User Stories

Implementation is ordered as a sequence of user stories, each building on the previous. Each story is independently shippable and testable.

### Story 1: Raw MCP Stdio Client

**As** a developer, **I want** a minimal JSON-RPC 2.0 stdio client in Go **so that** I can communicate with any MCP server over stdin/stdout.

**Scope:**
- New package `mcp/` with `client.go` and `types.go`
- `Client` struct that holds `exec.Cmd`, `stdin` (encoder), `stdout` (scanner)
- `NewClient(command string, args ...string) (*Client, error)` — spawns the subprocess
- `client.Initialize(ctx) (*InitializeResult, error)` — sends `initialize` + `notifications/initialized`
- `client.ListTools(ctx) ([]Tool, error)` — sends `tools/list`, returns tool definitions
- `client.CallTool(ctx, name string, args map[string]any) (*CallToolResult, error)` — sends `tools/call`
- `client.Close() error` — kills the subprocess
- Proper JSON-RPC 2.0 framing: `jsonrpc`, `id`, `method`, `params`, `result`, `error`
- Sequential request IDs (no need for concurrent request multiplexing)
- Context-based timeout support

**Acceptance criteria:**
- A test that spawns a mock MCP server (a simple Go program that reads JSON-RPC from stdin and writes responses to stdout) and verifies the full initialize → list → call → close lifecycle
- No external dependencies beyond the Go standard library

**Estimated size:** ~200 lines of production code + ~150 lines of test code

---

### Story 2: Playwright Tool Snapshot

**As** a developer, **I want** Playwright's default tool definitions embedded in clyde **so that** browser tools appear in the model's tool list without starting the Playwright server.

**Scope:**
- New file `mcp/playwright_tools.json` containing the 21 default tool definitions captured from a live `npx @playwright/mcp@latest --headless` server via `tools/list`
- `//go:embed playwright_tools.json` to include it in the binary
- A function `PlaywrightTools() []api.Tool` that parses the embedded JSON and returns Anthropic-formatted tool definitions with the `mcp_playwright_` prefix
- A script or test (`mcp/update_snapshot_test.go`) that can regenerate the snapshot from a live server and diff it against the embedded version, to detect drift

**Acceptance criteria:**
- `PlaywrightTools()` returns 21 tools with correct names, descriptions, and JSON schemas
- Each tool name is prefixed with `mcp_playwright_` (e.g., `mcp_playwright_browser_click`)
- The embedded JSON matches what a live server returns (verified by the snapshot test)
- No MCP server is started during normal clyde startup

**Estimated size:** ~60 lines of Go code + ~15 KB of embedded JSON + ~80 lines of test/snapshot tooling

---

### Story 3: Playwright Server Lifecycle

**As** a developer, **I want** the Playwright MCP server to start lazily and stay alive for the session **so that** browser state persists across tool calls without paying startup cost when the browser isn't used.

**Scope:**
- New file `mcp/playwright.go` with a `PlaywrightServer` struct
- `NewPlaywrightServer(args string) *PlaywrightServer` — configures but does not start
- `server.EnsureRunning(ctx) error` — idempotent start via `sync.Once`; spawns `npx @playwright/mcp@latest` plus any configured args, calls `Initialize`, verifies `tools/list` against the embedded snapshot
- `server.CallTool(ctx, name string, args map[string]any) (*CallToolResult, error)` — forwards to the running MCP client (strips the `mcp_playwright_` prefix before sending)
- `server.Close() error` — kills the subprocess, resets state
- A `sync.Once` guard so multiple concurrent tool calls don't race on startup

**Acceptance criteria:**
- Server starts only on first `EnsureRunning` call
- Subsequent calls are no-ops (subprocess already running)
- `CallTool` works correctly after `EnsureRunning`
- `Close` kills the subprocess cleanly
- If the subprocess dies unexpectedly mid-session, the next `CallTool` returns a clear error

**Estimated size:** ~140 lines

---

### Story 4: Wire MCP Tools into the Agent Loop

**As** a user, **I want** to be able to ask Clyde to browse the web using Playwright **so that** I get the same browser automation experience as a Claude Code user.

**Scope:**
- Modify `config/config.go`: add `MCPPlaywright bool` and `MCPPlaywrightArgs string` fields, populated from `MCP_PLAYWRIGHT` and `MCP_PLAYWRIGHT_ARGS` env vars
- Modify `tools/registry.go` or add `mcp/register.go`: when `MCP_PLAYWRIGHT=true`, call `PlaywrightTools()` to get the 21 tool definitions and register each one with an executor that delegates to `PlaywrightServer.CallTool`
- Modify `agent/agent.go` (or the tool execution path): the executor for MCP tools must call `server.EnsureRunning(ctx)` before the first `CallTool`, so the server starts lazily on first use
- Modify `main.go`: add a `defer server.Close()` to ensure the Playwright subprocess is killed on exit
- The display function for MCP tools should show: `→ Browser: browser_navigate {url: "https://..."}`

**Acceptance criteria:**
- With `MCP_PLAYWRIGHT=true` in config, the model sees 21 extra `mcp_playwright_*` tools
- The model can navigate to a URL, take a snapshot, click elements, and take screenshots
- Browser state persists across tool calls within a session
- Without `MCP_PLAYWRIGHT=true`, no MCP tools appear and no server is started
- The Playwright subprocess is always killed when clyde exits (REPL quit, CLI completion, Ctrl+C)

**Estimated size:** ~100 lines of glue code across config, registry, agent, and main

---

### Story 5: Integration Test

**As** a developer, **I want** an end-to-end test that verifies the full Playwright MCP flow **so that** regressions are caught automatically.

**Scope:**
- A test in `tests/` that:
  1. Starts a simple local HTTP server (serve a static HTML page)
  2. Configures clyde with `MCP_PLAYWRIGHT=true`
  3. Sends a prompt like "Navigate to http://localhost:PORT and tell me what's on the page"
  4. Verifies that the agent used `mcp_playwright_browser_navigate` and `mcp_playwright_browser_snapshot`
  5. Verifies the response contains content from the HTML page
  6. Verifies the Playwright subprocess is cleaned up after the test
- This test requires `npx` and network access, so it should be gated behind a build tag (e.g., `//go:build mcp_test`) or skipped when `npx` is not available

**Acceptance criteria:**
- Test passes when Playwright MCP is installed (`npx @playwright/mcp@latest` available)
- Test is skipped gracefully when prerequisites are missing
- No orphaned Playwright processes after test completion

**Estimated size:** ~120 lines

---

### Implementation Summary

| Story | What | New Lines | Deps | Files |
|-------|------|-----------|------|-------|
| 1 | MCP stdio client | ~200 | 0 | `mcp/client.go`, `mcp/types.go` |
| 2 | Playwright tool snapshot | ~60 + 15KB JSON | 0 | `mcp/playwright_tools.json`, snapshot test |
| 3 | Server lifecycle | ~140 | 0 | `mcp/playwright.go` |
| 4 | Agent wiring | ~100 | 0 | Modified: config, registry, agent, main |
| 5 | Integration test | ~120 | 0 | `tests/mcp_playwright_test.go` |
| **Total** | | **~620 lines** | **0 new deps** | **3 new files + 4 modified** |

Current clyde codebase: ~7,019 lines. This adds ~620 lines (8.8% growth) with zero new dependencies.
