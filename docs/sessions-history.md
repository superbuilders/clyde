# Sessions & History — Design Document

**Date:** July 2026
**Status:** Pre-spec discussion document
**Prerequisite for:** Compaction (CMP-1, CMP-2, CMP-3)

---

## 1. Purpose

This document designs file-based session persistence and conversation history for Clyde. The goals are:

1. **Enable compaction** — compaction needs a persistent history to measure, summarize, and search.
2. **Enable session resume** — users can return to a previous session, or pick up a teammate's session from a compaction point.
3. **Enable history search** — the agent can `grep`/`read_file` its own past conversations using existing tools.
4. **Enable shared team history** — sessions can optionally be committed to git.

---

## 2. Current State

**Readline history** (`~/.clyde/history`):
- Past *user inputs* only (not full conversations).
- Used solely for up-arrow recall in the REPL. Stays as-is — different purpose, different location.

**In-memory conversation history** (`agent/agent.go`):
- `a.history []providers.Message` — the full conversation (user messages, assistant responses, tool use/results, thinking blocks).
- Lost when the process exits.

**No session concept**: No session ID, no session directory, no persistence to disk, no resume.

---

## 3. Invariants

Hard constraints. Everything else is negotiable.

1. **History is stored as plaintext `.md` files.**

2. **File contents = terminal output at debug level + optional extra blank lines.** Nothing can appear in the history file that doesn't appear in the terminal log. No hidden metadata, no structural delimiters, no encoded headers.

3. **Any text file parsable as a message log is resumable**, regardless of origin — compaction output, terminal pipe, hand-written, artificially constructed.

4. **File structure and filenames are meaningful** — they preserve time order, identify users, and encode metadata. This is where non-content information lives.

5. **Concatenating sequential segment files produces a valid log.** No matter how we divide the conversation into files, `cat`-ing them in order gives a coherent, resumable message log.

6. **The input line and spinner are the only TUI elements not in the log.** The spinner must never display meaningful information that doesn't also appear in the permanent log.

---

## 4. Philosophy

### The file IS the terminal output

There is no "log format." The history file contains exactly what the terminal would display at debug verbosity — same markers, same ordering, same content, no truncation. If you ran `clyde --debug 2>&1 | tee session.md` and stripped ANSI codes, that file would be a valid session log.

This means:
- The **file always captures at debug level** regardless of the user's terminal verbosity. A user running `--silent` still gets a full debug-level log on disk. You can always filter down when reading; you can never recover information that was never written.
- A user can stop a session at `--normal`, restart it at `--debug`, and see the same log on disk — only the terminal display changes, not the file.
- **ANSI color codes are stripped** from the file. The file is clean Markdown. Role markers (`**You:**`, `**Claude:**`) are already Markdown bold syntax — readable and distinctive without color. ANSI codes are added back on resume for terminal display.

### Writing is frequent, resuming is rare

Every message is written to disk immediately (crash safety). Resuming happens infrequently. Therefore:
- The **write path** must be trivial — append the same string sent to the terminal.
- The **resume path** can be expensive — heuristic parsing, regex, even LLM calls to resolve ambiguity.

### Role markers use Markdown bold syntax

The terminal prints `You:` and `Claude:` with ANSI bold/color. The file writes `**You:**` and `**Claude:**` — Markdown bold syntax with ANSI stripped. This is a natural consequence of representing "bold" in Markdown instead of ANSI escape codes — the content is identical, only the bold encoding differs.

This gives us regex disambiguation for free: a tool call might output a fragment of a log file containing literal `You:` or `Claude:` text. The `**` wrapping lets a parser distinguish role markers from quoted content — `grep "^\*\*You:\*\*"` matches only actual role markers, not incidental occurrences in tool output.

### Metadata lives in the filesystem, not in file contents

Timestamps, usernames, compaction boundaries, sequence numbers — all encoded in directory names and filenames.

### Per-project in VCS, global otherwise

- **Inside a git repo**: `.clyde/sessions/` at the repository root (alongside `.git/`).
- **Outside any git repo**: `~/.clyde/sessions/` in the user's home directory.

Detection: `git rev-parse --show-toplevel` for repo root. Fallback to `~/.clyde/`.

---

## 5. Terminal Output Additions

The log file is the terminal output at debug level. A few small additions to the current terminal output ensure the log contains enough information for resume. These appear at all log levels unless noted.

### Tool use IDs on `→` lines

Currently: `→ Reading file: agent/agent.go`
Proposed: `→ Reading file: agent/agent.go [toolu_abc123]`

The bracketed ID on every `→` line gives the resume parser the tool_use_id needed to reconstruct tool_result blocks for the API. Small, unobtrusive, present at all log levels.

### Compaction as a system message

When compaction fires, the terminal shows the compaction marker and then the full summary as a system message. At Normal level the summary may be truncated; at Verbose/Debug it's shown in full. The log file (debug level) always captures the full summary:

```
🗜️ Compacting conversation history...

**System:**

# Compaction Summary

## Original Mission
...

## Progress
...
```

### Debug diagnostics (already exist)

Already present at debug level, captured in the log:
- `🔍 Tokens: input=4523 output=892 cache_read=3715 cache_create=0`
- `💾 Cache: 3715/4102 tokens | Creation: 387 tokens | Context: 2% (4102/200000)`
- `🔒 Redacted thinking block (encrypted by safety system)`

---

## 6. What a Session Log Looks Like

> **Note**: The example below is illustrative, not a pixel-perfect match to current terminal output. The exact formatting (blank line placement, fenced block style, diagnostic line content) will be finalized during implementation when we wire the dual write path. The key properties — role markers, tool IDs, diagnostic lines, compaction markers — are accurate to the design.

This is both what the terminal shows at `--debug` and what the file contains:

```markdown
**You:**

Implement the compaction trigger per CMP-1. The acceptance criteria are:
- Token counting tracks usage.input_tokens
- Threshold is context_window - reserve_tokens
- Trigger fires automatically before the next API call

💭 Let me start by reading the current agent code to understand the token
tracking that already exists...

→ Reading file: agent/agent.go [toolu_abc123]

```
package agent

import (
    "fmt"
    "strings"
...
```

🔍 Tokens: input=8234 output=1205 cache_read=7102 cache_create=0
💾 Cache: 7102/8234 tokens | Creation: 1132 tokens | Context: 4% (8234/200000)

**Claude:**

I can see the agent already tracks `lastUsage` after each API call on line 262.
Here's my plan:

1. Add a `checkCompactionThreshold()` method...
2. Call it before each API call in the `HandleMessage` loop...

**You:**

Looks good, proceed.

💭 I'll implement the threshold check first, then wire it into HandleMessage...

→ Patching file: agent/agent.go (+45 bytes) [toolu_def456]

```
@@ applied patch @@
```

→ Running bash: go test ./tests/... -run TestCompaction [toolu_ghi789]

```
=== RUN   TestCompactionTrigger
--- PASS: TestCompactionTrigger (0.01s)
PASS
ok      github.com/this-is-alpha-iota/clyde/tests  0.034s
```

🔍 Tokens: input=12450 output=2301 cache_read=11200 cache_create=0
💾 Cache: 11200/12450 tokens | Creation: 1250 tokens | Context: 6% (12450/200000)

**Claude:**

The compaction trigger is implemented and tests pass. Here's what I did:
- Added `checkCompactionThreshold()` to agent.go
- Wired it into `HandleMessage()` before each API call
- Added unit test `TestCompactionTrigger`

🗜️ Compacting conversation history...
```

At this point, compaction fires. The `🗜️` marker is the last content in this segment file. The compaction summary becomes the start of the next segment file:

```markdown
**System:**

# Compaction Summary

## Original Mission
Implement the compaction trigger per CMP-1...

## Progress
- ✅ Added `checkCompactionThreshold()` to agent.go
- ✅ Token counting via `lastUsage.InputTokens`
- ✅ Configurable `RESERVE_TOKENS` (default 16000)
- ✅ Unit test passing

## Key Decisions
- Trigger checks BEFORE the API call, not after
- Reserve token default is 16000 (8% of 200K window)

## Current State
- Branch: `feature/compaction`
- Commit: `f4a3b2c` — "Add compaction trigger"
- All tests passing

## Next Steps
- Wire up actual summarization (currently a stub)
- Add integration test with real compaction cycle

## Critical Context
- The first user message is pinned and must survive compaction verbatim
- `a.history` replacement happens in `HandleMessage()` before the next API call

**You:**

Now let's add the integration test...
```

The conversation continues in this new file.

---

## 7. Directory Structure & File Division

### Session directory

```
.clyde/sessions/
├── 2026-07-14T09-32-00_aj/
│   ├── 001_2026-07-14T09-32-00.md
│   ├── 002_2026-07-14T10-45-00.md
│   └── 003_2026-07-14T12-30-00.md
├── 2026-07-14T14-15-33_aj/
│   └── 001_2026-07-14T14-15-33.md
└── 2026-07-15T10-00-12_maria/
    ├── 001_2026-07-15T10-00-12.md
    └── 002_2026-07-15T11-30-45.md
```

**Session directory name**: `<session-start-time>_<username>`
- Sorts lexicographically by time, then by user.
- Username from `git config user.name` (lowercased, spaces to hyphens), fallback to `$USER`.

**Segment file name**: `NNN_<segment-start-time>.md`
- `NNN`: Zero-padded sequence number (001, 002, ...). Ensures sort order.
- `<segment-start-time>`: When this segment started (ISO-8601, hyphens for colons).

### File division: compaction boundaries

Files are divided at compaction events:

- **File 001**: Starts with the first `**You:**` message. Ends with `🗜️ Compacting conversation history...` (or session end if no compaction occurs). This is the complete initial conversation.

- **File 002**: Starts with the compaction summary as a `**System:**` message. Continues with new conversation. Ends with the next `🗜️ Compacting...` marker (or session end).

- **File NNN**: Same pattern — opens with compaction summary, conversation follows, ends at next compaction or session end.

**Properties**:
- `cat *.md` = complete session log (valid, coherent, resumable).
- Any single file after 001 is a valid, self-contained starting point — it opens with compaction context.
- File 001 alone is a valid log of the initial conversation.
- A session with no compaction events has a single file: `001_<time>.md`.

### Branching

When resuming from another user's session (or an older session), the entire session directory is copied to a new directory:

```
# Maria's session
.clyde/sessions/2026-07-15T10-00-12_maria/
├── 001_2026-07-15T10-00-12.md
└── 002_2026-07-15T11-30-45.md

# AJ resumes from Maria's session → full copy
.clyde/sessions/2026-07-16T09-00-00_aj_from_2026-07-15T10-00-12_maria/
├── 001_2026-07-15T10-00-12.md     # copied
├── 002_2026-07-15T11-30-45.md     # copied
└── 003_2026-07-16T09-00-00.md     # AJ's continuation
```

The new directory name encodes the provenance: `<new-start-time>_<user>_from_<source-session-id>`. The copied files are untouched — the new file (003) is where the branch diverges.

When resuming your own most recent session, no copy is needed — you continue writing to the same directory.

---

## 8. Session Lifecycle

### Creation

A new session is created when the REPL starts without `--resume`, or CLI mode runs a one-shot command.

1. Determine session location: `git rev-parse --show-toplevel` → `<repo>/.clyde/sessions/`, else `~/.clyde/sessions/`.
2. Determine user: `git config user.name` → lowercase, hyphens → fallback `$USER`.
3. Create `<session-location>/<timestamp>_<user>/`.
4. If `.clyde/sessions/` is new and inside a git repo, add `.clyde/sessions/` to `.gitignore`.
5. Open `001_<timestamp>.md` for writing.

### Writing history

History is written **synchronously after each message**. Two outputs are produced from the same content:
- **Terminal**: Formatted at the user's chosen log level, with ANSI color codes and truncation.
- **File**: Formatted at debug level, ANSI stripped, no truncation.

Both are generated from the same underlying data — the file just gets more of it (full tool output, full thinking, diagnostics) and without styling.

Crash safety: every complete message is on disk. If the process dies, the last partial write (at most one message) is the only data at risk.

### Compaction event

When compaction fires:
1. Append `🗜️ Compacting conversation history...` to the current segment file. Close it.
2. Run the compaction workflow, producing the handoff summary.
3. Open the next segment file (`NNN+1_<timestamp>.md`).
4. Write the compaction summary as a `**System:**` message to the new file.
5. The in-memory `a.history` is replaced with: pinned first message + compaction summary + recent kept messages.
6. Conversation continues — new turns append to the new segment file.

### Completion

On clean exit (`exit`, Ctrl+D): print the session path alongside the existing goodbye message:

```
Goodbye!
Session saved: .clyde/sessions/2026-07-14T09-32-00_aj/
```

This gives the user an easy reference for resuming (`clyde -r 2026-07-14T09-32-00_aj`) or inspecting the log files. The segment file simply ends with the last message — no special marker needed.

On crash: the last segment file ends mid-conversation. Resume handles this gracefully — it parses what exists.

---

## 9. Session Resume

### CLI flags

```bash
# Start a new session (default)
clyde

# Resume the most recent session for the current user
clyde --resume
clyde -r

# Resume a specific session by ID (directory name)
clyde --resume 2026-07-14T09-32-00_aj
clyde -r 2026-07-14T09-32-00_aj

# List past sessions
clyde --sessions
```

### What resume loads

Resume works **the same way regardless of whether compaction has occurred**:

1. Find the target session directory.
2. Find the latest segment file (highest sequence number).
3. Parse the file to reconstruct `a.history`:
   - Split on role markers (`**You:**`, `**Claude:**`, `**System:**`).
   - Associate `→ ... [toolu_id]` lines and their following output blocks with tool_use/tool_result messages.
   - Ignore diagnostic lines (`🔍`, `💾`) — they're informational, not conversation content.
   - `💭` blocks map to thinking content blocks.
4. If the file starts with a `**System:**` compaction summary, that's the context. If it starts with `**You:**`, it's the original conversation.
5. Continue — new messages append to the same segment file (or the next one if compaction fires).

If parsing encounters ambiguity (complex interleaved tool chains, partial writes from a crash), the resume logic can make LLM calls to help reconstruct the message structure. This is acceptable — resume is infrequent and can absorb the cost.

### `--resume` (no argument)

Finds the most recent session for the current user:
1. Determine session location (repo root or `~/.clyde/`).
2. Glob `<session-root>/*_<username>/`.
3. Sort by directory name (datetime prefix gives chronological order).
4. Pick the last one.
5. Parse and resume from the latest segment file.

### `--resume <session-id>`

Targets a specific session directory by name. If the session belongs to another user, the directory is copied (branching — see section 7) and the continuation happens in the new directory.

### `--sessions`

Lists sessions in reverse chronological order, derived entirely from the files:

```
Sessions in .clyde/sessions/:

  2026-07-15T10-00-12_maria   2 segments  "Implement CMP-1 trigger"
  2026-07-14T14-15-33_aj      1 segment   "Fix readline scroll bug"
  2026-07-14T09-32-00_aj      3 segments  "TUI-9 multiline input"

Use --resume <session-id> to resume, or --resume for your most recent.
```

All info derived from files:
- Segment count: count `*.md` files in the directory.
- Summary: first `**You:**` message (truncated), or the heading from the latest `**System:**` compaction summary.

### CLI → TUI transition

Run a one-shot CLI command: `clyde "implement the compaction trigger"`. This creates a session with `001_<time>.md`. Later, `clyde --resume` loads that session into the REPL and continues interactively.

---

## 10. Agent History Search

### No custom tool — system prompt guidance only

The agent searches its own history using existing tools. The system prompt includes:

```
CONVERSATION HISTORY:
Your session history is stored in:
  <session-path>/

Segment files (NNN_<timestamp>.md) contain conversation transcripts.
Files after 001 start with a compaction summary.

To search your current session:
  grep("pattern", "<session-path>/")

To search across ALL sessions:
  grep("pattern", ".clyde/sessions/", "*.md")

To find sessions by user:
  glob(".clyde/sessions/*_<username>/")

To read a specific segment:
  read_file("<session-path>/001_2026-07-14T09-32-00.md")
```

The `<session-path>` placeholder is replaced with the actual path at session creation.

### Cross-session search

All sessions are flat Markdown files in a known directory structure:
- *"How did we handle the migration last time?"* → `grep("migration", ".clyde/sessions/", "*.md")`
- *"What sessions has Maria run this week?"* → `glob(".clyde/sessions/2026-07-1*_maria/")`
- *"Find where we discussed the API redesign"* → `grep("API redesign", ".clyde/sessions/", "*.md")`

---

## 11. Security — Secrets in History

### The problem

Conversation history contains tool outputs which may include environment variables, API keys, config files, connection strings, or private keys.

### Defense: Opt-out sharing via .gitignore (primary)

Sessions are gitignored by default. On first session creation inside a git repo, `.clyde/sessions/` is added to `.gitignore`. Teams that want shared history remove that line.

### Defense: Pre-commit hook for secret scanning

For teams that opt in, a pre-commit hook scans only `.clyde/sessions/**` files for known secret patterns:
- Key prefixes: `sk-ant-`, `sk-live-`, `AKIA`, `ghp_`, `glpat-`, `xox[bpsa]-`
- Patterns: `password=`, `secret=`, `token=`, `-----BEGIN.*PRIVATE KEY-----`

Lightweight, deterministic, no LLM call. Optional agentic enhancement for deeper scanning.

### Defense: Scrubbing

Edit the `.md` files directly (they're plain text), or use the agent to scrub them, or `git filter-repo` if already pushed.

---

## 12. Resume Parsing Rules

The resume parser reconstructs `a.history` from the segment file by splitting on role markers:

- `**You:**` → starts a new user message. Everything until the next role marker is the message content.
- `**Claude:**` → starts the text portion of an assistant response.
- `**System:**` → starts a system message (compaction summaries).
- `💭` → thinking content block (part of the assistant turn).
- `→ <tool>: <args> [<toolu_id>]` + following fenced output → tool_use + tool_result pair. Multiple `→` blocks before a `**Claude:**` are grouped into one assistant message (tool_use blocks) and one user message (tool_result blocks), matching the API structure.
- `🔍`, `💾`, `🔒` → diagnostic lines. Ignored during reconstruction (not conversation content).
- `🗜️` → compaction marker. Signals the end of the current segment.

This heuristic approach is sufficient for reconstruction. If edge cases arise (e.g., user input containing a role marker), an extra whitespace delimiter character can be added to the terminal output — which would then also appear in the log per the invariants — to disambiguate.

---

## 13. Implementation Sequence

These will become user stories in `docs/todos.md` after review:

1. **Session infrastructure**: VCS-aware session location, directory creation, user identity, `.gitignore` defaults. Wire into CLI startup for both REPL and CLI mode.

2. **History persistence**: Dual write path — terminal at user's level, file at debug level (stripped of ANSI). Write after each message. Handle segment file creation.

3. **Terminal output additions**: Tool use IDs on `→` lines. Compaction summary as `**System:**` message. Ensure all meaningful info is in the permanent log (not just the spinner).

4. **Session resume**: `--resume` and `--resume <id>`. Parse segment files (split on role markers, reconstruct messages). Handle compaction-based resume. Handle crash recovery. Handle cross-user resume (copy + branch). LLM-assisted parsing fallback for edge cases.

5. **Session listing**: `--sessions` flag. Derive all info from files on disk.

6. **System prompt additions**: Tell the agent its session path and how to search history with existing tools.

7. **Security**: `.gitignore` defaults. Pre-commit hook. Documentation for shared history opt-in.

8. **Compaction integration** (delivered as part of CMP-1/CMP-2): Compaction markers in the log. Close current segment, open new segment with summary. Wire into the segment file lifecycle.

Stories 1–7 must land before CMP-1. Story 8 is part of CMP-1/CMP-2.
