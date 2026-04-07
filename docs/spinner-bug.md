# Spinner Bug Report: Frame Bleed on Long / Multi-Line Messages

## Summary

The loading spinner in REPL mode prints every animation frame as a new line instead of overwriting in place, creating a scrolling waterfall of braille characters. The bug affects tool spinners (primarily `browse`, but also `run_bash`, `web_search`, and others) while the "Thinking..." spinner works correctly.

## Symptoms

When the spinner message is long enough to wrap past the terminal width, or when it contains embedded newline characters, each animation frame leaves a ghost line above the cursor. Instead of a single in-place animation:

```
⠹ Browsing: https://example.com/very/long/path?query=foo&bar=baz...
```

The user sees every frame stacked vertically, scrolling rapidly:

```
⠋ Browsing: https://example.com/very/long/path?query=foo&bar=baz
⠙ Browsing: https://example.com/very/long/path?query=foo&bar=baz
⠹ Browsing: https://example.com/very/long/path?query=foo&bar=baz
⠸ Browsing: https://example.com/very/long/path?query=foo&bar=baz
⠼ Browsing: https://example.com/very/long/path?query=foo&bar=baz
...
```

At 30 symbols/second, this produces a torrent of output that floods the terminal scrollback.

## Root Cause

The spinner's `renderFrame` method in `spinner/spinner.go` uses `\r\033[K` (carriage return + clear-to-end-of-line) to overwrite the spinner text in place:

```go
func (s *Spinner) renderFrame(symbol, message string) {
    fmt.Fprintf(s.writer, "\r\033[K%s %s", symbol, message)
}
```

This technique **only works when the entire rendered line fits within a single terminal row**. It fails in two scenarios:

### Failure Mode 1: Line Wrapping (Primary — the "browsing" bug)

When the spinner message (braille symbol + space + message text) exceeds the terminal width, the terminal soft-wraps the text to the next line. On the next frame:

1. `\r` (carriage return) moves the cursor to the beginning of the **current physical line** — which is now the *wrapped* portion, not the beginning of the message.
2. `\033[K` (clear to end of line) clears only that last physical line.
3. The new frame text is written starting from the wrapped line, pushing the cursor down again.

The previous frame's **first line** (above the wrap point) is never cleared and remains visible.

**Concrete example** (80-column terminal):

```
Frame 1 writes:
  \r\033[K⠋ Browsing: https://long-url.example.com/api/v1/documents?page=1&format=json...
Terminal renders (wraps at col 80):
  Line N:   ⠋ Browsing: https://long-url.example.com/api/v1/documents?page=1&format=jso
  Line N+1: n...
  Cursor: end of line N+1.

Frame 2 writes:
  \r\033[K⠙ Browsing: https://long-url.example.com/api/v1/documents?page=1&format=json...
  \r moves to start of line N+1. \033[K clears line N+1.
  New text starts at line N+1, wraps to N+2.
Terminal renders:
  Line N:   ⠋ Browsing: https://long-url.example.com/api/v1/documents?page=1&format=jso  ← STALE
  Line N+1: ⠙ Browsing: https://long-url.example.com/api/v1/documents?page=1&format=jso
  Line N+2: n...

Frame 3: same pattern continues, leaving lines N and N+1 as ghosts.
```

Each frame leaves one ghost line. At 30 frames/second, the terminal scrolls rapidly with dozens of stale lines per second.

### Failure Mode 2: Embedded Newlines

When the spinner message contains literal `\n` characters (e.g., from a multi-line bash command), the same problem occurs: `\r` only returns to the start of the last line, not the start of the multi-line message.

This can happen with `run_bash` when Claude sends multi-line commands:

```go
// tools/run_bash.go — displayRunBash does NO newline filtering:
func displayRunBash(input map[string]interface{}) string {
    command, _ := input["command"].(string)
    return fmt.Sprintf("→ Running bash: %s", command)
}
```

If `command` is `"cd /tmp\nls -la\ngrep foo bar"`, the display message is:
```
→ Running bash: cd /tmp
ls -la
grep foo bar
```

After `FormatSpinnerMessage`, this becomes a 3-line spinner message. Each frame's `\r` only clears the last line, leaving the first two lines as artifacts on every frame.

### The `clearLine()` method in `Stop()` has the same flaw

When the spinner stops, it calls `clearLine()`:

```go
func (s *Spinner) clearLine() {
    fmt.Fprintf(s.writer, "\r\033[K")
}
```

If the last rendered frame wrapped or had multiple lines, `\r\033[K` only clears the last physical line. The wrapped/preceding lines remain as permanent artifacts in the scrollback — they are never cleaned up, even after the spinner stops and the permanent `→` progress line is printed below them.

## Why "Thinking..." Is Unaffected

The "Thinking..." spinner message is always exactly `"⠋ Thinking..."` — 15 characters. This trivially fits in a single terminal row on any terminal, so `\r\033[K` works perfectly. The spinner was developed and tested with short messages like this, which is why the bug wasn't caught.

## Affected Tools

Any tool whose display function can produce a message that (after `FormatSpinnerMessage` processing) exceeds the terminal width or contains newlines:

| Tool | Display Function | Risk | Reason |
|---|---|---|---|
| **browse** | `displayBrowse` | **High** | URLs are commonly 60–200+ chars. With the `"⠋ Browsing: "` prefix (14 chars), messages routinely exceed 80 columns. URLs with query parameters, paths, and the optional `(extract: "...")` suffix make this the most commonly triggered case. |
| **run_bash** | `displayRunBash` | **High** | Multi-line commands (embedded `\n`) directly inject newlines into the spinner message. Long single-line commands (complex pipelines, find/exec chains) also wrap. |
| **web_search** | `displayWebSearch` | **Medium** | Long search queries can push past terminal width, especially with the `"⠋ Searching web: \"...\" "` prefix. |
| **include_file** | `displayIncludeFile` | **Medium** | Long file paths or URLs. |
| **grep** | `displayGrep` | **Low–Medium** | Long regex patterns or deep paths. |
| **glob** | `displayGlob` | **Low** | Typically short patterns. |
| **read_file** | `displayReadFile` | **Low** | Only extremely deep paths. |
| **patch_file** | `displayPatchFile` | **Low** | Path + byte count. |
| **write_file** | `displayWriteFile` | **Low** | Path + size. |
| **list_files** | `displayListFiles` | **Negligible** | Very short messages. |
| **multi_patch** | `displayMultiPatch` | **Negligible** | Just a file count. |

## Contributing Factor: Removal of Display Truncation (TUI-7)

The TUI-7 story (in `todos.md`) explicitly removed the 60-character and 50-character truncation that previously existed in the `run_bash` and `web_search` display functions:

> *"Single-line bash commands and search queries are **never** truncated at Normal level (the existing 60-char and 50-char truncation in display functions is removed)."*

This was the correct decision for the **permanent scrollback log** (the `→` progress lines), where users need to see the full command. However, the same untrimmed messages now flow into the spinner via `FormatSpinnerMessage`, which does no truncation of its own. Before TUI-7, the implicit 60-char limit kept most spinner messages safely under the terminal width; its removal exposed this latent bug.

## `FormatSpinnerMessage` Performs No Length or Newline Handling

```go
// spinner/spinner.go
func FormatSpinnerMessage(progressMsg string) string {
    msg := progressMsg
    if len(msg) >= 4 && msg[:4] == "→ " {
        msg = msg[4:]
    }
    if len(msg) >= 3 && msg[len(msg)-3:] == "..." {
        return msg
    }
    if len(msg) > 0 {
        msg += "..."
    }
    return msg
}
```

This function:
- ✅ Strips the `→ ` prefix
- ✅ Appends `...` if missing
- ❌ Does **not** detect or strip embedded newlines
- ❌ Does **not** truncate to terminal width
- ❌ Does **not** truncate to any fixed maximum length

## Test Gap

The test suite (`spinner/spinner_test.go` and `tests/spinner_test.go`) only tests with short, single-line messages:

- `"Testing..."` (10 chars)
- `"Working"` (7 chars)  
- `"Patching file: agent.go"` (23 chars)
- `"Reading file: main.go..."` (24 chars)
- `"Browsing: https://pkg.go.dev/net/http"` (38 chars)

No test uses a message exceeding 80 characters or containing a newline. The bug is invisible in the test suite.

## Proposed Fix: Verb-Only Spinner Messages

The simplest and most robust fix is to **stop putting arguments in the spinner at all**. The spinner is an ephemeral preview — its only job is to tell the user *what kind of operation* is in progress. The full details (URL, command, file path, byte counts) are already preserved in the permanent `→` scrollback line that prints when the operation completes. Duplicating that information in the spinner is what created the bug in the first place.

### The Convention

Spinner messages should be a single **verb + ellipsis**, matching the "Thinking..." pattern that already works:

| Tool | Current Spinner Message | New Spinner Message |
|---|---|---|
| browse | `Browsing: https://long-url.example.com/api/v1/docs?q=foo...` | `Browsing...` |
| run_bash | `Running bash: cd /tmp && find . -name "*.go" -exec grep TODO {} \;...` | `Running...` |
| web_search | `Searching web: "golang http client best practices 2026"...` | `Searching...` |
| read_file | `Reading file: src/components/dashboard/MainPanel.tsx...` | `Reading...` |
| patch_file | `Patching file: agent.go (+48 bytes)...` | `Patching...` |
| write_file | `Writing file: progress.md (42.5 KB)...` | `Writing...` |
| list_files | `Listing files: . (current directory)...` | `Listing...` |
| grep | `Searching: 'TODO' in ./tools/*.go...` | `Searching...` |
| glob | `Finding files: **/*.go in current directory...` | `Finding...` |
| multi_patch | `Applying multi-patch: 3 files...` | `Patching...` |
| include_file | `Including file: /path/to/screenshot.png...` | `Loading...` |
| *(agent)* | `Thinking...` | `Thinking...` *(unchanged)* |

Every message is ≤12 characters. Line wrapping and embedded newlines become structurally impossible, regardless of terminal width, URL length, command complexity, or file path depth. No terminal-width detection is needed. No truncation logic is needed. `clearLine()` remains correct as-is.

### Why This Is Better Than Truncation

A truncation-based fix (detect terminal width, clip to fit) would:
- Require a new dependency or `ioctl` call for terminal width detection
- Need a fallback for non-TTY writers (tests, pipes, CI)
- Still need newline stripping for multi-line commands
- Still risk edge cases with wide Unicode characters
- Add complexity to `FormatSpinnerMessage` for information the user doesn't need in the spinner anyway

The verb-only approach eliminates the entire class of problems by construction.

### Implementation

`FormatSpinnerMessage` becomes a simple verb extractor. It maps each tool's progress prefix to its verb:

```go
// spinner/spinner.go

// spinnerVerbs maps the "→ Action" prefix of a progress message to a
// short verb for the spinner. The spinner is an ephemeral preview —
// full details appear in the permanent → scrollback line.
var spinnerVerbs = map[string]string{
    "Browsing":        "Browsing...",
    "Running bash":    "Running...",
    "Searching web":   "Searching...",
    "Searching":       "Searching...",
    "Reading file":    "Reading...",
    "Patching file":   "Patching...",
    "Writing file":    "Writing...",
    "Listing files":   "Listing...",
    "Finding files":   "Finding...",
    "Applying multi-patch": "Patching...",
    "Including file":  "Loading...",
}

func FormatSpinnerMessage(progressMsg string) string {
    msg := progressMsg
    // Strip "→ " prefix
    if len(msg) >= 4 && msg[:4] == "→ " {
        msg = msg[4:]
    }

    // Match known verb prefixes → return short form
    for prefix, verb := range spinnerVerbs {
        if strings.HasPrefix(msg, prefix) {
            return verb
        }
    }

    // Fallback: take text up to first ":" or newline, add "..."
    if idx := strings.IndexAny(msg, ":\n"); idx > 0 {
        return msg[:idx] + "..."
    }
    if len(msg) > 0 && !strings.HasSuffix(msg, "...") {
        return msg + "..."
    }
    return msg
}
```

The fallback clause handles any future tools that aren't in the map — it strips everything after the first `:` or newline, which is where arguments begin by convention in all display functions (`"→ Verb: args"`).

### What to Update

| File | Change |
|---|---|
| `spinner/spinner.go` | Replace `FormatSpinnerMessage` with verb-only logic (above) |
| `spinner/spinner_test.go` | Update `TestFormatSpinnerMessage` expected values to verb-only |
| `tests/spinner_test.go` | Update `TestFormatSpinnerMessage_Integration` expected values |

No changes needed in tool display functions, `renderFrame`, `clearLine`, the agent, or `main.go`. The permanent `→` scrollback lines remain detailed and untouched.
