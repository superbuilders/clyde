# ACP (Agent Client Protocol) Support — Design

**Status:** Draft
**Branch:** (not yet created)
**Date:** 2025-07-20

## Overview

Add ACP v1 compliance to Clyde so it can be used as a coding agent inside
any ACP-compatible editor (Zed, JetBrains, Neovim via CodeCompanion, etc.).

The implementation is a **new frontend package** (`acp/`) that adapts the
existing `agent/` library to the ACP wire protocol. The `cli/` package and
the `agent/` core remain unchanged. The pattern mirrors `cmd/clyde-qwen/` —
a purpose-built entry point that reuses the shared agent library.

### Key Design Decisions

| # | Decision | Rationale |
|---|----------|-----------|
| ITD-1 | Hand-roll a thin JSON-RPC + ACP layer instead of depending on `acp-go` | Keep the dependency tree minimal; the protocol surface we need is small (~6 methods). The `acp-go` library is unofficial and unstable. |
| ITD-2 | Run tools unconditionally — skip `session/request_permission` | Clyde already runs all tools without confirmation. ACP allows this. We avoid a permission UI that doesn't exist in our model. |
| ITD-3 | Leave `agent/` unchanged; all ACP-specific logic lives in `acp/` as a wrapper/facade | The agent's `HandleMessage()` + callback API already provides the right granularity. The ACP layer translates between JSON-RPC and these callbacks. No new interfaces are pushed into agent code. |
| ITD-4 | Cancellation is best-effort, implemented in the ACP layer | Like Ctrl+C in the TUI — we don't thread `context.Context` into `HandleMessage`. Instead, the ACP layer tracks a cancelled flag and returns `cancelled` stop reason after the current API call completes. |
| ITD-5 | Path resolution lives in the ACP layer | ACP requires absolute paths and provides `cwd` in `session/new`. The ACP layer calls `os.Chdir(cwd)` at session start. Clyde's tools already operate relative to cwd, so this is sufficient. |

### File Layout

```
clyde/
├── main.go                     # existing CLI entry point (UNCHANGED)
├── cmd/
│   ├── clyde-qwen/main.go      # existing (UNCHANGED)
│   └── clyde-acp/main.go       # NEW — ACP entry point
├── acp/                         # NEW — ACP protocol adapter
│   ├── server.go                # JSON-RPC stdio transport + dispatch loop
│   ├── handlers.go              # ACP method handlers (initialize, session/*, etc.)
│   ├── types.go                 # JSON-RPC envelope + ACP request/response types
│   ├── session.go               # ACP session state (wraps agent.Agent)
│   ├── server_test.go           # Transport-level tests
│   ├── handlers_test.go         # Handler-level tests
│   └── integration_test.go      # End-to-end subprocess tests
├── agent/                       # UNCHANGED
├── cli/                         # UNCHANGED
```

---

## User Stories

### ACP-1: JSON-RPC stdio transport

**As** an ACP client (editor),
**I want** to launch `clyde-acp` as a subprocess and exchange JSON-RPC 2.0
messages over stdin/stdout,
**so that** I can communicate with Clyde using the standard ACP transport.

#### Acceptance Criteria

- [ ] `acp/server.go` implements a blocking `Serve(in io.Reader, out io.Writer)`
      function that reads newline-delimited JSON-RPC messages from `in` and
      writes responses/notifications to `out`.
- [ ] Each input line is parsed as a JSON-RPC 2.0 message (request or
      notification). Malformed JSON or missing `jsonrpc: "2.0"` returns a
      JSON-RPC Parse Error (code -32700).
- [ ] Unknown methods return a JSON-RPC Method Not Found error (code -32601).
- [ ] Notifications (no `id` field) never produce a response.
- [ ] All output written to `out` is valid JSON-RPC, one object per line,
      with no embedded newlines.
- [ ] The server writes logging/diagnostics to stderr only; stdout is
      reserved for ACP messages.
- [ ] `acp/types.go` defines: `JSONRPCRequest`, `JSONRPCResponse`,
      `JSONRPCError`, `JSONRPCNotification` with proper `json` tags.
- [ ] `cmd/clyde-acp/main.go` calls `acp.Serve(os.Stdin, os.Stdout)`.
- [ ] On EOF from stdin, the server exits cleanly.

#### Test Plan

- Unit test: feed valid JSON-RPC lines into a `bytes.Buffer`, call `Serve`,
  assert well-formed JSON-RPC responses come out. Test parse errors,
  method-not-found, and notification (no response).
- Unit test: feed a line that is not valid JSON → expect -32700 error.
- Unit test: feed a request with an unknown method → expect -32601 error.
- Unit test: close stdin (EOF) → expect clean exit, no panic.

---

### ACP-2: `initialize` handler

**As** an ACP client,
**I want** to send an `initialize` request and receive Clyde's protocol
version and capabilities,
**so that** I can negotiate the protocol and know what features Clyde supports.

#### Acceptance Criteria

- [ ] The `initialize` handler reads `InitializeRequest` params
      (`protocolVersion`, `clientCapabilities`, `clientInfo`).
- [ ] It responds with `InitializeResponse` containing:
    - `protocolVersion: 1`
    - `agentCapabilities` with at minimum: `promptCapabilities: { image: true }`
      (Clyde supports images via `include_file`).
    - `agentInfo: { name: "clyde", title: "Clyde", version: "<version>" }`
    - `authMethods: []` (no auth required — Clyde reads its API key from
      `~/.clyde/config`).
- [ ] If the client sends a `protocolVersion` Clyde doesn't support, Clyde
      responds with `protocolVersion: 1` (the latest it supports). The client
      decides whether to continue.
- [ ] A second `initialize` call on the same connection returns an error.
- [ ] Any method other than `initialize` called before initialization returns
      an error (code -32600, "not initialized").

#### Test Plan

- Unit test: send `initialize` → assert response has `protocolVersion: 1`,
  `agentInfo.name == "clyde"`, `authMethods` is empty array.
- Unit test: send `session/new` before `initialize` → assert error response.
- Unit test: send `initialize` twice → assert second returns error.

---

### ACP-3: `session/new` handler

**As** an ACP client,
**I want** to create a new session with a working directory,
**so that** Clyde operates in the correct project context.

#### Acceptance Criteria

- [ ] The `session/new` handler reads `SessionNewRequest` params (`cwd`,
      optional `mcpServers`).
- [ ] It validates that `cwd` is an absolute path and exists on disk;
      returns an error otherwise.
- [ ] It calls `os.Chdir(cwd)` to set the process working directory.
- [ ] It loads config from `~/.clyde/config` (same as `cli/cli.go`'s
      `loadAgentConfig`) and creates an `agent.Agent` via `agent.New(cfg, ...opts)`.
- [ ] It generates a unique `sessionId` (UUID or similar) and stores the
      agent + session state in an in-memory map keyed by `sessionId`.
- [ ] It responds with `{ sessionId: "<id>" }`.
- [ ] `acp/session.go` defines a `Session` struct that holds: `sessionId`,
      `*agent.Agent`, a `cancelled` flag (atomic bool), and a mutex for
      prompt serialization (only one prompt at a time per session).
- [ ] MCP servers from the request are ignored for MVP (logged to stderr).
      Clyde manages its own MCP connections internally.

#### Test Plan

- Unit test: call `session/new` with a valid `cwd` → assert response has a
  non-empty `sessionId`.
- Unit test: call `session/new` with a relative path → assert error.
- Unit test: call `session/new` with a nonexistent path → assert error.
- Unit test: call `session/new` before `initialize` → assert error.

---

### ACP-4: `session/prompt` — basic text flow

**As** an ACP client,
**I want** to send a text prompt and receive the agent's text response,
**so that** I can have a conversation with Clyde.

This is the core story. It wires `agent.HandleMessage()` to the ACP prompt
turn lifecycle.

#### Acceptance Criteria

- [ ] The `session/prompt` handler reads `PromptRequest` params (`sessionId`,
      `prompt: ContentBlock[]`).
- [ ] It extracts text from the content blocks. For MVP, only `type: "text"`
      blocks are processed; other types are acknowledged but not used (logged
      to stderr).
- [ ] It looks up the session by `sessionId`; returns error if not found.
- [ ] It acquires the session's prompt mutex (only one prompt at a time);
      returns error if a prompt is already in progress.
- [ ] It resets the session's `cancelled` flag to `false`.
- [ ] It calls `agent.HandleMessage(textInput)` **synchronously** in the
      handler goroutine. While `HandleMessage` runs, the agent's callbacks
      emit `session/update` notifications to stdout (see ACP-5).
- [ ] When `HandleMessage` returns successfully, the handler responds with
      `{ stopReason: "end_turn" }`.
- [ ] When `HandleMessage` returns an error, the handler responds with
      `{ stopReason: "end_turn" }` and emits the error as a final
      `agent_message_chunk` text block so the client sees it.
- [ ] If the `cancelled` flag was set during processing, the handler
      responds with `{ stopReason: "cancelled" }` instead.
- [ ] Content block extraction concatenates multiple text blocks with
      newlines. `resource_link` blocks are converted to text describing
      the file path. `image` blocks are deferred to ACP-8.

#### Test Plan

- Integration test: subprocess `clyde-acp`, send `initialize` →
  `session/new` → `session/prompt` with `[{ type: "text", text: "say hello" }]`.
  Assert: at least one `session/update` notification with
  `agent_message_chunk` arrives, followed by a `session/prompt` response
  with `stopReason: "end_turn"`.
- Unit test (mock agent): verify content block extraction — multiple text
  blocks are concatenated, `resource_link` becomes text.
- Unit test: `session/prompt` with unknown `sessionId` → error.
- Unit test: two concurrent `session/prompt` on same session → second
  returns error.

---

### ACP-5: `session/update` notifications — streaming callbacks

**As** an ACP client,
**I want** to receive real-time `session/update` notifications as Clyde
thinks, calls tools, and produces output,
**so that** I can display live progress to the user.

This story wires each of Clyde's agent callbacks to the corresponding ACP
notification type.

#### Acceptance Criteria

- [ ] The ACP layer wires the following agent callbacks when creating the
      agent for a session. Each callback writes a `session/update`
      notification (JSON-RPC notification, no `id`) to stdout.
- [ ] **Callback mapping:**

| Agent Callback | ACP `session/update` type | Update content |
|----------------|--------------------------|----------------|
| `AssistantMessageCallback(text)` | `agent_message_chunk` | `{ type: "text", text: "<text>" }` |
| `ThinkingCallback(text, sig)` | `agent_message_chunk` | `{ type: "text", text: "<text>", annotations: { audience: ["agent"] } }` — thought content marked for agent display |
| `ProgressCallback(msg, toolUseID)` | `tool_call` | `{ toolCallId: "<toolUseID>", title: "<msg>", kind: "<inferred>", status: "in_progress" }` |
| `OutputCallback(output, toolUseID)` | `tool_call_update` | `{ toolCallId: "<toolUseID>", status: "completed", content: [{ type: "content", content: { type: "text", text: "<output>" } }] }` |
| `DiagnosticCallback(msg)` | (no notification emitted) | Logged to stderr only. |
| `SpinnerCallback(start, msg)` | (no notification emitted) | Ignored — the client manages its own UI. |
| `ErrorCallback(err)` | (no notification emitted) | Logged to stderr. |
| `CompactionCallback(marker, summary)` | `agent_message_chunk` | Emit `marker` as a text chunk (informational). |

- [ ] `ToolUseCallback` is wired but used internally only for enriching
      the `tool_call` notification with `kind` (derived from tool name:
      `read_file` → `read`, `write_file`/`patch_file`/`multi_patch` → `edit`,
      `run_bash` → `execute`, `grep`/`glob` → `search`, `browse`/`web_search`
      → `fetch`, default → `other`).
- [ ] All notifications include the correct `sessionId`.
- [ ] Notification writes are serialized (mutex or channel) to prevent
      interleaved JSON on stdout.
- [ ] The `UserMessageCallback` is **not** wired to a notification (the
      client already knows what the user said).

#### Test Plan

- Integration test: send a prompt that triggers a tool (e.g., "list files
  in the current directory"). Assert: receive at least one `tool_call`
  notification and one `tool_call_update` with `status: "completed"`.
- Unit test (mock agent): simulate callbacks firing in sequence, assert
  the correct JSON-RPC notifications are written to the output buffer
  in order, with correct `sessionUpdate` discriminators.
- Unit test: verify tool kind mapping — `read_file` → `read`,
  `run_bash` → `execute`, `web_search` → `fetch`, etc.

---

### ACP-6: `session/cancel` notification

**As** an ACP client,
**I want** to cancel an in-progress prompt turn,
**so that** I can interrupt Clyde when it's taking too long or going in
the wrong direction.

#### Acceptance Criteria

- [ ] `session/cancel` is handled as a JSON-RPC **notification** (no `id`,
      no response sent).
- [ ] It looks up the session by `sessionId` and sets the `cancelled`
      atomic bool to `true`.
- [ ] The prompt handler (ACP-4) checks the `cancelled` flag after
      `HandleMessage` returns and responds with `{ stopReason: "cancelled" }`
      if set.
- [ ] This is best-effort: if `HandleMessage` is mid-API-call, it will
      complete that call before the cancellation takes effect. The
      cancellation will be noticed before the next API call or tool
      execution in the agent's conversation loop.
- [ ] Future enhancement (out of scope): thread `context.Context` into
      `HandleMessage` → `apiClient.Call` for true mid-request cancellation.

#### Test Plan

- Integration test: send a prompt that will take multiple tool calls.
  Immediately send `session/cancel`. Assert: the `session/prompt`
  response eventually arrives with `stopReason: "cancelled"`.
- Unit test: set `cancelled` flag on a session, verify the prompt handler
  returns `cancelled` stop reason.

---

### ACP-7: Entry point and config

**As** a user,
**I want** to build and run `clyde-acp` as a standalone binary,
**so that** I can register it with my editor as an ACP agent.

#### Acceptance Criteria

- [ ] `cmd/clyde-acp/main.go` is a minimal entry point (~15 lines) that
      calls `acp.Serve(os.Stdin, os.Stdout)`.
- [ ] Config loading reuses the same `~/.clyde/config` file and env vars
      as the CLI. The ACP layer calls the same `loadAgentConfig` logic
      (extracted to a shared location if needed, or duplicated minimally).
- [ ] `go build -o clyde-acp ./cmd/clyde-acp` produces a working binary.
- [ ] The binary can be registered with Zed via the ACP agent registry
      format or manually in `settings.json`.
- [ ] stderr output includes a startup banner with the protocol version
      for debugging: `clyde-acp: ACP v1 agent ready (stdio)`.

#### Test Plan

- Build test: `go build ./cmd/clyde-acp` succeeds with no errors.
- Smoke test: launch the binary, send `initialize` + `session/new` +
  `session/prompt` over stdin, verify valid JSON-RPC on stdout.

---

### ACP-8: Image content blocks (stretch)

**As** an ACP client,
**I want** to include images in my prompt,
**so that** Clyde can analyze screenshots and diagrams.

#### Acceptance Criteria

- [ ] When a `ContentBlock` of `type: "image"` is present in the prompt,
      the ACP layer converts it to Clyde's `IMAGE_LOADED:` format and
      includes it in the conversation history.
- [ ] The `agentCapabilities.promptCapabilities.image` is advertised as
      `true` in the `initialize` response (already specified in ACP-2).
- [ ] `resource` blocks with image MIME types are also handled.

#### Test Plan

- Unit test: construct a prompt with an image content block, verify the
  ACP layer produces the correct `IMAGE_LOADED:` string for the agent.

---

## Implementation Order

The stories should be implemented in this order due to dependencies:

```
ACP-1  (transport)
  └─► ACP-2  (initialize)
        └─► ACP-3  (session/new)
              └─► ACP-4  (session/prompt — basic text)
                    ├─► ACP-5  (session/update notifications)
                    └─► ACP-6  (session/cancel)
ACP-7  (entry point) — can be done in parallel with ACP-2+
ACP-8  (images) — stretch, after everything else works
```

Estimated total: **~600-800 lines of new Go code** across 4-5 files,
plus **~400 lines of tests**.

---

## Appendix A: ACP Wire Protocol Reference

### Transport

- JSON-RPC 2.0 over stdio.
- Messages delimited by `\n`. No embedded newlines.
- Agent reads from stdin, writes to stdout.
- stderr is for logging only.

### Required Agent Methods

| Method | Type | Direction |
|--------|------|-----------|
| `initialize` | Request | Client → Agent |
| `session/new` | Request | Client → Agent |
| `session/prompt` | Request | Client → Agent |
| `session/cancel` | Notification | Client → Agent |

### Required Agent Notifications (Agent → Client)

| Method | Type |
|--------|------|
| `session/update` | Notification |

### Key Types

```
StopReason: "end_turn" | "max_tokens" | "cancelled" | "refusal"

SessionUpdate discriminator (sessionUpdate field):
  "agent_message_chunk" — streaming text/thought from the model
  "tool_call"           — tool call announced
  "tool_call_update"    — tool call progress/completion
  "plan"                — agent plan (optional, not implementing)
  "user_message_chunk"  — user message replay (session/load only)

ToolKind: "read" | "edit" | "execute" | "search" | "fetch" | "other"
ToolCallStatus: "pending" | "in_progress" | "completed" | "failed"
```

## Appendix B: Callback → ACP Notification Wiring Diagram

```
┌──────────────────────┐          ┌───────────────────────────┐
│  agent.HandleMessage  │          │   ACP stdout (JSON-RPC)   │
│                       │          │                           │
│  ThinkingCallback ────┼──────►   │  session/update            │
│                       │          │    agent_message_chunk      │
│                       │          │    (thought content)        │
│  AssistantMsgCB ──────┼──────►   │  session/update            │
│                       │          │    agent_message_chunk      │
│                       │          │    (text)                   │
│  ProgressCallback ────┼──────►   │  session/update            │
│                       │          │    tool_call                │
│  ToolUseCallback ─────┼──(enriches kind on tool_call)         │
│                       │          │                           │
│  OutputCallback ──────┼──────►   │  session/update            │
│                       │          │    tool_call_update         │
│                       │          │    (completed + content)    │
│  DiagnosticCB ────────┼──────►   │  (stderr only)            │
│  SpinnerCB ───────────┼──────►   │  (ignored)                │
│  ErrorCB ─────────────┼──────►   │  (stderr only)            │
│                       │          │                           │
│  return (text, err) ──┼──────►   │  Final agent_message_chunk │
│                       │          │  + session/prompt response  │
│                       │          │    { stopReason: "end_turn"}│
└───────────────────────┘          └───────────────────────────┘
```

## Appendix C: Concurrency Model

```
                    ┌─────────────────────────────┐
                    │  main goroutine              │
                    │  acp.Serve(stdin, stdout)    │
                    │                              │
                    │  reads stdin line-by-line     │
                    │  dispatches to handler        │
                    └──────────┬──────────────────┘
                               │
              ┌────────────────┼──────────────────┐
              ▼                ▼                   ▼
     initialize         session/new         session/prompt
     (sync, fast)       (sync, fast)        (blocks for duration
                                             of HandleMessage)
                                                   │
                                                   │ callbacks fire
                                                   │ during execution
                                                   ▼
                                            ┌──────────────┐
                                            │ stdout writer │
                                            │ (mutex-      │
                                            │  protected)   │
                                            └──────────────┘
```

The main goroutine reads one line at a time from stdin. For `session/prompt`,
the read loop blocks while `HandleMessage` runs. This means `session/cancel`
**must** be readable during a prompt — requiring the stdin reader to run in
its own goroutine, with incoming messages dispatched to the appropriate
handler via a channel or direct function call.

**Revised design:**

```
  stdin reader goroutine                    handler goroutine
  ──────────────────────                    ─────────────────
  reads lines from stdin                    blocks on channel
  parses JSON-RPC                           receives dispatched messages
  ─── notification? ──────────────────►     handles session/cancel immediately
  ─── request? ───────────────────────►     handles session/prompt (blocks)
                                            writes response to stdout
```

This ensures `session/cancel` can arrive and be processed while
`session/prompt` is executing. The stdin reader goroutine never blocks on
handler execution — it always returns to reading the next line immediately
after dispatching.
