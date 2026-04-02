# Clyde Terminal UI Specification

This document specifies the terminal user interface for Clyde's REPL (TUI) mode and CLI mode. The core design principle is that **CLI mode and TUI mode output should be as close as possible** — the TUI is a scrolling log, not a full-screen curses application.

---

## Table of Contents

1. [Design Principles](#design-principles)
2. [Screen Layout](#screen-layout)
3. [Log Levels](#log-levels)
4. [Color Scheme](#color-scheme)
5. [Thinking Traces](#thinking-traces)
6. [Tool Call Display](#tool-call-display)
7. [Truncation Rules](#truncation-rules)
8. [Loading Spinner](#loading-spinner)
9. [Prompt Line](#prompt-line)
10. [Text Input](#text-input)
11. [Cache & Context Display](#cache--context-display)

---

## Design Principles

1. **CLI ≈ TUI**: The output of CLI mode and TUI mode should be nearly identical. Both produce a scrolling log. The TUI simply adds an input widget and a spinner overlay.

2. **Minimal redraws**: Redraws (overwriting previously rendered content) are **only** permitted in two ephemeral zones:
   - The **input line** (bottom of screen) — because it hasn't been submitted yet.
   - The **spinner line** (second from bottom) — but any textual information displayed here **must** also appear in the permanent scrollback log once the operation completes.

3. **Log levels control display, not capture**: Log levels determine what the user *sees*. File logs (when implemented) always capture everything at the maximum detail level regardless of the display setting.

4. **Theme-aware colors**: Color choices must account for both dark and light terminal backgrounds. Avoid pure black or pure white as semantic colors. Prefer colors that are legible on both.

---

## Screen Layout

```
┌─────────────────────────────────────────────┐
│                                             │
│  Permanent scrollback log                   │  ← Normal terminal scrollback.
│  (user input, agent responses,              │    Append-only. Never redrawn.
│   tool logs, thinking traces)               │
│                                             │
│  ...                                        │
│                                             │
├─────────────────────────────────────────────┤
│  ⠹ Patching file: agent.go...              │  ← Spinner line (2nd from bottom).
├─────────────────────────────────────────────┤
│  main* 12% You:                             │  ← Input line (bottom).
└─────────────────────────────────────────────┘
```

The top area is standard terminal scrollback — content is appended and never modified. The bottom two lines are the only ephemeral/redrawn zones.

In **CLI mode**, there is no input line or spinner line. Output goes to stdout/stderr as a plain log.

---

## Log Levels

Five levels, controlled by a session-level flag. Applies to both CLI and TUI modes.

| Level | Flag | Description |
|---|---|---|
| **Silent** | `--silent` | No output at all — not even the final response. For environments where stdio is never read. Output is via side effects only (file writes, git commits, etc.). |
| **Quiet** | `-q` / `--quiet` | Single-line tool call logs (`→ Patching file: foo.go (+22 bytes)`), then the agent's final response. Essentially what Clyde produces today. No thinking traces. No tool output bodies. |
| **Normal** | *(default)* | Thinking traces (truncated), tool output bodies (truncated), full tool call logs, agent response. The full experience with reasonable limits. |
| **Verbose** | `-v` / `--verbose` | Same as Normal but **all truncation removed**. Full thinking traces, full tool outputs, no line or character limits. Cache stats shown as fraction. |
| **Debug** | `--debug` | Verbose + internal harness diagnostics: token counts, cache fractions, API latency, request/response sizes, model info, etc. Intended for Clyde core developers, not end users. |

### Log level does not affect file logs

When file-based logging is implemented, log files always capture at debug-equivalent detail regardless of the display level setting. The log level is purely a display filter.

---

## Color Scheme

Colors are applied via ANSI escape codes. All choices must be legible on both dark and light terminal backgrounds.

### Conversation Elements

| Element | Style | Rationale |
|---|---|---|
| **`You:` label** (and prompt line) | **Bold cyan** | Distinct prompt marker. Cyan reads well on both dark and light backgrounds. |
| **User input text** | Default terminal foreground | Clean, unadorned — it's what the user typed. |
| **`Claude:` label** | **Bold green** | Warm "response" feel, clearly distinct from user. |
| **Agent response text** | Default terminal foreground | Body text is always plain for readability. |
| **`{TOOLNAME}:` label** (the `→` progress lines) | **Bold yellow** | Action/activity color. Draws attention to tool invocations. |
| **Tool output body** (file listings, grep results, etc.) | **Dim** (gray/faint) | Secondary to the conversation. Scannable but not loud. |
| **Thinking trace text** | **Dim magenta** | Visually distinct "internal thought" — clearly not part of the response. |
| **Debug-level log lines** | **Red** | Only visible at debug level. Immediately distinguishable as harness internals. |

### Principle: Labels are bold and colored, body text is default or dim

A user should be able to scan a long session and reconstruct the conversation structure from labels alone.

### Theme Awareness

The implementation must not rely on "black text" or "white text" for semantic meaning. Use:
- **Default foreground** for body text (adapts to the user's theme).
- **Dim/faint** attribute for secondary content (works on both backgrounds).
- Named ANSI colors (cyan, green, yellow, magenta, red) for labels — these are generally theme-safe.
- Avoid hardcoded RGB values unless providing both dark-mode and light-mode variants.

---

## Thinking Traces

### Enabling Thinking

Thinking is enabled via the Claude API `thinking` parameter:

```json
{
  "thinking": {
    "type": "enabled",
    "budget_tokens": 8192
  }
}
```

This is **opt-in** at the API level — Clyde must send this parameter to receive `thinking` content blocks in responses. Without it, the model reasons internally but the reasoning is invisible.

When enabled, responses include `thinking` content blocks containing the model's reasoning before tool calls and final answers. These are valuable for understanding *why* the agent is making decisions.

**Thinking should be enabled by default.** The cost is modest (the thinking tokens are short for simple tasks and scale with complexity) and the transparency is worth it for a coding agent.

### Configuration

- `budget_tokens` should be configurable (default: 8192, or set via config).
- A `--no-think` flag could disable thinking entirely to save cost.

### Display by Log Level

| Level | Thinking Display |
|---|---|
| Silent | Not shown |
| Quiet | Not shown |
| Normal | Shown, truncated at 50 lines. Dim magenta text, prefixed with `💭`. |
| Verbose | Shown in full, no truncation. |
| Debug | Shown in full, no truncation. |

### Example (Normal level)

```
💭 The user wants to rename a function across multiple files.
   I should use grep first to find all occurrences, then
   use multi_patch to coordinate the changes. Let me check
   which files contain the function name...
   ... (12 more lines)

→ Searching: 'oldFunction' in . (*.go)
```

---

## Tool Call Display

### Progress Lines

Every tool call produces a single-line progress message. These use the `→` prefix and are styled with bold yellow for the tool label.

```
→ Patching file: agent.go (+48 bytes)
→ Running bash: go test ./...
→ Searching: 'TODO' in ./tools/*.go
→ Browsing: https://pkg.go.dev/net/http
```

These lines appear at **all log levels except Silent**.

### Tool Output Bodies

At **Normal** level and above, the actual output returned by tools is also displayed (in dim text), with truncation at Normal level:

```
→ Listing files: .

  total 64
  drwxr-xr-x  12 user  staff   384 Jun 15 10:00 .
  -rw-r--r--   1 user  staff  1234 Jun 15 09:00 main.go
  -rw-r--r--   1 user  staff   567 Jun 15 09:00 agent.go
  ... (8 more lines)

Claude: You have 12 files in the current directory...
```

At **Quiet** level, only the `→` progress line is shown. At **Verbose** and above, tool output is shown without truncation.

### Newline Separation

Tool progress lines and tool output bodies are separated from surrounding content by blank lines above and below, ensuring they don't visually merge with conversation text.

---

## Truncation Rules

Truncation only applies at **Normal** and **Quiet** levels. **Verbose** and **Debug** remove all truncation. **Quiet** doesn't show content that would be truncated anyway (no tool output bodies, no thinking traces).

### Line Truncation (Normal level)

| Content Type | Max Lines | Overflow Display |
|---|---|---|
| Tool output body | 25 lines | `... (N more lines)` |
| Thinking traces | 50 lines | `... (N more lines)` |
| Bash command display | No line limit for single-line commands | See character truncation |

### Character Truncation (Normal level)

| Content Type | Max Characters per Line | Overflow Display |
|---|---|---|
| Any single line | 2000 characters | `...` appended |

This handles edge cases like base64 data or minified files appearing in a single line.

### Bash & Search Query Display

**Single-line commands and queries are never truncated** at Normal level. The user must see exactly what is being executed. The current 60-character truncation in `run_bash` and 50-character truncation in `web_search` display functions are removed.

Multi-line bash commands (unusual but possible) follow the standard line truncation: up to 25 lines at Normal, full at Verbose.

### Verbose Behavior

At Verbose level:
- All line truncation limits are removed.
- All character-per-line truncation limits are removed.
- Full tool outputs, full thinking traces, full commands — everything displayed as-is.

---

## Loading Spinner

### Appearance

The spinner uses the **braille dots** Unicode symbol set, cycling through:

```
⠋ ⠙ ⠹ ⠸ ⠼ ⠴ ⠦ ⠧ ⠇ ⠏
```

### Animation Parameters

| Parameter | Value |
|---|---|
| Frame delay | `1/60` seconds (~16.7ms) |
| Frames per symbol | 2 |
| Effective rate | ~30 symbols/second |

### Position

The spinner occupies the **second line from the bottom** of the terminal. It is an ephemeral zone — content here is redrawn in place and cleared when the operation completes.

### Content

The spinner line shows the current operation:

```
⠹ Patching file: agent.go...
⠼ Running bash: go test ./...
⠧ Thinking...
```

### Persistence Rule

**Any textual information displayed on the spinner line must also appear in the permanent scrollback log.** The spinner is a live preview, not a replacement for the log entry. When a tool call completes, the spinner clears and the permanent `→` progress line is appended to the scrollback.

Example flow:
1. Spinner shows: `⠹ Patching file: agent.go...` (ephemeral, redrawn)
2. Tool completes. Spinner clears.
3. Scrollback receives: `→ Patching file: agent.go (+48 bytes)` (permanent)

### CLI Mode

In CLI mode, there is no spinner. Progress messages go directly to stderr as permanent log lines.

---

## Prompt Line

### Format

The prompt line sits at the bottom of the terminal and includes contextual information:

```
main* 12% You: 
```

Components:
- **Git branch** (`main`): Current branch name via `git rev-parse --abbrev-ref HEAD`.
- **Dirty indicator** (`*`): Present if there are uncommitted changes (`git status --porcelain`).
- **Context window usage** (`12%`): Percentage of the model's context window currently used by the conversation history.
- **`You:` label**: Bold cyan, per the color scheme.

### Git Info

Modeled after oh-my-zsh's git prompt integration:

| State | Display |
|---|---|
| Clean, on branch `main` | `main` |
| Dirty, on branch `main` | `main*` |
| Detached HEAD | `abc1234` (short hash) |
| Not a git repo | *(git info omitted entirely)* |

Git info is refreshed on each prompt render (cheap — these are fast local git commands).

### Context Window Usage

Displayed as a compact percentage like `12%`. Calculated as:

```
(total_input_tokens_last_turn / model_context_window_size) * 100
```

This gives the user a sense of how much conversation history has accumulated and how close they are to the context limit.

### CLI Mode

In CLI mode, there is no prompt line. The user provides input via arguments, files, or stdin.

---

## Text Input

### Current State

The current implementation uses `bufio.NewReader(os.Stdin)` with `ReadString('\n')` — the most basic possible line reader. Limitations:
- No cursor movement (can't press left/right to edit within the line).
- No multiline input (Enter always submits).
- Awkward behavior with long input.

### Required Capabilities

| Capability | Description |
|---|---|
| **Cursor movement** | Left/right arrow keys to navigate within the input. Home/End to jump to start/end. |
| **Multiline input** | A key combination (e.g., Shift+Enter, Alt+Enter, or Ctrl+J) inserts a newline. Enter submits. |
| **No length limit** | Input should handle arbitrarily long text without degradation. |
| **History recall** | Up/down arrow keys to recall previous inputs (nice-to-have). |

### Implementation Approach

Use a Go terminal input library that provides readline-like functionality. Candidates:
- `chzyer/readline` — mature, handles multiline, good readline compatibility.
- `peterh/liner` — simple, readline-like, well-tested.
- `charmbracelet/bubbletea` — full TUI framework; powerful but may be more than needed.
- `golang.org/x/term` — low-level building block; would require significant custom code on top.

The chosen library must integrate with the spinner (which occupies the line above the input) without conflict.

---

## Cache & Context Display

### Current State

Today, cache hits display as:
```
💾 Cache hit: 3715 tokens (100% of input)
```

This is nearly always "100%" because conversation history accumulates, making it noise.

### New Behavior

Cache information is only shown at **Verbose** and **Debug** levels, displayed as a token fraction:

```
💾 Cache: 3715/4102 tokens
```

At **Debug** level, additional detail:

```
💾 Cache: 3715/4102 tokens | Creation: 387 tokens | Context: 12% (4102/128000)
```

### Context Window Percentage

The context window usage percentage (`12%`) is surfaced on the **prompt line** (see [Prompt Line](#prompt-line)), making it always visible without adding log noise. This replaces the cache hit message as the primary "how full is my context?" indicator.

---

## Summary: What Changes at Each Log Level

| Feature | Silent | Quiet | Normal | Verbose | Debug |
|---|---|---|---|---|---|
| Final agent response | ✗ | ✓ | ✓ | ✓ | ✓ |
| `→` tool progress lines | ✗ | ✓ | ✓ | ✓ | ✓ |
| Tool output bodies | ✗ | ✗ | ✓ (truncated) | ✓ (full) | ✓ (full) |
| Thinking traces | ✗ | ✗ | ✓ (truncated) | ✓ (full) | ✓ (full) |
| Cache stats | ✗ | ✗ | ✗ | ✓ (fraction) | ✓ (detailed) |
| Harness diagnostics | ✗ | ✗ | ✗ | ✗ | ✓ |
| Context % on prompt | — | ✓ | ✓ | ✓ | ✓ |
| Spinner (TUI only) | ✗ | ✓ | ✓ | ✓ | ✓ |
