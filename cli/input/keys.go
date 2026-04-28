package input

import (
	"io"
	"unicode/utf8"
)

// keyType represents a special (non-printable) key.
type keyType int

const (
	keyNone keyType = iota
	keyEnter
	keyUp
	keyDown
	keyLeft
	keyRight
	keyHome
	keyEnd
	keyBackspace
	keyDelete
	keyCtrlC
	keyCtrlD
	keyCtrlJ // Ctrl+J / Alt+Enter — multiline trigger
	keyCtrlL
	keyCtrlU
	keyEscape
)

// key represents a decoded keystroke.
type key struct {
	r       rune    // printable character, or 0 for special keys
	special keyType // non-zero for special keys
	alt     bool    // Alt/Meta modifier
}

// readKey reads and decodes a single keystroke from r.
//
// Byte-level decoding:
//
//	0x0D (CR)          → Enter
//	0x0A (LF)          → Ctrl+J
//	0x1B ...            → escape sequences (arrows, home, end, etc.)
//	0x1B 0x0D           → Alt+Enter (treated as Ctrl+J)
//	0x7F / 0x08        → Backspace
//	0x03               → Ctrl+C
//	0x04               → Ctrl+D
//	0x0C               → Ctrl+L
//	0x15               → Ctrl+U
//	0x01               → Home (Ctrl+A)
//	0x05               → End (Ctrl+E)
//	0x20–0x7E          → printable ASCII
//	0x80+              → UTF-8 multi-byte → decoded to rune
func readKey(r io.Reader) (key, error) {
	b, err := readByte(r)
	if err != nil {
		return key{}, err
	}

	switch {
	case b == 0x1b:
		return readEscSequence(r)
	case b == 0x0d:
		return key{special: keyEnter}, nil
	case b == 0x0a:
		return key{special: keyCtrlJ}, nil
	case b == 0x7f, b == 0x08:
		return key{special: keyBackspace}, nil
	case b == 0x03:
		return key{special: keyCtrlC}, nil
	case b == 0x04:
		return key{special: keyCtrlD}, nil
	case b == 0x0c:
		return key{special: keyCtrlL}, nil
	case b == 0x15:
		return key{special: keyCtrlU}, nil
	case b == 0x01:
		return key{special: keyHome}, nil
	case b == 0x05:
		return key{special: keyEnd}, nil
	case b < 0x20:
		return key{}, nil // other control chars — ignore
	case b < 0x80:
		return key{r: rune(b)}, nil
	default:
		return readUTF8(r, b)
	}
}

// readEscSequence decodes an escape sequence starting after the initial ESC byte.
func readEscSequence(r io.Reader) (key, error) {
	b, err := readByte(r)
	if err != nil {
		return key{special: keyEscape}, nil // bare ESC at EOF
	}

	switch b {
	case '[':
		return readCSI(r)
	case 'O':
		return readSS3(r)
	case 0x0d:
		return key{special: keyCtrlJ}, nil // ESC + CR = Alt+Enter → Ctrl+J
	default:
		if b >= 0x20 && b < 0x7f {
			return key{r: rune(b), alt: true}, nil
		}
		return key{}, nil
	}
}

// readCSI decodes a CSI (ESC [) sequence.
//
// CSI sequences have the general form: ESC [ <params> <final>
// where <params> is zero or more digits and semicolons (0x30–0x3B),
// and <final> is a byte in 0x40–0x7E that identifies the key/action.
//
// Examples:
//
//	ESC [ A          — Up arrow (no params)
//	ESC [ 1 ; 5 A   — Ctrl+Up (params "1;5")
//	ESC [ 3 ~       — Delete (param "3", final "~")
//	ESC [ 3 ; 2 ~   — Shift+Delete (params "3;2", final "~")
//	ESC [ 1 ; 2 H   — Shift+Home
func readCSI(r io.Reader) (key, error) {
	// Read parameter bytes and the final byte.
	var params []byte
	b, err := readByte(r)
	if err != nil {
		return key{}, nil
	}
	for (b >= '0' && b <= '9') || b == ';' {
		params = append(params, b)
		b, err = readByte(r)
		if err != nil {
			return key{}, nil
		}
	}

	// b is now the final byte that identifies the key.
	switch b {
	case 'A':
		return key{special: keyUp}, nil
	case 'B':
		return key{special: keyDown}, nil
	case 'C':
		return key{special: keyRight}, nil
	case 'D':
		return key{special: keyLeft}, nil
	case 'H':
		return key{special: keyHome}, nil
	case 'F':
		return key{special: keyEnd}, nil
	case '~':
		// Tilde-terminated: ESC [ <n> ~
		// The first param digit identifies the key.
		if len(params) > 0 {
			switch params[0] {
			case '1', '7':
				return key{special: keyHome}, nil
			case '3':
				return key{special: keyDelete}, nil
			case '4', '8':
				return key{special: keyEnd}, nil
			// 2=Insert, 5=PageUp, 6=PageDown — unhandled, ignored
			}
		}
		return key{}, nil
	default:
		return key{}, nil // unknown CSI sequence
	}
}

// readSS3 decodes an SS3 (ESC O) sequence.
func readSS3(r io.Reader) (key, error) {
	b, err := readByte(r)
	if err != nil {
		return key{}, nil
	}

	switch b {
	case 'A':
		return key{special: keyUp}, nil
	case 'B':
		return key{special: keyDown}, nil
	case 'C':
		return key{special: keyRight}, nil
	case 'D':
		return key{special: keyLeft}, nil
	case 'H':
		return key{special: keyHome}, nil
	case 'F':
		return key{special: keyEnd}, nil
	default:
		return key{}, nil
	}
}

// readUTF8 decodes a multi-byte UTF-8 rune starting with first.
func readUTF8(r io.Reader, first byte) (key, error) {
	var expected int
	switch {
	case first&0xe0 == 0xc0:
		expected = 2
	case first&0xf0 == 0xe0:
		expected = 3
	case first&0xf8 == 0xf0:
		expected = 4
	default:
		return key{}, nil // invalid lead byte
	}

	buf := make([]byte, expected)
	buf[0] = first
	for i := 1; i < expected; i++ {
		b, err := readByte(r)
		if err != nil {
			return key{}, nil
		}
		buf[i] = b
	}

	ru, _ := utf8.DecodeRune(buf)
	if ru == utf8.RuneError {
		return key{}, nil
	}
	return key{r: ru}, nil
}

// readByte reads a single byte from r.
func readByte(r io.Reader) (byte, error) {
	var buf [1]byte
	n, err := r.Read(buf[:])
	if n == 0 {
		return 0, err
	}
	return buf[0], nil
}
