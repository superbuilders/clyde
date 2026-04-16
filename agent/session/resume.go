package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/this-is-alpha-iota/clyde/agent/providers"
)

// SessionInfo holds metadata about a session for listing purposes.
type SessionInfo struct {
	DirName      string // directory name (e.g., "2026-07-14T09-32-00_aj")
	Path         string // full path to the session directory
	Timestamp    string // session start timestamp
	Username     string // session owner
	MessageCount int    // total non-diagnostic/non-compaction message files
	Summary      string // first user message, truncated
}

// Open opens an existing session directory for continued writing.
// It initializes the monotonicity guard from the most recent file
// to prevent timestamp collisions with new messages.
func Open(sessionDir string) (*Session, error) {
	info, err := os.Stat(sessionDir)
	if err != nil {
		return nil, fmt.Errorf("session directory '%s' not found: %w", sessionDir, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("'%s' is not a directory", sessionDir)
	}

	sess := &Session{
		Dir:          sessionDir,
		SessionsRoot: filepath.Dir(sessionDir),
	}

	// Initialize lastTimestamp from the most recent file to avoid collisions
	entries, err := os.ReadDir(sessionDir)
	if err == nil && len(entries) > 0 {
		// Sort to ensure we get the last one
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".md") {
				names = append(names, e.Name())
			}
		}
		sort.Strings(names)
		if len(names) > 0 {
			lastFile := names[len(names)-1]
			if ts, err := ParseTimestampFromFilename(lastFile); err == nil {
				sess.lastTimestamp = ts
			}
		}
	}

	return sess, nil
}

// ParseTimestampFromFilename extracts the timestamp from a message filename.
// Format: "2026-07-14T09-32-05.123_type.md" → time.Time
// The timestamp is parsed in local time to match FormatTimestampFile which
// formats using local time (Go's default time.Format behavior).
func ParseTimestampFromFilename(filename string) (time.Time, error) {
	// The timestamp is everything before the first underscore after the ms portion
	// Format: YYYY-MM-DDTHH-MM-SS.mmm_type.md
	// Length: 23 chars for the timestamp part
	base := filepath.Base(filename)
	if len(base) < 24 { // 23 chars + at least 1 underscore
		return time.Time{}, fmt.Errorf("filename too short: %s", base)
	}

	tsStr := base[:23]
	// Parse in local time to match FormatTimestampFile (which uses time.Format
	// on local time values). Using time.Parse would return UTC, causing mismatches
	// with time.Now() on machines not in UTC.
	return time.ParseInLocation("2006-01-02T15-04-05.000", tsStr, time.Local)
}

// MessageTypeFromFilename extracts the message type from a filename.
// Format: "2026-07-14T09-32-05.123_tool-use.md" → "tool-use"
func MessageTypeFromFilename(filename string) string {
	base := filepath.Base(filename)
	base = strings.TrimSuffix(base, ".md")
	// Find the underscore after the timestamp (position 23)
	if len(base) > 24 {
		return base[24:] // everything after "YYYY-MM-DDTHH-MM-SS.mmm_"
	}
	return ""
}

// ReconstructHistory reads a session directory and reconstructs the conversation
// history as API messages suitable for passing to the Claude API.
//
// Reconstruction follows deterministic rules (per docs/sessions-history.md §12):
//   - user files → flush pending, new user message with text
//   - thinking files → accumulate on assistant message
//   - tool-use files → accumulate tool_use block on assistant message
//   - tool-result files → accumulate tool_result block on user message
//   - assistant files → flush pending, new assistant message with text, flush
//   - system files → flush pending, add system message
//   - diagnostic/compaction files → skipped
//
// If a compaction has occurred (*_system.md exists), reconstruction starts
// from the latest *_system.md forward. Otherwise all files are loaded.
//
// Malformed files are skipped with a warning (crash recovery).
func ReconstructHistory(sessionDir string) ([]providers.Message, []string, error) {
	entries, err := os.ReadDir(sessionDir)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read session directory: %w", err)
	}

	// Collect and sort .md files
	var files []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".md") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)

	if len(files) == 0 {
		return nil, nil, nil
	}

	// Find latest *_system.md for compaction-based resume
	startIdx := 0
	for i := len(files) - 1; i >= 0; i-- {
		if strings.HasSuffix(files[i], "_system.md") {
			startIdx = i
			break
		}
	}

	// Process files from startIdx forward
	var warnings []string
	var messages []providers.Message
	var pending *pendingMessage
	var toolUseIDs []string // ordered queue of tool_use_ids from tool-use files

	flush := func() {
		if pending != nil && len(pending.content) > 0 {
			msg := providers.Message{Role: pending.role}
			// For simple user text messages, use string content
			if pending.role == "user" && len(pending.content) == 1 && pending.content[0].Type == "text" {
				msg.Content = pending.content[0].Text
			} else {
				msg.Content = pending.content
			}
			messages = append(messages, msg)
		}
		pending = nil
	}

	ensureAssistant := func() {
		if pending == nil || pending.role != "assistant" {
			flush()
			pending = &pendingMessage{role: "assistant"}
		}
	}

	ensureUserToolResult := func() {
		if pending == nil || (pending.role != "user") {
			flush()
			pending = &pendingMessage{role: "user"}
		}
	}

	for i := startIdx; i < len(files); i++ {
		filename := files[i]
		msgType := MessageTypeFromFilename(filename)
		filePath := filepath.Join(sessionDir, filename)

		content, err := os.ReadFile(filePath)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("skipping unreadable file %s: %v", filename, err))
			continue
		}

		text := string(content)

		switch MessageType(msgType) {
		case TypeUser:
			flush()
			userText := extractUserText(text)
			pending = &pendingMessage{
				role: "user",
				content: []providers.ContentBlock{
					{Type: "text", Text: userText},
				},
			}
			flush() // user messages are immediately flushed

		case TypeThinking:
			// Reconstruct thinking blocks with their cryptographic signatures.
			// The signature is required by the API for round-tripping thinking
			// in conversation history. If no signature is found (legacy files),
			// the thinking block is skipped since the API would reject it.
			thinkingText, thinkingSig := extractThinkingWithSignature(text)
			if thinkingSig == "" {
				// Legacy file without signature — skip from API history.
				// The thinking text is still on disk for human reading.
				continue
			}
			ensureAssistant()
			pending.content = append(pending.content, providers.ContentBlock{
				Type:      "thinking",
				Thinking:  thinkingText,
				Signature: thinkingSig,
			})

		case TypeToolUse:
			ensureAssistant()
			toolUseID, toolName, input := extractToolUseMetadata(text)
			if toolUseID != "" {
				toolUseIDs = append(toolUseIDs, toolUseID)
				pending.content = append(pending.content, providers.ContentBlock{
					Type:  "tool_use",
					ID:    toolUseID,
					Name:  toolName,
					Input: input,
				})
			} else {
				warnings = append(warnings, fmt.Sprintf("skipping tool-use file without ID: %s", filename))
			}

		case TypeToolResult:
			ensureUserToolResult()
			resultContent, explicitID := extractToolResultContent(text)
			// Determine tool_use_id: prefer explicit ID in file, fallback to order-based matching
			var toolUseID string
			if explicitID != "" {
				toolUseID = explicitID
			} else if len(toolUseIDs) > 0 {
				toolUseID = toolUseIDs[0]
				toolUseIDs = toolUseIDs[1:]
			}
			if toolUseID != "" {
				pending.content = append(pending.content, providers.ContentBlock{
					Type:      "tool_result",
					ToolUseID: toolUseID,
					Content:   resultContent,
				})
			} else {
				warnings = append(warnings, fmt.Sprintf("skipping tool-result without matching tool_use_id: %s", filename))
			}

		case TypeAssistant:
			assistantText := extractAssistantText(text)
			if pending != nil && pending.role == "assistant" {
				// Append text to existing assistant message (e.g., after thinking blocks)
				if assistantText != "" {
					pending.content = append(pending.content, providers.ContentBlock{
						Type: "text",
						Text: assistantText,
					})
				}
				flush()
			} else {
				// No pending assistant — flush whatever is pending, create new assistant
				flush()
				pending = &pendingMessage{
					role: "assistant",
					content: []providers.ContentBlock{
						{Type: "text", Text: assistantText},
					},
				}
				flush() // assistant text messages are immediately flushed
			}

		case TypeSystem:
			flush()
			systemText := extractSystemText(text)
			// System messages from compaction are injected as user messages
			// with a clear marker, since the Claude API doesn't support
			// system role in message history
			messages = append(messages, providers.Message{
				Role:    "user",
				Content: "[System: Compaction Summary]\n\n" + systemText,
			})
			// Add an assistant acknowledgment for valid alternation
			messages = append(messages, providers.Message{
				Role:    "assistant",
				Content: "I've reviewed the compaction summary and understand the context. I'll continue from where we left off.",
			})

		case TypeDiagnostic, TypeCompaction:
			// Skip — not conversation content
			continue

		default:
			warnings = append(warnings, fmt.Sprintf("skipping unknown message type '%s' in file %s", msgType, filename))
		}
	}

	// Flush any remaining pending message
	flush()

	// API history cleanup: if the last message is a user message, it represents
	// an incomplete exchange (the user typed something but the process died or
	// errored before getting a response). The message file stays on disk as a
	// permanent record, but we drop it from the API history to maintain the
	// user/assistant alternation the Claude API requires. The user can retype
	// their message in the resumed session.
	for len(messages) > 0 && messages[len(messages)-1].Role == "user" {
		// Check if it's a plain text message (not tool_result which is part of a loop)
		lastContent := messages[len(messages)-1].Content
		if _, isString := lastContent.(string); isString {
			warnings = append(warnings, "trimming trailing user message from API history (incomplete exchange — file preserved on disk)")
			messages = messages[:len(messages)-1]
			break
		}
		// If it's a tool_result user message, also drop it — the agent was mid-loop
		if blocks, ok := lastContent.([]providers.ContentBlock); ok {
			hasToolResult := false
			for _, b := range blocks {
				if b.Type == "tool_result" {
					hasToolResult = true
					break
				}
			}
			if hasToolResult {
				// Also drop the preceding assistant message with tool_use that
				// started this incomplete loop
				warnings = append(warnings, "trimming trailing tool_result from API history (incomplete tool loop — files preserved on disk)")
				messages = messages[:len(messages)-1]
				if len(messages) > 0 && messages[len(messages)-1].Role == "assistant" {
					messages = messages[:len(messages)-1]
				}
				continue
			}
		}
		break
	}

	return messages, warnings, nil
}

// pendingMessage accumulates content blocks for a message being built.
type pendingMessage struct {
	role    string
	content []providers.ContentBlock
}

// FindMostRecentSession finds the most recent session directory for the given
// username in the sessions root. Returns the full path to the session directory.
func FindMostRecentSession(sessionsRoot, username string) (string, error) {
	entries, err := os.ReadDir(sessionsRoot)
	if err != nil {
		return "", fmt.Errorf("failed to read sessions directory: %w", err)
	}

	// Filter for directories belonging to this user, sorted reverse chronologically
	var matches []string
	suffix := "_" + username
	for _, e := range entries {
		if e.IsDir() && strings.HasSuffix(e.Name(), suffix) {
			matches = append(matches, e.Name())
		}
		// Also match _from_ sessions (branched sessions)
		if e.IsDir() && strings.Contains(e.Name(), "_"+username+"_from_") {
			matches = append(matches, e.Name())
		}
	}

	if len(matches) == 0 {
		return "", fmt.Errorf("no sessions found for user '%s' in %s", username, sessionsRoot)
	}

	sort.Strings(matches)
	mostRecent := matches[len(matches)-1]
	return filepath.Join(sessionsRoot, mostRecent), nil
}

// FindSessionByID finds a session directory by its directory name (or prefix).
// Searches the sessions root for an exact or prefix match.
func FindSessionByID(sessionsRoot, sessionID string) (string, error) {
	// Try exact match first
	fullPath := filepath.Join(sessionsRoot, sessionID)
	if info, err := os.Stat(fullPath); err == nil && info.IsDir() {
		return fullPath, nil
	}

	// Try prefix match
	entries, err := os.ReadDir(sessionsRoot)
	if err != nil {
		return "", fmt.Errorf("failed to read sessions directory: %w", err)
	}

	var matches []string
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), sessionID) {
			matches = append(matches, e.Name())
		}
	}

	if len(matches) == 0 {
		return "", fmt.Errorf("no session found matching '%s' in %s", sessionID, sessionsRoot)
	}
	if len(matches) > 1 {
		return "", fmt.Errorf("ambiguous session ID '%s' matches %d sessions: %v", sessionID, len(matches), matches)
	}

	return filepath.Join(sessionsRoot, matches[0]), nil
}

// ListSessions returns metadata for all sessions in the sessions root,
// sorted newest first. All info is derived from files on disk.
func ListSessions(sessionsRoot string) ([]SessionInfo, error) {
	entries, err := os.ReadDir(sessionsRoot)
	if err != nil {
		return nil, fmt.Errorf("failed to read sessions directory: %w", err)
	}

	var sessions []SessionInfo
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}

		dirName := e.Name()
		sessionPath := filepath.Join(sessionsRoot, dirName)

		// Parse directory name: <timestamp>_<username> or <timestamp>_<username>_from_<source>
		timestamp, username := parseDirName(dirName)

		// Count message files (exclude diagnostic and compaction)
		messageCount := 0
		summary := ""
		sessionFiles, err := os.ReadDir(sessionPath)
		if err != nil {
			continue
		}

		for _, f := range sessionFiles {
			name := f.Name()
			if !strings.HasSuffix(name, ".md") {
				continue
			}
			msgType := MessageTypeFromFilename(name)
			if msgType != string(TypeDiagnostic) && msgType != string(TypeCompaction) {
				messageCount++
			}
			// Get first user message for summary
			if summary == "" && msgType == string(TypeUser) {
				content, err := os.ReadFile(filepath.Join(sessionPath, name))
				if err == nil {
					summary = extractSummary(string(content))
				}
			}
		}

		sessions = append(sessions, SessionInfo{
			DirName:      dirName,
			Path:         sessionPath,
			Timestamp:    timestamp,
			Username:     username,
			MessageCount: messageCount,
			Summary:      summary,
		})
	}

	// Sort newest first (reverse chronological)
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].DirName > sessions[j].DirName
	})

	return sessions, nil
}

// CopyForResume copies a session directory for cross-user resume.
// Creates a new directory: <timestamp>_<user>_from_<source-session-id>/
// Returns the path to the new directory.
func CopyForResume(sourceDir, sessionsRoot, username string) (string, error) {
	sourceID := filepath.Base(sourceDir)
	now := time.Now()
	newDirName := FormatTimestampDir(now) + "_" + username + "_from_" + sourceID
	newDir := filepath.Join(sessionsRoot, newDirName)

	if err := os.MkdirAll(newDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create branch directory: %w", err)
	}

	// Copy all files from source to new directory
	entries, err := os.ReadDir(sourceDir)
	if err != nil {
		return "", fmt.Errorf("failed to read source session: %w", err)
	}

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		srcPath := filepath.Join(sourceDir, e.Name())
		dstPath := filepath.Join(newDir, e.Name())

		content, err := os.ReadFile(srcPath)
		if err != nil {
			return "", fmt.Errorf("failed to read %s: %w", e.Name(), err)
		}
		if err := os.WriteFile(dstPath, content, 0644); err != nil {
			return "", fmt.Errorf("failed to write %s: %w", e.Name(), err)
		}
	}

	return newDir, nil
}

// --- Content extraction helpers ---

// extractUserText extracts user text from a user message file.
// Strips the "**You:**" prefix.
func extractUserText(content string) string {
	content = strings.TrimSpace(content)
	content = strings.TrimPrefix(content, "**You:**")
	return strings.TrimSpace(content)
}

// extractAssistantText extracts assistant text from an assistant message file.
// Strips the "**Claude:**" prefix.
func extractAssistantText(content string) string {
	content = strings.TrimSpace(content)
	content = strings.TrimPrefix(content, "**Claude:**")
	return strings.TrimSpace(content)
}

// extractThinkingText extracts thinking text from a thinking file.
// Strips the "💭 " prefix. Used only for legacy files without signatures.
func extractThinkingText(content string) string {
	content = strings.TrimSpace(content)
	content = strings.TrimPrefix(content, "💭 ")
	content = strings.TrimPrefix(content, "💭")
	return strings.TrimSpace(content)
}

// extractThinkingWithSignature extracts thinking text and signature from a thinking file.
//
// New format (with signature):
//
//	💭 thinking text here
//	signature: <base64 signature>
//
// Legacy format (no signature):
//
//	💭 thinking text here
//
// Returns (text, signature). If no signature line is found, signature is "".
func extractThinkingWithSignature(content string) (string, string) {
	lines := strings.Split(strings.TrimSpace(content), "\n")
	if len(lines) == 0 {
		return "", ""
	}

	var signature string
	var textLines []string

	for _, line := range lines {
		if strings.HasPrefix(line, "signature: ") {
			signature = strings.TrimPrefix(line, "signature: ")
		} else {
			textLines = append(textLines, line)
		}
	}

	text := strings.Join(textLines, "\n")
	text = strings.TrimPrefix(text, "💭 ")
	text = strings.TrimPrefix(text, "💭")
	text = strings.TrimSpace(text)

	return text, signature
}

// extractSystemText extracts system text from a system message file.
// Strips the "**System:**" prefix.
func extractSystemText(content string) string {
	content = strings.TrimSpace(content)
	content = strings.TrimPrefix(content, "**System:**")
	return strings.TrimSpace(content)
}

// toolUseIDRegex matches [toolu_xxxx] at the end of a line.
var toolUseIDRegex = regexp.MustCompile(`\[(toolu_[a-zA-Z0-9_-]+)\]`)

// extractToolUseMetadata extracts tool_use_id, tool name, and input from a tool-use file.
//
// New format (SESS-2+):
//
//	→ Reading file: main.go [toolu_abc123]
//	name: read_file
//	input: {"path":"main.go"}
//
// Multi-line display (e.g. run_bash with newlines in command):
//
//	→ Running bash: cd /tmp [toolu_abc123]
//	ls -la
//	grep foo bar
//	name: run_bash
//	input: {"command":"cd /tmp\nls -la\ngrep foo bar"}
//
// Legacy multi-line (ID on last display line, before name/input):
//
//	→ Running bash: cd /tmp
//	ls -la
//	grep foo bar [toolu_abc123]
//	name: run_bash
//	input: {"command":"cd /tmp\nls -la\ngrep foo bar"}
//
// Legacy format (SESS-1):
//
//	→ Reading file: main.go [toolu_abc123]
func extractToolUseMetadata(content string) (toolUseID, toolName string, input map[string]interface{}) {
	lines := strings.Split(strings.TrimSpace(content), "\n")
	if len(lines) == 0 {
		return "", "", nil
	}

	// Extract tool_use_id by scanning ALL lines for [toolu_xxx].
	// For single-line display messages the ID is on line 0, but for
	// multi-line display messages (e.g. run_bash with newlines in the
	// command) the ID may appear on any line — either line 0 (new writer)
	// or the last display line (legacy writer that appended to the end).
	for _, line := range lines {
		matches := toolUseIDRegex.FindStringSubmatch(line)
		if len(matches) >= 2 {
			toolUseID = matches[1]
			break
		}
	}

	// Try to extract tool name and input from subsequent lines
	for _, line := range lines[1:] {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "name: ") {
			toolName = strings.TrimPrefix(line, "name: ")
		}
		if strings.HasPrefix(line, "input: ") {
			inputJSON := strings.TrimPrefix(line, "input: ")
			json.Unmarshal([]byte(inputJSON), &input)
		}
	}

	// Fallback: infer tool name from display message if not explicitly stored
	if toolName == "" {
		toolName = inferToolName(lines[0])
	}

	// Ensure input is non-nil (API requires it)
	if input == nil {
		input = map[string]interface{}{}
	}

	return toolUseID, toolName, input
}

// extractToolResultContent extracts tool result text from a tool-result file.
//
// New format (SESS-2+):
//
//	[toolu_abc123]
//	```
//	output content
//	```
//
// Legacy format (SESS-1):
//
//	```
//	output content
//	```
//
// Returns the content and an optional explicit tool_use_id.
func extractToolResultContent(content string) (resultText string, explicitID string) {
	content = strings.TrimSpace(content)
	lines := strings.Split(content, "\n")

	startIdx := 0

	// Check for explicit tool_use_id on first line
	if len(lines) > 0 {
		matches := toolUseIDRegex.FindStringSubmatch(lines[0])
		if len(matches) >= 2 {
			explicitID = matches[1]
			startIdx = 1
		}
	}

	// Extract content from fenced code block
	var resultLines []string
	inFence := false
	for i := startIdx; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			if inFence {
				break // closing fence
			}
			inFence = true
			continue
		}
		if inFence {
			resultLines = append(resultLines, line)
		}
	}

	if len(resultLines) > 0 {
		return strings.Join(resultLines, "\n"), explicitID
	}

	// Fallback: if no fences, return everything after the ID line
	if startIdx < len(lines) {
		return strings.Join(lines[startIdx:], "\n"), explicitID
	}
	return content, explicitID
}

// inferToolName attempts to determine the tool name from a display message line.
// Used for backward compatibility with SESS-1 tool-use files that lack explicit metadata.
func inferToolName(displayLine string) string {
	// Map display prefixes to tool names
	prefixMap := map[string]string{
		"→ Reading file:":        "read_file",
		"→ Listing files:":       "list_files",
		"→ Patching file:":       "patch_file",
		"→ Writing file:":        "write_file",
		"→ Running bash:":        "run_bash",
		"→ Searching:":           "grep",
		"→ Finding files:":       "glob",
		"→ Applying multi-patch:": "multi_patch",
		"→ Searching web:":       "web_search",
		"→ Browsing:":            "browse",
		"→ Including file:":      "include_file",
		"→ Browser:":             "mcp_playwright_browser_snapshot",
	}

	for prefix, name := range prefixMap {
		if strings.HasPrefix(displayLine, prefix) {
			return name
		}
	}

	return "unknown_tool"
}

// extractSummary extracts a short summary from a user message for session listing.
// Strips the "**You:**" prefix and truncates to 60 characters.
func extractSummary(content string) string {
	text := extractUserText(content)
	// Take first line
	if idx := strings.IndexByte(text, '\n'); idx >= 0 {
		text = text[:idx]
	}
	text = strings.TrimSpace(text)
	if len(text) > 60 {
		text = text[:57] + "..."
	}
	return text
}

// parseDirName parses a session directory name into timestamp and username.
// Handles both "2026-07-14T09-32-00_aj" and "2026-07-14T09-32-00_aj_from_2026-07-14T10-00-00_maria"
func parseDirName(dirName string) (timestamp, username string) {
	// Timestamp is always the first 19 characters
	if len(dirName) < 20 {
		return dirName, "unknown"
	}
	timestamp = dirName[:19]

	rest := dirName[20:] // skip the underscore after timestamp
	// Check for _from_ suffix (branched session)
	if idx := strings.Index(rest, "_from_"); idx >= 0 {
		username = rest[:idx]
	} else {
		username = rest
	}

	return timestamp, username
}

// SessionOwner extracts the username from a session directory name.
func SessionOwner(dirName string) string {
	_, username := parseDirName(dirName)
	return username
}

// InferToolNameExported is the exported version of inferToolName for testing.
func InferToolNameExported(displayLine string) string {
	return inferToolName(displayLine)
}

// ToolUseIDRegexExported returns the compiled regex for extracting tool use IDs.
func ToolUseIDRegexExported() *regexp.Regexp {
	return toolUseIDRegex
}
