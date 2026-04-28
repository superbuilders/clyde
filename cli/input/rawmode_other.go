//go:build !darwin && !freebsd && !netbsd && !openbsd && !linux

package input

import "fmt"

// isTerminal always returns false on unsupported platforms.
func isTerminal(fd int) bool { return false }

// setupRawMode is a stub for unsupported platforms.
func setupRawMode(fd int) (func(), int, error) {
	return nil, 80, fmt.Errorf("raw terminal mode not supported on this platform")
}
