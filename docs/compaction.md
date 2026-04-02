**Proposed Improvements to Traditional Compaction for Our Go Coding Agent**

**Date:** March 26, 2026  
**Authors:** Grok + team (AJ’s Go coding agent project)  
**Status:** Pre-spec discussion document

### 1. Executive Summary
Traditional compaction in coding agents is a necessary evil: it keeps long sessions alive inside finite context windows by summarizing old history. The current state of the art (Pi, OpenCode, Claude Code) all rely on the same basic pattern — **a single structured LLM call plus deterministic pruning** — which works but leaves a lot on the table.

We believe compaction can be dramatically better if we treat it as **an automated handoff document** written by one developer for the next (or for the same developer hours later). This document lays out:

- What the three leading implementations actually do today (confirmed March 2026).
- Our spiky philosophy for a serious coding agent.
- Concrete improvements we want to ship in our Go agent and why they matter.

The result will be higher-quality, more reliable long-running sessions that feel closer to real engineering practice than to chat compression.

### 2. State of the Art (March 2026)

We examined Pi, OpenCode, and Claude Code directly from their source/docs/gists/issues.

| Aspect                        | **Pi Coding Agent**                                                                 | **OpenCode**                                                                 | **Claude Code**                                                              |
|-------------------------------|-------------------------------------------------------------------------------------|------------------------------------------------------------------------------|------------------------------------------------------------------------------|
| **Trigger**                   | Auto: > `contextWindow - reserveTokens` (default ~16k reserve)<br>Manual: `/compact` | Auto: ~75–95% utilization<br>Manual: `/compact`                             | Auto: early (~75–95%) for safety<br>Manual: `/compact`                      |
| **Core Mechanism**            | Backward scan to `keepRecentTokens` cut → single LLM summary → append `CompactionEntry` → reload | Hidden “compaction” agent (single structured LLM call) + deterministic prune | Server-side single structured LLM summary; sometimes feels like “new session” |
| **Summary Generation**        | One-shot Markdown template (Goal, Constraints, Progress, Decisions, Next Steps, Critical Context + file lists) | One-shot structured summary via dedicated agent prompt (customizable via env var + plugins) | One-shot structured summary (task overview, accomplishments, files, next steps) with optional custom instructions |
| **Tool-Result Handling**      | Hard truncation (~2k chars per result)                                              | Deterministic prune of oldest tool outputs first (before summarization)      | Aggressive deterministic prune of old tool outputs                           |
| **File/State Tracking**       | Accumulates raw diffs + read/modified files across every compaction                | Prunes tool outputs; keeps recent turns                                      | Prunes aggressively; relies on persistent files (e.g. CLAUDE.md)            |
| **Preserved Content**         | Recent messages + cumulative file ops; full JSONL log on disk                      | Recent turns + injected summary                                              | Key decisions/files/current work; summary injected                           |
| **Extensibility**             | Extremely high (extensions, custom summarizer model, code-aware logic)             | Good (plugins, experimental compaction prompt)                               | Medium (custom instructions on `/compact`, persistent rules file)            |
| **Transparency / UX**         | Summary injected; full history viewable                                            | Shows the generated summary clearly                                          | Mentions “compacting”; context usage visible                                 |

**Crucial observation**: Even when OpenCode and Claude Code call it a “dedicated compaction agent,” it is still **one LLM call** with a carefully written prompt. There is no multi-turn reasoning, no tool use inside compaction itself, and no intelligence applied to pruning decisions.

### 3. Our Philosophy — Spiky Points of View

We are not building a general-purpose chat tool. We are building a **coding agent** that takes a prompt that could stand in for a complete Jira ticket and executes it end-to-end in long-running, largely autonomous 1-shot missions. This leads us to several non-negotiable stances:

- **Compaction is a handoff, not compression.** It should read like a developer writing a concise, accurate status update for the next shift — structured, actionable, and trustworthy.
- **Single-LLM-call compaction is the weakest link.** Real handoffs require multiple reasoning passes; we should make compaction agentic.
- **Git is the single source of truth for code state.** We want 1 agent turn/run ≈ 1 commit ≈ 1 PR. Therefore we should stop accumulating raw diffs and let git do what it was designed for.
- **The original user request is sacred.** In our use case the first message is often the entire mission statement; it should never be heavily summarized or dropped.
- **The agent must be able to search its own history after compaction.** Keeping the full log on disk but making it invisible to the model is a missed opportunity.
- **We optimize for long, high-stakes missions.** Spending a few extra tokens and LLM calls on compaction is not only acceptable — it is required — if it produces dramatically better results.

These are deliberate trade-offs. We are willing to be slower or more verbose during compaction in exchange for far higher session quality and developer-like reliability.

### 4. Our Specific Recommendations & Reasoning

Each recommendation below includes a short “why it matters” followed by a bulleted list of concrete implementation guidance (still at the conceptual level — no code or Go structs yet).

1. **Agentic Multi-Step Compaction**  
   Turn compaction into a small internal workflow instead of one giant LLM call.  
   *Why it matters*: Single prompts become brittle and hallucinate on long histories; multiple targeted passes produce far more consistent, higher-fidelity handoff documents. Because compaction quality is extremely valuable (analogous to paying $15+ for a single high-quality code review with Anthropic’s latest models), we deliberately spend maximum intelligence and tokens here.  
   - Break the process into distinct phases: goal/constraint extraction, decision capture, file-state analysis, tool-result synthesis, and final handoff drafting.  
   - Have each phase use a focused, high-quality prompt running on the strongest available model with a generous token budget.  
   - Let the workflow optionally loop back or refine earlier phases if the final draft reveals gaps.  
   - Output a single, well-structured Markdown handoff document that becomes the new summary message.  
   - Log each intermediate step internally so we can debug or replay compaction quality later.

2. **Smarter Tool-Result Summarization**  
   Replace hard cutoffs with intelligent, context-aware summarization of oversized tool outputs.  
   *Why it matters*: Critical details often live in the tail of tool results; a deterministic 2000-char chop loses them forever.  
   - For any tool output exceeding the size threshold, spin up a dedicated tiny-LLM summarizer pass (still using the high-intelligence model).  
   - Give that summarizer the original user prompt + the two most recent kept messages as anchoring context.  
   - Ask it to decide what to keep verbatim, what to condense, and what to drop entirely, rather than enforcing a fixed length.  
   - Store the summarized version alongside a tiny metadata note (e.g., “original length X → summarized to Y”).  
   - Make this step optional/configurable so we can fall back to truncation only during truly extreme token pressure.

3. **Git-Centric State Tracking**  
   Treat git as the authoritative record of code state instead of accumulating raw diffs.  
   *Why it matters*: Raw diffs go stale the instant the next edit happens; git gives us perfect, compact, always-current history that matches our “1 run = 1 commit = 1 PR” philosophy.  
   - At every compaction point, capture only the current commit SHA (and short commit message if one exists).  
   - Record a one-line “what changed since last compaction” note generated by the agent.  
   - Drop all cumulative raw-diff and modified-files lists that existing agents carry forward.  
   - When the handoff document needs to reference file state, instruct the summarizer to reference the latest commit SHA and let git handle the rest.  
   - Add a post-compaction hook that can optionally run `git commit` or `git status` to keep the repo in a clean state.

4. **Preserve the Initial User Message Verbatim**  
   Never let the original mission statement get summarized away.  
   *Why it matters*: In our Jira-style workflow the very first message is often the entire spec; losing its exact wording silently kills long-session coherence.  
   - Keep the first user message in full, placed immediately after the system prompt and before any compaction summary.  
   - Include it (verbatim) in every future summarization pass so the handoff document always knows the original ask.  
   - Never truncate or rephrase it even under extreme token pressure.  
   - Display it in any internal “full history” view with a special visual marker so the agent always sees the mission anchor.

5. **Add a History Search Tool**  
   Give the agent (but not the human user) the ability to query the full raw conversation log even after compaction.  
   *Why it matters*: All existing agents keep the complete log on disk but treat it as invisible after compaction; this turns lossy compression into queryable memory for our long-running autonomous agents.  
   - Store the entire conversation as a flat, plaintext, append-only log (one entry per turn).  
   - Expose an internal-only tool that the agent itself can call to query the log using ripgrep-style or git-grep semantics under the hood.  
   - Let the tool accept natural-language queries that get turned into precise grep patterns or semantic filters.  
   - Return results with timestamps, message IDs, and short context snippets so the agent can decide whether to pull full turns back into context if needed.

6. **Feed Recent Context into the Summarizer**  
   Let the summarizer peek at the messages that will stay in context.  
   *Why it matters*: The handoff document becomes noticeably more coherent when it can reference what’s still “hot” in the session.  
   - When launching the multi-step compaction workflow, include the last 1–2 full kept turns as extra context for every phase.  
   - Instruct the final handoff drafter to explicitly call out any open threads or decisions that bridge the summary and the kept messages.  
   - Keep this extra context small (just enough for continuity) so it does not meaningfully increase token usage.  
   - Allow the system to toggle this behavior with a flag if maximum token savings are ever required.

7. **Trigger Strategy**  
   Use only automatic, token-driven triggers with no user-facing commands.  
   *Why it matters*: Our focus is long-running autonomous agents that 1-shot complete tasks; manual intervention or slash-command triggers are unnecessary and out of scope.  
   - Trigger compaction automatically when context exceeds (contextWindow − reserveTokens).  
   - Make the exact threshold values (reserveTokens, keepRecentTokens) configurable per-project and globally.  
   - Keep the logic simple and predictable so the agent can anticipate and plan around upcoming compactions.

### 5. Expected Outcomes

- Compaction quality that actually feels like a thoughtful developer handoff instead of a lossy summary.
- Dramatically better long-running session reliability and fewer “I forgot what we were doing” moments.
- Closer alignment with real software engineering workflows (git + handoffs).
- A clear differentiation from Pi, OpenCode, and Claude Code — we will be the agent that gets *better* the longer you use it.

This is not a full implementation spec yet — it is the shared understanding we need before we write one. Once we align on these points, we can move to detailed pseudocode, Go struct designs, prompt templates, and testing plan.

