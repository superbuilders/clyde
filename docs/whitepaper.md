# The Anatomy of a Coding Agent

### Where 50,000 lines of code go when the agent loop is only 350

---

## Introduction

Every major coding agent — Claude Code, OpenCode, Pi, and a dozen others — does the same thing at its core: send messages to an LLM, parse out tool calls, execute them, and loop until the model produces a text response. This loop is not complicated. It is, in most implementations, a few hundred lines of code.

And yet these projects range from 3,800 to 229,000 lines. Where does the other 99% go?

This report answers that question empirically. We cloned four open-source (or extractable) coding agent codebases, categorized every line of production code by function, and built a comparative breakdown of where complexity actually lives. The four subjects span the full spectrum of the space:

- **Clyde** — A single-binary Go CLI. 3,793 lines of production code. 3 dependencies. One provider.
- **Pi** (pi-mono, by Mario Zechner) — An opinionated TypeScript toolkit. 98,516 lines across 7 packages. Custom TUI, custom LLM API, custom everything.
- **OpenCode** (by AnomalyCo) — A full-platform TypeScript monorepo. 229,295 lines across 19 packages. Desktop apps, IDE extensions, SaaS console, LSP integration.
- **Claude Code** (by Anthropic) — The commercial product, shipped as a single 13MB minified JavaScript bundle. ~30,000–60,000 lines of estimated first-party code. 21 tools, multi-agent orchestration, seccomp sandboxing.

The core finding is stark: **the agent loop and tool implementations account for 8–15% of every codebase, regardless of total size.** The remaining 85–92% is product infrastructure — TUI rendering, provider abstraction, session persistence, permission systems, plugin architectures, server infrastructure, and configuration management. These are the things that make a coding agent into a *product*. They have nothing to do with talking to an LLM.

Clyde is the control group. It implements the agent and the tools and almost nothing else. It exists to demonstrate what you get — and what you lose — when you strip a coding agent down to its minimum viable surface area.

---

## Methodology

### How we measured

**Clyde, Pi, and OpenCode** were cloned from GitHub and measured with `find` + `wc -l`, categorized by directory and file. Only production TypeScript/Go files were counted — test files, type declarations (`.d.ts`), `node_modules`, and generated code were excluded. Each file was assigned to exactly one category based on its directory location and contents.

**Claude Code** required a different approach. Anthropic ships it as a single bundled `cli.js` file via npm (`@anthropic-ai/claude-code`). We extracted the package with `npm pack`, beautified it with Prettier (655,368 lines), and used reference-count analysis — counting occurrences of category-specific identifiers (e.g., `permission`, `Session`, `createElement`, `worktree`) — to estimate the proportional allocation of first-party code. The absolute LOC for Claude Code's first-party portion is estimated at 30,000–60,000 lines based on the ~1,912 bundled modules, of which ~200–400 appear to be Anthropic-authored. The tool list was extracted definitively from the `sdk-tools.d.ts` type definition file shipped alongside the bundle.

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

Claude Code's numbers are proportional estimates. We have high confidence in the tool list (extracted from typed definitions) and feature set (documented in the DeepWiki analysis), but the LOC-per-category is directional, not precise.

Pi's "config/utils/other" category (26%) is inflated by subsystems that defy clean categorization — a package manager (2,254 LOC), HTML session export (3,638 LOC), auth storage (493 LOC), and the `pi-ai` utility layer (3,068 LOC). These aren't agent logic, but they aren't cleanly infrastructure either.

---

## The Four Agents at a Glance

|  | **Clyde** | **Pi** | **OpenCode** | **Claude Code** |
|---|---|---|---|---|
| Language | Go | TypeScript | TypeScript | TypeScript (bundled) |
| Production LOC | **3,793** | **98,516** | **229,295** (72,634 core) | **~30K–60K** est. first-party |
| Source files | 47 | 567 | 1,091 | 1 (`cli.js`, 13MB) |
| Packages | 13 (flat) | 7 | 19 | monolithic |
| Direct dependencies | 3 | ~20 | heavy | heavy (bundled) |
| Built-in tools | 11 | 7 | 8 + MCP + plugins | 21 + MCP |
| Providers | 1 (Anthropic) | 11 | 20+ | 3 (Anthropic, Bedrock, Vertex) |
| Test LOC | 10,376 | 48,007 | 63,184 | unknown |

**Clyde** describes itself as "a single-file Go CLI that provides a REPL interface for talking to Claude AI." It supports 11 tools (read, write, patch, bash, grep, glob, multi-patch, web search, browse, include image), prompt caching, extended thinking, CLI and REPL modes, and can be embedded as a Go library. It does not support session persistence, multiple providers, streaming, permissions, plugins, or a server API.

**Pi** is Mario Zechner's "opinionated and minimal coding agent." It's built from the ground up: a custom LLM API (`pi-ai`, 11 provider implementations, no Vercel AI SDK), a custom terminal UI framework (`pi-tui`, differential rendering, retained-mode components), and a custom agent runtime (`pi-agent-core`). It deliberately excludes MCP, permissions, subagents, plan mode, and compaction, arguing these are either unnecessary or counterproductive. Its system prompt and tool definitions total ~1,000 tokens.

**OpenCode** is positioned as the open-source alternative to Claude Code. It's a full platform: HTTP server with SSE, SolidJS-based UI (shared across TUI, desktop, and web), SQLite persistence via Drizzle ORM, LSP integration for 20+ languages, a plugin system with MCP support, and distribution via npm, Homebrew, Docker, Tauri, and Nix. The monorepo includes a SaaS console, VS Code/Zed extensions, and an SDK with auto-generated OpenAPI bindings.

**Claude Code** is Anthropic's commercial product. It ships as a single minified bundle with 21 built-in tools, multi-agent orchestration (subagents in isolated git worktrees), seccomp sandboxing on Linux, a plugin marketplace with a hook system (PreToolUse/PostToolUse/Stop), session persistence with optimized transcript streaming, context compaction, remote session handoff (`--teleport`), and GitHub automation (issue triage, @claude mentions, scheduled tasks).

---

## Where the Code Goes

### The detailed breakdown

For each agent, every line of production code was assigned to a single category. OpenCode percentages use the core package (72.6K LOC). Claude Code percentages are derived from reference-count proportions.

```
                          Clyde    Pi      OpenCode    Claude Code
                          3.8K    98.5K    72.6K       ~30-60K est.
────────────────────────────────────────────────────────────────────

Core agent loop            9%      8%       10%         14%
Tool implementations      40%      3%        5%          6%
Provider / LLM             6%     12%       11%         10%
TUI / CLI / rendering     36%     27%       13%          6%
Persistence / sessions     —       3%        3%         13%
Compaction                 —       1%        *           3%
Permission / security      —       —         1%          7%
Plugin / extensions        —       4%        6%         10%
LSP integration            —       —         4%          —
Server infrastructure      —       —         9%         10%
MCP                        —       —         2%          7%
Config / utils / other     8%     26%       18%         10%
Web UI                     —      12%        —           —
Chat (Slack)               —       4%        —           —
Infra (vLLM)               —       2%        —           —
Worktree isolation         —       —         1%          2%

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
  (agent loop + tools)     49%     11%       15%         20%

"Talking to models"
  (providers + LLM API)     6%     12%       11%         10%

"Looking good"
  (TUI + CLI + UI)         36%     39%       13%          6%

"Everything else"           8%     38%       61%         64%
  (persistence, perms,
   plugins, LSP, server,
   MCP, config, infra)
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
| **Claude Code** | unknown | unknown | ~10K–15K (estimated from proportions) |

Pi, OpenCode, and Claude Code all converge on roughly 10,000–15,000 lines for their agent engines despite having wildly different total sizes (98K vs. 229K vs. ~30–60K). The reason is straightforward: the agent loop is a thin orchestration layer. The LLM does the reasoning. The loop's job is mechanical — format a request, parse the response, dispatch tool calls, collect results, decide whether to loop or return. There are only so many edge cases (error handling, abort signals, thinking traces, image attachments, tool validation) before the code is complete.

Clyde's 1,882 lines represent the functional minimum for this pattern: a `for {}` loop in Go that calls the Anthropic API, iterates over content blocks, executes tools by name, and appends results to the conversation history. The entire `HandleMessage` function is 194 lines. It has no streaming, no subagent dispatch, no compaction triggers, no permission checks — and it works.

Pi and OpenCode reach ~10K by adding the features Clyde omits: structured event streaming, abort support with partial results, session state management, model resolution, prompt template injection, and (in OpenCode's case) compaction triggers. But none of them reach 15K. The loop just isn't that complicated.

This has implications for anyone evaluating coding agents. **If a project has 100,000 lines of code, roughly 10,000 of them are the agent.** The other 90,000 are the product around it. When you choose between coding agents, you are mostly choosing between those 90,000 lines — the UI, the persistence model, the provider support, the extension system — not between fundamentally different agent architectures.

### 2. Provider abstraction is a constant tax

Every agent that supports multiple providers spends 10–12% of its codebase on the LLM abstraction layer:

| | Provider LOC | % of codebase | Providers supported |
|---|---|---|---|
| **Clyde** | 241 | 6% | 1 (Anthropic) |
| **Pi** | 11,746 | 12% | 11 |
| **OpenCode** | 7,927 | 11% | 20+ |
| **Claude Code** | ~10% (ref. proportion) | 10% | 3 |

Clyde's 241 lines are what provider support looks like when you support exactly one provider: an HTTP POST to `api.anthropic.com`, JSON marshaling, and error handling with contextual suggestions. That's it. No streaming. No provider switching. No credential management for multiple services.

Pi's 11,746 lines are the price of building multi-provider support from scratch. Mario Zechner explicitly rejected the Vercel AI SDK — the standard abstraction used by OpenCode and many others — in favor of writing directly against the four underlying LLM APIs (OpenAI Completions, OpenAI Responses, Anthropic Messages, Google Generative AI). The result is 11 hand-rolled provider implementations, each handling the quirks of its target: Cerebras and xAI don't like the `store` field, Mistral uses `max_tokens` instead of `max_completion_tokens`, Google still doesn't support tool call streaming, and different providers report reasoning content in different fields.

OpenCode achieves broader provider coverage (20+) in less code (7,927) by building on the Vercel AI SDK, which handles the per-provider normalization. This is the classic build-vs-buy tradeoff: Pi gets full control and a smaller surface area at the cost of more code; OpenCode gets breadth at the cost of a dependency.

Claude Code, supporting only three providers (Anthropic's own API, plus AWS Bedrock and Google Vertex for enterprise customers), spends ~10% proportionally — comparable to the multi-provider agents despite its narrower scope. This likely reflects the complexity of Bedrock and Vertex integration (credential management, region handling, the AWS SDK alone accounts for hundreds of references in the bundle).

The takeaway: **if you want to support multiple providers, budget ~10–12% of your codebase for it.** If you only need one provider, the cost drops to nearly nothing.

### 3. The TUI: a hidden complexity sink

Terminal rendering is where the philosophical differences between these projects become most visible:

| | TUI/CLI LOC | % of codebase | Approach |
|---|---|---|---|
| **Clyde** | 1,358 | 36% | readline + spinner + ANSI colors |
| **Pi** | 26,920 | 27% | Custom TUI framework from scratch |
| **OpenCode** | 9,435 (core) / 88,604 (full) | 13% (core) | SolidJS component library |
| **Claude Code** | ~6% (ref. proportion) | 6% | Ink (React for terminals) |

Clyde and Pi both spend their largest single budget on the terminal experience — 36% and 27% respectively. But the absolute numbers reveal the gulf: Clyde's 1,358 lines buy a readline wrapper with history, a braille-dot spinner, ANSI color styling, and a git-aware prompt. Pi's 26,920 lines buy a retained-mode rendering engine with differential updates, synchronized output escape sequences for flicker-free drawing, a custom text editor with fuzzy file search and path completion, theme support with live reload, and 17 interactive components (session selector, config editor, model picker, diff viewer, and more).

Pi's `interactive-mode.ts` alone — the file that wires the TUI together — is 4,689 lines. That single file is larger than Clyde's entire codebase.

This is the most revealing data point about Pi's design philosophy. Mario Zechner's blog post describes pi as "minimal" and "opinionated," and by feature count it is — no permissions, no MCP, no subagents, no plan mode. But by LOC allocation, **Pi is primarily a terminal UI project.** Nearly 40% of its code (counting `pi-tui` + interactive mode + CLI + RPC mode together) is dedicated to making the terminal experience feel crafted. The "minimalism" is in the feature set, not the engineering investment.

OpenCode's 13% for the core TUI is misleading in isolation. The `packages/opencode` CLI subsystem is 9,435 lines, but the full monorepo includes `packages/app` (51,361 lines of shared SolidJS UI logic) and `packages/ui` (27,808 lines of UI components). These are shared across the TUI, desktop apps, and web interface. If you count the full UI surface, OpenCode's rendering investment is 88,604 lines — the largest absolute investment of any agent in any category.

Claude Code's 6% reflects a deliberate architectural choice: use Ink (React for terminals) off the shelf. Rather than building a custom rendering engine, Anthropic composes React components. This keeps the TUI code small relative to the rest of the product.

### 4. Infrastructure: the 60% majority

The most important table in this report:

```
"Everything else"           Clyde    Pi      OpenCode    Claude Code
  (persistence, perms,        8%     38%       61%         64%
   plugins, LSP, server,
   MCP, config, infra)
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

The LSP integration (4%, 2,919 lines) is a genuine differentiator. By running Language Server Protocol servers for 20+ languages, OpenCode gives its agent semantic code understanding — go-to-definition, find-references, diagnostics. No other agent in this comparison has this. Whether it's worth 2,919 lines depends on whether you believe agents benefit from structured code intelligence versus just reading files (the approach Clyde, Pi, and Claude Code all take).

**Claude Code's 64% breaks down as (proportional):**

| Subsystem | % of refs |
|---|---|
| Persistence / sessions / transcripts | 13% |
| Server / remote control / teleport | 10% |
| Plugins / hooks / skills / marketplace | 10% |
| Config / remote managed settings | 10% |
| Permission / security / seccomp | 7% |
| MCP integration | 7% |
| Compaction | 3% |
| Worktree isolation | 2% |

Claude Code's infrastructure is distinguished by its *depth* rather than its *breadth*. The permission system (7%) includes seccomp sandboxing on Linux, TOCTOU hardening for PowerShell, and a fail-closed remote settings mechanism where the agent won't start if it can't fetch managed policies. The session system (13%) includes optimized transcript streaming to prevent quadratic slowdowns on large conversations, prompt cache expiry hints, and cross-device session handoff via `--teleport`. The plugin system (10%) includes a marketplace, a development kit, and official plugins for code review, feature development, and security guidance.

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
- No LSP or semantic code understanding
- No server API — no way to connect other clients
- No session resume, no remote handoff, no scheduled tasks

Is this a good tradeoff? That depends entirely on the use case. For a single developer working in a terminal on one project at a time, using Claude models — which is a large fraction of how coding agents are actually used — the features Clyde omits are features that never trigger. You don't need session resume if you finish your task before closing the terminal. You don't need multi-provider support if you use one provider. You don't need permissions if you trust the agent (and as Zechner notes, "everybody is running in YOLO mode anyway").

The point isn't that Clyde is better than the alternatives. It's that Clyde demonstrates the *minimum*. Everything above 3,793 lines is a choice about which product concerns to invest in — and those choices account for 85–95% of the code in every other agent.

---

## Conclusion

The empirical finding of this analysis is simple and consistent across four very different codebases: **a coding agent is a small program wrapped in a large product.**

The agent loop — the part that actually calls an LLM, dispatches tool calls, and collects results — is 8–14% of every codebase. The tool implementations add another 3–6%. Together, the core engine that makes a coding agent *an agent* accounts for 11–20% of the total code, converging on roughly 10,000 lines in absolute terms across Pi, OpenCode, and Claude Code.

The other 80–89% is the answer to a different set of questions: How do users interact with this? (TUI frameworks, component libraries, IDE extensions.) How does it remember? (Session persistence, transcript management, compaction.) How does it stay safe? (Permission systems, sandboxing, hook-based approval workflows.) How do others extend it? (Plugin architectures, MCP integration, skill systems.) How do enterprises deploy it? (Server infrastructure, remote control, provider abstraction, managed settings.)

These are legitimate engineering concerns. In many cases they're the *reason* people choose one agent over another. OpenCode's LSP integration gives agents semantic code understanding. Claude Code's seccomp sandboxing provides real security guarantees. Pi's custom TUI delivers a noticeably smoother terminal experience. These aren't bloat — they're product decisions backed by tens of thousands of lines of carefully written code.

But Clyde's existence as a control group makes the underlying structure visible. You can build a fully functional coding agent — 11 tools, prompt caching, extended thinking, vision support, web access, CLI and REPL modes, library embedding — in 3,793 lines with 3 dependencies. Everything above that line is a product decision. And the data shows exactly how much each decision costs.

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

### Claude Code — proportional breakdown (from reference analysis)

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

### Tool inventories

| **Clyde** (11) | **Pi** (7) | **OpenCode** (8) | **Claude Code** (21) |
|---|---|---|---|
| list_files | read | read | FileRead |
| read_file | bash | bash | Bash |
| write_file | edit | edit | FileWrite |
| patch_file | write | write | FileEdit |
| multi_patch | grep | grep | Grep |
| run_bash | find | glob | Glob |
| grep | ls | task | Agent |
| glob | | apply_patch | NotebookEdit |
| web_search | | | TodoWrite |
| browse | | | WebFetch |
| include_file | | | WebSearch |
| | | | Mcp |
| | | | ListMcpResources |
| | | | ReadMcpResource |
| | | | AskUserQuestion |
| | | | Config |
| | | | EnterWorktree |
| | | | ExitWorktree |
| | | | ExitPlanMode |
| | | | TaskOutput |
| | | | TaskStop |
