# The Anatomy of a Coding Agent

### Where 500,000 lines of code go when the agent loop is only 350

---

## Introduction

Every major coding agent — Claude Code, OpenCode, Pi, and a dozen others — does the same thing at its core: send messages to an LLM, parse out tool calls, execute them, and loop until the model produces a text response. This loop is not complicated. It is, in most implementations, a few hundred lines of code.

And yet these projects range from 3,800 to 512,000 lines. Where does the other 99% go?

This report answers that question empirically. We cloned four open-source (or extractable) coding agent codebases, categorized every line of production code by function, and built a comparative breakdown of where complexity actually lives. The four subjects span the full spectrum of the space:

- **Clyde** — A single-binary Go CLI. 3,793 lines of production code. 3 dependencies. One provider.
- **Pi** (pi-mono, by Mario Zechner) — An opinionated TypeScript toolkit. 98,516 lines across 7 packages. Custom TUI, custom LLM API, custom everything.
- **OpenCode** (by AnomalyCo) — A full-platform TypeScript monorepo. 229,295 lines across 19 packages. Desktop apps, IDE extensions, SaaS console, LSP integration.
- **Claude Code** (by Anthropic) — The commercial product. 512,664 lines of TypeScript/TSX across 1,884 source files (recovered from a source-map leak in March 2026). 40 tools, multi-agent "teammate" orchestration, voice input, computer use, seccomp sandboxing.

The core finding is stark: **the agent loop and tool implementations account for 8–15% of every codebase, regardless of total size.** The remaining 85–92% is product infrastructure — TUI rendering, provider abstraction, session persistence, permission systems, plugin architectures, server infrastructure, and configuration management. These are the things that make a coding agent into a *product*. They have nothing to do with talking to an LLM.

Clyde is the control group. It implements the agent and the tools and almost nothing else. It exists to demonstrate what you get — and what you lose — when you strip a coding agent down to its minimum viable surface area.

---

## Methodology

### How we measured

**Clyde, Pi, and OpenCode** were cloned from GitHub and measured with `find` + `wc -l`, categorized by directory and file. Only production TypeScript/Go files were counted — test files, type declarations (`.d.ts`), `node_modules`, and generated code were excluded. Each file was assigned to exactly one category based on its directory location and contents.

**Claude Code** was originally analyzed via its bundled `cli.js` artifact using reference-count heuristics. That initial analysis estimated 30,000–60,000 first-party lines and identified 21 tools from the `sdk-tools.d.ts` type definition file. On 31 March 2026, the published npm package was discovered to include a source map pointing back to the original TypeScript source tree; the full `src/` directory was recoverable from Anthropic's R2 storage bucket. This report has been updated to use **direct measurement of the recovered source**: `find` + `wc -l` on 1,884 `.ts`/`.tsx` files, categorized by directory and file contents — the same methodology used for the other three agents. The original reference-count estimates are preserved in the appendix for comparison, but all percentages and LOC figures in the body now reflect the actual source.

### Category definitions

- **Core agent loop**: The LLM call → tool dispatch → result collection cycle. The code that decides "call the model, check for tool_use blocks, execute them, repeat."
- **Tool implementations**: The actual tool code — reading files, executing bash, patching, searching. Includes the tool registry.
- **Provider / LLM abstraction**: Everything required to talk to model providers — HTTP clients, request/response types, streaming, multi-provider adapters.
- **TUI / CLI / rendering**: Terminal UI, input handling, spinners, styling, color formatting, readline, component frameworks.
- **Persistence / sessions**: Saving and restoring conversation state — databases, transcript files, session managers, migration logic.
- **Permission / security**: Tool approval workflows, sandboxing, seccomp, hooks that gate execution.
- **Plugin / extensions**: Systems for third-party or user-defined extensions — plugin loaders, skill frameworks, MCP integration, hook systems.
- **Server infrastructure**: HTTP APIs, SSE endpoints, event buses, remote control, agent protocols.
- **Config / utils / other**: Configuration loading, logging, utilities, git integration, file system helpers — the connective tissue.

### Caveats

OpenCode's monorepo includes a documentation site (206,000 lines of MDX), a SaaS console (35,600 lines), and desktop apps — these are separate products, not the agent. We report both the full monorepo (229K) and the core `packages/opencode` package (72.6K) and use the latter for percentage comparisons.

Claude Code's numbers were originally proportional estimates from reference-count analysis of the minified bundle. Following the March 2026 source-map leak, all Claude Code figures have been updated to direct measurements of the recovered TypeScript source tree (512,664 LOC across 1,884 files). The original estimates are preserved in the appendix; the body uses actual LOC. Note that the `sdk-tools.d.ts` file only exposed 21 of Claude Code's 40 tools — the remaining 19 are internal or feature-gated tools not surfaced to the SDK.

Pi's "config/utils/other" category (26%) is inflated by subsystems that defy clean categorization — a package manager (2,254 LOC), HTML session export (3,638 LOC), auth storage (493 LOC), and the `pi-ai` utility layer (3,068 LOC). These aren't agent logic, but they aren't cleanly infrastructure either.

---

## The Four Agents at a Glance

|  | **Clyde** | **Pi** | **OpenCode** | **Claude Code** |
|---|---|---|---|---|
| Language | Go | TypeScript | TypeScript | TypeScript |
| Production LOC | **3,793** | **98,516** | **229,295** (72,634 core) | **512,664** |
| Source files | 47 | 567 | 1,091 | 1,884 |
| Packages | 13 (flat) | 7 | 19 | monolithic |
| Direct dependencies | 3 | ~20 | heavy | heavy (bundled) |
| Built-in tools | 11 | 7 | 8 + MCP + plugins | 40 + MCP |
| Providers | 1 (Anthropic) | 11 | 20+ | 4 (Anthropic, Bedrock, Vertex, Foundry) |
| Test LOC | 10,376 | 48,007 | 63,184 | unknown |

**Clyde** describes itself as "a single-file Go CLI that provides a REPL interface for talking to Claude AI." It supports 11 tools (read, write, patch, bash, grep, glob, multi-patch, web search, browse, include image), prompt caching, extended thinking, CLI and REPL modes, and can be embedded as a Go library. It does not support session persistence, multiple providers, streaming, permissions, plugins, or a server API.

**Pi** is Mario Zechner's "opinionated and minimal coding agent." It's built from the ground up: a custom LLM API (`pi-ai`, 11 provider implementations, no Vercel AI SDK), a custom terminal UI framework (`pi-tui`, differential rendering, retained-mode components), and a custom agent runtime (`pi-agent-core`). It deliberately excludes MCP, permissions, subagents, plan mode, and compaction, arguing these are either unnecessary or counterproductive. Its system prompt and tool definitions total ~1,000 tokens.

**OpenCode** is positioned as the open-source alternative to Claude Code. It's a full platform: HTTP server with SSE, SolidJS-based UI (shared across TUI, desktop, and web), SQLite persistence via Drizzle ORM, LSP integration for 20+ languages, a plugin system with MCP support, and distribution via npm, Homebrew, Docker, Tauri, and Nix. The monorepo includes a SaaS console, VS Code/Zed extensions, and an SDK with auto-generated OpenAPI bindings.

**Claude Code** is Anthropic's commercial product. The recovered source tree reveals 512,664 lines across 1,884 files — far larger than the 30K–60K originally estimated from the minified bundle. It includes 40 built-in tools (nearly double the 21 exposed via the SDK type definitions), a multi-agent "teammate" system with tmux, iTerm, and in-process backends, seccomp sandboxing on Linux, voice input via push-to-talk with native audio capture and WebSocket speech-to-text, computer use integration (mouse, keyboard, screenshots via native Rust and Swift modules), a 19,842-line customized Ink fork for terminal rendering, a 12,613-line bridge system for Claude Desktop integration, a structured memory directory system, a plugin marketplace with a hook system (PreToolUse/PostToolUse/Stop), session persistence with optimized transcript streaming, context compaction, vim keybindings, 101 subcommands, cron scheduling, remote session handoff (`--teleport`), LSP integration, and GitHub automation (issue triage, @claude mentions, scheduled tasks).

---

## Where the Code Goes

### The detailed breakdown

For each agent, every line of production code was assigned to a single category. OpenCode percentages use the core package (72.6K LOC). Claude Code figures are from direct measurement of the recovered source tree (512.7K LOC).

```
                          Clyde    Pi      OpenCode    Claude Code
                          3.8K    98.5K    72.6K       512.7K
────────────────────────────────────────────────────────────────────

Core agent loop            9%      8%       10%          3%
Tool implementations      40%      3%        5%         10%
Provider / LLM             6%     12%       11%          3%
TUI / CLI / rendering     36%     27%       13%         22%
Persistence / sessions     —       3%        3%          2%
Compaction                 —       1%        *           1%
Permission / security      —       —         1%          2%
Plugin / extensions        —       4%        6%          6%
LSP integration            —       —         4%         <1%
Server infrastructure      —       —         9%         <1%
MCP                        —       —         2%          2%
Config / utils / other     8%     26%       18%         35%
Web UI                     —      12%        —           —
Chat (Slack)               —       4%        —           —
Infra (vLLM)               —       2%        —           —
Worktree isolation         —       —         1%         <1%
Bridge (Desktop ↔ CLI)     —       —         —           2%
Teammate / swarm           —       —         —           2%
Voice input                —       —         —          <1%
Computer use               —       —         —          <1%
Memory system              —       —         —          <1%
Vim / keybindings          —       —         —           1%
Commands / subcommands     —       —         —           5%

────────────────────────────────────────────────────────────────────
— = 0% (feature does not exist in this codebase)
* = included within session/ subsystem
```

### Simplified: the Big 5

Collapsing into five macro-categories:

```
                          Clyde    Pi      OpenCode    Claude Code
────────────────────────────────────────────────────────────────────
"The thing itself"
  (agent loop + tools)     49%     11%       15%         13%

"Talking to models"
  (providers + LLM API)     6%     12%       11%          3%

"Looking good"
  (TUI + CLI + UI)         36%     39%       13%         22%

"Everything else"           8%     38%       61%         62%
  (persistence, perms,
   plugins, LSP, server,
   MCP, config, infra,
   bridge, teammates,
   voice, memory, vim)
────────────────────────────────────────────────────────────────────
```

---

## Analysis

### 1. The agent engine hits a ceiling

The most striking pattern in the data is that the core agent loop is roughly the same size in every codebase — not proportionally, but in absolute terms:

| | Core loop LOC | Tool LOC | Total "engine" |
|---|---|---|---|
| **Clyde** | 351 | 1,531 | **1,882** |
| **Pi** | 7,713 | 2,784 | **10,497** |
| **OpenCode** | 7,034 | 3,781 | **10,815** |
| **Claude Code** | ~15,000 | 50,828 | **~66,000** |

Pi and OpenCode converge on roughly 10,000–11,000 lines for their core agent engines despite having wildly different total sizes (98K vs. 229K). Claude Code's engine is larger in absolute terms, but the core loop itself (the `QueryEngine`, `query.ts`, `Task.ts`, `Tool.ts`, and `tools.ts` files that orchestrate LLM calls, tool dispatch, and result collection) is roughly 15,000 lines — in the same order of magnitude. Where Claude Code diverges sharply is in tool implementations: 50,828 lines across 40 tools, compared to Pi's 2,784 across 7 and OpenCode's 3,781 across 8. Much of this expansion comes from security-hardened tools — BashTool alone includes `bashPermissions.ts` (2,621 LOC), `bashSecurity.ts` (2,592 LOC), and `readOnlyValidation.ts` (1,990 LOC) — plus entirely new tool categories like PowerShellTool (with its own 2,049-line path validation), TeamCreateTool, SendMessageTool, ScheduleCronTool, LSPTool, and computer use support. The reason is straightforward: the agent loop is a thin orchestration layer. The LLM does the reasoning. The loop's job is mechanical — format a request, parse the response, dispatch tool calls, collect results, decide whether to loop or return. There are only so many edge cases (error handling, abort signals, thinking traces, image attachments, tool validation) before the code is complete.

Clyde's 1,882 lines represent the functional minimum for this pattern: a `for {}` loop in Go that calls the Anthropic API, iterates over content blocks, executes tools by name, and appends results to the conversation history. The entire `HandleMessage` function is 194 lines. It has no streaming, no subagent dispatch, no compaction triggers, no permission checks — and it works.

Pi and OpenCode reach ~10K by adding the features Clyde omits: structured event streaming, abort support with partial results, session state management, model resolution, prompt template injection, and (in OpenCode's case) compaction triggers. But none of them reach 15K. The loop just isn't that complicated.

This has implications for anyone evaluating coding agents. **If a project has 100,000 lines of code, roughly 10,000–15,000 of them are the agent loop.** The rest is the product around it — tools, UI, persistence, security, plugins, and infrastructure. When you choose between coding agents, you are mostly choosing between those surrounding systems — the UI, the persistence model, the provider support, the extension system — not between fundamentally different agent architectures. Claude Code's 50,828-line tool budget demonstrates that even *tools* can become a major product investment when each one must handle security validation, permission checks, read-only modes, and platform-specific variants.

### 2. Provider abstraction is a constant tax

Every agent that supports multiple providers spends 10–12% of its codebase on the LLM abstraction layer:

| | Provider LOC | % of codebase | Providers supported |
|---|---|---|---|
| **Clyde** | 241 | 6% | 1 (Anthropic) |
| **Pi** | 11,746 | 12% | 11 |
| **OpenCode** | 7,927 | 11% | 20+ |
| **Claude Code** | 13,187 | 3% | 4 |

Clyde's 241 lines are what provider support looks like when you support exactly one provider: an HTTP POST to `api.anthropic.com`, JSON marshaling, and error handling with contextual suggestions. That's it. No streaming. No provider switching. No credential management for multiple services.

Pi's 11,746 lines are the price of building multi-provider support from scratch. Mario Zechner explicitly rejected the Vercel AI SDK — the standard abstraction used by OpenCode and many others — in favor of writing directly against the four underlying LLM APIs (OpenAI Completions, OpenAI Responses, Anthropic Messages, Google Generative AI). The result is 11 hand-rolled provider implementations, each handling the quirks of its target: Cerebras and xAI don't like the `store` field, Mistral uses `max_tokens` instead of `max_completion_tokens`, Google still doesn't support tool call streaming, and different providers report reasoning content in different fields.

OpenCode achieves broader provider coverage (20+) in less code (7,927) by building on the Vercel AI SDK, which handles the per-provider normalization. This is the classic build-vs-buy tradeoff: Pi gets full control and a smaller surface area at the cost of more code; OpenCode gets breadth at the cost of a dependency.

Claude Code supports four providers (Anthropic's first-party API, AWS Bedrock, Google Vertex, and Foundry) in 13,187 lines — the `services/api/` client layer (10,477 LOC) plus the `utils/model/` subsystem (2,710 LOC) covering model configs, Bedrock inference profiles, model capabilities, deprecation, and validation. In absolute terms this is comparable to Pi's multi-provider investment, but as a percentage of the total codebase (3%) it's dwarfed by Claude Code's massive infrastructure spend elsewhere. The cost of Bedrock integration alone is nontrivial: `utils/model/bedrock.ts` dynamically lists inference profiles via the AWS SDK, and each of the ~11 supported model families requires per-provider ID mappings across all four providers.

The takeaway: **if you want to support multiple providers, budget ~10–12% of your codebase for it.** If you only need one provider, the cost drops to nearly nothing.

### 3. The TUI: a hidden complexity sink

Terminal rendering is where the philosophical differences between these projects become most visible:

| | TUI/CLI LOC | % of codebase | Approach |
|---|---|---|---|
| **Clyde** | 1,358 | 36% | readline + spinner + ANSI colors |
| **Pi** | 26,920 | 27% | Custom TUI framework from scratch |
| **OpenCode** | 9,435 (core) / 88,604 (full) | 13% (core) | SolidJS component library |
| **Claude Code** | ~112,000 | 22% | Customized Ink fork + React components |

Clyde and Pi both spend their largest single budget on the terminal experience — 36% and 27% respectively. But the absolute numbers reveal the gulf: Clyde's 1,358 lines buy a readline wrapper with history, a braille-dot spinner, ANSI color styling, and a git-aware prompt. Pi's 26,920 lines buy a retained-mode rendering engine with differential updates, synchronized output escape sequences for flicker-free drawing, a custom text editor with fuzzy file search and path completion, theme support with live reload, and 17 interactive components (session selector, config editor, model picker, diff viewer, and more).

Pi's `interactive-mode.ts` alone — the file that wires the TUI together — is 4,689 lines. That single file is larger than Clyde's entire codebase.

This is the most revealing data point about Pi's design philosophy. Mario Zechner's blog post describes pi as "minimal" and "opinionated," and by feature count it is — no permissions, no MCP, no subagents, no plan mode. But by LOC allocation, **Pi is primarily a terminal UI project.** Nearly 40% of its code (counting `pi-tui` + interactive mode + CLI + RPC mode together) is dedicated to making the terminal experience feel crafted. The "minimalism" is in the feature set, not the engineering investment.

OpenCode's 13% for the core TUI is misleading in isolation. The `packages/opencode` CLI subsystem is 9,435 lines, but the full monorepo includes `packages/app` (51,361 lines of shared SolidJS UI logic) and `packages/ui` (27,808 lines of UI components). These are shared across the TUI, desktop apps, and web interface. If you count the full UI surface, OpenCode's rendering investment is 88,604 lines — the largest absolute investment of any agent in any category.

Claude Code's 22% is the biggest surprise from the source-code analysis. The original reference-count estimate of 6% suggested Anthropic was using Ink off the shelf with minimal customization. The reality is dramatically different. Claude Code's UI investment breaks down as: `components/` (81,546 LOC of React/Ink components — prompt input, settings, log viewer, diff rendering, status indicators, and more), `ink/` (19,842 LOC — a heavily customized Ink fork with a custom reconciler, layout engine via yoga, a custom event system with click/keyboard/focus dispatching, selection handling, search highlighting, and a full terminal I/O parser with CSI, OSC, SGR, and DEC sequence support), `screens/` (5,977 LOC — the REPL screen alone is 5,005 lines), `keybindings/` (3,159 LOC — a full user-configurable key binding system with parser, validator, resolver, and default bindings), and `vim/` (1,513 LOC — motions, operators, text objects, and mode transitions for vi-style editing in the prompt).

This makes Claude Code's TUI the largest single investment in the codebase by category, and the largest absolute TUI investment among all four agents — surpassing even OpenCode's full-monorepo UI at 88,604 lines. The "use a framework off the shelf" characterization was wrong. Anthropic forked and substantially rewrote Ink, then built a component library on top of it that rivals a small React application.

### 4. Infrastructure: the 60% majority

The most important table in this report:

```
"Everything else"           Clyde    Pi      OpenCode    Claude Code
  (persistence, perms,        8%     38%       61%         62%
   plugins, LSP, server,
   MCP, config, infra,
   bridge, teammates,
   voice, memory, vim)
```

OpenCode and Claude Code spend nearly two-thirds of their code on systems that don't exist in Clyde at all. Here's where that budget goes:

**OpenCode's 61% breaks down as:**

| Subsystem | LOC | % of core |
|---|---|---|
| Config / utils / file handling | 13,297 | 18% |
| Server + event bus + ACP protocol | 6,743 | 9% |
| Plugins + skills + MCP | 4,159 | 6% |
| LSP integration (20+ languages) | 2,919 | 4% |
| Persistence + snapshots + sync | 2,002 | 3% |
| Permission system | 520 | 1% |
| Worktree isolation | 612 | 1% |
| Other (git, pty, format, etc.) | ~14,000 | 19% |

The server infrastructure (9%) is particularly notable. OpenCode runs a Hono-based HTTP server with REST endpoints and SSE streaming. This isn't for the agent — it's for the *clients*. The TUI, desktop apps, IDE extensions, and SDK all communicate with the agent through this server layer. It's an architectural choice that enables multi-surface support at the cost of a permanent ~6,700-line tax.

The LSP integration (4%, 2,919 lines) gives OpenCode's agent semantic code understanding — go-to-definition, find-references, diagnostics. Claude Code also has LSP integration (`services/lsp/` at 2,460 LOC plus an `LSPTool` and plugin-level LSP hooks), so this is no longer unique to OpenCode, though OpenCode's implementation covers 20+ languages while Claude Code's appears more narrowly scoped. Whether LSP integration justifies its cost depends on whether you believe agents benefit from structured code intelligence versus reading files and using grep — the approach Clyde and Pi take.

**Claude Code's 62% breaks down as (direct measurement):**

| Subsystem | LOC | % of codebase |
|---|---|---|
| Config / utils / other | 180,472 | 35% |
| Plugins / hooks / skills / marketplace | ~30,000 | 6% |
| Commands / subcommands (101 total) | 26,428 | 5% |
| Hooks (React UI hooks) | 19,204 | 4% |
| Bridge (Desktop ↔ CLI) | 12,613 | 2% |
| MCP integration | 12,310 | 2% |
| Persistence / sessions / transcripts | ~10,000 | 2% |
| Permission / security | 9,409 | 2% |
| Teammate / swarm orchestration | 9,288 | 2% |
| Compaction | 3,960 | 1% |
| Worktree isolation | ~1,500 | <1% |
| Teleport / remote handoff | 2,180 | <1% |
| Computer use | 2,161 | <1% |
| Memory system (memdir) | 1,736 | <1% |
| Cron / scheduling | 1,601 | <1% |
| Voice input | 1,175 | <1% |
| Buddy (companion sprites) | 1,298 | <1% |
| Server / direct connect | 358 | <1% |
| LSP integration | 2,460 | <1% |

The `utils/` directory alone — at 180,472 lines — is larger than the entire Pi codebase (98,516) and dwarfs OpenCode's core package (72,634). This is where the majority of Claude Code's product complexity lives: `sessionStorage.ts` (5,105 LOC), `hooks.ts` (5,022 LOC), `messages.ts` (5,512 LOC), `attachments.ts` (3,997 LOC), `config.ts` (1,817 LOC), the full `bash/` subsystem (including a 4,436-line bash parser and 2,679-line AST), the `permissions/` directory (9,409 LOC with filesystem guards, YOLO classifier, shell rule matching, and path validation), the `plugins/` directory (18,979 LOC with marketplace management, plugin loading, validation, and auto-update), and the `swarm/` directory (7,548 LOC with teammate backends for tmux, iTerm, and in-process execution).

Claude Code's infrastructure is distinguished by both its *depth* and *breadth*. The permission system (9,409 LOC) includes filesystem guards, a YOLO mode classifier, bash command classification, shell rule matching, path validation, and denial tracking. BashTool's security alone is 7,203 LOC (permissions, security analysis, and read-only validation). There's a separate PowerShellTool with its own 3,872-line security stack including TOCTOU hardening. The session system includes optimized transcript streaming (`sessionStorage.ts` at 5,105 LOC), cross-device session handoff via `--teleport` (2,180 LOC), and conversation recovery (21,077 LOC in `conversationRecovery.ts`). The plugin system (18,979 LOC in `utils/plugins/` plus 1,616 in `services/plugins/`) includes a marketplace manager, dependency resolver, plugin loader, blocklist, versioning, auto-update, and LSP/MCP integration hooks. The teammate system (9,288 LOC total) goes far beyond simple subagent spawning: it includes mailbox-based inter-agent messaging, permission synchronization across agents, reconnection handling, and team memory synchronization.

Several entire subsystems that didn't appear in the original reference-count analysis are now visible:

- **Bridge** (12,613 LOC): The communication layer between Claude Desktop and the CLI. Includes `bridgeMain.ts` (2,999 LOC), `replBridge.ts` (2,406 LOC), `remoteBridgeCore.ts` (1,008 LOC), JWT auth, session runners, and inbound message/attachment handling. This is what makes "Open in Claude Code" work from the desktop app.
- **Voice input** (1,175 LOC in services + 54 LOC gate): Push-to-talk with native audio capture via NAPI (CoreAudio on macOS, ALSA on Linux, fallback to SoX), streaming speech-to-text via Anthropic's `voice_stream` WebSocket endpoint.
- **Computer use** (2,161 LOC): Mouse, keyboard, and screenshot integration via native Rust (`@ant/computer-use-input` via enigo) and Swift (`@ant/computer-use-swift` via SCContentFilter) modules. Clipboard via `pbcopy`/`pbpaste`, terminal-as-surrogate-host handling.
- **Memory system** (1,736 LOC in `memdir/` plus supporting files): Structured memory directories with team-level memory, memory extraction, session memory, and relevance-based memory retrieval.
- **Cron scheduling** (1,601 LOC): `ScheduleCronTool` plus a full scheduler, task runner, and jitter configuration for background task automation.

None of this exists in Clyde. Not because Clyde chose a different approach — because Clyde chose *no* approach. And that's the point.

**Pi's 38% falls between the extremes:**

| Subsystem | LOC | % |
|---|---|---|
| Config / utils / other | 25,719 | 26% |
| Web UI components (`pi-web-ui`) | 11,973 | 12% |
| Plugin / extension system | 3,621 | 4% |
| Slack bot (`pi-mom`) | 3,770 | 4% |
| Persistence / sessions | 2,642 | 3% |
| vLLM deployment (`pi-pods`) | 1,773 | 2% |
| RPC mode | 1,520 | 2% |
| Compaction | 1,355 | 1% |

Pi's infrastructure investment is front-loaded into things Mario Zechner personally uses: session management with branching and resume, an extension system for custom slash commands, a Slack bot for team use, web UI components for browser-based chat, and vLLM pod management for self-hosted models. It's back-loaded (or absent) on things he considers unnecessary: no permissions, no MCP, no plan mode, no background process management, no subagent orchestration.

This is what "opinionated minimalism" actually looks like in practice. It's not that Pi has less infrastructure — at 38%, it has substantial infrastructure. It's that the infrastructure is *chosen* rather than *comprehensive*.

### 5. Clyde's thesis: what if you just... didn't?

Clyde makes the most extreme bet in the comparison: skip almost all product infrastructure and see what's left.

Here's what's left:

```
Clyde's 3,793 lines:

  49% — The agent and its tools
         351 LOC agent loop
         1,531 LOC across 11 tools
         (read, write, patch, multi-patch, bash, grep,
          glob, web search, browse, include image, list files)

  36% — A minimal but functional TUI
         readline with history and multiline input
         braille-dot spinner with operation messages
         ANSI color styling
         git-aware prompt with context window percentage

   6% — A single-provider API client
         241 LOC to talk to Anthropic

   8% — Config and utilities
         .env-style config loader
         log levels
         output truncation
```

And here are the features this still buys:

- Interactive REPL and non-interactive CLI mode
- 11 tools covering file ops, shell execution, search, web access, and vision
- Prompt caching (~90% cost reduction on long conversations)
- Extended thinking (adaptive and manual modes)
- Multiline input with three methods (Ctrl+J, Alt+Enter, backslash continuation)
- Embeddable as a Go library with functional options
- Stdin piping, file-based prompts, composability with Unix tools
- Context window tracking in the prompt

Here's what you lose:

- No session persistence — close the terminal and the conversation is gone
- No streaming — the UI blocks during API calls
- No multi-provider support — Anthropic or nothing
- No permission system — every tool runs unconditionally
- No plugin or extension system — tools are compiled in
- No context compaction — eventually the context fills up and you start a new conversation
- No subagent orchestration — you can use tmux manually
- No LSP integration
- No server API — no way to connect other clients
- No session resume, no remote handoff, no scheduled tasks
- No voice input, no computer use, no vim mode
- No bridge to desktop apps

Is this a good tradeoff? That depends entirely on the use case. For a single developer working in a terminal on one project at a time, using Claude models — which is a large fraction of how coding agents are actually used — the features Clyde omits are features that never trigger. You don't need session resume if you finish your task before closing the terminal. You don't need multi-provider support if you use one provider. You don't need permissions if you trust the agent (and as Zechner notes, "everybody is running in YOLO mode anyway").

The point isn't that Clyde is better than the alternatives. It's that Clyde demonstrates the *minimum*. Everything above 3,793 lines is a choice about which product concerns to invest in — and those choices account for 85–95% of the code in every other agent.

---

## Conclusion

The empirical finding of this analysis is simple and consistent across four very different codebases: **a coding agent is a small program wrapped in a large product.**

The agent loop — the part that actually calls an LLM, dispatches tool calls, and collects results — is 3–10% of every codebase. The tool implementations add another 3–10%. Together, the core engine that makes a coding agent *an agent* accounts for 11–15% of the total code, with the core loop converging on roughly 10,000–15,000 lines in absolute terms across Pi, OpenCode, and Claude Code (though Claude Code's tools are dramatically larger at 50,828 lines due to per-tool security hardening).

The other 80–89% is the answer to a different set of questions: How do users interact with this? (TUI frameworks, component libraries, IDE extensions.) How does it remember? (Session persistence, transcript management, compaction.) How does it stay safe? (Permission systems, sandboxing, hook-based approval workflows.) How do others extend it? (Plugin architectures, MCP integration, skill systems.) How do enterprises deploy it? (Server infrastructure, remote control, provider abstraction, managed settings.)

These are legitimate engineering concerns. In many cases they're the *reason* people choose one agent over another. OpenCode's LSP integration gives agents semantic code understanding. Claude Code's seccomp sandboxing and 9,409-line permission system provide real security guarantees. Its 19,842-line Ink fork and 81,546 lines of React components deliver a deeply customized terminal experience. Its teammate system enables multi-agent collaboration with mailbox messaging and permission synchronization. Pi's custom TUI delivers a noticeably smoother terminal experience through a different philosophy — building from scratch rather than forking. These aren't bloat — they're product decisions backed by tens of thousands of lines of carefully written code.

But Clyde's existence as a control group makes the underlying structure visible. You can build a fully functional coding agent — 11 tools, prompt caching, extended thinking, vision support, web access, CLI and REPL modes, library embedding — in 3,793 lines with 3 dependencies. Everything above that line is a product decision. And now, with direct access to Claude Code's source, the data shows exactly how much each decision costs — not in rough proportional estimates, but in actual lines of code.

---

## Appendix: Raw Data

### Clyde — 3,793 production LOC

| Category | LOC | % |
|---|---|---|
| Core agent loop | 351 | 9.3% |
| Tool implementations | 1,531 | 40.4% |
| Provider / LLM | 241 | 6.4% |
| TUI / CLI / rendering | 1,358 | 35.8% |
| Config / utils | 312 | 8.2% |

### Pi — 98,516 production LOC

| Category | LOC | % |
|---|---|---|
| Core agent loop | 7,713 | 7.8% |
| Tool implementations | 2,784 | 2.8% |
| Provider / LLM | 11,746 | 11.9% |
| TUI / CLI / rendering | 26,920 | 27.3% |
| Persistence / sessions | 2,642 | 2.7% |
| Compaction | 1,355 | 1.4% |
| Plugin / extensions | 3,621 | 3.7% |
| Web UI components | 11,973 | 12.2% |
| Chat (Slack bot) | 3,770 | 3.8% |
| Infra tooling (vLLM) | 1,773 | 1.8% |
| Config / utils / other | 25,719 | 26.1% |

### OpenCode — 72,634 production LOC (core package)

| Category | LOC | % |
|---|---|---|
| Core agent loop | 7,034 | 9.7% |
| Tool implementations | 3,781 | 5.2% |
| Provider / LLM | 7,927 | 10.9% |
| TUI / CLI | 9,435 | 13.0% |
| Persistence / sessions | 2,002 | 2.8% |
| Permission / security | 520 | 0.7% |
| Plugin + skill + MCP | 4,159 | 5.7% |
| LSP integration | 2,919 | 4.0% |
| Server + bus + ACP | 6,743 | 9.3% |
| Config / utils / other | 13,297 | 18.3% |
| Worktree isolation | 612 | 0.8% |

OpenCode full monorepo also includes: app UI (51,361), ui components (27,808), SDK (18,719), console SaaS (35,628), desktop apps (5,855), web/docs (215,118), enterprise (946), Slack (145).

### Claude Code — 512,664 production LOC (from recovered source)

| Category | LOC | % |
|---|---|---|
| Core agent loop (QueryEngine, query, Task, Tool, tools) | ~15,000 | 2.9% |
| Tool implementations (40 tools) | 50,828 | 9.9% |
| Provider / LLM (services/api + utils/model) | 13,187 | 2.6% |
| TUI / UI (components + ink + screens + keybindings + vim) | ~112,000 | 21.8% |
| Hooks (React UI hooks) | 19,204 | 3.7% |
| Commands / subcommands (101 total) | 26,428 | 5.2% |
| Persistence / sessions | ~10,000 | 2.0% |
| Compaction | 3,960 | 0.8% |
| Permission / security | 9,409 | 1.8% |
| Plugin / hooks / skills / marketplace | ~30,000 | 5.9% |
| MCP integration | 12,310 | 2.4% |
| Bridge (Desktop ↔ CLI) | 12,613 | 2.5% |
| Teammate / swarm | 9,288 | 1.8% |
| Teleport / remote | 2,180 | 0.4% |
| Computer use | 2,161 | 0.4% |
| Memory system (memdir) | 1,736 | 0.3% |
| Voice input | 1,229 | 0.2% |
| Cron / scheduling | 1,601 | 0.3% |
| Buddy (companion sprites) | 1,298 | 0.3% |
| LSP integration | 2,460 | 0.5% |
| Server / direct connect | 358 | 0.1% |
| Config / utils / other (remainder) | ~176,000 | 34.3% |

Note: "Config / utils / other" is dominated by `utils/` (180,472 LOC total), which contains cross-cutting infrastructure including bash parsing (7,115 LOC), git operations (30,270 LOC in `git.ts` alone), file operations (24,230 LOC), auth (2,372 LOC), shell management (16,929 + 14,138 LOC), status/notifications (48,635 + 30,638 LOC), IDE integration (46,585 LOC), and hundreds of smaller utility modules.

#### Original reference-count estimates (from minified bundle, pre-leak)

For comparison, the original proportional estimates from reference-count analysis of the beautified `cli.js` bundle:

| Category | Ref count | % |
|---|---|---|
| Core agent loop | 792 | 14.1% |
| Tool system | 359 | 6.4% |
| Provider / LLM | 568 | 10.1% |
| TUI / UI | 353 | 6.3% |
| Persistence / sessions | 756 | 13.5% |
| Compaction | 159 | 2.8% |
| Permission / security | 406 | 7.2% |
| Plugin / hooks / skills | 529 | 9.4% |
| Config / settings | 588 | 10.5% |
| Server / remote | 570 | 10.1% |
| Worktree isolation | 127 | 2.3% |
| MCP | 412 | 7.3% |

The reference-count methodology substantially overestimated the *proportional* weight of the agent loop, persistence, and server/remote systems while dramatically underestimating the UI/rendering investment (6.3% estimated vs. 21.8% actual). This is likely because identifier frequency in minified code correlates poorly with line count — UI components are JSX-heavy with fewer distinctive identifiers per line, while infrastructure modules use identifiers densely.

### Tool inventories

| **Clyde** (11) | **Pi** (7) | **OpenCode** (8) | **Claude Code** (40) |
|---|---|---|---|
| list_files | read | read | FileRead |
| read_file | bash | bash | FileWrite |
| write_file | edit | edit | FileEdit |
| patch_file | write | write | Bash |
| multi_patch | grep | grep | PowerShell |
| run_bash | find | glob | Grep |
| grep | ls | task | Glob |
| glob | | apply_patch | Agent |
| web_search | | | NotebookEdit |
| browse | | | TodoWrite |
| include_file | | | WebFetch |
| | | | WebSearch |
| | | | Mcp |
| | | | McpAuth |
| | | | ListMcpResources |
| | | | ReadMcpResource |
| | | | AskUserQuestion |
| | | | Config |
| | | | EnterWorktree |
| | | | ExitWorktree |
| | | | EnterPlanMode |
| | | | ExitPlanMode |
| | | | TaskCreate |
| | | | TaskGet |
| | | | TaskList |
| | | | TaskUpdate |
| | | | TaskOutput |
| | | | TaskStop |
| | | | TeamCreate |
| | | | TeamDelete |
| | | | SendMessage |
| | | | ScheduleCron |
| | | | RemoteTrigger |
| | | | Skill |
| | | | ToolSearch |
| | | | Brief |
| | | | REPL |
| | | | LSP |
| | | | Sleep |
| | | | SyntheticOutput |

Tools in **bold** below were not visible in the original `sdk-tools.d.ts` analysis: **PowerShell**, **McpAuth**, **EnterPlanMode**, **TaskCreate**, **TaskGet**, **TaskList**, **TaskUpdate**, **TeamCreate**, **TeamDelete**, **SendMessage**, **ScheduleCron**, **RemoteTrigger**, **Skill**, **ToolSearch**, **Brief**, **REPL**, **LSP**, **Sleep**, **SyntheticOutput**.
