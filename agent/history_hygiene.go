package agent

import (
	"fmt"
	"strings"

	"github.com/superbuilders/clyde/agent/providers"
)

// This file holds the tool_use/tool_result pairing contract for assembled
// conversation history. See issue #2: auto-compaction produced a user message
// whose tool_result had no matching tool_use in the preceding assistant
// message, Bedrock answered 400, and the in-memory session was unrecoverable.
//
// Three layers, in order of preference:
//
//  1. SnapPreserveIndicesToPairs — root cause. Preservation is index-based
//     over a structure with pair constraints, so the model can and does select
//     one half of a pair. Snapping the selection to pair boundaries means an
//     orphan is never constructed in the first place.
//  2. SanitizeToolPairs — belt and braces. Repairs any orphan that still
//     reaches assembly (splices, bridges, resumed history).
//  3. ValidateHistory — hard assertion. A violation is an internal error
//     naming the offending id; the history is never shipped to the provider.

// orphanToolResultPlaceholder replaces a user message whose every block was a
// dangling tool_result. Something must remain so user/assistant alternation
// survives the removal.
const orphanToolResultPlaceholder = "[System: prior tool results omitted during compaction]"

// synthesizedToolResultText is the stand-in body for a tool_use whose real
// result was dropped by compaction.
const synthesizedToolResultText = "[Tool result omitted during compaction — see the compaction summary for the outcome.]"

// contentBlocks normalizes a Message's content into a block slice.
// Returns ok=false for string (plain text) content.
func contentBlocks(msg providers.Message) ([]providers.ContentBlock, bool) {
	blocks, ok := msg.Content.([]providers.ContentBlock)
	return blocks, ok
}

// toolUseIDs returns the ids of every tool_use block in an assistant message.
func toolUseIDs(msg providers.Message) []string {
	if msg.Role != "assistant" {
		return nil
	}
	blocks, ok := contentBlocks(msg)
	if !ok {
		return nil
	}
	var ids []string
	for _, b := range blocks {
		if b.Type == "tool_use" && b.ID != "" {
			ids = append(ids, b.ID)
		}
	}
	return ids
}

// toolResultIDs returns the tool_use_ids answered by a user message.
func toolResultIDs(msg providers.Message) []string {
	if msg.Role != "user" {
		return nil
	}
	blocks, ok := contentBlocks(msg)
	if !ok {
		return nil
	}
	var ids []string
	for _, b := range blocks {
		if b.Type == "tool_result" {
			ids = append(ids, b.ToolUseID)
		}
	}
	return ids
}

// SnapPreserveIndicesToPairs expands an LLM-selected set of preserve indices
// so that it never splits a tool_use/tool_result pair.
//
// This is the root-cause fix for issue #2. Calls 1 and 2 of the compaction
// workflow name message indices ("Message 97", "Message 101") over a structure
// that has adjacency constraints the model knows nothing about. Selecting the
// user message that carries a tool_result without the assistant message that
// issued the tool_use is perfectly representable, and in practice the model
// does it. Rather than repairing the damage after assembly, the selection is
// snapped to pair boundaries before assembly:
//
//   - if a selected user message carries tool_results, every assistant message
//     holding a matching tool_use is selected too;
//   - if a selected assistant message issues tool_uses, every user message
//     holding a matching tool_result is selected too.
//
// Expansion runs to a fixed point (pulling in one half can expose another
// pair) and out-of-range indices are ignored. The input map is not mutated.
func SnapPreserveIndicesToPairs(msgs []providers.Message, indices map[int]bool) map[int]bool {
	out := make(map[int]bool, len(indices))
	for i, ok := range indices {
		if ok && i >= 0 && i < len(msgs) {
			out[i] = true
		}
	}
	if len(out) == 0 {
		return out
	}

	// id -> indices of the messages that issue / answer it.
	issuedBy := map[string][]int{}
	answeredBy := map[string][]int{}
	for i, msg := range msgs {
		for _, id := range toolUseIDs(msg) {
			issuedBy[id] = append(issuedBy[id], i)
		}
		for _, id := range toolResultIDs(msg) {
			answeredBy[id] = append(answeredBy[id], i)
		}
	}

	for {
		added := false
		for i := range out {
			partners := func(ids []string, table map[string][]int) {
				for _, id := range ids {
					for _, p := range table[id] {
						if !out[p] {
							out[p] = true
							added = true
						}
					}
				}
			}
			partners(toolUseIDs(msgs[i]), answeredBy)
			partners(toolResultIDs(msgs[i]), issuedBy)
		}
		if !added {
			return out
		}
	}
}

// SanitizeToolPairs repairs tool_use/tool_result adjacency in an assembled
// history. It is the second line of defence behind SnapPreserveIndicesToPairs,
// and also covers history that arrives from a resumed session or an
// interrupted tool loop.
//
//  1. Every tool_result must reference a tool_use in the immediately preceding
//     assistant message. Orphans are dropped.
//  2. Surviving tool_result blocks must lead the user message that carries
//     them, so they are hoisted to the front.
//  3. Every tool_use must be answered by a tool_result in the immediately
//     following user message. Missing answers are synthesized as placeholders;
//     a trailing assistant tool_use is left alone because the agent is
//     mid-turn and the real result is about to be appended.
//
// Messages are never dropped or reordered — only their content is rewritten,
// or a carrier user message is spliced in — so alternation is preserved.
// The function is idempotent.
func SanitizeToolPairs(msgs []providers.Message) []providers.Message {
	return sanitizeToolPairs(msgs)
}

func sanitizeToolPairs(msgs []providers.Message) []providers.Message {
	if len(msgs) == 0 {
		return msgs
	}

	out := make([]providers.Message, len(msgs))
	copy(out, msgs)

	// Pass 1: drop orphan tool_results and hoist the survivors to the front.
	for i := range out {
		if out[i].Role != "user" {
			continue
		}
		blocks, ok := contentBlocks(out[i])
		if !ok {
			continue
		}

		available := map[string]bool{}
		if i > 0 && out[i-1].Role == "assistant" {
			for _, id := range toolUseIDs(out[i-1]) {
				available[id] = true
			}
		}

		results := make([]providers.ContentBlock, 0, len(blocks))
		rest := make([]providers.ContentBlock, 0, len(blocks))
		changed := false
		for _, b := range blocks {
			if b.Type == "tool_result" {
				if !available[b.ToolUseID] {
					changed = true
					continue
				}
				if len(rest) > 0 {
					// This result trails a non-result block — reorder needed.
					changed = true
				}
				results = append(results, b)
				continue
			}
			rest = append(rest, b)
		}
		if !changed {
			continue
		}
		kept := append(results, rest...)
		if len(kept) == 0 {
			out[i].Content = orphanToolResultPlaceholder
			continue
		}
		out[i].Content = kept
	}

	// Pass 2: synthesize placeholder tool_results for unanswered tool_use
	// blocks.
	//
	// The loop is index-based (not `range`) because the slice can grow when a
	// carrier user message is spliced in; a `range` evaluates len() once and
	// would skip the shifted tail. lastIdx tracks where the originally-final
	// message now lives, so the "agent mid-turn" exemption is granted only to
	// the genuine tail and not to a message that merely became non-final
	// because of an earlier splice.
	lastIdx := len(out) - 1
	for i := 0; i < len(out); i++ {
		ids := toolUseIDs(out[i])
		if len(ids) == 0 {
			continue
		}

		// Trailing assistant tool_use: agent mid-turn, leave it alone.
		if i == len(out)-1 && i == lastIdx {
			continue
		}

		var answered map[string]bool
		nextIsCarrier := i+1 < len(out) && out[i+1].Role == "user"
		if nextIsCarrier {
			answered = map[string]bool{}
			for _, id := range toolResultIDs(out[i+1]) {
				answered[id] = true
			}
		}

		var missing []providers.ContentBlock
		for _, id := range ids {
			if answered[id] {
				continue
			}
			missing = append(missing, providers.ContentBlock{
				Type:      "tool_result",
				ToolUseID: id,
				Content:   synthesizedToolResultText,
			})
		}
		if len(missing) == 0 {
			continue
		}

		if nextIsCarrier {
			if nextBlocks, ok := contentBlocks(out[i+1]); ok {
				// tool_results must lead the user message.
				out[i+1].Content = append(missing, nextBlocks...)
			} else {
				// Plain-text user message (e.g. an alternation bridge):
				// promote it to blocks so the results can lead it.
				text, _ := out[i+1].Content.(string)
				merged := missing
				if strings.TrimSpace(text) != "" {
					merged = append(merged, providers.ContentBlock{Type: "text", Text: text})
				}
				out[i+1].Content = merged
			}
			continue
		}

		// Next message is another assistant (or there is none) — splice in a
		// user message carrying just the synthesized results.
		out = append(out, providers.Message{})
		copy(out[i+2:], out[i+1:])
		out[i+1] = providers.Message{Role: "user", Content: missing}
		if i+1 <= lastIdx {
			lastIdx++
		}
	}

	return out
}

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
// is sent to the provider. It is the guard rail for compaction: a compacted
// history that violates them hard-fails every subsequent request in the
// session, so a violation must be a loud internal error naming the offending
// tool_use_id rather than a 400 from the API.
//
// Invariants:
//  1. Roles are user/assistant and alternate strictly.
//  2. No message has empty content.
//  3. Every tool_result references a tool_use in the immediately preceding
//     assistant message.
//  4. tool_result blocks lead the user message that carries them.
//  5. Every tool_use is answered by a tool_result in the immediately following
//     user message — except a trailing assistant turn (agent mid-turn).
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
			available := map[string]bool{}
			if i > 0 {
				for _, id := range toolUseIDs(msgs[i-1]) {
					available[id] = true
				}
			}
			seenNonResult := false
			for _, b := range blocks {
				if b.Type != "tool_result" {
					seenNonResult = true
					continue
				}
				if !available[b.ToolUseID] {
					return fmt.Errorf("message %d: orphan tool_result %q (no matching tool_use in the previous message)", i, b.ToolUseID)
				}
				if seenNonResult {
					return fmt.Errorf("message %d: tool_result %q does not lead the message", i, b.ToolUseID)
				}
			}
			continue
		}

		ids := toolUseIDs(msg)
		for _, b := range blocks {
			if b.Type == "tool_use" && b.ID == "" {
				return fmt.Errorf("message %d: tool_use block with empty id", i)
			}
		}
		if len(ids) == 0 || i == len(msgs)-1 {
			continue
		}

		answered := map[string]bool{}
		for _, id := range toolResultIDs(msgs[i+1]) {
			answered[id] = true
		}
		for _, id := range ids {
			if !answered[id] {
				return fmt.Errorf("message %d: tool_use %q has no tool_result in the following message", i, id)
			}
		}
	}
	return nil
}
