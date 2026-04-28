package input

// lineBuffer holds a single line's runes and cursor position.
type lineBuffer struct {
	runes  []rune
	cursor int
}

func (b *lineBuffer) insert(r rune) {
	b.runes = append(b.runes, 0)
	copy(b.runes[b.cursor+1:], b.runes[b.cursor:])
	b.runes[b.cursor] = r
	b.cursor++
}

func (b *lineBuffer) backspace() bool {
	if b.cursor == 0 {
		return false
	}
	copy(b.runes[b.cursor-1:], b.runes[b.cursor:])
	b.runes = b.runes[:len(b.runes)-1]
	b.cursor--
	return true
}

func (b *lineBuffer) delete() bool {
	if b.cursor >= len(b.runes) {
		return false
	}
	copy(b.runes[b.cursor:], b.runes[b.cursor+1:])
	b.runes = b.runes[:len(b.runes)-1]
	return true
}

func (b *lineBuffer) moveLeft() {
	if b.cursor > 0 {
		b.cursor--
	}
}

func (b *lineBuffer) moveRight() {
	if b.cursor < len(b.runes) {
		b.cursor++
	}
}

func (b *lineBuffer) moveHome() { b.cursor = 0 }

func (b *lineBuffer) moveEnd() { b.cursor = len(b.runes) }

func (b *lineBuffer) clear() {
	b.runes = b.runes[:0]
	b.cursor = 0
}

func (b *lineBuffer) set(s string) {
	b.runes = []rune(s)
	b.cursor = len(b.runes)
}

// String returns the buffer content as a string.
func (b *lineBuffer) String() string { return string(b.runes) }

// Len returns the number of runes in the buffer.
func (b *lineBuffer) Len() int { return len(b.runes) }
