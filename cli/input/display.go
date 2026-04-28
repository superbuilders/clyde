package input

import "fmt"

// redraw repaints the entire multi-line editing block.
//
// Strategy:
//  1. Move cursor to the top of the block (up cursorRow physical rows).
//  2. Clear from cursor to end of screen (\033[J) — this correctly
//     handles wrapped lines that span multiple physical rows.
//  3. For each logical line: print prompt + content (terminal auto-wraps).
//  4. Position cursor at the correct physical row and column within the
//     active line.
//
// cursorRow tracks the physical terminal row offset from the top of the
// editing block. This accounts for line wrapping: a single logical line
// that exceeds termWidth occupies multiple physical rows.
func (r *Reader) redraw() {
	if !r.isTTY {
		return
	}

	var buf []byte

	// Move to the first line of our block (cursorRow is physical rows)
	if r.cursorRow > 0 {
		buf = append(buf, fmt.Sprintf("\033[%dA", r.cursorRow)...)
	}
	buf = append(buf, '\r')

	// Clear from cursor to end of screen — handles any wrapped content
	buf = append(buf, "\033[J"...)

	// Determine total logical lines to display (including virtual new-line position)
	n := len(r.lines)
	if r.activeIdx >= n {
		n = r.activeIdx + 1
	}

	activeRow := min(r.activeIdx, n-1)
	totalPhysRows := 0
	activePhysRow := 0

	for i := 0; i < n; i++ {
		if i == activeRow {
			activePhysRow = totalPhysRows
		}

		// Prompt
		prompt := r.contPrompt
		if i == 0 {
			prompt = r.prompt
		}
		buf = append(buf, prompt...)

		// Content
		var content string
		if i < len(r.lines) {
			content = r.lines[i].String()
		}
		buf = append(buf, content...)

		// Count physical rows this logical line occupies
		lineWidth := visibleLen(prompt) + len([]rune(content))
		totalPhysRows += physRowCount(lineWidth, r.termWidth)

		if i < n-1 {
			buf = append(buf, '\n')
		}
	}

	// Calculate the cursor's physical row and column.
	cursorPrompt := r.contPrompt
	if activeRow == 0 {
		cursorPrompt = r.prompt
	}
	cursorOffset := visibleLen(cursorPrompt)
	if activeRow < len(r.lines) {
		cursorOffset += r.lines[activeRow].cursor
	}

	cursorPhysInLine := 0
	cursorCol := cursorOffset
	if r.termWidth > 0 && cursorOffset >= r.termWidth {
		cursorPhysInLine = cursorOffset / r.termWidth
		cursorCol = cursorOffset % r.termWidth
		// Exact boundary: cursor is in deferred-wrap state at end of
		// the previous physical row, not at column 0 of the next one.
		if cursorCol == 0 && cursorPhysInLine > 0 {
			cursorPhysInLine--
			cursorCol = r.termWidth
		}
	}

	targetPhysRow := activePhysRow + cursorPhysInLine
	currentPhysRow := totalPhysRows - 1 // we're at the end of the last line

	rowsUp := currentPhysRow - targetPhysRow
	if rowsUp > 0 {
		buf = append(buf, fmt.Sprintf("\033[%dA", rowsUp)...)
	}

	buf = append(buf, '\r')
	if cursorCol > 0 {
		buf = append(buf, fmt.Sprintf("\033[%dC", cursorCol)...)
	}

	r.cursorRow = targetPhysRow
	r.stdout.Write(buf)
}

// finishDisplay moves the cursor past the editing block and starts a new line.
// Called when the input is submitted or interrupted.
func (r *Reader) finishDisplay() {
	if !r.isTTY {
		return
	}

	// Calculate total physical rows in the editing block.
	n := len(r.lines)
	if r.activeIdx >= n {
		n = r.activeIdx + 1
	}
	totalPhysRows := 0
	for i := 0; i < n; i++ {
		prompt := r.contPrompt
		if i == 0 {
			prompt = r.prompt
		}
		var content string
		if i < len(r.lines) {
			content = r.lines[i].String()
		}
		lineWidth := visibleLen(prompt) + len([]rune(content))
		totalPhysRows += physRowCount(lineWidth, r.termWidth)
	}

	rowsDown := (totalPhysRows - 1) - r.cursorRow
	if rowsDown > 0 {
		fmt.Fprintf(r.stdout, "\033[%dB", rowsDown)
	}
	fmt.Fprint(r.stdout, "\n")
	r.cursorRow = 0
}

// physRowCount returns the number of physical terminal rows needed to
// display content of the given visible width. Returns 1 for empty or
// non-wrapping content, and handles non-TTY (termWidth=0) gracefully.
func physRowCount(width, termWidth int) int {
	if termWidth <= 0 || width <= termWidth {
		return 1
	}
	return (width + termWidth - 1) / termWidth
}

// clearScreen clears the terminal and redraws the editing block.
func (r *Reader) clearScreen() {
	if !r.isTTY {
		return
	}
	fmt.Fprint(r.stdout, "\033[2J\033[H")
	r.cursorRow = 0
	r.redraw()
}

// visibleLen returns the visible (printed) width of s,
// skipping ANSI escape sequences.
func visibleLen(s string) int {
	n := 0
	inEsc := false
	for _, r := range s {
		if inEsc {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEsc = false
			}
			continue
		}
		if r == '\033' {
			inEsc = true
			continue
		}
		n++
	}
	return n
}
