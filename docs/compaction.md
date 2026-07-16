# Compaction Design

**Status:** Active design — replaces the CMP-2 five-phase pipeline  
**Date:** July 2026

## Problem Statement

Compaction replaces conversation history with a smaller equivalent when the
context window fills up. The current implementation has three critical problems:

1. **Variable, unbounded LLM calls.** Per-tool-result summarization makes one
   LLM call for every oversized tool output. In a heavy tool-use session this
   can be 50-100+ sequential API calls before the 5-phase pipeline even starts.
   Compaction routinely takes 5-10 minutes.

2. **The current objective gets lost.** The 5-phase pipeline sends every phase
   `## Original Mission` as the first section of input, followed by a
   conversation that is 90%+ work on that original mission. The "current
   objective" is a footnote instruction asking the LLM to override what 95% of
   the content is about. In practice the handoff document restates the original
   goal, the LLM sees "goal completed," and no-ops.

3. **Unnecessary truncation.** Tool results are hard-truncated at 2000 chars
   before being fed to the pipeline. The full conversation already fit in the
   context window moments before compaction triggered — there is no reason to
   destroy information before the summarization calls can see it.

## Philosophy (unchanged)

- Compaction is a **handoff**, not compression.
- The original user request is **sacred** — pinned verbatim, never summarized.
- Git is the source of truth for code state.
- We optimize for long, high-stakes, largely autonomous missions.
- A static, predictable number of LLM calls is required — never variable.

## Design: 3 LLM Calls + Deterministic Assembly

The core idea: **don't summarize everything into a document. Triage the
conversation — preserve critical messages verbatim, summarize the rest.**

Post-compaction history becomes a mix of real messages and summary, not just a
doc. Preserved messages carry full signal because the LLM sees them as genuine
conversation history, not quoted text inside a summary.

### Why 3 calls

The 4 cases we must handle (from `compaction-2.txt`):

- Conversations with 1 user message where the objective does not change
- Conversations with 1 user message where the objective changes during LLM turns
- Conversations with many user messages where the user changes the objective
- Conversations with many user messages where the LLM changes the objective

1 call is too few — a single prompt cannot reliably both triage a long
conversation AND write a high-quality summary of the non-preserved parts.

2 calls is tempting (merge "identify pivots" and "identify tool results") but
wrong. Identifying which tool results matter requires knowing the decision
points first. This dependency must be **enforced sequentially** — if merged
into one call, the LLM can cut corners and select tool results that aren't
actually tied to the identified pivots.

3 calls with enforced sequential dependency:

```
Call 1: Identify decision/pivot points
          ↓ (output feeds into Call 2)
Call 2: Identify tool results relevant to those pivots
          ↓ (outputs of 1 + 2 feed into Call 3)
Call 3: Write summary covering everything NOT preserved
          ↓
Step 4: Deterministic assembly of new history
```

### No truncation

The full conversation already fit in the context window — that is how we got
here. Serializing messages to text produces roughly the same token count.
Send the full conversation to each call with no truncation of tool results.

With only 3 calls (and each seeing the conversation once), total token spend is
**less** than the old 5-phase design which sent the full conversation 5 times
plus N tool-result summarization calls.

Safety valve: if the serialized text exceeds the model's context window (rare
edge case from serialization format inflation), fall back to truncating tool
results at 10K chars. This is a fallback, not the default path.

### Call 1: Identify Decision Points

**Input:** Numbered serialized conversation (full, no truncation).

**Task:** Analyze the conversation and identify every point where the objective
changes — whether initiated by the user or discovered by the LLM.

**System prompt direction:**

```
Analyze this coding session and identify decision/pivot points — moments
where the objective changes. This includes:

- User explicitly redirecting work ("stop X, now do Y")
- LLM discovering an approach won't work and pivoting
- Scope changes, new requirements emerging
- Completion of one objective and transition to the next

For each pivot, identify the specific message pair (user + assistant 
exchange, or single assistant message) where the change happens.

Output:

## Current Objective
[What the session is actively working on RIGHT NOW — this is the
most important output of this entire analysis]

## Objective Timeline
1. [Messages 1-N]: Original objective — [description]
2. [Messages N-M]: Pivoted to — [description] — [reason]
3. [Messages M-present]: Current — [description]

## Preserve (message indices)
- Messages X-Y: [which pivot this captures]
- Messages A-B: [which pivot this captures]
```

**Output:** Current objective, timeline of objective evolution, specific message
indices to preserve at each pivot point.

### Call 2: Identify Tool Results to Preserve

**Input:** Numbered serialized conversation + Call 1's full output.

**Task:** For each decision point identified in Call 1, identify tool results
that are critical evidence for that decision or for the current objective.

**System prompt direction:**

```
A triage analysis of this coding session identified specific decision
points (provided below). Now identify tool results that should be
preserved verbatim because they are critical to those decisions or to
the current objective.

PRESERVE tool results that:
- Directly caused or informed an identified pivot (e.g., test failure
  that triggered a change in approach)
- Are critical to the CURRENT objective (e.g., error output still
  being debugged, test results validating current approach)
- Contain information that would be lost and cannot be re-derived
  (e.g., search results that guided a decision)

DO NOT preserve:
- Routine file reads, directory listings, successful command runs
- Tool results from completed/superseded objectives (summarize instead)
- Results that can trivially be re-run

Output:

## Preserve (message indices)
- Message Z: [which decision point this supports, or "current objective"]
- Message W: [reason]
```

**Output:** Specific tool-result message indices to preserve, each tied back to
a decision point from Call 1 or to the current objective.

### Call 3: Write Summary

**Input:** Full conversation + outputs of Call 1 and Call 2 (so it knows what
is being preserved and can avoid repeating it).

**Task:** Write a concise summary covering everything NOT being preserved.

**System prompt direction:**

```
You are writing a summary to fill the gaps between preserved messages
in a compacted coding session. The triage analysis below shows which
messages will be kept verbatim — do NOT repeat their content.

Cover:
- Work completed (that isn't captured in preserved messages)
- Decisions made and their rationale
- Errors encountered and how they were resolved
- Current state of the codebase
- Anything a future reader needs to continue the work

Do NOT include a "Goal" or "Objective" section — the current objective
is captured in the triage analysis and preserved messages. The original
mission is preserved separately as a pinned message.

Be concise. This summary exists to provide continuity between the
preserved messages, not to replace them.
```

**Output:** Gap-filling summary narrative.

### Step 4: Deterministic Assembly

Build the new history from concrete pieces:

```
 1. [user]       Pinned first user message (verbatim, always)
 2. [assistant]   Ack
 3. [user]       "[System: Compaction Summary]" + summary from Call 3
                  + current objective from Call 1
 4. [assistant]   Ack
 5+  [user/asst]  Preserved message pairs from Calls 1 & 2 (chronological)
 ... 
 N.  [user/asst]  Recent kept messages (last ~4, as today)
```

Preserved messages are **real messages** with their original role and content.
The LLM sees them as genuine conversation history, not as quoted text inside a
document. This is the critical quality difference from the old approach — a
real user message saying "now do Y" carries far more weight than a summary
mentioning "the user redirected to Y."

The current objective from Call 1 is included at the end of the summary message
(message 3) so it appears prominently right before the preserved messages and
recent context.

Message alternation (user/assistant) must be maintained. If preserved messages
create gaps in alternation, insert minimal bridging messages as needed.

### Handling the 4 Cases

| Case | Behavior |
|------|----------|
| 1 user msg, objective unchanged | Call 1 finds no pivots → no messages preserved → summary only (simple case) |
| 1 user msg, LLM pivots | Call 1 preserves the assistant message where pivot was decided; Call 2 preserves the tool result that caused it |
| Many user msgs, user changes objective | Call 1 preserves user messages with new instructions |
| Many user msgs, LLM changes objective | Call 1 preserves the exchange where LLM explains the pivot |

### What to Delete

The following code is removed entirely:

- `summarizeToolResult` — no per-tool-result LLM calls
- `serializeMessagesWithSummarization` — replaced by flat serialization, no truncation
- The 5-phase pipeline (`runCompactionWorkflow` phases 1-5) — replaced by 3-call design
- `compactionPhaseCall` in its current form — replaced by new call functions
- `DefaultToolResultThreshold` / `toolResultThreshold` — no truncation threshold

### Performance Comparison

|                        | Old (5-phase + tool summarization) | New (3-call)           |
|------------------------|------------------------------------|------------------------|
| LLM calls              | 5 + N (N = oversized tool results) | 3 (constant)           |
| Full conversation sent | 5 times (each phase)               | 3 times (each call)    |
| Tool result handling   | LLM summarization per result       | No truncation          |
| Latency (typical)      | 1-10+ minutes (variable)           | ~30-45s (predictable)  |
| Current objective      | Footnote, routinely lost           | Call 1's primary output |

### Open Questions

- **Preserved message budget:** Should there be a cap on how many messages
  Calls 1 and 2 can preserve? Preserving too many defeats the purpose of
  compaction. Likely needs a soft guidance ("preserve the minimum needed")
  rather than a hard cap.

- **Bridging message format:** When preserved messages create alternation gaps,
  what should bridging messages say? Minimal filler ("Continuing...") or
  something contextual?

- **Interaction with session resume:** The session resume code
  (`agent/session/resume.go`) reconstructs history from `*_system.md` after
  compaction. The new design preserves real messages — these need to be
  persisted individually (as `*_user.md`, `*_assistant.md`, etc.) after the
  compaction marker, not just embedded in a single `*_system.md` file.
