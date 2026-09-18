package agent

import (
	"fmt"
	"strings"

	"github.com/superbuilders/clyde/agent/providers"
)

// strippedThinkingPlaceholder replaces an assistant message whose entire
// content was thinking/redacted_thinking blocks. Something must remain so
// user/assistant alternation survives the removal.
const strippedThinkingPlaceholder = "[System: reasoning trace omitted during compaction]"

// StripThinkingBlocks removes every `thinking` and `redacted_thinking` block
// from the given messages, returning a new slice (the input is not mutated).
//
// Thinking blocks carry a cryptographic signature computed over the exact
// conversation prefix that produced them. Compaction rewrites that prefix, so
// replaying a preserved thinking block makes the API reject the request
// ("Expected `thinking` or `redacted_thinking`", signature verification
// failure), which permanently bricks the session. Compacted history therefore
// never replays thinking.
//
// Messages that consisted solely of thinking blocks are replaced with a
// placeholder text body rather than dropped, so alternation is preserved.
func StripThinkingBlocks(msgs []providers.Message) []providers.Message {
	if len(msgs) == 0 {
		return msgs
	}

	out := make([]providers.Message, len(msgs))
	copy(out, msgs)

	for i := range out {
		blocks, ok := contentBlocks(out[i])
		if !ok {
			continue
		}

		kept := make([]providers.ContentBlock, 0, len(blocks))
		stripped := false
		for _, b := range blocks {
			if b.Type == "thinking" || b.Type == "redacted_thinking" {
				stripped = true
				continue
			}
			kept = append(kept, b)
		}
		if !stripped {
			continue
		}
		if len(kept) == 0 {
			out[i].Content = strippedThinkingPlaceholder
			continue
		}
		out[i].Content = kept
	}

	return out
}

// ValidateHistory asserts the invariants a message list must satisfy before it
// is sent to the Claude API. It is the guard rail for compaction: a compacted
// history that fails validation would hard-fail every subsequent request in
// the session, so Compact() validates and falls back instead.
//
// Invariants:
//  1. No thinking / redacted_thinking blocks (stale signatures after a splice).
//  2. Strict user/assistant alternation (no two consecutive same-role turns).
//  3. Every tool_result references a tool_use in the immediately preceding
//     assistant message.
//  4. Every tool_use is answered by a tool_result in the immediately following
//     user message — except a trailing assistant turn (agent mid-turn).
//  5. tool_result blocks lead the user message that carries them.
//  6. No message has empty content.
func ValidateHistory(msgs []providers.Message) error {
	for i, msg := range msgs {
		if msg.Role != "user" && msg.Role != "assistant" {
			return fmt.Errorf("message %d: invalid role %q", i, msg.Role)
		}
		if i > 0 && msgs[i-1].Role == msg.Role {
			return fmt.Errorf("message %d: two consecutive %q messages (alternation broken)", i, msg.Role)
		}

		blocks, hasBlocks := contentBlocks(msg)
		if !hasBlocks {
			text, _ := msg.Content.(string)
			if strings.TrimSpace(text) == "" {
				return fmt.Errorf("message %d (%s): empty content", i, msg.Role)
			}
			continue
		}
		if len(blocks) == 0 {
			return fmt.Errorf("message %d (%s): empty content block list", i, msg.Role)
		}

		for _, b := range blocks {
			if b.Type == "thinking" || b.Type == "redacted_thinking" {
				return fmt.Errorf("message %d (%s): %s block replayed after compaction", i, msg.Role, b.Type)
			}
		}

		if msg.Role == "user" {
			// tool_results must exist in the previous assistant turn and must
			// lead this message.
			available := map[string]bool{}
			if i > 0 && msgs[i-1].Role == "assistant" {
				if prev, ok := contentBlocks(msgs[i-1]); ok {
					for _, b := range prev {
						if b.Type == "tool_use" && b.ID != "" {
							available[b.ID] = true
						}
					}
				}
			}
			seenNonResult := false
			for _, b := range blocks {
				if b.Type != "tool_result" {
					seenNonResult = true
					continue
				}
				if !available[b.ToolUseID] {
					return fmt.Errorf("message %d: orphan tool_result %q (no matching tool_use in previous message)", i, b.ToolUseID)
				}
				if seenNonResult {
					return fmt.Errorf("message %d: tool_result %q does not lead the message", i, b.ToolUseID)
				}
			}
			continue
		}

		// assistant: every tool_use must be answered next.
		var ids []string
		for _, b := range blocks {
			if b.Type == "tool_use" {
				if b.ID == "" {
					return fmt.Errorf("message %d: tool_use block with empty id", i)
				}
				ids = append(ids, b.ID)
			}
		}
		if len(ids) == 0 || i == len(msgs)-1 {
			continue
		}

		answered := map[string]bool{}
		if next := msgs[i+1]; next.Role == "user" {
			if nb, ok := contentBlocks(next); ok {
				for _, b := range nb {
					if b.Type == "tool_result" {
						answered[b.ToolUseID] = true
					}
				}
			}
		}
		for _, id := range ids {
			if !answered[id] {
				return fmt.Errorf("message %d: tool_use %q has no tool_result in the following message", i, id)
			}
		}
	}
	return nil
}
