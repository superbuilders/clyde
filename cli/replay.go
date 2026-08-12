package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/superbuilders/clyde/agent/session"
	"github.com/superbuilders/clyde/cli/loglevel"
	"github.com/superbuilders/clyde/cli/style"
	"github.com/superbuilders/clyde/cli/truncate"
)

// ReplaySession reads all session message files from sessionDir and prints
// them to stdout, respecting the given verbosity level. This is used when
// resuming a session with -r and when the /verbosity command is issued.
//
// Display rules match the live output callbacks:
//   - User messages:      always shown (styled with "You:" prefix)
//   - Assistant messages:  always shown (styled with "Claude:" prefix)
//   - Thinking traces:     shown at Normal+ (truncated unless Verbose+)
//   - Tool use (progress): shown at Quiet+
//   - Tool results:        shown at Normal+ (truncated unless Verbose+)
//   - Diagnostics:         shown at Debug (cache lines at Verbose/Debug)
//   - Compaction markers:  shown at Quiet+
//   - System messages:     shown at Quiet+
func ReplaySession(sessionDir string, level loglevel.Level) {
	entries, err := os.ReadDir(sessionDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not read session directory for replay: %v\n", err)
		return
	}

	// Collect and sort .md files chronologically
	var files []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".md") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)

	if len(files) == 0 {
		return
	}

	// Find the latest system message (compaction boundary) — replay from there
	startIdx := 0
	for i := len(files) - 1; i >= 0; i-- {
		if strings.HasSuffix(files[i], "_system.md") {
			startIdx = i
			break
		}
	}

	for i := startIdx; i < len(files); i++ {
		filename := files[i]
		msgType := session.MessageTypeFromFilename(filename)
		filePath := filepath.Join(sessionDir, filename)

		content, err := os.ReadFile(filePath)
		if err != nil {
			continue // skip unreadable files
		}

		text := strings.TrimSpace(string(content))
		if text == "" {
			continue
		}

		switch session.MessageType(msgType) {
		case session.TypeUser:
			// Always shown — extract and display with styled prefix
			userText := extractReplayUserText(text)
			fmt.Printf("\n%s%s\n", style.FormatUserPrompt(), userText)

		case session.TypeAssistant:
			// Always shown — extract and display with styled prefix
			assistantText := extractReplayAssistantText(text)
			fmt.Printf("\n%s%s\n", style.FormatAgentPrefix(), assistantText)

		case session.TypeThinking:
			if !level.ShouldShow(loglevel.Normal) {
				continue
			}
			thinkingText := extractReplayThinkingText(text)
			displayed := truncateForLevel(thinkingText, truncate.ThinkingLineLimit, level)
			fmt.Println(style.FormatThinking(displayed))

		case session.TypeToolUse:
			if !level.ShouldShow(loglevel.Quiet) {
				continue
			}
			// Display the progress line (first line of the file content)
			progressLine := extractReplayProgressLine(text)
			fmt.Println(StyleMessage(loglevel.Quiet, progressLine))

		case session.TypeToolResult:
			if !level.ShouldShow(loglevel.Normal) {
				continue
			}
			resultText := extractReplayToolResult(text)
			displayed := truncateForLevel(resultText, truncate.ToolOutputLineLimit, level)
			fmt.Println()
			fmt.Println(StyleMessage(loglevel.Normal, displayed))
			fmt.Println()

		case session.TypeDiagnostic:
			// Cache lines at verbose+, everything else at debug
			if strings.HasPrefix(text, "💾 Cache:") && !strings.Contains(text, "|") {
				if !level.ShouldShow(loglevel.Verbose) {
					continue
				}
			} else if strings.HasPrefix(text, "💾 Cache:") && strings.Contains(text, "|") {
				if !level.ShouldShow(loglevel.Debug) {
					continue
				}
			} else {
				if !level.ShouldShow(loglevel.Debug) {
					continue
				}
			}
			fmt.Println(StyleMessage(loglevel.Debug, text))

		case session.TypeCompaction:
			if level.ShouldShow(loglevel.Quiet) {
				fmt.Println(StyleMessage(loglevel.Quiet, text))
			}

		case session.TypeSystem:
			if level.ShouldShow(loglevel.Quiet) {
				systemText := extractReplaySystemText(text)
				fmt.Printf("\n[System] %s\n", systemText)
			}
		}
	}
}

// --- Extraction helpers for replay ---
// These mirror the session/resume.go extraction functions but are simpler
// since we only need display text, not API reconstruction.

func extractReplayUserText(content string) string {
	content = strings.TrimPrefix(content, "**You:**")
	return strings.TrimSpace(content)
}

func extractReplayAssistantText(content string) string {
	content = strings.TrimPrefix(content, "**Claude:**")
	return strings.TrimSpace(content)
}

func extractReplayThinkingText(content string) string {
	lines := strings.Split(content, "\n")
	var textLines []string
	for _, line := range lines {
		if strings.HasPrefix(line, "signature: ") {
			continue // skip signature line
		}
		textLines = append(textLines, line)
	}
	text := strings.Join(textLines, "\n")
	text = strings.TrimPrefix(text, "💭 ")
	text = strings.TrimPrefix(text, "💭")
	return strings.TrimSpace(text)
}

func extractReplayProgressLine(content string) string {
	// The first line of a tool-use file is the progress line
	// e.g., "→ Reading file: main.go [toolu_abc123]"
	lines := strings.SplitN(content, "\n", 2)
	return lines[0]
}

func extractReplayToolResult(content string) string {
	lines := strings.Split(content, "\n")
	startIdx := 0

	// Skip explicit tool_use_id line if present
	if len(lines) > 0 && strings.HasPrefix(lines[0], "[toolu_") {
		startIdx = 1
	}

	// Extract content from fenced code block
	var resultLines []string
	inFence := false
	for i := startIdx; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trimmed, "```") {
			if inFence {
				break
			}
			inFence = true
			continue
		}
		if inFence {
			resultLines = append(resultLines, lines[i])
		}
	}

	if len(resultLines) > 0 {
		return strings.Join(resultLines, "\n")
	}

	// Fallback: return everything after the ID line
	if startIdx < len(lines) {
		return strings.Join(lines[startIdx:], "\n")
	}
	return content
}

func extractReplaySystemText(content string) string {
	content = strings.TrimPrefix(content, "**System:**")
	return strings.TrimSpace(content)
}
