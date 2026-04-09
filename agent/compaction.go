package agent

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/this-is-alpha-iota/clyde/agent/providers"
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

// Compact performs conversation compaction. It:
//  1. Identifies the first (pinned) user message
//  2. Runs a multi-phase summarization workflow
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

	// Step 4: Run multi-phase compaction workflow
	summary, err := a.runCompactionWorkflow(firstUserMsg, toSummarize, keptMessages)
	if err != nil {
		return fmt.Errorf("compaction failed: %w", err)
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

// --- Multi-phase compaction workflow (CMP-2) ---

// runCompactionWorkflow executes the 5-phase compaction pipeline.
// Each phase makes a focused LLM call and produces intermediate output
// that feeds into the next phase.
//
// Phases:
//  1. Goal/constraint extraction
//  2. Decision capture
//  3. File-state analysis (git-centric)
//  4. Tool-result synthesis
//  5. Handoff drafting
func (a *Agent) runCompactionWorkflow(
	firstUserMsg providers.Message,
	toSummarize []providers.Message,
	keptMessages []providers.Message,
) (string, error) {

	// Serialize the conversation once for reuse across phases
	convText := serializeMessages(toSummarize)
	missionText := messageText(firstUserMsg)

	// Build recent-context block if enabled (feeds into every phase)
	recentCtx := ""
	if a.compactIncludeRecentContext {
		recentCtx = serializeMessages(keptMessages)
	}

	// Phase 1: Goal/constraint extraction
	a.emitCompactionProgress("🗜️ Compaction phase 1/5: extracting goals & constraints...")
	goals, err := a.compactionPhaseCall(
		"You are analyzing a conversation to extract the original goal and any constraints.\n"+
			"Return a concise Markdown section with:\n"+
			"- **Goal**: The core task/mission in 1-3 sentences\n"+
			"- **Constraints**: Any requirements, limitations, or acceptance criteria mentioned\n"+
			"Be precise. Quote exact requirements when possible.",
		missionText, convText, recentCtx,
	)
	if err != nil {
		return "", fmt.Errorf("phase 1 (goals) failed: %w", err)
	}
	a.emitCompactionDebug("Phase 1 output", goals)

	// Phase 2: Decision capture
	a.emitCompactionProgress("🗜️ Compaction phase 2/5: capturing decisions...")
	decisions, err := a.compactionPhaseCall(
		"You are analyzing a conversation to extract key technical decisions.\n"+
			"Return a concise Markdown section with:\n"+
			"- **Decisions Made**: Each significant choice, what was chosen, and why\n"+
			"- **Alternatives Rejected**: Notable alternatives that were considered but not chosen\n"+
			"Focus on decisions that a future reader would need to understand to continue the work.\n"+
			"Preserve specific names, paths, and technical details.",
		missionText, convText, recentCtx,
	)
	if err != nil {
		return "", fmt.Errorf("phase 2 (decisions) failed: %w", err)
	}
	a.emitCompactionDebug("Phase 2 output", decisions)

	// Phase 3: File-state analysis (git-centric)
	a.emitCompactionProgress("🗜️ Compaction phase 3/5: analyzing file & git state...")
	gitState := CaptureGitState()
	fileState, err := a.compactionPhaseCall(
		"You are analyzing a conversation to summarize the current state of the codebase.\n"+
			"Return a concise Markdown section with:\n"+
			"- **Files Modified/Created**: Key files that were changed or created, with brief descriptions\n"+
			"- **Current State**: What state the code is in right now\n"+
			"Do NOT include raw diffs. Reference file paths precisely.\n\n"+
			"Git state information:\n"+gitState,
		missionText, convText, recentCtx,
	)
	if err != nil {
		return "", fmt.Errorf("phase 3 (file-state) failed: %w", err)
	}
	a.emitCompactionDebug("Phase 3 output", fileState)

	// Phase 4: Tool-result synthesis
	a.emitCompactionProgress("🗜️ Compaction phase 4/5: synthesizing tool outputs...")
	toolSynthesis, err := a.compactionPhaseCall(
		"You are analyzing a conversation to summarize significant tool outputs.\n"+
			"Return a concise Markdown section with:\n"+
			"- **Significant Outputs**: Key results from tool executions (test results, errors encountered, search findings)\n"+
			"- **Errors Resolved**: Any errors that were encountered and how they were fixed\n"+
			"Skip routine outputs (simple file reads, directory listings). Focus on outputs that informed decisions.",
		missionText, convText, recentCtx,
	)
	if err != nil {
		return "", fmt.Errorf("phase 4 (tool-results) failed: %w", err)
	}
	a.emitCompactionDebug("Phase 4 output", toolSynthesis)

	// Phase 5: Handoff drafting — assemble everything into a structured document
	a.emitCompactionProgress("🗜️ Compaction phase 5/5: drafting handoff document...")

	assemblyInput := fmt.Sprintf(
		"## Phase Outputs\n\n"+
			"### Goals & Constraints\n%s\n\n"+
			"### Decisions\n%s\n\n"+
			"### File & Git State\n%s\n\n"+
			"### Tool Output Synthesis\n%s\n\n"+
			"### Git State\n%s",
		goals, decisions, fileState, toolSynthesis, gitState,
	)

	// Add recent context for bridging if enabled
	bridgeInstruction := ""
	if a.compactIncludeRecentContext && recentCtx != "" {
		assemblyInput += "\n\n### Recent Messages (still in context)\n" + recentCtx
		bridgeInstruction = "\n\nIMPORTANT: The 'Recent Messages' section shows what will remain in context after compaction. " +
			"Call out any open threads, pending actions, or decisions that bridge between your summary and those recent messages."
	}

	handoff, err := a.compactionPhaseCall(
		"You are writing a developer handoff document from phase outputs.\n"+
			"Combine the provided phase outputs into a single, well-structured Markdown document with these sections:\n\n"+
			"## Goal\n(from phase 1)\n\n"+
			"## Constraints\n(from phase 1)\n\n"+
			"## Progress\n(synthesize from all phases — what has been accomplished)\n\n"+
			"## Key Decisions\n(from phase 2)\n\n"+
			"## Current State\n(from phase 3 — include git SHA/branch if available)\n\n"+
			"## Next Steps\n(infer from the conversation what should happen next)\n\n"+
			"## Critical Context\n(anything a future reader must know — errors, gotchas, important details)\n\n"+
			"Be concise but thorough. This document replaces the conversation history, so nothing important should be lost.\n"+
			"Do NOT include the original user message — it is preserved separately."+
			bridgeInstruction,
		missionText, assemblyInput, "",
	)
	if err != nil {
		return "", fmt.Errorf("phase 5 (handoff) failed: %w", err)
	}
	a.emitCompactionDebug("Phase 5 output (final handoff)", handoff)

	// Post-compaction: check for uncommitted changes
	if gitState != "" && !strings.Contains(gitState, "not a git repo") {
		status := captureGitStatus()
		if status != "" {
			handoff += "\n\n---\n⚠️ **Uncommitted changes detected at compaction time:**\n```\n" + status + "\n```\n"
		}
	}

	return handoff, nil
}

// compactionPhaseCall makes a single LLM call for one compaction phase.
// It builds a user message from the mission, conversation, and optional recent context,
// then sends it with the given system prompt.
func (a *Agent) compactionPhaseCall(
	systemPrompt string,
	missionText string,
	conversationOrInput string,
	recentContext string,
) (string, error) {
	var content strings.Builder
	content.WriteString("## Original Mission\n\n")
	content.WriteString(missionText)
	content.WriteString("\n\n## Conversation\n\n")
	content.WriteString(conversationOrInput)
	if recentContext != "" {
		content.WriteString("\n\n## Recent Context (messages being kept)\n\n")
		content.WriteString(recentContext)
	}

	messages := []providers.Message{
		{Role: "user", Content: content.String()},
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
		return "", fmt.Errorf("empty response from compaction phase")
	}
	return strings.Join(parts, "\n"), nil
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
		// Truncate for diagnostic display (full content is in the final handoff)
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

	// Check if we're in a git repo
	if err := exec.Command("git", "rev-parse", "--is-inside-work-tree").Run(); err != nil {
		return state
	}
	state.IsRepo = true

	// Branch
	if out, err := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD").Output(); err == nil {
		state.Branch = strings.TrimSpace(string(out))
	}

	// Commit SHA (short)
	if out, err := exec.Command("git", "rev-parse", "--short", "HEAD").Output(); err == nil {
		state.CommitSHA = strings.TrimSpace(string(out))
	}

	// Commit message (first line)
	if out, err := exec.Command("git", "log", "-1", "--format=%s").Output(); err == nil {
		state.CommitMessage = strings.TrimSpace(string(out))
	}

	// Uncommitted changes
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

// --- Message serialization helpers ---

// SerializeMessages converts a slice of messages to a readable text format
// for feeding into compaction phases. Exported for testing.
func SerializeMessages(msgs []providers.Message) string {
	return serializeMessages(msgs)
}

// serializeMessages converts a slice of messages to a readable text format
// for feeding into compaction phases.
func serializeMessages(msgs []providers.Message) string {
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
						if len(s) > 2000 {
							resultText = s[:2000] + "\n... (truncated)"
						} else {
							resultText = s
						}
					}
					sb.WriteString(fmt.Sprintf("**tool_result**: %s\n\n", resultText))
				case "thinking":
					// Skip thinking blocks in serialization
				}
			}
		}
	}
	return sb.String()
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
