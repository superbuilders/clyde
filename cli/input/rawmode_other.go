//go:build !darwin && !freebsd && !netbsd && !openbsd && !linux

package input

import "fmt"

// setupRawMode is a stub for unsupported platforms.
func setupRawMode(fd int) (func(), int, error) {
	return nil, 80, fmt.Errorf("raw terminal mode not supported on this platform")
}
