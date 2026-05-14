# Design: Ollama Provider Support

**Status:** Implemented  
**Branch:** `design/ollama-support`  
**Date:** 2025-07-20  
**Model:** `qwen3.5:35b` (via Ollama)  
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

## Future Work (out of scope for MVP)

- **Streaming responses** for better UX with slow local models
- **Image support** for multimodal Ollama models
- **Model auto-detection** — query `/api/tags` to validate model exists
- **Context window auto-detection** — query `/api/show` for model metadata
- **Ollama-optimized system prompt** — shorter prompt for smaller context windows
- **Provider-specific compaction** — different phase count or token budgets
  for smaller models
- **OpenAI-compatible provider** — same interface, different translation
  (most of the Ollama work applies since Ollama's API is OpenAI-influenced)

---

## Summary

The change is **surgical**: introduce a one-method `Provider` interface, write
one new translation client (~250 lines), and update three existing files with
~35 lines of changes. Everything else — tools, compaction, sessions, MCP,
skills, CLI display — works unchanged because the agent already communicates
through a well-defined boundary (`Call(systemPrompt, messages, tools) →
*Response`). The Ollama client is purely a protocol translator that maps
between Ollama's wire format and the existing Claude-canonical types.
