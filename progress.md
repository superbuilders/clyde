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
