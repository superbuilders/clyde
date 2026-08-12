# Compaction Design — v4: Paging + Progressive Summarization

**Status:** Active design — replaces the 3-call pipeline  
**Date:** July 2026

## Problem Statement

Compaction replaces conversation history with a smaller equivalent when the
context window fills up. The 3-call pipeline (v3) has two critical problems:

1. **The current objective gets lost.** The first user message is "sacred" —
   pinned verbatim in position 1, the highest-attention slot. The current
   objective is buried inside a `[System: Compaction Summary]` message. The
   model treats the pinned message as the real request and the summary as
   metadata. After compaction, the model frequently reverts to the original
   goal instead of continuing the evolved objective.

2. **Unnecessary LLM calls.** Tool results dominate context usage. In a
   typical coding session, 80-95% of tokens are tool outputs (file reads,
   command outputs, search results). Summarizing these with LLM calls is
   slow, expensive, and lossy — when the raw content already lives on disk
   as session files.

## Core Insight

Tool results are already persisted as `*_tool-result.md` files in the session
directory. Tool inputs for mutation tools are persisted as `*_tool-use.md`
files. **Compaction can "page out" tool content by replacing it with file
references.** The model retains awareness of what tools were called and can
`read_file` any session file to page the content back in if needed.

This turns compaction from a **lossy compression** problem into a **reversible
paging** problem — analogous to virtual memory, where the context window is
RAM and the session directory is disk.

LLM summarization is only needed as a last resort, when the user/assistant
text messages themselves exceed the context window.

## Philosophy

- Compaction is **paging**, not summarization. Prefer mechanical linking over
  LLM calls in all cases.
- **Zero information loss** in the common case. Linked content is still
  accessible via `read_file`.
- **No sacred messages.** Every user message has equal status. The most recent
  messages naturally receive the most attention due to recency bias.
- Git is the source of truth for code state.
- LLM involvement is a last resort, not the primary mechanism.

## Algorithm

Compaction triggers when `inputTokens > contextWindowSize - reserveTokens`
(default `reserveTokens` = 16,000). It always triggers mid-turn — the agent
is between tool calls when the threshold is exceeded.

The algorithm proceeds through escalating steps. Each step checks whether the
resulting context fits within the usable window. If it fits, stop. If not,
proceed to the next step.

### Step 0: Link Prior Tool Content

Replace tool content from **all turns before the current agent turn** (i.e.,
before the last user message):

- **All tool results** (`*_tool-result.md`): replace content with session file
  path. Applies to every tool type — `read_file`, `run_bash`, `grep`, `glob`,
  `list_files`, `browse`, `web_search`, etc.

- **Tool calls for bash, write, patch, multi_patch**: replace the full
  `tool_use` input with just the terse descriptor (already the first line of
  the `*_tool-use.md` file). All other tool call types (read_file, grep, etc.)
  have small inputs and are kept as-is.

**What the model sees after linking:**

Before:
```
assistant: [tool_use: run_bash] {"command": "go test ./... 2>&1"}
user:      [tool_result] FAIL TestAuth ... (500 lines of output)
```

After:
```
assistant: [tool_use: run_bash] → Running bash: go test ./... 2>&1
user:      [tool_result: .clyde/sessions/.../16-14-02_tool-result.md]
```

Before (mutation tool):
```
assistant: [tool_use: write_file] {"path": "main.go", "content": "package main\n...300 lines..."}
user:      [tool_result] File written successfully.
```

After:
```
assistant: [tool_use: write_file] → Writing file: main.go (7.3 KB)
user:      [tool_result: .clyde/sessions/.../16-14-03_tool-result.md]
```

**If the context fits after this step → done.** This handles **Case A** (the
vast majority of sessions): multi-turn conversations where tool outputs
dominate context usage. All user/assistant text is preserved. No LLM calls.

### Step 1: Link Current Turn's Tools (Progressive)

If Step 0 wasn't enough, also link tool content from the **current agent turn**
(after the last user message). But preserve the most recent tools — these are
the ones the agent is actively working with.

Start with `N = 5` (keep the last 5 tool call/result pairs from the current
turn unlinked). Link everything older in the current turn.

If still too large, decrement: `N = 4`, `N = 3`, ... `N = 0`.

**If the context fits at some N → done.** This handles **Case B**: a single
user message followed by enough tool calls to fill the context window. The
model retains the original prompt and its most recent tool interactions.

### Step 2: Summarize Middle Messages

All tool content is now linked (Step 0 + Step 1 with N=0). If the context
STILL doesn't fit, the user/assistant text messages themselves are too large.
This is **Case C** — rare for coding agents, common for conversational
sessions with long exchanges.

Divide the message history into thirds by exchange count (an "exchange" is a
user message plus all subsequent messages until the next user message):

```
exchanges:  [  0  |  1  |  2  |  3  |  4  |  5  |  6  |  7  |  8  ]
            |<--- head --->|<----- middle ----->|<----- tail ------>|
            |  preserve    |    summarize       |    preserve       |
            |  [0..n)      |    [n..m)          |    [m..len)       |
```

Initial values: `n = len/3`, `m = 2*len/3` (summarize the middle third).

The summary is generated by a **single LLM call** and injected as a
`[System:]` message replacing the middle exchanges:

```
[0..n)  preserved user/assistant exchanges (real messages)
        [System: Summary of exchanges n through m-1]
[m..len) preserved user/assistant exchanges (real messages)
```

If the summary + preserved head + preserved tail still doesn't fit, widen
the summary window: decrement `n`, increment `m`. Repeat until it fits.

At the extreme: `n = 1`, `m = len - 1` — preserve only the first exchange
and the last exchange, summarize everything between.

**If it fits at some n, m → done.** The model sees the opening context, a
summary of the middle, and the full recent context including the current goal.

### Step 3: Preserve Only Last User Message

If even `n = 1, m = len - 1` doesn't fit (the first exchange or the summary
itself is very large), drop to the minimum viable context:

- Summarize **everything** before the last user message into a single
  `[System:]` message
- Preserve **only the last user message** verbatim

The last user message IS the current goal. The summary provides background.

**If it fits → done.**

**If it doesn't fit → error.** The last user message itself exceeds the usable
context window. There is nothing to compact; the prompt is too large.

## Cases Solved

| # | Scenario | Steps Used | What Model Sees |
|---|----------|-----------|-----------------|
| 1 | Single prompt, long tool-use run | 0 → 1 | Prompt + last N tool results + linked history |
| 2 | Multi-turn then long run | 0 | All messages + linked old tools + full current tools |
| 3 | Multi-turn, moderate tools per turn | 0 | All messages + linked tools (plenty of room) |
| 4 | Multi-turn, heavy message content | 0 → 2 | Head exchanges + summary + tail exchanges |
| 5 | One huge message + many others | 0 → 2 → 3 | Summary + last user message |
| 6 | Last prompt exceeds context | Error | N/A |

## Why This Doesn't Lose the Goal

The current v3 design loses the goal because the "sacred" first message sits
in position 1 (maximum primacy bias) as a real user message, while the current
objective is buried in a synthetic `[System: Compaction Summary]`. The model
treats the first message as the real request.

This design eliminates the problem at every step:

**Steps 0–1 (common path):** No summarization at all. Every user message is
preserved verbatim in its original position. The model sees the full evolution
of the user's intent — including "actually, stop X and do Y instead." Nothing
to regress to because nothing was removed except tool output blobs.

**Step 2 (rare):** The most recent messages (including the current goal) are
preserved as **real messages at the tail** — high recency bias, full
conversational weight. The summary is in the **middle** — the lowest-attention
region of the context window. The attention gradient matches the relevance
gradient: recent = important = high attention; old middle = stale = low
attention.

**Step 3 (extreme):** The last user message IS the goal. It's the only real
message. Impossible to regress.

There is no "sacred first message" competing with the current objective. No
synthetic injection fighting with real messages for attention. The model sees
real conversation history with the natural recency gradient intact.

## Linking Format

### Tool Results (all types)

The `tool_result` content block's text is replaced with the session file path:

```
.clyde/sessions/<session-dir>/<timestamp>_tool-result.md
```

The model can `read_file` this path to retrieve the full output if needed.

### Tool Calls (bash, write_file, patch_file, multi_patch only)

The `tool_use` content block is replaced with the terse descriptor from the
first line of the `*_tool-use.md` file. These descriptors already exist:

| Tool | Terse Descriptor |
|------|-----------------|
| `run_bash` | `→ Running bash: git status && echo "---"` |
| `write_file` | `→ Writing file: tests/foo_test.go (7.3 KB)` |
| `patch_file` | `→ Patching file: agent/agent.go (+323 bytes)` |
| `multi_patch` | `→ Multi-patching: 3 files` |

Tool calls for `read_file`, `grep`, `glob`, `list_files`, `browse`,
`web_search`, and other tools are **not linked** — their inputs are already
small (a path, a pattern, a URL) and serve as useful context about what
information was gathered.

### Tool Call + Result Pairing

After linking, a typical tool exchange looks like:

```
assistant: [tool_use: read_file] {"path": "src/auth.go"}
user:      [tool_result: .clyde/sessions/.../16-14-02_tool-result.md]
```

The model knows: "I read src/auth.go. The result is on disk if I need it."

For mutation tools:

```
assistant: [tool_use: write_file] → Writing file: src/auth.go (2.1 KB)
user:      [tool_result: .clyde/sessions/.../16-14-03_tool-result.md]
```

The model knows: "I wrote to src/auth.go. Both the content I wrote and the
result are on disk."

## Summary Format (Step 2 / Step 3)

When LLM summarization is needed, a single call generates the summary. The
input is the messages being summarized (with all tools already linked, so the
input is compact). The output replaces those messages as a `[System:]` message.

No structured template is required. The prompt instructs the LLM to write a
concise summary covering:

- Work completed in the summarized exchanges
- Decisions made and rationale
- Errors encountered and resolutions
- Any context needed to continue

The summary is explicitly a **low-priority background** — it sits between
preserved real messages. The model should (and does, due to positional
attention) prioritize the preserved head and tail over the summary.

## Performance

|                        | v3 (3-call pipeline)    | v4 (paging)                    |
|------------------------|-------------------------|--------------------------------|
| LLM calls (typical)   | 3 (always)              | **0** (mechanical linking)     |
| LLM calls (worst case) | 3                       | 1 (summary, only in Case C)   |
| Latency (typical)      | 30-45s                  | **< 1s** (no API calls)       |
| Latency (worst case)   | 30-45s                  | ~15s (1 summary call)          |
| Information loss        | Lossy (LLM summary)    | **Lossless** (content on disk) |
| Goal regression         | Frequent                | **Not possible** in Cases A/B  |
| Token cost              | 3× conversation         | **0** in Cases A/B             |

## Interaction with Session Resume

On resume (`-r`), `ReconstructHistory` loads messages from session files. The
linked format must be compatible:

- **No new file format needed.** Linking modifies the in-memory message
  history, not the session files on disk. The original `*_tool-result.md` and
  `*_tool-use.md` files remain intact.

- **Compaction state must be persisted.** After compaction, write a
  `*_compaction.md` marker (as today) followed by metadata indicating which
  messages were linked and (if Step 2/3) the summary text as a `*_system.md`.

- **Resume after Step 0/1 compaction:** Reconstruct history from session files,
  re-apply linking to all tool content before the current turn. This is
  deterministic — no state beyond "compaction happened" needs to be stored.

- **Resume after Step 2/3 compaction:** Load from `*_system.md` as today,
  but the system message now sits between preserved head and tail messages
  rather than replacing everything.

## Interaction with Multiple Compaction Cycles

On the second compaction, previously-linked tool content is already just a
file path (a few tokens). Step 0 only needs to link **new** tool results
accumulated since the last compaction. Old links are negligible. Each
compaction cycle processes less content than the last.

If Step 2 was used in a prior compaction, the summary from that cycle is a
regular message in the history. On the next compaction it may fall in the
"head" (preserved), "middle" (re-summarized), or "tail" (preserved) depending
on its position. This is correct behavior — old summaries are treated like
any other message.

## What to Delete

The following code from v3 is removed entirely:

- `runCompactionWorkflow` (3-call pipeline)
- `compactionCall` (LLM call helper for compaction)
- `serializeNumberedMessages` (full conversation serialization for LLM)
- `ParsePreserveIndices` / `parseMessageReferences` (LLM output parsing)
- `extractPreservedMessages` / `describePreservedMessages`
- `ExtractCurrentObjective`
- `appendPreservedMessages` (alternation-maintaining assembly)
- The "sacred first message" concept (`findFirstUserMessage` for pinning)

Retained and modified:

- `ShouldCompact` — unchanged (threshold check)
- `recentKeepCount` — replaced by progressive N in Step 1
- `CaptureGitState` — still useful, can be appended to Step 2/3 summaries
- `CompactionCallback` — still needed for TUI display
- Session file persistence — compaction marker + optional system.md

## Open Questions

- **Linking format in API messages.** Tool results are `content` fields in
  `tool_result` blocks. Replacing content with a file path works semantically,
  but the model needs to understand it can `read_file` on that path. May need
  a brief system prompt addition: "Linked tool results show a file path. Use
  `read_file` to retrieve the full output if needed."

- **Token estimation for fit checks.** Each step needs to estimate whether the
  modified history fits. The existing `estimateTokens` (char count / 3.5) is
  sufficient for this — exact counts aren't needed, just a conservative
  estimate to decide whether to proceed to the next step.

- **Exchange boundary detection.** Step 2 divides by "exchanges" (user message
  + subsequent assistant/tool messages until next user message). Need to handle
  edge cases: tool results that follow a user message (they're part of the
  prior exchange, not the new one), system injections, etc.
