# Clyde Progress

## Features Added

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

### Orphaned tool-use session files breaking --resume (2026-07-02)

**Problem:** Certain tool invocations wrote `*_tool-use.md` session files but never
wrote a corresponding `*_tool-result.md` file. This left orphaned tool-use records
that caused Claude API 400 errors on `--resume` (every `tool_use` must have a
matching `tool_result`).

**Two root causes:**
1. **include_file (image) — structural bug:** In `agent/agent.go`, the output callback
   (which writes `*_tool-result.md`) was guarded by
   `!strings.HasPrefix(resultContent, "Image loaded")`. Every image include_file call
   matched this prefix, so the callback never fired → no tool-result file ever written.
   100% reproducible for all image includes.
2. **Session interruption:** When a session is killed (Ctrl-C, crash) mid-tool-execution,
   the tool-use file exists (written at dispatch time) but no result comes back.

**Fix:**
- **agent/agent.go:** Removed the `HasPrefix("Image loaded")` guard on the output
  callback. The callback now fires for all results including image confirmations.
  The display layer can still suppress visual output; session persistence is unaffected.
- **agent/session/resume.go:** Added `synthesizeOrphanToolResults()` post-processing
  step in `ReconstructHistory` that detects orphaned `tool_use` blocks and injects
  placeholder `tool_result` messages (`"[Tool result unavailable — session was interrupted]"`).
  Handles three cases: (a) mid-conversation partial orphans (some tools have results,
  others don't), (b) end-of-session orphans, (c) mixed assistant messages (tool_use +
  text combined due to missing tool-result files) that need splitting into proper
  API message structure.

**Tests:** 3 new tests: `TestReconstructHistory_OrphanToolUse_MidConversation`,
`_EndOfSession`, `_AllOrphaned`. All 30 session resume tests pass.

**Documented in:** `docs/bug-orphaned-tool-use-files.md`

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

### Session Viewer (2026-07-09 – 2026-07-11)

Built a standalone web app for discovering and interacting with Clyde sessions across the machine.

**Architecture:**
- Go backend (Echo framework) with embedded static files, runs on `:8787`
- Single-page Alpine.js + DaisyUI frontend (CDN imports, no bundler)
- Tmux-based agent management: starts `clyde` in tmux for correct CWD + clean stop
- Session discovery: scans `~/code`, `~/Downloads`, `~` for `.clyde/` directories
- Process matching: `pgrep`/`lsof` for terminal-started sessions, tmux for viewer-managed

**Key Features:**
- Session list with filters (project, status, age), search, sort
- Message view with markdown rendering (marked.js), chat bubbles for user/assistant
- Collapsible thinking/tool-call blocks, plaintext tool results
- Message type toggles (user, assistant, thinking, tools, compaction, diagnostic)
- Reply box with ⌘+Enter, starts clyde agent if needed
- Agent busy detection (tmux pane output heuristic for `You:` prompt)
- Stop button for viewer-managed agents

**Performance:**
- Batch tmux lookup: single `tmux list-sessions` instead of N `has-session` calls (4.4s → 1.0s)
- 5s session list cache
- Server-side age filter (default 30 days: 112 sessions vs 915 total)
- Server-side pagination (default 100 messages, "Load older" button)

**Files:** `session-viewer/main.go` (~750 lines), `session-viewer/static/index.html` (~550 lines)
**Branch:** `session-viewer`
**Not in go.work** — build with `cd session-viewer && GOWORK=off go build`

### Terminal Process Takeover (2026-07-14)

**Problem:** When a clyde session is running in a plain macOS terminal (process_type `"sh"`),
sending a message from the session viewer would silently launch a *second* clyde process
in tmux on the same session directory. This created a split-brain: two processes interleaving
writes to the same session files with no coordination, compounding the file-collision bug.
The terminal user had no indication anything happened.

**Fix:**
- **Backend** (`session-viewer/main.go`): Added `findTerminalProcess()` to detect shell clyde
  processes on a session, and `killTerminalProcess()` using SIGTERM → 5s wait → SIGKILL.
  Modified `postSessionMessage()` to check for terminal processes before launching tmux.
  If found without `force: true`, returns 409 with `{"error": "terminal_process_running", "pid", "tty"}`.
  With `force: true`, kills the terminal process first, then proceeds normally.
- **Frontend** (`session-viewer/static/index.html`): `sendReply()` detects the
  `terminal_process_running` response and shows a DaisyUI modal warning the user that
  their terminal process will be killed. "Kill & Take Over" confirms with `force: true`;
  "Cancel" aborts. Escape key and clicking outside also dismiss.
