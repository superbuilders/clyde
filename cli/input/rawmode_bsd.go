//go:build darwin || freebsd || netbsd || openbsd

package input

import "golang.org/x/sys/unix"

// isTerminal returns true if fd refers to a terminal.
func isTerminal(fd int) bool {
	_, err := unix.IoctlGetTermios(fd, unix.TIOCGETA)
	return err == nil
}

// setupRawMode puts the terminal into raw mode for direct keystroke reading.
// Returns a restore function, the terminal width, and any error.
func setupRawMode(fd int) (restore func(), width int, err error) {
	orig, err := unix.IoctlGetTermios(fd, unix.TIOCGETA)
	if err != nil {
		return nil, 0, err
	}

	raw := *orig
	// Input: disable break/parity/strip/CR-NL translation/flow control
	raw.Iflag &^= unix.IGNBRK | unix.BRKINT | unix.PARMRK | unix.ISTRIP |
		unix.INLCR | unix.IGNCR | unix.ICRNL | unix.IXON
	// Output: keep OPOST enabled so \n → \r\n translation still works
	// for agent output written to stdout between ReadLine calls.
	// Local: disable echo, canonical mode, signals, extended processing
	raw.Lflag &^= unix.ECHO | unix.ECHONL | unix.ICANON | unix.ISIG | unix.IEXTEN
	// Control: 8-bit clean
	raw.Cflag &^= unix.CSIZE | unix.PARENB
	raw.Cflag |= unix.CS8
	// Read returns after 1 byte, no timeout
	raw.Cc[unix.VMIN] = 1
	raw.Cc[unix.VTIME] = 0

	if err := unix.IoctlSetTermios(fd, unix.TIOCSETA, &raw); err != nil {
		return nil, 0, err
	}

	w := 80
	if ws, err := unix.IoctlGetWinsize(fd, unix.TIOCGWINSZ); err == nil && ws.Col > 0 {
		w = int(ws.Col)
	}

	return func() { unix.IoctlSetTermios(fd, unix.TIOCSETA, orig) }, w, nil
}
