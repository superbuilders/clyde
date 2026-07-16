package agent

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/superbuilders/clyde/agent/providers"
)

// DefaultReserveTokens is the default number of tokens to reserve for the
// agent's next response. When input tokens exceed (contextWindowSize - reserveTokens),
// compaction is triggered automatically.
const DefaultReserveTokens = 16000

// CompactionCallback is called when compaction occurs.
// It receives the compaction marker message and the system summary.
// marker is non-empty for progress/status lines (displayed to user).
// summary is non-empty once the final handoff document is ready (persisted to session).
type CompactionCallback func(marker string, summary string)

// WithCompactionCallback sets the callback for compaction events.
// Called with the compaction marker ("🗜️ Compacting...") and the summary text.
func WithCompactionCallback(cb CompactionCallback) AgentOption {
	return func(a *Agent) {
		a.compactionCallback = cb
	}
}

// ShouldCompact checks whether the conversation history has grown large enough
// to require compaction. It returns true when the total input tokens from the
// last API response exceed (contextWindowSize - reserveTokens).
//
// Returns false if:
//   - No API call has been made yet (lastUsage is zero)
//   - contextWindowSize is not configured (zero)
//   - reserveTokens is not configured (zero — uses DefaultReserveTokens)
//   - The threshold has not been exceeded
func (a *Agent) ShouldCompact() bool {
	if a.contextWindowSize == 0 {
		return false
	}

	totalInput := a.lastUsage.InputTokens + a.lastUsage.CacheReadInputTokens
	if totalInput == 0 {
		return false
	}

	reserve := a.reserveTokens
	if reserve == 0 {
		reserve = DefaultReserveTokens
	}

	threshold := a.contextWindowSize - reserve
	return totalInput > threshold
}

// Compact performs conversation compaction using a 3-call pipeline:
//  1. Identify decision/pivot points in the conversation
//  2. Identify tool results relevant to those pivot points
//  3. Write a gap-filling summary covering everything not preserved
//  4. Deterministically assemble new history from preserved messages + summary
//
// Returns an error if any LLM call fails.
func (a *Agent) Compact() error {
	if len(a.history) < 4 {
		// Too few messages to compact meaningfully
		return nil
	}

	// Step 1: Find the first user message (pinned/sacred)
	firstUserMsg, firstUserIdx := a.findFirstUserMessage()
	if firstUserIdx < 0 {
		return fmt.Errorf("compaction: no user message found in history")
	}

	// Step 2: Determine what to keep vs. summarize.
	keepCount := a.recentKeepCount()
	summarizeEnd := len(a.history) - keepCount
	if summarizeEnd <= firstUserIdx+1 {
		return nil
	}

	toSummarize := a.history[firstUserIdx+1 : summarizeEnd]
	keptMessages := a.history[summarizeEnd:]

	// Step 3: Emit compaction marker
	if a.compactionCallback != nil {
		a.compactionCallback("🗜️ Compacting conversation history...", "")
	}
	if a.diagnosticCallback != nil {
		a.diagnosticCallback(fmt.Sprintf("🗜️ Compacting: %d messages → summary + preserved + %d recent messages",
			len(a.history), keepCount))
	}

	// Step 4: Run the 3-call compaction workflow.
	summary, preservedMessages, err := a.runCompactionWorkflow(firstUserMsg, toSummarize, keptMessages)
	if err != nil {
		return fmt.Errorf("compaction failed: %w", err)
	}

	// Step 5: Emit the summary via callback for session persistence
	if a.compactionCallback != nil {
		a.compactionCallback("", summary)
	}

	// Step 6: Deterministic assembly of new history.
	// Structure: [pinned first msg] [ack] [summary] [ack] [preserved messages...] [kept messages...]
	var newHistory []providers.Message

	// First user message — pinned, verbatim
	newHistory = append(newHistory, firstUserMsg)

	// Assistant acknowledgment of first message (required for alternation)
	newHistory = append(newHistory, providers.Message{
		Role:    "assistant",
		Content: "I understand the task. Let me work on this.",
	})

	// Compaction summary injected as a user message
	newHistory = append(newHistory, providers.Message{
		Role:    "user",
		Content: "[System: Compaction Summary]\n\n" + summary,
	})

	// Assistant acknowledgment of compaction summary
	newHistory = append(newHistory, providers.Message{
		Role:    "assistant",
		Content: "I've reviewed the compaction summary and understand the context. I'll continue from where we left off.",
	})

	// Preserved messages (pivot points and critical tool results) in chronological order.
	// Ensure proper user/assistant alternation.
	if len(preservedMessages) > 0 {
		newHistory = appendPreservedMessages(newHistory, preservedMessages)
	}

	// Append recent kept messages, maintaining alternation at the boundary.
	if len(keptMessages) > 0 {
		newHistory = appendPreservedMessages(newHistory, keptMessages)
	}

	a.history = newHistory

	return nil
}

// appendPreservedMessages appends preserved messages to history while maintaining
// proper user/assistant alternation. Inserts bridging messages as needed.
func appendPreservedMessages(history []providers.Message, preserved []providers.Message) []providers.Message {
	if len(preserved) == 0 {
		return history
	}

	lastRole := history[len(history)-1].Role

	for _, msg := range preserved {
		// If we'd have two consecutive messages with the same role, insert a bridge
		if msg.Role == lastRole {
			if lastRole == "user" {
				history = append(history, providers.Message{
					Role:    "assistant",
					Content: "Continuing with the task.",
				})
			} else {
				history = append(history, providers.Message{
					Role:    "user",
					Content: "[System: continued]",
				})
			}
		}
		history = append(history, msg)
		lastRole = msg.Role
	}

	return history
}

// FindFirstUserMessage locates the first user text message in history.
// This is the "pinned" / "sacred" original mission message.
// Exported for testing; used internally by Compact().
func (a *Agent) FindFirstUserMessage() (providers.Message, int) {
	return a.findFirstUserMessage()
}

// findFirstUserMessage locates the first user text message in history.
func (a *Agent) findFirstUserMessage() (providers.Message, int) {
	for i, msg := range a.history {
		if msg.Role == "user" {
			if text, ok := msg.Content.(string); ok {
				if !strings.HasPrefix(text, "[System:") {
					return msg, i
				}
			}
		}
	}
	return providers.Message{}, -1
}

// FindLastUserMessage locates the most recent user text message in history.
// Exported for testing.
func (a *Agent) FindLastUserMessage() (providers.Message, int) {
	return a.findLastUserMessage()
}

// findLastUserMessage locates the most recent user text message in history.
func (a *Agent) findLastUserMessage() (providers.Message, int) {
	for i := len(a.history) - 1; i >= 0; i-- {
		msg := a.history[i]
		if msg.Role == "user" {
			if text, ok := msg.Content.(string); ok {
				if !strings.HasPrefix(text, "[System:") {
					return msg, i
				}
			}
		}
	}
	return providers.Message{}, -1
}

// RecentKeepCount returns the number of recent messages to keep after compaction.
// Exported for testing; used internally by Compact().
func (a *Agent) RecentKeepCount() int {
	return a.recentKeepCount()
}

// recentKeepCount determines how many recent messages to keep after compaction.
// Keeps the last 4 messages (2 exchanges) as recent context, or fewer if
// the history is short.
func (a *Agent) recentKeepCount() int {
	keep := 4
	if len(a.history) < keep+4 {
		keep = 2
	}
	if keep > len(a.history)-2 {
		keep = len(a.history) - 2
	}
	if keep < 0 {
		keep = 0
	}
	return keep
}

// --- 3-call compaction workflow ---

// runCompactionWorkflow executes the 3-call compaction pipeline:
//
//  1. Identify decision/pivot points
//  2. Identify tool results relevant to those pivots (sequentially dependent on 1)
//  3. Write gap-filling summary (sequentially dependent on 1 and 2)
//
// Returns the summary text and a slice of preserved messages to include
// in the post-compaction history.
func (a *Agent) runCompactionWorkflow(
	firstUserMsg providers.Message,
	toSummarize []providers.Message,
	keptMessages []providers.Message,
) (string, []providers.Message, error) {

	missionText := messageText(firstUserMsg)

	// Serialize the full conversation with message numbers (no truncation).
	numberedConv := serializeNumberedMessages(toSummarize)

	// Capture git state for context.
	gitState := CaptureGitState()

	// --- Call 1: Identify decision/pivot points ---
	a.emitCompactionProgress("🗜️ Compaction call 1/3: identifying decision points...")

	call1System := "You are analyzing a coding session to identify decision and pivot points — " +
		"moments where the objective changes. This includes:\n\n" +
		"- User explicitly redirecting work (\"stop X, now do Y\")\n" +
		"- LLM discovering an approach won't work and pivoting\n" +
		"- Scope changes or new requirements emerging\n" +
		"- Completion of one objective and transition to the next\n\n" +
		"For each pivot, identify the specific message number(s) where the change happens.\n\n" +
		"Output EXACTLY this format:\n\n" +
		"## Current Objective\n" +
		"[What the session is actively working on RIGHT NOW — this is the most important output]\n\n" +
		"## Objective Timeline\n" +
		"1. [Messages N-M]: [objective description]\n" +
		"2. [Messages M-P]: Pivoted to [description] because [reason]\n" +
		"...\n\n" +
		"## Preserve\n" +
		"- Message N: [which pivot this captures and why]\n" +
		"- Messages X-Y: [which pivot this captures and why]\n\n" +
		"If there are no pivots (single objective throughout), say so and list nothing to preserve.\n" +
		"Preserve the MINIMUM messages needed — only the actual pivot moments."

	call1Input := fmt.Sprintf("## Original Mission (preserved separately — do NOT restate)\n\n%s\n\n## Conversation\n\n%s",
		missionText, numberedConv)

	call1Output, err := a.compactionCall(call1System, call1Input)
	if err != nil {
		return "", nil, fmt.Errorf("call 1 (decision points) failed: %w", err)
	}
	a.emitCompactionDebug("Call 1 output (decision points)", call1Output)

	// --- Call 2: Identify tool results to preserve ---
	a.emitCompactionProgress("🗜️ Compaction call 2/3: identifying critical tool results...")

	call2System := "A triage analysis of this coding session identified specific decision points " +
		"(provided below as 'Triage Analysis'). Now identify tool results that should be preserved " +
		"verbatim because they are critical evidence.\n\n" +
		"PRESERVE tool results that:\n" +
		"- Directly caused or informed an identified pivot (e.g., test failure that triggered a rethink)\n" +
		"- Are critical to the CURRENT OBJECTIVE (e.g., error output still being debugged)\n" +
		"- Contain information that cannot be trivially re-derived\n\n" +
		"DO NOT preserve:\n" +
		"- Routine file reads, directory listings, successful command runs\n" +
		"- Tool results from completed/superseded objectives\n" +
		"- Results that can trivially be re-run\n\n" +
		"Output EXACTLY this format:\n\n" +
		"## Preserve\n" +
		"- Message N: [which decision point this supports, or \"current objective\"]\n\n" +
		"If no tool results need preservation, say \"No tool results to preserve.\""

	call2Input := fmt.Sprintf("## Triage Analysis (from previous step)\n\n%s\n\n## Conversation\n\n%s",
		call1Output, numberedConv)

	call2Output, err := a.compactionCall(call2System, call2Input)
	if err != nil {
		return "", nil, fmt.Errorf("call 2 (tool results) failed: %w", err)
	}
	a.emitCompactionDebug("Call 2 output (tool results)", call2Output)

	// Parse preserved message indices from calls 1 and 2.
	preserveIndices := ParsePreserveIndices(call1Output, call2Output)
	preserved := extractPreservedMessages(toSummarize, preserveIndices)

	if a.diagnosticCallback != nil {
		a.diagnosticCallback(fmt.Sprintf("🗜️ Preserving %d messages from calls 1+2", len(preserved)))
	}

	// --- Call 3: Write gap-filling summary ---
	a.emitCompactionProgress("🗜️ Compaction call 3/3: writing summary...")

	// Build the list of what's being preserved for Call 3's awareness.
	preservedDesc := describePreservedMessages(toSummarize, preserveIndices)

	// Serialize recent kept messages for bridging context.
	recentCtx := serializeMessagesPlain(keptMessages)

	call3System := "You are writing a summary to fill the gaps between preserved messages " +
		"in a compacted coding session. The triage analyses below show which messages will be " +
		"kept verbatim — do NOT repeat their content.\n\n" +
		"Cover:\n" +
		"- Work completed (that isn't captured in preserved messages)\n" +
		"- Decisions made and their rationale\n" +
		"- Errors encountered and how they were resolved\n" +
		"- Current state of the codebase\n\n" +
		"Do NOT include a \"Goal\" or \"Objective\" section — the current objective is captured " +
		"in the triage analysis below, and the original mission is preserved separately.\n\n" +
		"Be concise. This summary exists to provide continuity between preserved messages, " +
		"not to replace them."

	call3Input := fmt.Sprintf(
		"## Triage Analysis (decision points)\n\n%s\n\n"+
			"## Triage Analysis (tool results)\n\n%s\n\n"+
			"## Messages Being Preserved Verbatim\n\n%s\n\n"+
			"## Conversation\n\n%s\n\n"+
			"## Git State\n\n%s\n\n"+
			"## Recent Messages (still in context after compaction)\n\n%s",
		call1Output, call2Output, preservedDesc, numberedConv, gitState, recentCtx,
	)

	call3Output, err := a.compactionCall(call3System, call3Input)
	if err != nil {
		return "", nil, fmt.Errorf("call 3 (summary) failed: %w", err)
	}
	a.emitCompactionDebug("Call 3 output (summary)", call3Output)

	// Extract current objective from Call 1 and prepend it to the summary.
	currentObj := ExtractCurrentObjective(call1Output)
	var summary strings.Builder
	if currentObj != "" {
		summary.WriteString("## Current Objective\n\n")
		summary.WriteString(currentObj)
		summary.WriteString("\n\n")
	}
	summary.WriteString(call3Output)

	// Append git state.
	if gitState != "" && !strings.Contains(gitState, "not a git repo") {
		summary.WriteString("\n\n## Git State\n\n")
		summary.WriteString(gitState)
		status := captureGitStatus()
		if status != "" {
			summary.WriteString("\n⚠️ **Uncommitted changes at compaction time:**\n```\n")
			summary.WriteString(status)
			summary.WriteString("\n```\n")
		}
	}

	return summary.String(), preserved, nil
}

// compactionCall makes a single LLM call for one compaction step.
func (a *Agent) compactionCall(systemPrompt string, userContent string) (string, error) {
	messages := []providers.Message{
		{Role: "user", Content: userContent},
	}

	resp, err := a.apiClient.Call(systemPrompt, messages, nil)
	if err != nil {
		return "", err
	}

	var parts []string
	for _, block := range resp.Content {
		if block.Type == "text" && block.Text != "" {
			parts = append(parts, block.Text)
		}
	}
	if len(parts) == 0 {
		return "", fmt.Errorf("empty response from compaction call")
	}
	return strings.Join(parts, "\n"), nil
}

// --- Message serialization ---

// serializeNumberedMessages converts messages to numbered text format
// with NO truncation of tool results.
func serializeNumberedMessages(msgs []providers.Message) string {
	var sb strings.Builder
	for i, msg := range msgs {
		role := msg.Role
		switch content := msg.Content.(type) {
		case string:
			sb.WriteString(fmt.Sprintf("[%d] **%s**: %s\n\n", i, role, content))
		case []providers.ContentBlock:
			for _, block := range content {
				switch block.Type {
				case "text":
					sb.WriteString(fmt.Sprintf("[%d] **%s**: %s\n\n", i, role, block.Text))
				case "tool_use":
					sb.WriteString(fmt.Sprintf("[%d] **%s** [tool_use: %s]: %v\n\n", i, role, block.Name, block.Input))
				case "tool_result":
					resultText := ""
					if s, ok := block.Content.(string); ok {
						resultText = s // No truncation
					}
					sb.WriteString(fmt.Sprintf("[%d] **tool_result**: %s\n\n", i, resultText))
				case "thinking":
					// Skip thinking blocks
				}
			}
		}
	}
	return sb.String()
}

// serializeMessagesPlain converts messages to plain text without numbering.
// Used for recent context. No truncation.
func serializeMessagesPlain(msgs []providers.Message) string {
	var sb strings.Builder
	for _, msg := range msgs {
		role := msg.Role
		switch content := msg.Content.(type) {
		case string:
			sb.WriteString(fmt.Sprintf("**%s**: %s\n\n", role, content))
		case []providers.ContentBlock:
			for _, block := range content {
				switch block.Type {
				case "text":
					sb.WriteString(fmt.Sprintf("**%s**: %s\n\n", role, block.Text))
				case "tool_use":
					sb.WriteString(fmt.Sprintf("**%s** [tool_use: %s]: %v\n\n", role, block.Name, block.Input))
				case "tool_result":
					resultText := ""
					if s, ok := block.Content.(string); ok {
						resultText = s
					}
					sb.WriteString(fmt.Sprintf("**tool_result**: %s\n\n", resultText))
				case "thinking":
					// Skip thinking blocks
				}
			}
		}
	}
	return sb.String()
}

// SerializeMessages is the exported serializer for tests. No truncation.
func SerializeMessages(msgs []providers.Message) string {
	return serializeMessagesPlain(msgs)
}

// --- Parse/extract helpers ---

// ParsePreserveIndices extracts message indices from Call 1 and Call 2 output.
// Looks for patterns like "Message N", "Messages N-M" in ## Preserve sections.
// Exported for testing.
func ParsePreserveIndices(call1Output, call2Output string) map[int]bool {
	indices := make(map[int]bool)
	for _, output := range []string{call1Output, call2Output} {
		lines := strings.Split(output, "\n")
		inPreserve := false
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "## Preserve") {
				inPreserve = true
				continue
			}
			if strings.HasPrefix(trimmed, "## ") && inPreserve {
				inPreserve = false
				continue
			}
			if !inPreserve {
				continue
			}
			// Parse "- Message N:" or "- Messages N-M:"
			parseMessageReferences(trimmed, indices)
		}
	}
	return indices
}

// parseMessageReferences extracts message indices from a line like
// "- Message 5: ..." or "- Messages 3-7: ..."
func parseMessageReferences(line string, indices map[int]bool) {
	// Look for "Message(s) N" or "Message(s) N-M" patterns
	lower := strings.ToLower(line)

	// Find all occurrences of "message" followed by numbers
	for _, prefix := range []string{"messages ", "message "} {
		idx := 0
		for {
			pos := strings.Index(lower[idx:], prefix)
			if pos < 0 {
				break
			}
			pos += idx + len(prefix)
			idx = pos

			// Parse the number(s) after "message(s) "
			numStr := ""
			for pos < len(lower) && (lower[pos] >= '0' && lower[pos] <= '9') {
				numStr += string(lower[pos])
				pos++
			}
			if numStr == "" {
				continue
			}
			n := 0
			for _, c := range numStr {
				n = n*10 + int(c-'0')
			}
			indices[n] = true

			// Check for range: "N-M"
			if pos < len(lower) && lower[pos] == '-' {
				pos++
				numStr2 := ""
				for pos < len(lower) && (lower[pos] >= '0' && lower[pos] <= '9') {
					numStr2 += string(lower[pos])
					pos++
				}
				if numStr2 != "" {
					m := 0
					for _, c := range numStr2 {
						m = m*10 + int(c-'0')
					}
					for i := n + 1; i <= m; i++ {
						indices[i] = true
					}
				}
			}
		}
	}
}

// extractPreservedMessages returns the messages at the given indices.
func extractPreservedMessages(msgs []providers.Message, indices map[int]bool) []providers.Message {
	var preserved []providers.Message
	for i, msg := range msgs {
		if indices[i] {
			preserved = append(preserved, msg)
		}
	}
	return preserved
}

// describePreservedMessages creates a human-readable description of which
// messages are being preserved, for Call 3's awareness.
func describePreservedMessages(msgs []providers.Message, indices map[int]bool) string {
	if len(indices) == 0 {
		return "No messages being preserved verbatim."
	}
	var sb strings.Builder
	for i := range msgs {
		if !indices[i] {
			continue
		}
		msg := msgs[i]
		preview := messageText(msg)
		if len(preview) > 200 {
			preview = preview[:200] + "..."
		}
		sb.WriteString(fmt.Sprintf("- Message %d (%s): %s\n", i, msg.Role, preview))
	}
	return sb.String()
}

// ExtractCurrentObjective pulls the "## Current Objective" section from Call 1's output.
// Exported for testing.
func ExtractCurrentObjective(call1Output string) string {
	lines := strings.Split(call1Output, "\n")
	var result []string
	inSection := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "## Current Objective" {
			inSection = true
			continue
		}
		if strings.HasPrefix(trimmed, "## ") && inSection {
			break
		}
		if inSection {
			result = append(result, line)
		}
	}
	return strings.TrimSpace(strings.Join(result, "\n"))
}

// emitCompactionProgress sends a compaction progress message via the callback.
func (a *Agent) emitCompactionProgress(msg string) {
	if a.compactionCallback != nil {
		a.compactionCallback(msg, "")
	}
}

// emitCompactionDebug sends intermediate compaction output via the diagnostic callback.
func (a *Agent) emitCompactionDebug(label, content string) {
	if a.diagnosticCallback != nil {
		preview := content
		if len(preview) > 500 {
			preview = preview[:500] + "..."
		}
		a.diagnosticCallback(fmt.Sprintf("🗜️ %s:\n%s", label, preview))
	}
}

// --- Git state capture ---

// GitState holds captured git repository state.
type GitState struct {
	IsRepo        bool
	Branch        string
	CommitSHA     string
	CommitMessage string
	HasChanges    bool
}

// CaptureGitState captures the current git repository state as a formatted string.
// Returns empty string if not in a git repo.
// Exported for testing.
func CaptureGitState() string {
	state := captureGitStateStruct()
	if !state.IsRepo {
		return "(not a git repo)"
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("- Branch: %s\n", state.Branch))
	sb.WriteString(fmt.Sprintf("- Commit: %s\n", state.CommitSHA))
	if state.CommitMessage != "" {
		sb.WriteString(fmt.Sprintf("- Message: %s\n", state.CommitMessage))
	}
	if state.HasChanges {
		sb.WriteString("- Working tree: has uncommitted changes\n")
	} else {
		sb.WriteString("- Working tree: clean\n")
	}
	return sb.String()
}

// captureGitStateStruct captures git state into a struct.
func captureGitStateStruct() GitState {
	state := GitState{}

	if err := exec.Command("git", "rev-parse", "--is-inside-work-tree").Run(); err != nil {
		return state
	}
	state.IsRepo = true

	if out, err := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD").Output(); err == nil {
		state.Branch = strings.TrimSpace(string(out))
	}

	if out, err := exec.Command("git", "rev-parse", "--short", "HEAD").Output(); err == nil {
		state.CommitSHA = strings.TrimSpace(string(out))
	}

	if out, err := exec.Command("git", "log", "-1", "--format=%s").Output(); err == nil {
		state.CommitMessage = strings.TrimSpace(string(out))
	}

	if out, err := exec.Command("git", "status", "--porcelain").Output(); err == nil {
		state.HasChanges = len(strings.TrimSpace(string(out))) > 0
	}

	return state
}

// captureGitStatus returns `git status --short` output, or empty string.
func captureGitStatus() string {
	out, err := exec.Command("git", "status", "--short").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// MessageText extracts plain text from a message. Exported for testing.
func MessageText(msg providers.Message) string {
	return messageText(msg)
}

// messageText extracts plain text from a message.
func messageText(msg providers.Message) string {
	if text, ok := msg.Content.(string); ok {
		return text
	}
	if blocks, ok := msg.Content.([]providers.ContentBlock); ok {
		var parts []string
		for _, b := range blocks {
			if b.Type == "text" && b.Text != "" {
				parts = append(parts, b.Text)
			}
		}
		return strings.Join(parts, "\n")
	}
	return ""
}
