# Design: Ollama Provider Support

**Status:** Implemented (MVP); Phase 2 in progress  
**Branch:** `design/ollama-support`  
**Date:** 2025-07-20 (MVP), 2025-07-21 (Phase 2)  
**Model:** `qwen3.6:27b` (via Ollama) — upgraded from `qwen3.5:35b`  
**Binary:** `clyde-qwen` (`cmd/clyde-qwen/main.go`)  

## Goal

Support calling local Ollama models as an alternative to the Claude API, with
the minimum set of changes to the existing codebase. The user should be able to
switch between Claude and Ollama via configuration alone — no code changes, no
recompilation.

---

## Current Architecture (relevant pieces)

```
cli.go
  └─ loadAgentConfig()          → agent.Config{APIKey, APIURL, ModelID, ...}
       └─ agent.New(cfg)
            └─ providers.NewClient(apiKey, apiURL, modelID, maxTokens)
                 └─ client.Call(systemPrompt, messages, tools) → *Response
```

**Key observations:**

1. **Single concrete client.** `providers.Client` directly builds Claude HTTP
   requests (`x-api-key`, `anthropic-version` headers) and parses Claude-shaped
   JSON responses. There is no interface — the agent holds a `*providers.Client`.

2. **Claude-native types everywhere.** `providers.Request`, `providers.Response`,
   `providers.ContentBlock`, `providers.Tool` are Claude's wire format. The agent
   loop (`HandleMessage`) switches on `block.Type` values like `"text"`,
   `"tool_use"`, `"thinking"`, `"redacted_thinking"` — all Claude-specific.

3. **Tool definitions use Claude's schema.** `providers.Tool` has `InputSchema
   interface{}` (Claude's `input_schema`). Ollama wraps this in
   `{type: "function", function: {name, description, parameters}}`.

4. **Compaction uses the same client.** `compactionPhaseCall()` calls
   `a.apiClient.Call(...)` — it must work with whatever provider is active.

5. **Thinking is Claude-specific.** `ThinkingConfig{Type: "adaptive"}` and
   the thinking/signature/redacted_thinking content blocks are Claude features.
   Ollama has `think: true` but returns thinking in a simpler `thinking` field
   on the message, with no signatures or redacted blocks.

6. **Tool results differ.** Claude uses `role: "user"` with `ContentBlock{Type:
   "tool_result", ToolUseID: ...}`. Ollama uses `role: "tool"` with `tool_name`
   and plain `content`.

7. **No tool_use IDs in Ollama.** Claude assigns each tool call a unique `id`
   (`toolu_abc123`) and requires matching `tool_use_id` on results. Ollama has
   no such concept — tool calls and results are correlated positionally.

---

## Ollama `/api/chat` Wire Format (summary)

```jsonc
// Request
{
  "model": "qwen2.5:14b",
  "messages": [
    {"role": "system", "content": "..."},
    {"role": "user", "content": "..."},
    {"role": "assistant", "content": "", "tool_calls": [
      {"function": {"name": "read_file", "arguments": {"path": "main.go"}}}
    ]},
    {"role": "tool", "content": "package main...", "tool_name": "read_file"}
  ],
  "tools": [
    {
      "type": "function",
      "function": {
        "name": "read_file",
        "description": "Read a file",
        "parameters": {"type": "object", "properties": {...}, "required": [...]}
      }
    }
  ],
  "stream": false,
  "think": true         // optional, for thinking models
}

// Response
{
  "model": "qwen2.5:14b",
  "message": {
    "role": "assistant",
    "content": "Here's the file...",
    "thinking": "Let me read...",      // only if think: true
    "tool_calls": [                     // only if model wants tools
      {"function": {"name": "...", "arguments": {...}}}
    ]
  },
  "done": true,
  "done_reason": "stop",
  "prompt_eval_count": 1234,
  "eval_count": 567
}
```

---

## Design: Provider Interface

### The core change

Extract an **interface** from `providers.Client` that the agent depends on.
Both the existing Claude client and a new Ollama client implement it. The
agent never knows which backend it's talking to.

```go
// providers/provider.go (NEW)

// Provider is the interface that all LLM backends must implement.
// The agent calls Provider.Call() and receives a normalized Response.
type Provider interface {
    // Call sends a request to the LLM with the given system prompt,
    // conversation history, and available tools. Returns a normalized
    // Response in Claude-canonical format.
    Call(systemPrompt string, messages []Message, tools []Tool) (*Response, error)
}
```

**That's it.** One method. The existing `*Client` already satisfies this
interface — zero changes needed to the Claude client's public API.

### Agent changes

```diff
// agent/agent.go

 type Agent struct {
-    apiClient          *providers.Client
+    apiClient          providers.Provider
     systemPrompt       string
     ...
 }
```

Update `New()` and `NewAgent()` signatures to accept `providers.Provider`.
Since Go interfaces are satisfied implicitly, the existing `*providers.Client`
still works — all existing call sites compile unchanged.

### The Ollama client

```go
// providers/ollama.go (NEW)

type OllamaClient struct {
    baseURL string   // e.g. "http://localhost:11434"
    modelID string   // e.g. "qwen2.5:14b"
    think   bool     // enable thinking mode
}

func NewOllamaClient(baseURL, modelID string, think bool) *OllamaClient { ... }

func (c *OllamaClient) Call(
    systemPrompt string,
    messages []Message,
    tools []Tool,
) (*Response, error) {
    // 1. Convert messages    → Ollama message format
    // 2. Convert tools       → Ollama tool format
    // 3. POST /api/chat
    // 4. Convert response    → providers.Response (Claude-canonical)
    ...
}
```

The Ollama client is a **translator**: it accepts Claude-canonical types,
converts to Ollama wire format, calls the Ollama API, and converts the
response back to Claude-canonical types. The agent never sees Ollama-specific
structures.

---

## Translation Layer Detail

### Messages: Claude → Ollama

| Claude format | Ollama format |
|---|---|
| `{role: "user", content: "hello"}` | `{role: "user", content: "hello"}` |
| `{role: "assistant", content: [{type: "text", text: "hi"}]}` | `{role: "assistant", content: "hi"}` |
| `{role: "assistant", content: [{type: "tool_use", id: "t1", name: "foo", input: {...}}]}` | `{role: "assistant", tool_calls: [{function: {name: "foo", arguments: {...}}}]}` |
| `{role: "user", content: [{type: "tool_result", tool_use_id: "t1", content: "bar"}]}` | `{role: "tool", content: "bar", tool_name: "foo"}` |
| `{role: "assistant", content: [{type: "thinking", thinking: "..."}]}` | *(stripped — thinking is on the response, not in history)* |
| `{role: "assistant", content: [{type: "redacted_thinking", ...}]}` | *(stripped)* |

**Key detail:** Tool results in Claude carry `tool_use_id` but no tool name.
Ollama needs `tool_name` but has no IDs. The translator must look back at the
preceding assistant message to find which tool name corresponds to which
tool_use_id. Since tool calls and results are positional in both APIs, this is
a straightforward index-based mapping.

### Messages: Ollama → Claude (response normalization)

| Ollama response field | Claude-canonical ContentBlock |
|---|---|
| `message.content` (non-empty) | `{type: "text", text: "..."}` |
| `message.thinking` (non-empty) | `{type: "thinking", thinking: "...", signature: ""}` |
| `message.tool_calls[i]` | `{type: "tool_use", id: "ollama_N", name: "...", input: {...}}` |

**Synthetic IDs:** Since the agent loop uses `tool_use_id` to correlate tool
calls with results, the Ollama client generates synthetic IDs (`"ollama_0"`,
`"ollama_1"`, etc.) per response. These flow through the existing agent loop
unchanged and appear in tool_result blocks, which the Ollama client then
strips when converting back to Ollama format on the next call.

### Tools: Claude → Ollama

```go
// Claude:  {name: "read_file", description: "...", input_schema: {...}}
// Ollama:  {type: "function", function: {name: "read_file", description: "...", parameters: {...}}}
```

Direct field mapping. `input_schema` → `parameters`.

### System prompt

Claude takes `system` as a top-level string in the request. Ollama takes it as
a `{role: "system", content: "..."}` message prepended to the messages array.
The Ollama client handles this in `Call()`.

### Usage / token counts

| Ollama field | Maps to |
|---|---|
| `prompt_eval_count` | `Usage.InputTokens` |
| `eval_count` | `Usage.OutputTokens` |
| *(not available)* | `Usage.CacheCreationInputTokens` = 0 |
| *(not available)* | `Usage.CacheReadInputTokens` = 0 |

Cache fields stay zero. The diagnostic callback will show 0% cache, which is
correct — Ollama has no prompt caching.

### Stop reason

| Ollama `done_reason` | Maps to |
|---|---|
| `"stop"` (no tool_calls) | `StopReason: "end_turn"` |
| `"stop"` (has tool_calls) | `StopReason: "tool_use"` |

---

## Config Changes

### New environment variables (in `~/.clyde/config`)

```bash
# Provider selection: "claude" (default) or "ollama"
PROVIDER=ollama

# Ollama-specific settings (only used when PROVIDER=ollama)
OLLAMA_BASE_URL=http://localhost:11434    # default
OLLAMA_MODEL=qwen2.5:14b                 # required when PROVIDER=ollama
```

### agent.Config additions

```go
type Config struct {
    // ... existing fields ...

    // Provider selects the LLM backend: "claude" (default) or "ollama".
    Provider string

    // OllamaBaseURL is the Ollama server URL (default "http://localhost:11434").
    OllamaBaseURL string

    // OllamaModel is the Ollama model name (e.g. "qwen2.5:14b").
    OllamaModel string
}
```

### agent.New() provider dispatch

```go
func New(cfg Config, opts ...AgentOption) *Agent {
    var provider providers.Provider

    switch cfg.Provider {
    case "ollama":
        think := !cfg.NoThink
        provider = providers.NewOllamaClient(cfg.OllamaBaseURL, cfg.OllamaModel, think)
    default: // "claude" or ""
        client := providers.NewClient(cfg.APIKey, cfg.APIURL, cfg.ModelID, cfg.MaxTokens)
        // ... existing thinking config ...
        provider = client
    }

    a := &Agent{
        apiClient: provider,
        // ...
    }
    // ...
}
```

### CLI config loading

```go
// In loadAgentConfig():
provider := os.Getenv("PROVIDER") // "" defaults to "claude"
ollamaBaseURL := os.Getenv("OLLAMA_BASE_URL")
if ollamaBaseURL == "" {
    ollamaBaseURL = "http://localhost:11434"
}

// Skip API key validation when using ollama
if provider != "ollama" && apiKey == "" {
    return ..., fmt.Errorf("TS_AGENT_API_KEY not found...")
}

return agent.Config{
    // ...
    Provider:      provider,
    OllamaBaseURL: ollamaBaseURL,
    OllamaModel:   os.Getenv("OLLAMA_MODEL"),
}
```

---

## What Does NOT Change

| Component | Why |
|---|---|
| **Tool registry** (`agent/tools/`) | Tools are provider-agnostic. They receive `map[string]interface{}` and return strings. |
| **Tool definitions** (`providers.Tool`) | Same struct. Ollama client wraps it in `{type: "function", function: ...}` on the wire. |
| **System prompt** (`prompts/system.txt`) | Same prompt works for capable Ollama models. May want a shorter version later, but not for MVP. |
| **Session persistence** | Sessions record `providers.Message` in Claude-canonical form. Resuming a session that was started with Ollama works because the history is in the same normalized format. |
| **Compaction** | Calls `a.apiClient.Call()` which goes through the interface. Works with any provider. |
| **MCP/Playwright** | MCP tools register normally. They don't touch the provider. |
| **Agent Skills** | Skills are discovered and loaded into the system prompt. Provider-agnostic. |
| **CLI display** (spinner, truncation, styling) | All provider-agnostic. Keyed off `ContentBlock.Type`, which the Ollama client normalizes. |
| **Input editor** | No provider awareness at all. |

---

## Files Changed (minimum set)

| File | Change | Lines (est.) |
|---|---|---|
| `agent/providers/provider.go` | **NEW** — `Provider` interface definition | ~15 |
| `agent/providers/ollama.go` | **NEW** — `OllamaClient` + translation layer | ~250 |
| `agent/providers/types.go` | No change (types are already the canonical format) | 0 |
| `agent/providers/client.go` | No change (already satisfies `Provider` interface) | 0 |
| `agent/agent.go` | Change `apiClient` field type from `*providers.Client` to `providers.Provider`. Update `New()` to dispatch on `cfg.Provider`. | ~20 |
| `agent/compaction.go` | No change — already calls `a.apiClient.Call()` | 0 |
| `agent/config/config.go` | Add `Provider`, `OllamaBaseURL`, `OllamaModel` fields | ~10 |
| `cli/cli.go` | Parse `PROVIDER`, `OLLAMA_*` env vars. Skip API key check for Ollama. | ~15 |

**Total: 2 new files, 3 modified files, ~310 lines of new/changed code.**

---

## Translation Edge Cases & Decisions

### 1. Images

Claude supports `{type: "image", source: {type: "base64", ...}}` content
blocks. Ollama supports `images: [base64_data]` on user messages. The
`include_file` tool returns `IMAGE_LOADED:` markers that the agent loop
converts to image content blocks. The Ollama translator would need to convert
these to Ollama's `images` field on user messages.

**MVP decision:** Skip image support for Ollama initially. If an image block is
encountered, the Ollama client logs a warning and omits it. Most coding tasks
don't need images. This avoids a complex translation for the initial release.

### 2. Thinking signatures

Claude's thinking blocks carry cryptographic signatures that must be
round-tripped in conversation history. Ollama's thinking is a plain string
with no signature. The Ollama client:
- Sets `signature: ""` on thinking ContentBlocks it creates
- Strips thinking blocks from history when converting to Ollama format
  (Ollama doesn't accept thinking in input messages)

### 3. Multiple content blocks per message

Claude's assistant messages can contain multiple content blocks (text +
tool_use + thinking in one message). Ollama's messages have flat fields
(`content`, `tool_calls`, `thinking`). The Ollama client must:
- **Outbound:** Flatten `[]ContentBlock` → single Ollama message
- **Inbound:** Expand Ollama message → `[]ContentBlock`

### 4. Multiple tool calls per turn

Both Claude and Ollama support multiple tool calls in a single assistant
response. Claude groups all results in a single `role: "user"` message with
multiple `tool_result` blocks. Ollama expects separate `role: "tool"` messages
for each result. The translator handles this by splitting/merging during
conversion.

### 5. Prompt caching

Claude supports prompt caching (`CacheControl`, cache tokens in usage).
Ollama has no equivalent. The Ollama client simply doesn't set cache fields.
The diagnostic callback will show zero cache stats, which is fine.

### 6. MaxTokens

Claude requires `max_tokens` in every request. Ollama uses model defaults
unless overridden via `options.num_predict`. The Ollama client can optionally
pass this if `cfg.MaxTokens` is set, or omit it to use model defaults.

**MVP decision:** Omit `num_predict` — let Ollama use model defaults. Avoids
needing to know each model's context window size. Can be added later.

### 7. Context window / compaction threshold

Claude Opus has a 200K context window. Ollama models vary widely (4K–128K).
With `PROVIDER=ollama`, the user should set `CONTEXT_WINDOW_SIZE` in config
to match their model, or we default to a conservative value (e.g., 8192).

**MVP decision:** Add `CONTEXT_WINDOW_SIZE` as an env var (currently hardcoded
to 200000 in `loadAgentConfig`). When `PROVIDER=ollama`, default to 8192 if
not explicitly set.

### 8. Streaming

Both clients use `stream: false` for simplicity. The existing Claude client
doesn't stream. The Ollama client won't either. This can be revisited later
for better UX with slow local models.

---

## Testing Strategy

1. **Unit tests for the translation layer** (`providers/ollama_test.go`):
   - `TestConvertMessagesToOllama` — all message type conversions
   - `TestConvertToolsToOllama` — tool definition wrapping
   - `TestConvertOllamaResponse` — response normalization
   - `TestToolResultCorrelation` — tool_use_id → tool_name mapping
   - `TestSyntheticIDs` — ID generation and round-tripping

2. **Integration test with mock Ollama server**:
   - HTTP test server returning canned Ollama responses
   - Verify full agent loop: message → tool_call → tool_result → final response

3. **Existing tests unchanged**: All current tests use `providers.NewClient()`
   which still satisfies `providers.Provider`. No test modifications needed.

---

## Phase 2: Closing the Local-Model UX Gap

### Context

The MVP Ollama provider works but has significant UX gaps compared to both the
Claude provider and a reference implementation (`qwlyde` — a minimal single-file
Ollama coding agent). With local models generating at ~12–25 tok/s (vs ~50–80
tok/s for the Claude API), these gaps are amplified. Phase 2 addresses the three
concrete deficiencies identified by comparing the two implementations.

### Default Model Change: `qwen3.5:35b` → `qwen3.6:27b`

The default Ollama model changes from `qwen3.5:35b` (MoE, 35B total / 3B
active) to `qwen3.6:27b` (dense, 27B). Rationale from verified benchmarks:

| Metric | qwen3.5:35b (old) | qwen3.6:27b (new) | Delta |
|---|:---:|:---:|:---:|
| SWE-bench Verified | 69.2 | **77.2** | **+8.0** |
| Terminal-Bench 2.0 | 40.5 | **59.3** | **+18.8** |
| GPQA Diamond | 84.2 | **87.8** | **+3.6** |
| Q4_K_M weight size | 23 GB | **16.8 GB** | **−6.2 GB** |
| Intelligence tier | ≈ Claude Sonnet 4 | **≈ Claude Sonnet 4.5** | +1 tier |

The 3.6 dense model is smaller, faster (fewer total weights to read), and
dramatically better at agentic coding. On 48 GB M4 Pro hardware, it leaves
31 GB free for KV cache and OS — comfortable headroom for long sessions.

**Files to update:**
- `cmd/clyde-qwen/main.go` — default `OLLAMA_MODEL`
- `agent/agent.go` — doc comment example
- `agent/providers/ollama.go` — doc comment example
- `agent/providers/ollama_test.go` — test model strings
- `docs/design-ollama-provider.md` — this document

---

### User Stories

#### US-1: Streaming responses (token-by-token output)

**As a** developer using Clyde with a local Ollama model,  
**I want to** see the assistant's response appear token-by-token as it generates,  
**so that** I don't stare at a frozen spinner for 30–60 seconds while the model
produces a response.

**Current behavior:** Clyde's Ollama client sets `stream: false` and waits for
the complete response before displaying anything. With a 27B model at ~12 t/s,
a 500-token response means ~40 seconds of spinner before any text appears.

**Desired behavior:** Tokens stream to the terminal as they arrive (identical to
qwlyde's behavior). The spinner stops as soon as the first token arrives and
text flows in real-time. Tool calls are collected from the stream and dispatched
after the response completes.

**Acceptance criteria:**
- [ ] First token appears on screen within ~1 second of model starting generation
- [ ] Text tokens display incrementally (no buffering until completion)
- [ ] Tool calls embedded in the stream are collected and dispatched correctly
- [ ] Thinking content (if `think: true`) is collected and emitted via the
      existing `ThinkingCallback` before text tokens display
- [ ] Claude provider behavior is completely unchanged (still non-streaming)
- [ ] Session persistence still captures the full final response

**Technical approach:**

Add a `StreamCall` method to the `Provider` interface (or a `StreamingProvider`
optional interface to avoid breaking Claude):

```go
// StreamingProvider extends Provider with token-by-token streaming.
// Providers that don't support streaming simply don't implement it,
// and the agent falls back to Call().
type StreamingProvider interface {
    Provider
    StreamCall(
        systemPrompt string,
        messages []Message,
        tools []Tool,
        onToken func(text string),        // called per text token
        onThinking func(text string),      // called per thinking token
    ) (*Response, error)
}
```

The Ollama client implements `StreamingProvider` using Ollama's NDJSON streaming
(`stream: true`). Each line is a `chatStreamChunk` with partial content:

```go
type chatStreamChunk struct {
    Message struct {
        Content   string     `json:"content"`
        Thinking  string     `json:"thinking"`
        ToolCalls []toolCall `json:"tool_calls,omitempty"`
    } `json:"message"`
    Done bool `json:"done"`
}
```

The agent loop checks `if sp, ok := a.apiClient.(StreamingProvider); ok` and
uses `StreamCall` when available. The `onToken` callback wires to the CLI's
output (replacing the spinner with live text). The final assembled `*Response`
is identical to what `Call()` would return — all downstream code (history,
session persistence, compaction) is unaffected.

**Estimated effort:** Medium-high  
**Files changed:** `providers/provider.go`, `providers/ollama.go`, `agent/agent.go`, `cli/cli.go`

---

#### US-2: Auto-start Ollama and model verification

**As a** developer starting Clyde with `PROVIDER=ollama`,  
**I want** Clyde to automatically start Ollama if it's not running and verify
the model is pulled,  
**so that** I get a working session without manually running `ollama serve` in
another terminal or debugging cryptic connection errors.

**Current behavior:** If Ollama isn't running, the first API call fails with a
connection-refused error wrapped in a suggestion to run `ollama serve`. If the
model isn't pulled, Ollama returns a 404 that surfaces as a generic API error.

**Desired behavior:**
1. On startup (before the first API call), check if Ollama is reachable
2. If not, start `ollama serve` as a background process and wait for readiness
3. Query `/api/tags` to verify the configured model is available
4. If the model isn't found, print a clear message: `Model "qwen3.6:27b" not
   found. Run: ollama pull qwen3.6:27b`
5. All checks complete in <5 seconds when Ollama is already running

**Acceptance criteria:**
- [ ] `clyde-qwen` works from a cold start (Ollama not running) without manual intervention
- [ ] Model-not-found produces actionable error with the exact `ollama pull` command
- [ ] When Ollama is already running, preflight adds <500ms to startup
- [ ] Preflight timeout is configurable (default 15 seconds)
- [ ] Claude provider is completely unaffected (no preflight for `PROVIDER=claude`)

**Technical approach:**

Add a `Preflight() error` method to the Ollama client (not on the `Provider`
interface — this is Ollama-specific lifecycle, not an LLM call):

```go
func (c *OllamaClient) Preflight() error {
    // 1. Check connectivity: GET baseURL/
    // 2. If unreachable, exec "ollama serve" and poll for readiness
    // 3. Check model: GET baseURL/api/tags, search for c.modelID
    // 4. Return nil or descriptive error
}
```

The agent constructor (`agent.New`) or the CLI startup calls `Preflight()` when
the provider is Ollama. This is a one-time check, not per-call.

**Estimated effort:** Low  
**Files changed:** `providers/ollama.go`, `agent/agent.go` or `cli/cli.go`

---

#### US-3: Pass `num_predict` to control output length

**As a** developer using Clyde with a local Ollama model,  
**I want** the output token limit (`max_tokens`) to be forwarded to Ollama as
`num_predict`,  
**so that** the model doesn't generate runaway responses that waste time on
slow local hardware.

**Current behavior:** Clyde's Ollama client sends no `options` field. Ollama
uses the model's default `num_predict`, which varies by model and can be
unlimited. With a model generating at ~12 t/s, a 4000-token rambling response
takes over 5 minutes.

**Desired behavior:** The configured `MaxTokens` value (currently 64000 for
Claude, but should have an Ollama-appropriate default) is passed as
`options.num_predict` in the Ollama request. For local models, a sensible
default like 4096 tokens prevents runaway generation while allowing substantial
responses.

**Acceptance criteria:**
- [x] Ollama requests include `options.num_predict` set from configuration
- [x] Default `num_predict` for Ollama is 4096 (not Claude's 64000)
- [x] Configurable via `OLLAMA_NUM_PREDICT` environment variable
- [x] Claude provider is unaffected

**Technical approach:**

Add an `options` field to the Ollama request struct:

```go
type ollamaRequest struct {
    Model    string           `json:"model"`
    Messages []ollamaMessage  `json:"messages"`
    Tools    []ollamaTool     `json:"tools,omitempty"`
    Stream   bool             `json:"stream"`
    Think    bool             `json:"think,omitempty"`
    Options  *ollamaOptions   `json:"options,omitempty"`
}

type ollamaOptions struct {
    NumPredict int `json:"num_predict,omitempty"`
}
```

The `OllamaClient` stores a `numPredict` field set from config. If non-zero,
it's included in every request.

**Estimated effort:** Trivial  
**Files changed:** `providers/ollama.go`, `agent/agent.go`, `cli/cli.go`

---

### Implementation Order

| Priority | Story | Rationale |
|---|---|---|
| **1** | Default model → `qwen3.6:27b` | Zero-effort +8 SWE-bench points. Just string changes. |
| **2** | US-3: `num_predict` | Trivial to implement. Prevents the worst UX issue (runaway generation). |
| **3** | US-2: Preflight | Low effort. Eliminates the #1 source of "it doesn't work" confusion. |
| **4** | US-1: Streaming | Highest impact but highest effort. Transforms the UX from "frozen" to "live". |

---

## Future Work (out of scope for Phase 2)

- **Image support** for multimodal Ollama models
- **Context window auto-detection** — query `/api/show` for model metadata
- **Ollama-optimized system prompt** — shorter prompt for smaller context windows
- **Provider-specific compaction** — different phase count or token budgets
  for smaller models
- **OpenAI-compatible provider** — same interface, different translation
  (most of the Ollama work applies since Ollama's API is OpenAI-influenced)
- **Batched inference** — for parallel agent workflows on local hardware
  (requires vLLM/SGLang, not Ollama)

---

## Summary

The change is **surgical**: introduce a one-method `Provider` interface, write
one new translation client (~250 lines), and update three existing files with
~35 lines of changes. Everything else — tools, compaction, sessions, MCP,
skills, CLI display — works unchanged because the agent already communicates
through a well-defined boundary (`Call(systemPrompt, messages, tools) →
*Response`). The Ollama client is purely a protocol translator that maps
between Ollama's wire format and the existing Claude-canonical types.
