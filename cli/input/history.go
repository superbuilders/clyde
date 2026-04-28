package input

import (
	"os"
	"path/filepath"
	"strings"
)

// history manages command history with file persistence.
type history struct {
	entries []string
	pos     int    // browse position: -1 = not browsing
	path    string // file path for persistence (empty = no persistence)
	limit   int    // max entries to keep
}

func newHistory(path string, limit int) *history {
	return &history{pos: -1, path: path, limit: limit}
}

// Load reads history entries from the file (one per line).
// Missing or unreadable files are silently ignored.
func (h *history) Load() error {
	if h.path == "" {
		return nil
	}
	data, err := os.ReadFile(h.path)
	if err != nil {
		return nil // silently ignore
	}
	for _, line := range strings.Split(string(data), "\n") {
		if line != "" {
			h.entries = append(h.entries, line)
		}
	}
	if len(h.entries) > h.limit {
		h.entries = h.entries[len(h.entries)-h.limit:]
	}
	return nil
}

// Add appends an entry to the in-memory list and to the history file.
func (h *history) Add(entry string) error {
	h.entries = append(h.entries, entry)
	if len(h.entries) > h.limit {
		h.entries = h.entries[len(h.entries)-h.limit:]
	}
	if h.path == "" {
		return nil
	}
	dir := filepath.Dir(h.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	f, err := os.OpenFile(h.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(entry + "\n")
	return err
}

// Prev moves backward in history, returning the entry and true.
// Returns ("", false) when already at the oldest entry.
func (h *history) Prev() (string, bool) {
	if len(h.entries) == 0 {
		return "", false
	}
	if h.pos == -1 {
		h.pos = len(h.entries) - 1
	} else if h.pos > 0 {
		h.pos--
	} else {
		return h.entries[0], false
	}
	return h.entries[h.pos], true
}

// Next moves forward in history, returning the entry and true.
// When past the newest entry, returns ("", true) and stops browsing.
func (h *history) Next() (string, bool) {
	if h.pos == -1 {
		return "", false
	}
	h.pos++
	if h.pos >= len(h.entries) {
		h.pos = -1
		return "", true
	}
	return h.entries[h.pos], true
}

// Reset stops history browsing.
func (h *history) Reset() { h.pos = -1 }
