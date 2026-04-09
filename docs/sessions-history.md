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

2. **File contents = terminal output at debug level.** Running `clyde --debug 2>&1 | tee session.md`, stripping ANSI codes, splitting on message boundaries, and naming each piece by type would produce a valid session directory. At `--debug`, every line written to a session file is also emitted to the terminal. Lower verbosity levels hide content (signatures, tool metadata, diagnostics) but never add content — the file and the debug terminal are identical.

3. **Any text file parsable as a message log is resumable**, regardless of origin — compaction output, terminal pipe, hand-written, artificially constructed.

4. **File structure and filenames are meaningful** — they preserve time order, identify message types, and encode metadata. This is where non-content information lives.

5. **Concatenating message files in sorted order produces a valid log.** `cat *.md` in a session directory gives a coherent, readable, resumable conversation transcript.

6. **The input line and spinner are the only TUI elements not in the log.** The spinner must never display meaningful information that doesn't also appear in the permanent log.

---

## 4. Philosophy

### The file IS the terminal output

There is no "log format." Each message file contains exactly what the terminal would display for that message at debug verbosity — same markers, same ordering, same content, no truncation. If you ran `clyde --debug 2>&1 | tee session.md` and stripped ANSI codes, split on message boundaries, and named each piece by type, you'd have a valid session directory.

This means:
- The **file always captures at debug level** regardless of the user's terminal verbosity. A user running `--silent` still gets a full debug-level log on disk. You can always filter down when reading; you can never recover information that was never written.
- A user can stop a session at `--normal`, restart it at `--debug`, and see the same log on disk — only the terminal display changes, not the file.
- **ANSI color codes are stripped** from the file. The file is clean Markdown. Role markers (`**You:**`, `**Claude:**`) are already Markdown bold syntax — readable and distinctive without color. ANSI codes are added back on resume for terminal display.

### One file per message — the filesystem is the index

Each message, content block, or logical unit of output is written to its own file. The filename encodes the message type (`user`, `assistant`, `tool-use`, `tool-result`, etc.) and the timestamp provides natural ordering.

This means:
- **Message boundaries are explicit** — they're file boundaries, not regex matches inside a larger document.
- **Message types are explicit** — encoded in the filename, not inferred from content parsing.
- **Filtering is a glob** — `cat *_user.md` for user messages, `cat *_tool-result.md` for tool output. No custom parser needed.
- **Selective truncation is trivial** — truncate `*_tool-result.md` files for a compact view; leave `*_user.md` and `*_assistant.md` intact. Different log levels are just different glob patterns + different truncation policies applied at the `cat` step.
- **Reconstruction is deterministic** — read filenames, sort, group by type. No heuristics, no ambiguity, no LLM-assisted fallback needed.
- **Crash isolation** — corruption affects at most one file. The rest of the session is intact.

See [ITD-1: File Division & Naming](#itd-1-file-division--naming) for the full design rationale.

### Timestamps are the natural ordering — no sequence numbers

Filenames use timestamps as the sole ordering mechanism. There are no autoincrementing sequence numbers. See [ITD-1](#itd-1-file-division--naming) for why.

### Writing is frequent, resuming is rare (but both are cheap)

Every message is written to disk immediately (crash safety). Resuming happens infrequently. The write path must be trivial — create a file, write content, close. With per-message files, the resume path is also trivially cheap — list files, sort by name, read — but the write path remains the primary optimization target.

### Role markers use Markdown bold syntax

The terminal prints `You:` and `Claude:` with ANSI bold/color. The file writes `**You:**` and `**Claude:**` — Markdown bold syntax with ANSI stripped. This is a natural consequence of representing "bold" in Markdown instead of ANSI escape codes — the content is identical, only the bold encoding differs.

Role markers in file content serve **human readability** when files are concatenated (`cat *.md`). The filename suffix serves **machine readability** for filtering and reconstruction. These are complementary — the filename type is authoritative for parsing, the role marker is for reading.

### Metadata lives in the filesystem, not in file contents

Timestamps, usernames, message types, compaction boundaries — all encoded in directory names and filenames.

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
- `❌ Error: API error (status 400) ...` (errors persisted for the historical record)

### Thinking signatures

Thinking files include the API's cryptographic signature on a metadata line:

```
💭 Let me read the code...
signature: erEN8bJMAsENjQGFDb/TIX5H5KR2sT...
```

The signature is stable per thinking block — it doesn't change when subsequent messages are appended. It's a tamper-detection token the API uses to verify thinking content hasn't been modified. Including it in the file enables full-fidelity reconstruction: thinking blocks can be round-tripped through the API on resume. Without the signature, the API rejects the thinking block.

### Tool-use metadata

Tool-use files include the tool name and input JSON on metadata lines:

```
→ Reading file: agent/agent.go [toolu_abc123]
name: read_file
input: {"path":"agent/agent.go"}
```

This enriched format (SESS-2+) enables exact API reconstruction of tool_use blocks. Legacy SESS-1 files without metadata lines are handled via `inferToolName()` which maps display message prefixes to tool names.

### Tool-result IDs

Tool-result files include the tool_use_id they correspond to:

```
[toolu_abc123]
```
output content
```
```

This enables explicit matching between tool_use and tool_result blocks. Legacy files without the ID line fall back to order-based matching.

---

## 6. What a Session Log Looks Like

> **Note**: The example below is illustrative, not a pixel-perfect match to current terminal output. The exact formatting (blank line placement, fenced block style, diagnostic line content) will be finalized during implementation when we wire the dual write path. The key properties — role markers, tool IDs, diagnostic lines, compaction markers — are accurate to the design.

### Directory listing

A session directory after several turns with tool use and a compaction event:

```
.clyde/sessions/2026-07-14T09-32-00_aj/
├── 2026-07-14T09-32-00.000_user.md
├── 2026-07-14T09-32-03.412_thinking.md
├── 2026-07-14T09-32-03.789_tool-use.md
├── 2026-07-14T09-32-04.102_tool-result.md
├── 2026-07-14T09-32-04.300_diagnostic.md
├── 2026-07-14T09-32-07.556_assistant.md
├── 2026-07-14T09-32-20.100_user.md
├── 2026-07-14T09-32-22.834_thinking.md
├── 2026-07-14T09-32-23.001_tool-use.md
├── 2026-07-14T09-32-23.567_tool-result.md
├── 2026-07-14T09-32-24.012_tool-use.md
├── 2026-07-14T09-32-25.890_tool-result.md
├── 2026-07-14T09-32-26.100_diagnostic.md
├── 2026-07-14T09-32-30.445_assistant.md
├── 2026-07-14T09-32-30.500_compaction.md
├── 2026-07-14T09-32-35.000_system.md
├── 2026-07-14T09-32-45.200_user.md
└── ...
```

### Individual file contents

**`2026-07-14T09-32-00.000_user.md`**:
```markdown
**You:**

Implement the compaction trigger per CMP-1. The acceptance criteria are:
- Token counting tracks usage.input_tokens
- Threshold is context_window - reserve_tokens
- Trigger fires automatically before the next API call
```

**`2026-07-14T09-32-03.412_thinking.md`**:
```markdown
💭 Let me start by reading the current agent code to understand the token
tracking that already exists...
signature: erEN8bJMAsENjQGFDb/TIX5H5KR2sT...
```

**`2026-07-14T09-32-03.789_tool-use.md`**:
```markdown
→ Reading file: agent/agent.go [toolu_abc123]
name: read_file
input: {"path":"agent/agent.go"}
```

**`2026-07-14T09-32-04.102_tool-result.md`**:
````markdown
[toolu_abc123]
```
package agent

import (
    "fmt"
    "strings"
...
```
````

**`2026-07-14T09-32-04.300_diagnostic.md`**:
```markdown
🔍 Tokens: input=8234 output=1205 cache_read=7102 cache_create=0
💾 Cache: 7102/8234 tokens | Creation: 1132 tokens | Context: 4% (8234/200000)
```

**`2026-07-14T09-32-07.556_assistant.md`**:
```markdown
**Claude:**

I can see the agent already tracks `lastUsage` after each API call on line 262.
Here's my plan:

1. Add a `checkCompactionThreshold()` method...
2. Call it before each API call in the `HandleMessage` loop...
```

**`2026-07-14T09-32-30.500_compaction.md`**:
```markdown
🗜️ Compacting conversation history...
```

**`2026-07-14T09-32-35.000_system.md`**:
```markdown
**System:**

# Compaction Summary

## Original Mission
Implement the compaction trigger per CMP-1...

## Progress
- ✅ Added `checkCompactionThreshold()` to agent.go
- ✅ Token counting via `lastUsage.InputTokens`
...
```

### Concatenated view

`cat *.md` produces the same readable conversation transcript as the previous single-file design — role markers, tool output, diagnostics, compaction markers, all in order. The only difference is that the source is many files instead of one.

### Unix filtering examples

```bash
# Just user messages
cat *_user.md

# Just assistant text responses
cat *_assistant.md

# What tools were called? (just the → lines)
cat *_tool-use.md

# Tool output only (the verbose part)
cat *_tool-result.md

# Compact view: truncate only tool output
for f in *.md; do
  case "$f" in *_tool-result.md) head -25 "$f";; *) cat "$f";; esac
done

# How many tool calls?
ls *_tool-use.md | wc -l

# Token usage across the session
cat *_diagnostic.md

# Everything except diagnostics (conversation only)
cat *_user.md *_thinking.md *_tool-use.md *_tool-result.md *_assistant.md *_system.md

# Messages after compaction
ls *.md | sort | sed -n '/compaction/,$p' | tail -n +2 | xargs cat
```

---

## 7. Directory Structure & File Division

### Session directory

```
.clyde/sessions/
├── 2026-07-14T09-32-00_aj/
│   ├── 2026-07-14T09-32-00.000_user.md
│   ├── 2026-07-14T09-32-03.412_thinking.md
│   ├── 2026-07-14T09-32-03.789_tool-use.md
│   ├── ...
│   ├── 2026-07-14T10-45-00.000_compaction.md
│   ├── 2026-07-14T10-45-05.000_system.md
│   ├── 2026-07-14T10-45-10.000_user.md
│   └── ...
├── 2026-07-14T14-15-33_aj/
│   ├── 2026-07-14T14-15-33.000_user.md
│   └── ...
└── 2026-07-15T10-00-12_maria/
    ├── 2026-07-15T10-00-12.000_user.md
    └── ...
```

**Session directory name**: `<session-start-time>_<username>`
- Sorts lexicographically by time, then by user.
- Username from `git config user.name` (lowercased, spaces to hyphens), fallback to `$USER`.

**Message file name**: `<timestamp>_<type>.md`
- `<timestamp>`: When this message was written. ISO-8601 with milliseconds, hyphens for colons (e.g., `2026-07-14T09-32-05.123`). Provides natural lexicographic sort order.
- `<type>`: The message type. One of: `user`, `assistant`, `system`, `thinking`, `tool-use`, `tool-result`, `diagnostic`, `compaction`.

No sequence numbers. The timestamp is the sole ordering mechanism. See [ITD-1](#itd-1-file-division--naming).

### Message types

| Type | Filename suffix | Content | API mapping |
|---|---|---|---|
| `user` | `_user.md` | `**You:**` + text | User message, text content block |
| `assistant` | `_assistant.md` | `**Claude:**` + text | Assistant message, text content block |
| `system` | `_system.md` | `**System:**` + text | System message (compaction summaries) |
| `thinking` | `_thinking.md` | `💭` + thinking text + `signature: <sig>` | Assistant message, thinking content block (requires signature for API round-trip) |
| `tool-use` | `_tool-use.md` | `→ <tool>: <args> [<toolu_id>]` + `name: <name>` + `input: <json>` | Assistant message, tool_use content block |
| `tool-result` | `_tool-result.md` | `[<toolu_id>]` + fenced output body | User message, tool_result content block |
| `diagnostic` | `_diagnostic.md` | `🔍`/`💾`/`🔒`/`❌` lines | Not a message — metadata, skipped during reconstruction |
| `compaction` | `_compaction.md` | `🗜️ Compacting...` | Not a message — boundary marker, skipped during reconstruction |

### Compaction boundaries

Compaction is visible in the file listing as a type transition — a `compaction` file followed by a `system` file:

```
...
2026-07-14T10-44-59.500_assistant.md    ← last response before compaction
2026-07-14T10-45-00.000_compaction.md   ← 🗜️ boundary marker
2026-07-14T10-45-05.000_system.md       ← compaction summary
2026-07-14T10-45-10.000_user.md         ← first message after compaction
...
```

**Properties**:
- `cat *.md` = complete session log (valid, coherent, readable).
- For resume, the latest `*_system.md` file is the compaction context; load it plus all subsequent files. If no `*_system.md` exists, load all files.
- A session with no compaction events has no `compaction` or `system` files — just `user`, `assistant`, `thinking`, `tool-use`, `tool-result`, and `diagnostic` files.

### Branching

When resuming from another user's session (or an older session), the entire session directory is copied to a new directory:

```
# Maria's session
.clyde/sessions/2026-07-15T10-00-12_maria/
├── 2026-07-15T10-00-12.000_user.md
├── ...
└── 2026-07-15T11-30-45.000_system.md

# AJ resumes from Maria's session → full copy
.clyde/sessions/2026-07-16T09-00-00_aj_from_2026-07-15T10-00-12_maria/
├── 2026-07-15T10-00-12.000_user.md     # copied
├── ...
├── 2026-07-15T11-30-45.000_system.md   # copied
└── 2026-07-16T09-00-00.000_user.md     # AJ's continuation
```

The new directory name encodes the provenance: `<new-start-time>_<user>_from_<source-session-id>`. The copied files are untouched — AJ's new files have later timestamps and naturally sort after the copied ones.

When resuming your own most recent session, no copy is needed — you continue writing new message files to the same directory.

---

## 8. Session Lifecycle

### Creation

A new session is created when the REPL starts without `--resume`, or CLI mode runs a one-shot command.

1. Determine session location: `git rev-parse --show-toplevel` → `<repo>/.clyde/sessions/`, else `~/.clyde/sessions/`.
2. Determine user: `git config user.name` → lowercase, hyphens → fallback `$USER`.
3. Create `<session-location>/<timestamp>_<user>/`.
4. If `.clyde/sessions/` is new and inside a git repo, add `.clyde/sessions/` to `.gitignore`.

### Writing history

History is written **synchronously after each message**. For each message or content block, two outputs are produced from the same content:
- **Terminal**: Formatted at the user's chosen log level, with ANSI color codes and truncation.
- **File**: A new file in the session directory, formatted at debug level, ANSI stripped, no truncation. The filename is `<time.Now()>_<type>.md`.

Both are generated from the same underlying data — the file just gets more of it (full tool output, full thinking, diagnostics) and without styling.

Crash safety: every complete message is an independent file on disk. If the process dies, the last partial write (at most one incomplete file) is the only data at risk. All prior messages are intact in their own files.

### Compaction event

When compaction fires:
1. Write `<timestamp>_compaction.md` containing `🗜️ Compacting conversation history...`.
2. Run the compaction workflow, producing the handoff summary.
3. Write `<timestamp>_system.md` containing the compaction summary as a `**System:**` message.
4. The in-memory `a.history` is replaced with: pinned first message + compaction summary + recent kept messages.
5. Conversation continues — new turns write new message files to the same session directory.

### Completion

On clean exit (`exit`, Ctrl+D): print the session path alongside the existing goodbye message:

```
Goodbye!
Session saved: .clyde/sessions/2026-07-14T09-32-00_aj/
```

This gives the user an easy reference for resuming (`clyde -r 2026-07-14T09-32-00_aj`) or inspecting the log files.

On crash: the session directory contains all completed message files. At most one file may be incomplete. Resume handles this gracefully — it skips or truncates the last file if malformed.

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

1. Find the target session directory.
2. List all `*.md` files, sorted by filename (timestamps give chronological order).
3. Find the latest `*_system.md` file, if any — this is the most recent compaction summary.
4. Load from that `system` file forward (or from the beginning if no compaction has occurred).
5. Reconstruct `a.history` using the grouping rules in [section 12](#12-reconstruction-rules).
6. Continue — new messages write new files to the same directory.

**No regex parsing of message boundaries.** Message boundaries are file boundaries. Message types are filename suffixes. Content blocks are file contents. The entire reconstruction is deterministic — no heuristics, no ambiguity, no LLM-assisted fallback.

### `--resume` (no argument)

Finds the most recent session for the current user:
1. Determine session location (repo root or `~/.clyde/`).
2. Glob `<session-root>/*_<username>/`.
3. Sort by directory name (datetime prefix gives chronological order).
4. Pick the last one.
5. Reconstruct from the latest compaction point (or from the beginning).

### `--resume <session-id>`

Targets a specific session directory by name. If the session belongs to another user, the directory is copied (branching — see section 7) and the continuation happens in the new directory.

### `--sessions`

Lists sessions in reverse chronological order, derived entirely from the files:

```
Sessions in .clyde/sessions/:

  2026-07-15T10-00-12_maria   47 messages  "Implement CMP-1 trigger"
  2026-07-14T14-15-33_aj      12 messages  "Fix readline scroll bug"
  2026-07-14T09-32-00_aj      203 messages  "TUI-9 multiline input"

Use --resume <session-id> to resume, or --resume for your most recent.
```

All info derived from files:
- Message count: count `*.md` files in the directory (or count non-diagnostic/non-compaction files).
- Summary: first `*_user.md` file content (truncated), or the heading from the latest `*_system.md` compaction summary.

### CLI → TUI transition

Run a one-shot CLI command: `clyde "implement the compaction trigger"`. This creates a session directory with message files. Later, `clyde --resume` loads that session into the REPL and continues interactively.

---

## 10. Agent History Search

### No custom tool — system prompt guidance only

The agent searches its own history using existing tools. The system prompt includes:

```
CONVERSATION HISTORY:
Your session history is stored in:
  <session-path>/

Message files are named <timestamp>_<type>.md where type is one of:
user, assistant, system, thinking, tool-use, tool-result, diagnostic, compaction.

To search your current session:
  grep("pattern", "<session-path>/")

To read just your user messages:
  run_bash("cat <session-path>/*_user.md")

To read just tool output:
  run_bash("cat <session-path>/*_tool-result.md")

To search across ALL sessions:
  grep("pattern", ".clyde/sessions/", "*.md")

To find sessions by user:
  glob(".clyde/sessions/*_<username>/")
```

The `<session-path>` placeholder is replaced with the actual path at session creation.

### Cross-session search

All sessions are flat Markdown files in a known directory structure:
- *"How did we handle the migration last time?"* → `grep("migration", ".clyde/sessions/", "*.md")`
- *"What sessions has Maria run this week?"* → `glob(".clyde/sessions/2026-07-1*_maria/")`
- *"Find where we discussed the API redesign"* → `grep("API redesign", ".clyde/sessions/", "*.md")`
- *"How many tool calls were in the last session?"* → `run_bash("ls .clyde/sessions/2026-07-15T10-00-12_maria/*_tool-use.md | wc -l")`

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

## 12. Reconstruction Rules

Reconstruction reads message files in sorted order and groups them into API messages. The rules are deterministic — no heuristics, no regex, no ambiguity.

### File-to-API mapping

Files are consumed in timestamp (filename) order. A "pending message" accumulates content blocks until a type transition flushes it:

- **`user`** → Flush pending. New user message with text content block. Flush immediately.
- **`thinking`** → If file contains a `signature:` line, add thinking block (with signature) to pending assistant message. If no signature (legacy SESS-1 files), skip — the API requires signatures for round-tripping thinking blocks. The text is preserved on disk for human reading.
- **`tool-use`** → If pending is an assistant message, add tool_use block. Otherwise flush pending, start new assistant message with tool_use block. Tool name and input are extracted from `name:` and `input:` metadata lines; for legacy files, the tool name is inferred from the display message prefix. The `toolu_id` is extracted from the `→ ... [toolu_id]` line.
- **`tool-result`** → If pending is a user/tool-result message, add tool_result block. Otherwise flush pending, start new user message with tool_result block. The `tool_use_id` is extracted from the explicit `[toolu_id]` line (SESS-2 format) or matched by order from preceding tool-use files (legacy format).
- **`assistant`** → If pending is an assistant message (from preceding thinking/tool_use files), append text block and flush. Otherwise flush pending, new assistant message with text block, flush. This merging prevents consecutive assistant messages that violate the API's alternation requirement.
- **`system`** → Flush pending. Inject as user message (compaction summary) + assistant acknowledgment pair.
- **`diagnostic`** → Skip. Not conversation content. (Includes `❌ Error:` entries.)
- **`compaction`** → Skip. Boundary marker only.

### Trailing message trimming

After all files are processed, if the last message is a user message (plain text or tool_result), it represents an incomplete exchange — the user typed something but the process crashed or errored before getting a response. The message is trimmed from the API history to maintain user/assistant alternation. The file stays on disk as a permanent record. A warning is logged so the user knows to retype.

### Example reconstruction

Given these files:
```
T1_user.md          → USER msg: [text]                                   → flush
T2_thinking.md      → (start ASSISTANT msg) [thinking + signature]
T3_tool-use.md      → (accumulate) [tool_use toolu_1]
T4_tool-use.md      → (accumulate) [tool_use toolu_2]
T5_tool-result.md   → (flush ASSISTANT, start USER msg) [tool_result for toolu_1]
T6_tool-result.md   → (accumulate) [tool_result for toolu_2]
T7_assistant.md     → (flush USER, new ASSISTANT msg) [text]             → flush
```

Produces:
```
Message 1: {role: "user",      content: [{type: "text", ...}]}
Message 2: {role: "assistant", content: [{type: "thinking", ..., signature: "..."}, {type: "tool_use", id: "toolu_1"}, {type: "tool_use", id: "toolu_2"}]}
Message 3: {role: "user",      content: [{type: "tool_result", tool_use_id: "toolu_1"}, {type: "tool_result", tool_use_id: "toolu_2"}]}
Message 4: {role: "assistant", content: [{type: "text", ...}]}
```

This is the exact structure the Claude API expects.

---

## 13. Implementation Sequence

These will become user stories in `docs/todos.md` after review:

1. **Session infrastructure**: VCS-aware session location, directory creation, user identity, `.gitignore` defaults. Wire into CLI startup for both REPL and CLI mode.

2. **History persistence**: Dual write path — terminal at user's level, file at debug level (stripped of ANSI). One file per message/content block. Timestamp-based filenames with type suffix.

3. **Terminal output additions**: Tool use IDs on `→` lines. Compaction summary as `**System:**` message. Ensure all meaningful info is in the permanent log (not just the spinner).

4. **Session resume**: `--resume` and `--resume <id>`. Read message files, sort, group by type, reconstruct `a.history` per the reconstruction rules. Handle compaction-based resume (load from latest `system` file). Handle crash recovery (skip malformed last file). Handle cross-user resume (copy + branch).

5. **Session listing**: `--sessions` flag. Derive all info from files on disk.

6. **System prompt additions**: Tell the agent its session path, the file naming convention, and how to search/filter history with existing tools and globs.

7. **Security**: `.gitignore` defaults. Pre-commit hook. Documentation for shared history opt-in.

8. **Compaction integration** (delivered as part of CMP-1/CMP-2): Compaction marker file, system summary file. Wire into the session file lifecycle.

Stories 1–7 must land before CMP-1. Story 8 is part of CMP-1/CMP-2.

---

## ITD-1: File Division & Naming

**Incremental Technical Decision**
**Date:** July 2026
**Status:** Accepted

### Context

The original design (v1 of this document) used **one file per compaction segment**: all messages between compaction events were appended to a single `.md` file, with role markers (`**You:**`, `**Claude:**`) as in-content delimiters. Resume required regex parsing of the file to split on role markers and heuristically associate tool-use/tool-result pairs. The design explicitly acknowledged this was fragile — section 9 stated "the resume logic can make LLM calls to help reconstruct the message structure" for ambiguous cases.

Additionally, segment files used zero-padded sequence numbers (`001_`, `002_`, ...) as a sort-order prefix alongside timestamps.

Two questions prompted this revision:
1. Is there a performance/robustness benefit to one file per message vs. regex parsing of a single file?
2. Are sequence numbers necessary when timestamps already sort lexicographically?

### Alternatives Considered

#### A. One file per compaction segment, regex parsing (original design)

**Pros:**
- Fewer files (3–5 per session vs. hundreds).
- Simpler write path (append to open file descriptor).
- Each file is a complete, human-readable conversation fragment.
- Git-friendly (fewer files to track for teams that opt in).

**Cons:**
- **Parsing is heuristic.** Role markers can appear inside tool output. The `**` Markdown wrapping mitigates but doesn't eliminate ambiguity. The design itself acknowledged the need for LLM-assisted fallback.
- **Filtering requires parsing.** Extracting "just user messages" or "just tool output" requires understanding content structure — you can't do it with filesystem tools alone.
- **Selective truncation requires parsing.** You can't truncate tool output while preserving user messages without first splitting the file.
- **Resume is expensive.** O(n) content scan, regex matching, heuristic grouping, potential LLM calls.
- **Crash corruption is broad.** A partial write can corrupt the tail of a large file, making the entire segment ambiguous.

#### B. One file per message, sequence number + timestamp + type

Same as the chosen approach but with a zero-padded sequence prefix: `00001_2026-07-14T09-32-00.000_user.md`.

**Pros over A:** All the per-message benefits (filtering, robustness, trivial resume).

**Cons:**
- **Width problem.** How many digits? 3 is too few (>999 messages in long sessions is routine). 5 handles 99,999. 6 handles 999,999. Every choice is a guess about an upper bound that shouldn't need one.
- **Requires state.** Must track the current sequence number, persist it across writes, handle gaps/resets. The timestamp alone is stateless (`time.Now()`).
- **Redundant.** The sequence number's only job is ordering, which the timestamp already provides via ISO-8601 lexicographic sort.
- **Design smell.** An autoincrementing counter in a file-based log system is an artifact of database thinking. It imposes centralized state management where none is needed.

#### C. Hybrid — one file per turn

Group an entire assistant turn (thinking + tool calls + text response) into one file.

**Pros:** Fewer files (~100–200 per session). Some filename-based filtering (user vs. assistant turns).

**Cons:** Loses per-type filtering (can't separate tool output from assistant text). Still requires internal parsing for tool blocks within a turn. More complex write logic (must know when a turn ends). Halves the benefit for half the simplicity.

#### D. One file per message, timestamp + type only (chosen)

`2026-07-14T09-32-05.123_tool-use.md`

### Decision

**Option D**: One file per message/content block, with timestamp and type suffix as the filename. No sequence numbers.

### Rationale

**Per-message files eliminate the hardest problem in the original design.** Heuristic parsing with LLM fallback was acknowledged as necessary in the original design. Per-message files make it unnecessary. Message boundaries are file boundaries. Message types are filename suffixes. Reconstruction is a deterministic `readdir` + `sort` + `group-by-type`. No regex, no ambiguity, no fallback.

**Filename-based filtering is a strict superset.** Everything achievable with regex on a single file is achievable with globs on a directory — plus capabilities that are impossible with the single-file approach: `cat *_user.md`, `cat *_tool-result.md`, selective truncation by type, log-level filtering as glob patterns. This aligns with clyde's Unix philosophy (CLI composability, pipes, standard tools).

**Timestamps are the natural ordering.** Messages are written sequentially by a single-threaded agent loop. Each write follows an API call or user input that takes at minimum milliseconds. With millisecond-resolution ISO-8601 timestamps, collisions are structurally impossible in this architecture. The timestamp is stateless (`time.Now()`), unbounded (no width to guess), self-describing (tells you *when*, not just *what order*), and cross-referenceable (matches `git log --after`, server logs, session directory names).

**Sequence numbers are design smell.** They require state tracking, impose an artificial bound, and are redundant with the timestamp. The fact that you must choose a width (3 digits? 5? 6?) reveals that you're solving a problem that doesn't exist. The original 3-digit scheme (`001`–`999`) was adequate for compaction segments (rarely >10) but would overflow for individual messages (routinely >1000 in long sessions). Rather than increase the width, the correct response is to recognize that the autoincrement was never necessary.

**File count is not a concern.** A 100-turn session with tool use generates 500–1000 files. Modern filesystems handle this trivially (inode creation is microseconds). `ls` and `cat` on 1000 small files complete sub-second. Sessions are `.gitignore`d by default, so git noise only matters for teams that explicitly opt in. The write-path overhead (create file vs. append) is negligible against API call latency (1–10 seconds per round-trip).

**Crash safety improves.** Each message is an independent atomic write. Corruption is isolated to one file. In the single-file design, a crash mid-write could corrupt the tail of a segment containing hundreds of messages.

### Consequences

- Resume implementation is dramatically simpler — no regex parser, no heuristic grouper, no LLM fallback. Estimated implementation effort drops significantly.
- Session directories will contain hundreds of files for typical sessions. This is a feature, not a bug — each file is independently addressable and filterable.
- The `cat *.md` concatenation invariant is preserved — sorted filenames produce a valid, readable conversation transcript.
- Compaction boundaries are visible in file listings as type transitions (`compaction` → `system`) rather than as separate segment files.
- The system prompt for history search can teach the agent glob-based filtering patterns (`cat *_user.md`, `cat *_tool-result.md`) that are impossible with the single-file design.

### Monotonicity guarantee

In the unlikely event that a system clock adjustment causes `time.Now()` to return a timestamp ≤ the last written file's timestamp, the write path should detect this and bump the timestamp by 1 millisecond. This is simpler than maintaining a sequence counter and handles the edge case without adding persistent state:

```go
if now <= lastWritten {
    now = lastWritten + 1ms
}
```

This is an implementation detail, not a design concern — it's strictly simpler than autoincrement state management.
