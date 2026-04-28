# Input Editor Rewrite: Drop readline, Own the Terminal

## Motivation

The current input system wraps `chzyer/readline` to provide multiline editing.
This works for data capture (up/down arrow navigation between lines, history
suppression when the buffer has content) but **fails at display**: readline is a
single-line editor and has no concept of a multi-line block. When the user
navigates up to a previous line, readline swaps the buffer content on the
*current* terminal row, causing visual duplication instead of proper cursor
movement between lines.

Fixing the display within readline requires fighting its display model — writing
raw ANSI escapes behind its back and hoping its next `Refresh()` agrees with
where we left the cursor. This is fragile and unworkable long-term.

The solution: replace readline with a purpose-built multi-line editor (~500-600
lines of Go) that we fully control.

## Current Public API (preserved)

`cli/cli.go` uses only this surface from `cli/input`:

```go
reader, err := input.New(input.Config{
    Prompt:      "...",
    HistoryFile: "...",
    // Stdin, Stdout, Stderr overrides for testing
})
defer reader.Close()

reader.SetPrompt("updated prompt")
line, err := reader.ReadLine()   // returns assembled multiline string
writer := reader.Stdout()        // safe-to-write-while-editing writer
writer := reader.Stderr()        // same for stderr
```

**This API does not change.** `cli/cli.go` requires zero modifications.

## Architecture

The rewrite decomposes into 5 internal components, all within `cli/input/`:

### 1. Raw Mode (`rawmode.go` or inline)

~20 lines. Enter/exit raw terminal mode via `tcsetattr`/`tcgetattr` syscalls
using `golang.org/x/sys/unix`.

```go
func enterRawMode(fd int) (*unix.Termios, error)  // returns original state
func exitRawMode(fd int, original *unix.Termios) error
```

Raw mode disables:
- Canonical mode (line buffering) — we get bytes immediately
- Echo — we render ourselves
- Signal generation for Ctrl+C (we handle it as a key event)

### 2. Key Reader (`keys.go` or inline)

~100-150 lines. Reads stdin byte-by-byte, decodes into logical key events.

```go
type Key struct {
    Rune    rune    // printable character, or 0 for special keys
    Special KeyType // Enter, Up, Down, Left, Right, Home, End, Backspace, etc.
    Alt     bool    // Alt/Meta modifier
    Ctrl    bool    // Ctrl modifier
}

func readKey(r io.Reader) (Key, error)
```

Escape sequence decoding:
- `ESC [ A` → Up, `ESC [ B` → Down, `ESC [ C` → Right, `ESC [ D` → Left
- `ESC [ H` → Home, `ESC [ F` → End, `ESC [ 3 ~` → Delete
- `ESC O A/B/C/D/H/F` → same (SS3 variant)
- `ESC CR` → Alt+Enter (multiline trigger, same as current metaCRReader)
- Ctrl+A through Ctrl+Z → derived from byte value (1-26)
- UTF-8 multi-byte sequences → decoded into runes
- Bare ESC with no followup within ~50ms → literal Escape key

### 3. Line Buffer (`buffer.go` or inline)

~80 lines. A `[]rune` with cursor position and edit operations.

```go
type lineBuffer struct {
    runes  []rune
    cursor int
}

func (b *lineBuffer) Insert(r rune)
func (b *lineBuffer) Backspace() bool
func (b *lineBuffer) Delete() bool
func (b *lineBuffer) MoveLeft() / MoveRight() / MoveHome() / MoveEnd()
func (b *lineBuffer) Clear()
func (b *lineBuffer) Set(s string)
func (b *lineBuffer) String() string
func (b *lineBuffer) Len() int
```

Nothing surprising here — standard gap-buffer-style editing.

### 4. Display (`display.go` or inline)

~80 lines. Redraws the entire multi-line block using ANSI escape codes.

```go
func (e *Editor) redraw()
```

On every change (keystroke, navigation), the display function:

1. Moves cursor to the **top of the block** (`\033[<N>A` — move up N rows)
2. For each line in the block:
   - Clears the row (`\033[2K`)
   - Prints prompt + line content
   - Moves to next row (`\n` or `\033[B`)
3. Positions cursor at `(activeLineIdx, cursorCol)` within the block

Key detail: we track `displayedRows` — the number of terminal rows our block
currently occupies. This is updated after each redraw and used in step 1 to know
how far up to move. On the first draw of a line, `displayedRows` is 0 and we
just print forward.

**Line wrapping:** For lines longer than terminal width, we calculate wrapped row
count: `ceil((promptLen + runeCount) / termWidth)`. This affects `displayedRows`
tracking. Terminal width is queried via `TIOCGWINSZ` ioctl and refreshed on
`SIGWINCH`.

### 5. Editor (main `input.go`)

~200 lines. Wires everything together into the `Reader` struct.

```go
type Reader struct {
    lines       []lineBuffer  // all lines in the current input block
    activeIdx   int           // which line the cursor is on
    prompt      string        // main prompt ("You: ", etc.)
    contPrompt  string        // continuation prompt ("  > ")
    history     *history      // history stack with file persistence
    // ... terminal state, display state
}
```

**Keystroke dispatch:**

| Key | Single-line (1 line, not multiline) | Multiline (after Ctrl+J / Alt+Enter / `\`) |
|---|---|---|
| Printable char | Insert at cursor | Insert at cursor on active line |
| Enter | Submit | Submit entire block (join with `\n`) |
| Ctrl+J / Alt+Enter | Save current line, add new empty line below, enter multiline | Same — add new line below active line |
| Backslash + Enter | Strip `\`, save line, add new empty line, enter multiline | Same |
| Up | If buffer empty: history prev. Else: nothing. | Move to previous line (if not at first) |
| Down | If browsing history: history next. Else: nothing. | Move to next line (if not past last) |
| Left / Right | Move cursor within line | Move cursor within active line |
| Home / End | Move to start/end of line | Move to start/end of active line |
| Backspace | Delete char before cursor | Delete char; if at col 0 and not first line, merge with line above |
| Ctrl+C | Discard input, return ErrInterrupt | Discard all lines, return ErrInterrupt |
| Ctrl+D | If empty: return io.EOF. Else: delete forward. | If all lines empty: return io.EOF. |
| Ctrl+U | Clear line | Clear active line |
| Ctrl+L | Clear screen, redraw | Clear screen, redraw all lines |

**History** is a simple `[]string` loaded from / appended to a file. Up/down
on an empty, non-multiline prompt navigates. Assembled multiline blocks are saved
as a single entry with embedded `\n`.

### 6. History (`history.go` or inline)

~50 lines. Load/save from file, navigate with prev/next.

```go
type history struct {
    entries []string
    pos     int        // current browse position (-1 = not browsing)
    path    string     // file path for persistence
    limit   int        // max entries
}

func (h *history) Load() error
func (h *history) Add(entry string) error  // appends to file
func (h *history) Prev() (string, bool)
func (h *history) Next() (string, bool)
func (h *history) Reset()                  // stop browsing
```

## Files Touched

| File | Change |
|---|---|
| `cli/input/input.go` | Full rewrite (~500-600 lines) |
| `tests/input_test.go` | Update tests (same mock stdin approach, same scenarios) |
| `go.mod` / `go.sum` | Remove `github.com/chzyer/readline`, maybe add `golang.org/x/sys` |

**`cli/cli.go` requires zero changes** — the public API is preserved.

## What We Gain

- **Correct multi-line display**: up/down visually moves between lines, all lines
  stay visible, edits happen in-place
- **No dependency**: drop `chzyer/readline` (unmaintained, last real commit 2022)
- **No concurrency hacks**: no atomic variables to mirror state between goroutines,
  no Listener callback tricks — everything runs in a single goroutine
- **Full control**: easy to add future features (syntax highlighting, auto-indent,
  line numbers, etc.)

## What We Lose (and mitigations)

- **Vi/Emacs keybindings**: readline had optional vi mode. We don't need it — our
  users are in a chat REPL, not a shell. Basic movement keys suffice.
- **Tab completion**: readline had an AutoComplete interface. We don't use it
  (no tab completion in the Clyde prompt).
- **Edge case hardening**: readline accumulated years of terminal compatibility
  fixes. We mitigate by: (a) targeting modern terminals only (iTerm2, Terminal.app,
  Alacritty, kitty, standard xterm-256color), (b) keeping the code simple so edge
  cases are easy to fix when found.
- **Meta+b / Meta+f (word movement)**: Nice to have, easy to add later.

## Implementation Plan

1. **Build the editor** in `cli/input/input.go`, preserving the existing public API
2. **Update tests** — same test scenarios, adapted for the new internals
3. **Remove readline dependency** — `go mod tidy`
4. **Manual testing** — verify in iTerm2 and Terminal.app
5. **Commit** — single commit on the `rewrite-input-editor` branch

## Risk

Low. The blast radius is 1 package (`cli/input`) with a stable, narrow API. If
the rewrite has issues, reverting is a single `git revert`. The rest of the
codebase (agent, tools, MCP, sessions, compaction) is completely untouched.
