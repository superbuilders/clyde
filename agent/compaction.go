package agent

import (
	"fmt"
	"strings"

	"github.com/this-is-alpha-iota/clyde/agent/providers"
)

// DefaultReserveTokens is the default number of tokens to reserve for the
// agent's next response. When input tokens exceed (contextWindowSize - reserveTokens),
// compaction is triggered automatically.
const DefaultReserveTokens = 16000

// CompactionCallback is called when compaction occurs.
// It receives the compaction marker message and the system summary.
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

// Compact performs conversation compaction. It:
//  1. Identifies the first (pinned) user message
//  2. Sends the conversation to Claude for summarization
//  3. Replaces history with: first user message + summary + recent messages
//  4. Emits callbacks for session persistence
//
// Returns an error if summarization fails.
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
	// Keep the last few messages (recent context) and summarize the rest.
	keepCount := a.recentKeepCount()
	summarizeEnd := len(a.history) - keepCount
	if summarizeEnd <= firstUserIdx+1 {
		// Not enough to summarize — the "old" portion is just the first message
		return nil
	}

	// The messages to summarize: everything between first user message and the kept tail.
	// We skip the first user message itself (it's preserved verbatim).
	toSummarize := a.history[firstUserIdx+1 : summarizeEnd]
	keptMessages := a.history[summarizeEnd:]

	// Step 3: Emit compaction marker
	if a.compactionCallback != nil {
		a.compactionCallback("🗜️ Compacting conversation history...", "")
	}
	if a.diagnosticCallback != nil {
		a.diagnosticCallback(fmt.Sprintf("🗜️ Compacting: %d messages → summary + %d recent messages",
			len(a.history), keepCount))
	}

	// Step 4: Generate summary via single LLM call (CMP-1 stub; CMP-2 replaces with multi-step)
	summary, err := a.generateCompactionSummary(firstUserMsg, toSummarize, keptMessages)
	if err != nil {
		return fmt.Errorf("compaction summary failed: %w", err)
	}

	// Step 5: Emit the summary via callback for session persistence
	if a.compactionCallback != nil {
		a.compactionCallback("", summary)
	}

	// Step 6: Replace history with compacted version.
	// Structure: [first user msg] [assistant ack] [summary as user] [assistant ack] [kept messages...]
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

	// Append recent kept messages
	newHistory = append(newHistory, keptMessages...)

	a.history = newHistory

	return nil
}

// FindFirstUserMessage locates the first user text message in history.
// This is the "pinned" / "sacred" original mission message.
// Exported for testing; used internally by Compact().
func (a *Agent) FindFirstUserMessage() (providers.Message, int) {
	return a.findFirstUserMessage()
}

// findFirstUserMessage locates the first user text message in history.
// This is the "pinned" / "sacred" original mission message.
func (a *Agent) findFirstUserMessage() (providers.Message, int) {
	for i, msg := range a.history {
		if msg.Role == "user" {
			// Check if it's a plain text message (not a tool_result or system injection)
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
	if len(a.history) < keep+4 { // need at least 4 messages to summarize + 4 to keep
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

// generateCompactionSummary creates a summary of the conversation so far
// using a single LLM call. This is the CMP-1 stub; CMP-2 will replace it
// with a multi-step agentic workflow.
func (a *Agent) generateCompactionSummary(
	firstUserMsg providers.Message,
	toSummarize []providers.Message,
	keptMessages []providers.Message,
) (string, error) {
	// Build the summarization prompt
	var sb strings.Builder
	sb.WriteString("You are summarizing a conversation for context compaction. ")
	sb.WriteString("Create a structured handoff document in Markdown that captures:\n\n")
	sb.WriteString("1. **Goal**: The original task/mission\n")
	sb.WriteString("2. **Progress**: What has been accomplished so far\n")
	sb.WriteString("3. **Key Decisions**: Important choices made and their rationale\n")
	sb.WriteString("4. **Current State**: Where things stand right now\n")
	sb.WriteString("5. **Next Steps**: What needs to happen next\n")
	sb.WriteString("6. **Critical Context**: Any important details that must not be lost\n\n")
	sb.WriteString("Be concise but thorough. Preserve specific file paths, function names, error messages, and technical details.\n")
	sb.WriteString("Do NOT include the original user message — it will be preserved separately.\n")

	// Build the conversation content to summarize
	var convContent strings.Builder
	convContent.WriteString("## Original Mission\n\n")
	if text, ok := firstUserMsg.Content.(string); ok {
		convContent.WriteString(text)
	}
	convContent.WriteString("\n\n## Conversation to Summarize\n\n")

	for _, msg := range toSummarize {
		role := msg.Role
		switch content := msg.Content.(type) {
		case string:
			convContent.WriteString(fmt.Sprintf("**%s**: %s\n\n", role, content))
		case []providers.ContentBlock:
			for _, block := range content {
				switch block.Type {
				case "text":
					convContent.WriteString(fmt.Sprintf("**%s**: %s\n\n", role, block.Text))
				case "tool_use":
					convContent.WriteString(fmt.Sprintf("**%s** [tool_use: %s]: %v\n\n", role, block.Name, block.Input))
				case "tool_result":
					resultText := ""
					if s, ok := block.Content.(string); ok {
						// Truncate large tool results for summarization
						if len(s) > 2000 {
							resultText = s[:2000] + "\n... (truncated)"
						} else {
							resultText = s
						}
					}
					convContent.WriteString(fmt.Sprintf("**tool_result**: %s\n\n", resultText))
				case "thinking":
					// Skip thinking blocks in summarization input
				}
			}
		}
	}

	if len(keptMessages) > 0 {
		convContent.WriteString("\n## Messages Being Kept (for reference)\n\n")
		for _, msg := range keptMessages {
			role := msg.Role
			if text, ok := msg.Content.(string); ok {
				convContent.WriteString(fmt.Sprintf("**%s**: %s\n\n", role, text))
			}
		}
	}

	// Make the summarization API call
	summaryMessages := []providers.Message{
		{Role: "user", Content: convContent.String()},
	}

	resp, err := a.apiClient.Call(sb.String(), summaryMessages, nil)
	if err != nil {
		return "", fmt.Errorf("summarization API call failed: %w", err)
	}

	// Extract text from response
	var summaryParts []string
	for _, block := range resp.Content {
		if block.Type == "text" && block.Text != "" {
			summaryParts = append(summaryParts, block.Text)
		}
	}

	if len(summaryParts) == 0 {
		return "", fmt.Errorf("summarization returned empty response")
	}

	return strings.Join(summaryParts, "\n"), nil
}
