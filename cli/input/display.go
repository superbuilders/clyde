package input

import "fmt"

// redraw repaints the entire multi-line editing block.
//
// Strategy:
//  1. Move cursor to the top of the block (up displayedRows rows).
//  2. For each line: clear row, print prompt + content.
//  3. Position cursor at (activeIdx, cursor column).
func (r *Reader) redraw() {
	if !r.isTTY {
		return
	}

	var buf []byte

	// Move to the first line of our block
	if r.displayedRows > 0 {
		buf = append(buf, fmt.Sprintf("\033[%dA", r.displayedRows)...)
	}
	buf = append(buf, '\r')

	// Determine total lines to display (including virtual new-line position)
	n := len(r.lines)
	if r.activeIdx >= n {
		n = r.activeIdx + 1
	}

	for i := 0; i < n; i++ {
		buf = append(buf, "\033[2K"...) // clear entire row

		// Prompt
		if i == 0 {
			buf = append(buf, r.prompt...)
		} else {
			buf = append(buf, r.contPrompt...)
		}

		// Content
		if i < len(r.lines) {
			buf = append(buf, r.lines[i].String()...)
		}

		if i < n-1 {
			buf = append(buf, '\n')
		}
	}

	// Cursor is now at end of the last displayed line.
	// Move it to the active line and the correct column.
	activeRow := min(r.activeIdx, n-1)
	rowsUp := (n - 1) - activeRow
	if rowsUp > 0 {
		buf = append(buf, fmt.Sprintf("\033[%dA", rowsUp)...)
	}

	// Calculate the visible column for the cursor
	prompt := r.prompt
	if r.activeIdx > 0 {
		prompt = r.contPrompt
	}
	col := visibleLen(prompt)
	if r.activeIdx < len(r.lines) {
		col += r.lines[r.activeIdx].cursor
	}

	buf = append(buf, '\r')
	if col > 0 {
		buf = append(buf, fmt.Sprintf("\033[%dC", col)...)
	}

	r.displayedRows = n - 1
	r.stdout.Write(buf)
}

// finishDisplay moves the cursor past the editing block and starts a new line.
// Called when the input is submitted or interrupted.
func (r *Reader) finishDisplay() {
	if !r.isTTY {
		return
	}
	n := len(r.lines)
	if r.activeIdx >= n {
		n = r.activeIdx + 1
	}
	activeRow := min(r.activeIdx, n-1)
	rowsDown := (n - 1) - activeRow
	if rowsDown > 0 {
		fmt.Fprintf(r.stdout, "\033[%dB", rowsDown)
	}
	fmt.Fprint(r.stdout, "\n")
	r.displayedRows = 0
}

// clearScreen clears the terminal and redraws the editing block.
func (r *Reader) clearScreen() {
	if !r.isTTY {
		return
	}
	fmt.Fprint(r.stdout, "\033[2J\033[H")
	r.displayedRows = 0
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
