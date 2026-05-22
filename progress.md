# Clyde Progress

## Features Added

### Ollama Provider Support (2025-05-14)

**What:** Added Ollama as an alternative LLM backend, enabling local model usage
(e.g. qwen3.5:35b) alongside the existing Claude API. Separate `clyde-qwen` binary
pre-configured for Ollama with qwen3.5:35b.

**Architecture:**
- New `Provider` interface in `agent/providers/provider.go` — single method:
  `Call(systemPrompt, messages, tools) → (*Response, error)`.
- New `OllamaClient` in `agent/providers/ollama.go` (~300 lines) — translates
  between Claude-canonical types and Ollama's `/api/chat` wire format.
- Agent's `apiClient` field changed from `*providers.Client` to `providers.Provider`.
- `ExecutorFunc` in tool registry changed to accept `providers.Provider`.
- All 12 tool files + MCP register updated (signature change only).
- New `cmd/clyde-qwen/main.go` — sets PROVIDER=ollama, OLLAMA_MODEL=qwen3.5:35b
  as defaults, then delegates to `cli.Run()`.
- `Makefile` gains `build-qwen` target.

**Config (env vars / ~/.clyde/config):**
- `PROVIDER=ollama` — selects Ollama backend (default: claude)
- `OLLAMA_BASE_URL=http://localhost:11434` — Ollama server URL
- `OLLAMA_MODEL=qwen3.5:35b` — model name
- `CONTEXT_WINDOW_SIZE=N` — overrides default (200K for Claude, 8192 for Ollama)
- Config file is optional when PROVIDER=ollama (env vars suffice).

**Translation layer handles:**
- Messages: Claude content blocks ↔ flat Ollama messages
- Tool definitions: `input_schema` → `{type: "function", function: {parameters}}`
- Tool results: Claude `role:user + tool_result` → Ollama `role:tool`
- Thinking: Ollama `thinking` field → Claude thinking ContentBlock (signature="")
- Thinking/redacted_thinking blocks stripped from outbound history
- Synthetic IDs (`ollama_0`, `ollama_1`) for tool_use correlation
- System prompt: top-level string → prepended system message
- Usage: `prompt_eval_count`/`eval_count` → InputTokens/OutputTokens
- Stop reason: presence of tool_calls → "tool_use" vs "end_turn"

**MVP limitations (documented in design doc):**
- ~~No streaming (stream: false)~~ → Streaming implemented (Phase 2 US-1)
- ~~No auto-start / model verification~~ → Preflight implemented (Phase 2 US-2)
- No image support — `skipImages` flag on Agent silently drops image content
  blocks for non-multimodal providers; Ollama client omits any image blocks
  that slip through rather than inserting confusing placeholder text
- ~~No `num_predict` (uses Ollama model defaults)~~ → num_predict implemented (Phase 2 US-3)
- Cache stats always zero (Ollama has no prompt caching)

**Tests:** 31 unit tests in `agent/providers/ollama_test.go`:
- Tool conversion, message conversion (text, tool_use, tool_result, thinking)
- System prompt prepending, empty system prompt
- Response normalization (text, thinking, tool calls, synthetic IDs)
- Synthetic ID round-trip through tool_use → tool_result cycle
- Mock HTTP server integration (simple call, tool-use loop)
- Error handling (server error, connection refused)
- Compile-time Provider and StreamingProvider interface satisfaction checks
- Streaming: text-only, thinking+text, tool calls, tool-only, nil callbacks,
  empty stream, model fallback, multiple tool calls, request format verification
- Preflight: connectivity check (reachable, unreachable, non-OK status), model
  verification (exact match, :latest fallback, no fallback with tag, not found,
  empty list, server error, malformed JSON), timeout configuration, fast path
  timing, full integration (preflight → API call)

**What did NOT change:** Tool registry (tool implementations), system prompt,
session persistence, compaction, MCP/Playwright, Agent Skills, CLI display,
input editor. All existing tests pass unchanged.

### Ollama Streaming Responses — Phase 2 US-1 (2025-07-21)

**What:** Added token-by-token streaming for the Ollama provider, replacing the
"frozen spinner for 30-60 seconds" experience with real-time text output. Text
tokens flow to the terminal as they arrive from the model, identical to how
streaming works in commercial API providers.

**Architecture:**
- New `StreamingProvider` interface in `agent/providers/provider.go` — extends
  `Provider` with `StreamCall(systemPrompt, messages, tools, onText, onThinking)`
  that returns the same `*Response` as `Call()`.
- `OllamaClient` implements `StreamingProvider`. Uses Ollama's NDJSON streaming
  (`stream: true`) — reads chunks via `bufio.Scanner`, fires callbacks per token,
  accumulates content for the final assembled Response.
- Agent's `HandleMessage` checks `if sp, ok := a.apiClient.(StreamingProvider)`
  when `streamTextCallback` is set. Falls back to `Call()` for non-streaming
  providers (Claude) or when callbacks aren't configured.
- Two new agent callbacks: `StreamTextCallback` (per text token) and
  `StreamDoneCallback` (after streaming completes, for trailing newline).
- CLI wires callbacks in all 3 REPL/CLI modes. Tracks `streamedText` bool to
  avoid double-printing the response.

**Streaming behavior:**
- Spinner shows "Thinking..." during thinking token accumulation
- When first text token arrives: spinner stops, thinking emitted via
  ThinkingCallback (full text, for session persistence), then text streams
  token-by-token via StreamTextCallback
- StreamDoneCallback fires after each streaming API call (prints newline)
- Tool calls collected from stream, dispatched after stream completes
- Claude provider is completely unchanged (doesn't implement StreamingProvider)
- Session persistence captures the full final response (identical to non-streaming)
- Compaction uses `Call()` directly, never streams

**Key design decisions:**
- Optional `StreamingProvider` interface (not on base `Provider`) — avoids
  breaking any non-streaming provider. Agent uses type assertion at runtime.
- Thinking tokens accumulated silently during streaming, emitted as a single
  block when text starts. Spinner stays active during thinking (accurate UX).
- `streamedText` flag in CLI closures prevents double-printing.
- `StreamDoneCallback` ensures trailing newline before spinner/progress can
  overwrite the streamed text line.
- `bufio.Scanner` with 1MB buffer handles long NDJSON lines (tool call args).

**Files changed:**
- `agent/providers/provider.go` — added `StreamingProvider` interface
- `agent/providers/ollama.go` — added `StreamCall` method (~100 lines)
- `agent/agent.go` — added callbacks, streaming path in `HandleMessage`
- `cli/cli.go` — wired streaming in `runCLIMode`, `runREPLMode`,
  `runREPLModeWithSession`, updated `runREPLBasicMode` signature

**Tests:** 13 new streaming tests added to `agent/providers/ollama_test.go`:
- `TestStreamCall_TextOnly` — verifies token callbacks + assembled response
- `TestStreamCall_WithThinking` — thinking then text, correct ordering
- `TestStreamCall_WithToolCalls` — text + tool_calls, correct stop reason
- `TestStreamCall_ToolCallsOnly` — no text tokens emitted
- `TestStreamCall_NilCallbacks` — nil callbacks don't panic
- `TestStreamCall_ServerError` — HTTP error handling
- `TestStreamCall_EmptyStream` — done-only chunk, empty content
- `TestStreamCall_ModelFallback` — falls back to configured modelID
- `TestStreamCall_MultipleToolCalls` — synthetic ID generation
- `TestStreamCall_ConnectionRefused` — network error handling
- `TestStreamCall_RequestFormat` — verifies stream=true, model, think, tools
- `TestStreamingProviderInterfaceSatisfied` — compile-time check
- `TestClaudeNotStreamingProvider` — Claude does NOT implement streaming

### Ollama Auto-Start & Model Verification — Phase 2 US-2 (2025-07-21)

**What:** Added automatic Ollama server startup and model verification as a
preflight check. Running `clyde-qwen` from a cold start (Ollama not running)
now works without manual intervention. Model-not-found errors produce actionable
messages with the exact `ollama pull` command.

**Architecture:**
- New `Preflight()` method on `OllamaClient` — three-step check:
  1. `checkConnectivity()`: GET `baseURL/` to verify server is responding.
  2. `startOllamaServe()`: If unreachable, `exec.LookPath("ollama")` to find
     the binary, start `ollama serve` as a detached background process (own
     process group via `SysProcAttr.Setpgid`, stdout/stderr to `/dev/null`),
     then poll for readiness at 250ms intervals.
  3. `checkModel()`: GET `baseURL/api/tags`, search for the configured model.
     Supports exact match and `:latest` fallback (e.g. `qwen3.6` matches
     `qwen3.6:latest`). Error lists available models if not found.
- New `WithPreflightTimeout(d time.Duration)` method for configurable timeout
  (default 15 seconds).
- New `RunPreflight(cfg Config) error` exported function in `agent` package —
  the CLI calls this between `loadAgentConfig()` and `agent.New()`. Creates a
  temporary `OllamaClient` for the check; no-op for non-Ollama providers.
- CLI calls `RunPreflight` in all three startup paths: `runCLIMode`,
  `runREPLMode`, `runResumeMode`.

**Config:**
- `OllamaPreflightTimeout` field on `agent.Config` (defaults to provider's 15s)
- `OLLAMA_PREFLIGHT_TIMEOUT` env var in `~/.clyde/config` (integer seconds)

**Key design decisions:**
- Preflight runs in the CLI layer, not inside `agent.New()`, to avoid changing
  `New()`'s `*Agent` return signature. The `RunPreflight(cfg)` function keeps
  the CLI import to just the `agent` package.
- `ollama serve` is started with `Setpgid: true` so Ctrl+C on the terminal
  doesn't kill it — Ollama should persist as a server across sessions.
- Background process reaping via `go cmd.Wait()` prevents zombie processes.
- Model name matching: exact first, then `name:latest` fallback only when
  the configured model has no colon (tag). `qwen3.6:27b` will NOT match
  `qwen3.6:latest`.

**Files changed:**
- `agent/providers/ollama.go` — added `Preflight()`, `checkConnectivity()`,
  `startOllamaServe()`, `checkModel()`, `WithPreflightTimeout()`,
  `preflightTimeout` field, `ollamaTagsResponse`/`ollamaModelInfo` types
- `agent/agent.go` — added `RunPreflight()` function, `OllamaPreflightTimeout`
  config field, `time` import
- `cli/cli.go` — added `RunPreflight` calls in 3 startup paths, added
  `OLLAMA_PREFLIGHT_TIMEOUT` env var parsing, `time` import

**Tests:** 16 new preflight tests in `agent/providers/ollama_test.go`:
- `TestPreflight_ServerReachable_ModelFound` — happy path
- `TestPreflight_ServerReachable_ModelNotFound` — error has pull command + available models
- `TestPreflight_ServerReachable_NoModels` — empty model list
- `TestPreflight_ServerUnreachable` — auto-start path (exercises timeout)
- `TestCheckConnectivity_Reachable` — mock server responds 200
- `TestCheckConnectivity_Unreachable` — connection refused
- `TestCheckConnectivity_NonOKStatus` — 500 from server
- `TestCheckModel_ExactMatch` — direct model name match
- `TestCheckModel_MatchesLatestTag` — `qwen3.6` matches `qwen3.6:latest`
- `TestCheckModel_NoLatestFallbackWhenTagPresent` — `qwen3.6:27b` does NOT match `qwen3.6:latest`
- `TestCheckModel_NotFound_ShowsAvailable` — error lists all available models
- `TestCheckModel_EmptyResponse` — no models pulled at all
- `TestCheckModel_ServerError` — /api/tags returns 500
- `TestCheckModel_MalformedJSON` — /api/tags returns invalid JSON
- `TestWithPreflightTimeout` — default and overridden timeout values
- `TestPreflight_FastWhenAlreadyRunning` — <500ms with mock server
- `TestPreflight_FullIntegration_ToolCallAfterPreflight` — preflight then Call() works

### Ollama `num_predict` Output Limit — Phase 2 US-3 (2025-07-21)

**What:** Added `options.num_predict` support to the Ollama provider so the
configured max output tokens are forwarded to Ollama, preventing runaway
generation on slow local hardware. Default for `clyde-qwen` is 4096 tokens.

**Architecture:**
- New `numPredict` field on `OllamaClient`, set via constructor parameter.
- New `ollamaOptions` struct with `NumPredict` field (JSON: `num_predict`).
- New `buildOptions()` method returns `*ollamaOptions` if `numPredict > 0`,
  or `nil` to omit the `options` field entirely from JSON (clean requests
  when unconfigured).
- Both `Call()` and `StreamCall()` include `Options: c.buildOptions()` in
  the request body.
- `NewOllamaClient` gains a 4th parameter: `numPredict int`.

**Config:**
- `OllamaNumPredict` field on `agent.Config`
- `OLLAMA_NUM_PREDICT` env var in `~/.clyde/config` (positive integer)
- `cmd/clyde-qwen/main.go` sets default `OLLAMA_NUM_PREDICT=4096`
- Claude provider is completely unaffected (no `options` field in Claude requests)

**Key design decisions:**
- `numPredict=0` means "omit from request" (use Ollama model default), not
  "zero tokens." This matches Go's zero-value semantics and keeps backward
  compatibility.
- `buildOptions()` returns `nil` (not `&ollamaOptions{}`) when no options are
  set, leveraging `omitempty` on the JSON tag to keep the request clean.
- Default of 4096 for `clyde-qwen` is a pragmatic balance: large enough for
  substantial code generation responses, small enough to prevent the 5-minute
  runaway-generation problem described in the design doc.

**Files changed:**
- `agent/providers/ollama.go` — added `numPredict` field, `ollamaOptions` struct,
  `buildOptions()` method, updated `ollamaRequest` struct, updated `Call()` and
  `StreamCall()` request building, updated `NewOllamaClient` signature
- `agent/agent.go` — added `OllamaNumPredict` config field, updated provider
  construction in `New()` and `RunPreflight()`
- `cli/cli.go` — added `OLLAMA_NUM_PREDICT` env var parsing with validation
- `cmd/clyde-qwen/main.go` — added `OLLAMA_NUM_PREDICT=4096` default
- `agent/providers/ollama_test.go` — updated all 43 existing `NewOllamaClient`
  calls to 4-arg form, added 10 new US-3 tests

**Tests:** 10 new tests in `agent/providers/ollama_test.go`:
- `TestBuildOptions_ZeroNumPredict` — returns nil (omit options)
- `TestBuildOptions_NegativeNumPredict` — returns nil
- `TestBuildOptions_PositiveNumPredict` — returns populated options
- `TestNewOllamaClient_NumPredictStored` — constructor stores value
- `TestCall_NumPredict_IncludedInRequest` — mock server verifies options present
- `TestCall_NumPredict_OmittedWhenZero` — mock server verifies options absent
- `TestStreamCall_NumPredict_IncludedInRequest` — streaming includes options
- `TestStreamCall_NumPredict_OmittedWhenZero` — streaming omits options when 0
- `TestCall_NumPredict_JSONFormat` — verifies exact `{"options":{"num_predict":N}}`
- `TestOllamaOptions_JSONOmitEmpty` — verifies omitempty behavior on struct

### Agent Skills Support (2025-07-20)

**What:** Implemented the open Agent Skills standard (agentskills.io / SKILL.md format)
following Pi's philosophy — no new tools, no slash commands. Skills are discovered
automatically and the model loads them via the existing `read_file` tool.

**Architecture:**
- New `agent/skills/` package: `types.go` (SkillMetadata struct), `catalog.go`
  (discovery + YAML frontmatter parsing), `registry.go` (Registry + catalog builder).
- Discovery locations (in priority order): `./.agents/skills/` (project-local),
  `~/.agents/skills/` (user-global). Project-local wins on name conflicts.
- YAML frontmatter parsed from `SKILL.md` files (name, description, version, triggers).
  Missing/malformed frontmatter falls back to folder name with a logged warning.
- Catalog block (~100-300 tokens) is dynamically appended to the system prompt at
  startup. Zero overhead when no skills exist (empty string, no tokens).
- `Agent` struct gains `skillsRegistry` field and `ReloadSkills()` method.
- Added `gopkg.in/yaml.v3` dependency to the agent module.

**Key design decisions:**
- No new tools: model uses existing `read_file` to load full SKILL.md content.
- No `.clyde/` paths — only the universal `.agents/` standard.
- Catalog includes file paths so the model knows exactly what to `read_file`.
- `stripSkillsCatalog()` enables clean reload without prompt duplication.
- Skills enabled by default; zero-cost when no `.agents/skills/` dirs exist.

**Tests:** 20 unit tests in `agent/skills/skills_test.go` covering:
- Valid skill discovery, multi-skill, dedup (local wins over global)
- Fallbacks: no frontmatter, malformed YAML, empty name, empty frontmatter
- Edge cases: nonexistent dir, no SKILL.md, files-not-folders, extra YAML fields
- Registry lifecycle: build catalog, reload, no-skills empty block
- US-2 verification: paths are readable, catalog contains paths + read_file instruction

**User stories implemented:**
- US-1: Skills automatically discovered and catalogued at startup ✅
- US-2: Model can load and follow a skill using existing read_file tooling ✅

## Rewrites

### Input Editor: Drop chzyer/readline, Own the Terminal (2026-04-27)

**Motivation:** The `chzyer/readline` wrapper handled data capture (multiline
accumulation, history) but failed at display: readline is a single-line editor
and had no concept of a multi-line block. Navigating between lines caused visual
duplication. The wrapper required 7 atomic variables and a Listener/FuncFilterInputRune
dance across goroutines to intercept keystrokes. Unmaintained (last real commit 2022).

**What changed:**
- Replaced 1 file (517 lines wrapping readline) with 8 files (957 lines owning
  the terminal): `input.go` (editor), `keys.go` (key reader), `buffer.go` (line
  buffer), `history.go` (file-backed history), `display.go` (ANSI rendering),
  `rawmode_bsd.go` / `rawmode_linux.go` / `rawmode_other.go` (platform raw mode).
- Removed `chzyer/readline` dependency (was 5,425 lines of third-party Go).
  `golang.org/x/sys` promoted from indirect to direct (already in tree via readline).
- **Public API unchanged** — `cli/cli.go` required zero modifications.
- All 40 input tests pass (removed 6 metaCRReader tests that tested a now-obsolete
  internal; the key reader handles ESC+CR natively).

**Architecture:** Single-goroutine event loop. `readKey()` decodes stdin bytes into
logical key events (ESC sequences, UTF-8, Ctrl+X). The editor maintains a
`[]lineBuffer` with a virtual "new line" position. Display redraws the entire block
on each keystroke using ANSI escapes. No atomic variables, no goroutine
communication, no callback hacks.

**Design decisions:**
- `activeIdx` can be `len(lines)` (virtual new-line position). `activeLine()`
  materializes on demand; navigation away doesn't materialize empty lines. This
  matches the old system's behavior where phantom empty trailing lines were avoided.
- OPOST left enabled so `\n → \r\n` translation works for agent output between
  ReadLine calls. Only ICANON/ECHO/ISIG disabled.
- History file format: one entry per line (newlines in multiline entries span
  multiple lines). Matches old readline format for backward compatibility.

### CSI Parser Fix: Parameterized Escape Sequences (2025-07-20)

**Problem:** Down/Delete keys "sometimes malfunction" — only when modifier keys
(Shift/Ctrl/Alt) are held. When modifiers are held, terminals switch from simple
sequences (`ESC[A`) to parameterized ones (`ESC[1;5A`). The initial `readCSI()`
dispatched on the first byte after `ESC[`, which broke when that byte was a digit
(parameter) instead of a letter (final byte).

**Two failure modes:**
- Parameterized arrows (`ESC[1;5A`): `1` didn't match any case → arrow silently
  swallowed (key lost)
- Parameterized tilde sequences (`ESC[3;2~`): `3` matched Delete, consumed `;`
  instead of `~` → `2~` leaked as typed characters into the input

**Fix (`cli/input/keys.go`):** Rewrote `readCSI()` to follow the standard CSI
format: consume all parameter bytes (digits + semicolons) first, then dispatch on
the final byte. Also added tilde-terminated mappings for Home (`ESC[1~`, `ESC[7~`),
End (`ESC[4~`, `ESC[8~`), and Delete (`ESC[3~`) used by rxvt and older xterm modes.
Modifier values in parameters are consumed but ignored (Ctrl+Up = Up), matching
typical shell behavior.

**Tests:** Added 4 regression tests: `TestReadLine_ParameterizedUpArrow`,
`TestReadLine_ParameterizedDownArrow`, `TestReadLine_ParameterizedDelete`,
`TestReadLine_TildeHomeEnd`.

**LOC summary:** 984 lines total across 8 files (was 957 pre-fix), 45 test
functions in 1475 lines (was 41 in 1342 pre-fix).

### Line-Wrap Duplication Fix in Display (2025-07-20)

**Problem:** When typing a line long enough to wrap past the terminal width,
every subsequent keystroke duplicated the entire editing block one row further
down the screen. The display became increasingly garbled as typing continued.

**Root cause:** `redraw()` tracked `cursorRow` as a *logical line index*
(0 = first line, 1 = second line, etc.) but the terminal cursor moves in
*physical rows*. When content wraps past `termWidth`, a single logical line
occupies multiple physical rows. The code moved up by `cursorRow` rows to
reach the top of the editing block, but the terminal cursor was further down
than that — so each redraw started one physical row too low, printing a
duplicate below the previous content.

Three specific sub-bugs:
1. `\033[2K` (clear line) cleared one physical row per logical line — didn't
   clear extra rows created by wrapping.
2. Cursor-up/down movement used logical line counts, not physical row counts.
3. `cursorRow` was set to logical `activeRow`, not the physical row offset.

**Fix (`cli/input/display.go`):**
- `cursorRow` now tracks physical terminal rows, not logical line indices.
- Added `physRowCount(width, termWidth)` helper: `ceil(width / termWidth)`,
  returning 1 for content that fits one row (and for non-TTY where termWidth=0).
- Replaced per-line `\033[2K` with a single `\033[J` (clear to end of screen)
  after moving to the top of the block — correctly cleans up any number of
  wrapped physical rows.
- Cursor positioning after redraw computes the physical row within the active
  line based on `cursorOffset / termWidth`.
- `finishDisplay()` updated to use physical row counts for cursor-down movement.
- Deferred-wrap edge case (cursor at exact `termWidth` boundary) handled:
  cursor stays at end of previous physical row rather than jumping to column 0
  of a phantom next row.

**Non-TTY backward compatibility:** When `termWidth=0` (testing/non-interactive),
`physRowCount` returns 1 for every line, so all physical-row math degenerates to
the old logical-line math. Existing tests unaffected.

## Bugs Fixed

### run_bash hangs on backgrounded processes (2025-07-20)

**Problem:** When the model runs a command with `&` (e.g. `python3 -m http.server &`),
`CombinedOutput()` blocks indefinitely because the backgrounded child inherits
stdout/stderr pipes, keeping them open even after the parent bash exits. The tool
call never returns, eventually stalling the entire agent session.

**Fix (`agent/tools/run_bash.go`):**
- Replaced `CombinedOutput()` with `Start()` + manual `Wait()` in a goroutine
- Added 120-second timeout via `context.WithTimeout` + `exec.CommandContext`
- Set `SysProcAttr.Setpgid = true` to put bash and all children in their own
  process group
- On timeout, kill the entire process group: `syscall.Kill(-pid, SIGKILL)` —
  ensures orphaned children from `&` are cleaned up, not just the parent bash
- Timeout error message explicitly tells the model to use tmux for background
  processes: includes example `tmux new-session -d -s myserver '<command>'`
- Tool description updated with tmux guidance and warning against `&`

**Why 120 seconds:** Ollama inference is slow (~0.3 lines/sec for qwen3.5:35b
at Q4_K_M). Commands that involve waiting for model responses (e.g. nested clyde
calls) legitimately need this much time. The timeout message is the teaching
mechanism — models read tool descriptions and error messages, so they self-correct
after one timeout.

### Image data confuses Ollama / non-multimodal providers (2025-07-20)

**Problem:** Playwright MCP screenshot tool injects base64 PNG data into
conversation as image content blocks. When sent to qwen3.5:35b (text-only),
the model produces 100+ line monologues about whether it can see images,
derailing the task.

**Fix (two layers):**
1. `agent/agent.go`: Added `skipImages` bool field, set to `true` when
   `cfg.Provider == "ollama"`. When set, `pendingImages` are silently dropped
   from tool results instead of being appended to the conversation.
2. `agent/providers/ollama.go`: Image content blocks that slip through
   (e.g. from session replay) are silently omitted rather than inserting
   a confusing `[Image content not supported]` placeholder that itself
   prompted the model to comment at length about images.

**Design choice:** Config-flag approach (`skipImages`) rather than a provider
interface method (`SupportsImages() bool`) — simpler, and the flag is set once
in `New()` based on `cfg.Provider`. Future non-multimodal providers benefit by
extending the condition.

### Compaction loop with small Ollama context windows (2025-05-14)

**Problem:** When using `clyde-qwen` (Ollama provider), the agent entered an
infinite compaction loop — compacting after every single API response, never
making progress on the actual task. Observed in a Nim game build session:
14 compaction events, 3 compaction cycles, handoff docs growing from 3.6KB
to 6.3KB, and input tokens climbing (6929 → 7916 → 10697 → 11395).

**Root cause:** `CONTEXT_WINDOW_SIZE` defaulted to 8192 for Ollama, but
`DefaultReserveTokens` is 16000. The compaction threshold =
`contextWindowSize - reserveTokens` = 8192 - 16000 = **-7808**. Every
response with *any* tokens exceeded -7808, so `ShouldCompact()` always
returned true. Each 5-phase compaction generated a ~3-6KB handoff summary
that *itself* consumed context, making the next threshold check even worse.

**Fix (two parts):**
1. `cli/cli.go`: Ollama default `CONTEXT_WINDOW_SIZE` → 131072 (qwen3.5:35b
   supports 262K natively; 128K is conservative)
2. `agent/compaction.go`: Safety clamp in `ShouldCompact()` — `reserveTokens`
   capped at `contextWindowSize / 2` so the threshold can never go negative,
   regardless of user configuration

**Lesson:** When adding new providers with different context windows, always
verify that `DefaultReserveTokens` (16000, sized for Claude's 200K) doesn't
create impossible thresholds. The safety clamp prevents this class of bug
for any future provider or config combination.

### Brave Search 429s on concurrent requests (2025-07-17)

**Problem:** When multiple `web_search` tool calls fire in the same turn (parallel
execution), all requests hit the Brave API simultaneously. Brave's free tier
rate-limits to ~1 query/second, so only the first request succeeds and the rest
get 429'd.

The original 429 error message was also misleading — it claimed "You've reached
your monthly search limit (2000 free searches)" regardless of whether the 429 was
from per-second throttling or actual quota exhaustion. (The free tier is actually
~1,000 searches/month via $5 of credits, not 2,000.)

**Fix (`agent/tools/web_search.go`):**
- Added retry loop with exponential backoff (up to 3 retries: 1s, 2s, 4s) on 429
  responses. This handles the common concurrent-search case transparently.
- Updated the 429 error message (when retries are exhausted) to accurately
  distinguish per-second rate limiting from monthly quota issues, and points to the
  Brave dashboard for usage checking.
- Worst-case adds ~7s latency per search if all retries fire, but in practice most
  concurrent searches succeed on the first 1s retry.

**Root cause analysis:** The issue was diagnosed by observing that 1 of 4
simultaneous searches succeeded while 3 failed, and subsequent individual searches
worked fine — ruling out monthly quota exhaustion.
