package main

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"github.com/superbuilders/clyde/agent"
	"github.com/superbuilders/clyde/agent/providers"
)

// --- Regression coverage for issue #2 -------------------------------------
//
// Auto-compaction assembled a history in which a preserved user message
// carried a tool_result block whose originating assistant tool_use message was
// NOT preserved. Bedrock rejected the very next call with:
//
//	messages.8.content.0: unexpected `tool_use_id` found in `tool_result`
//	blocks: toolu_017oK28QHiFcjQWLt9L4z7L8.
//
// and the in-memory session became unrecoverable.

// assertToolPairing verifies the two adjacency invariants the Claude/Bedrock
// API enforces on a message list:
//
//  1. every tool_result references a tool_use in the immediately preceding
//     assistant message;
//  2. every tool_use is answered by a tool_result in the immediately following
//     user message (a trailing assistant turn is exempt — agent mid-turn).
func assertToolPairing(t *testing.T, label string, msgs []providers.Message) {
	t.Helper()

	blocksOf := func(m providers.Message) []providers.ContentBlock {
		b, _ := m.Content.([]providers.ContentBlock)
		return b
	}

	for i, msg := range msgs {
		if msg.Role == "user" {
			available := map[string]bool{}
			if i > 0 && msgs[i-1].Role == "assistant" {
				for _, b := range blocksOf(msgs[i-1]) {
					if b.Type == "tool_use" && b.ID != "" {
						available[b.ID] = true
					}
				}
			}
			for _, b := range blocksOf(msg) {
				if b.Type != "tool_result" {
					continue
				}
				if !available[b.ToolUseID] {
					t.Errorf("%s: message %d: orphan tool_result %q — no matching tool_use in the previous message\n%s",
						label, i, b.ToolUseID, dumpHistory(msgs))
				}
			}
			continue
		}

		var ids []string
		for _, b := range blocksOf(msg) {
			if b.Type == "tool_use" && b.ID != "" {
				ids = append(ids, b.ID)
			}
		}
		if len(ids) == 0 || i == len(msgs)-1 {
			continue
		}
		answered := map[string]bool{}
		if msgs[i+1].Role == "user" {
			for _, b := range blocksOf(msgs[i+1]) {
				if b.Type == "tool_result" {
					answered[b.ToolUseID] = true
				}
			}
		}
		for _, id := range ids {
			if !answered[id] {
				t.Errorf("%s: message %d: tool_use %q has no tool_result in the following message\n%s",
					label, i, id, dumpHistory(msgs))
			}
		}
	}
}

func dumpHistory(msgs []providers.Message) string {
	var sb strings.Builder
	for i, m := range msgs {
		sb.WriteString(fmt.Sprintf("  [%d] %s:", i, m.Role))
		switch c := m.Content.(type) {
		case string:
			preview := c
			if len(preview) > 50 {
				preview = preview[:50] + "..."
			}
			sb.WriteString(" " + strings.ReplaceAll(preview, "\n", " "))
		case []providers.ContentBlock:
			for _, b := range c {
				switch b.Type {
				case "tool_use":
					sb.WriteString(" tool_use(" + b.ID + ")")
				case "tool_result":
					sb.WriteString(" tool_result(" + b.ToolUseID + ")")
				default:
					sb.WriteString(" " + b.Type)
				}
			}
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

func toolUseMsg(ids ...string) providers.Message {
	blocks := []providers.ContentBlock{{Type: "text", Text: "Running tools."}}
	for _, id := range ids {
		blocks = append(blocks, providers.ContentBlock{
			Type:  "tool_use",
			ID:    id,
			Name:  "run_bash",
			Input: map[string]interface{}{"command": "echo " + id},
		})
	}
	return providers.Message{Role: "assistant", Content: blocks}
}

func toolResultMsg(ids ...string) providers.Message {
	var blocks []providers.ContentBlock
	for _, id := range ids {
		blocks = append(blocks, providers.ContentBlock{
			Type:      "tool_result",
			ToolUseID: id,
			Content:   "output for " + id,
		})
	}
	return providers.Message{Role: "user", Content: blocks}
}

// preserveList renders the "## Preserve" block a compaction call would emit.
func preserveList(indices []int) string {
	if len(indices) == 0 {
		return "No messages to preserve."
	}
	var sb strings.Builder
	sb.WriteString("## Preserve\n")
	for _, i := range indices {
		sb.WriteString(fmt.Sprintf("- Message %d: critical evidence\n", i))
	}
	return sb.String()
}

// toolHeavyHistory builds a synthetic history whose summarizable region
// (indices 1..9 of the raw history == 0..8 of toSummarize) contains three
// tool_use/tool_result pairs, one of them multi-block.
//
// raw index -> toSummarize index
//
//	1 -> 0  assistant tool_use  t1
//	2 -> 1  user      tool_result t1
//	3 -> 2  assistant text
//	4 -> 3  user      text
//	5 -> 4  assistant tool_use  t2
//	6 -> 5  user      tool_result t2
//	7 -> 6  assistant tool_use  t3, t4
//	8 -> 7  user      tool_result t3, t4
//	9 -> 8  assistant text
func toolHeavyHistory() []providers.Message {
	return []providers.Message{
		{Role: "user", Content: "Investigate the latency regression."},
		toolUseMsg("toolu_t1"),
		toolResultMsg("toolu_t1"),
		{Role: "assistant", Content: "p90 rose on the 14th."},
		{Role: "user", Content: "Check the queue split."},
		toolUseMsg("toolu_t2"),
		toolResultMsg("toolu_t2"),
		toolUseMsg("toolu_t3", "toolu_t4"),
		toolResultMsg("toolu_t3", "toolu_t4"),
		{Role: "assistant", Content: "Adjudicator-gather is the sole mover."},
		// --- kept-recent tail (last 4) ---
		{Role: "user", Content: "Confirm against the deploy log."},
		toolUseMsg("toolu_t5"),
		toolResultMsg("toolu_t5"),
		{Role: "assistant", Content: "Confirmed: bd7cf35e."},
	}
}

// compactWithPreserved runs a full compaction over history, forcing the
// workflow's calls 1 and 2 to select exactly the given toSummarize indices.
func compactWithPreserved(t *testing.T, history []providers.Message, call1, call2 []int) []providers.Message {
	t.Helper()

	callCount := 0
	ts := startMockCompactionServer(t, func(body string) string {
		callCount++
		switch callCount {
		case 1:
			return "## Current Objective\nFind the regression.\n\n## Objective Timeline\n1. investigate\n\n" + preserveList(call1)
		case 2:
			return preserveList(call2)
		default:
			return "Summary of the elided work."
		}
	})
	defer ts.Close()

	client := providers.NewClient("fake-key", ts.URL, "m", 4096)
	a := agent.NewAgent(client, "test", agent.WithContextWindowSize(200000))
	a.SetHistory(history)

	if err := a.Compact(); err != nil {
		t.Fatalf("Compact() error = %v", err)
	}
	return a.GetHistory()
}

// TestCompact_ArtifactShape_OrphanToolResult reproduces the exact selection
// shape from session 2026-09-22T09-36-59_thisistheaj: a user message carrying
// tool_result blocks is preserved while its partner assistant tool_use message
// is not. Before the fix this assembled an orphan tool_result and the next API
// call died with a 400.
func TestCompact_ArtifactShape_OrphanToolResult(t *testing.T) {
	history := toolHeavyHistory()

	// toSummarize index 7 == raw index 8 == the user tool_result for t3/t4.
	// Its partner (toSummarize index 6) is deliberately NOT selected — this is
	// what the live model did.
	got := compactWithPreserved(t, history, []int{3}, []int{7})

	assertToolPairing(t, "artifact shape", got)

	if err := agent.ValidateHistory(got); err != nil {
		t.Errorf("ValidateHistory() = %v, want nil\n%s", err, dumpHistory(got))
	}

	// The preserved evidence must actually survive — snapping to the pair
	// boundary is supposed to pull the tool_use in, not drop the result.
	if !strings.Contains(dumpHistory(got), "tool_result(toolu_t3)") {
		t.Errorf("preserved tool_result toolu_t3 was dropped instead of paired\n%s", dumpHistory(got))
	}
}

// TestCompact_ArtifactShape_OrphanToolUse is the mirror case: the assistant
// tool_use message is preserved but the user message carrying its results is
// not, which yields "tool_use ids were found without tool_result blocks".
func TestCompact_ArtifactShape_OrphanToolUse(t *testing.T) {
	history := toolHeavyHistory()
	got := compactWithPreserved(t, history, []int{4}, nil)

	assertToolPairing(t, "orphan tool_use", got)
	if err := agent.ValidateHistory(got); err != nil {
		t.Errorf("ValidateHistory() = %v, want nil\n%s", err, dumpHistory(got))
	}
}

// TestCompact_PreservedSubsets_Invariants is the property/table test required
// by issue #2: for EVERY subset of preserved indices over a tool-heavy
// history, the assembled history must satisfy both adjacency invariants.
func TestCompact_PreservedSubsets_Invariants(t *testing.T) {
	const n = 9 // toSummarize indices 0..8

	for mask := 0; mask < (1 << n); mask++ {
		var selected []int
		for i := 0; i < n; i++ {
			if mask&(1<<i) != 0 {
				selected = append(selected, i)
			}
		}
		name := "preserve_" + strings.Trim(strings.Join(strings.Fields(fmt.Sprint(selected)), "_"), "[]")
		if len(selected) == 0 {
			name = "preserve_none"
		}

		t.Run(name, func(t *testing.T) {
			// Split the selection across calls 1 and 2 the way the real
			// workflow does, so both parse paths are exercised.
			var call1, call2 []int
			for i, idx := range selected {
				if i%2 == 0 {
					call1 = append(call1, idx)
				} else {
					call2 = append(call2, idx)
				}
			}
			sort.Ints(call1)
			sort.Ints(call2)

			got := compactWithPreserved(t, toolHeavyHistory(), call1, call2)
			assertToolPairing(t, name, got)
			if err := agent.ValidateHistory(got); err != nil {
				t.Errorf("ValidateHistory() = %v, want nil\n%s", err, dumpHistory(got))
			}
		})
	}
}

// --- SnapPreserveIndicesToPairs (root-cause fix) ---------------------------

func TestSnapPreserveIndicesToPairs(t *testing.T) {
	msgs := toolHeavyHistory()[1:10] // the toSummarize region

	tests := []struct {
		name string
		in   []int
		want []int
	}{
		{"user_result_pulls_in_its_tool_use", []int{1}, []int{0, 1}},
		{"tool_use_pulls_in_its_results", []int{0}, []int{0, 1}},
		{"multi_block_pair", []int{7}, []int{6, 7}},
		{"plain_messages_untouched", []int{2, 3, 8}, []int{2, 3, 8}},
		{"already_paired_is_stable", []int{4, 5}, []int{4, 5}},
		{"mixed", []int{3, 5, 6}, []int{3, 4, 5, 6, 7}},
		{"empty", nil, nil},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			in := map[int]bool{}
			for _, i := range tc.in {
				in[i] = true
			}
			out := agent.SnapPreserveIndicesToPairs(msgs, in)

			var got []int
			for i, ok := range out {
				if ok {
					got = append(got, i)
				}
			}
			sort.Ints(got)

			if fmt.Sprint(got) != fmt.Sprint(tc.want) {
				t.Errorf("SnapPreserveIndicesToPairs(%v) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

// TestSnapPreserveIndicesToPairs_OutOfRange guards against index noise from
// the model (it emits indices that do not exist).
func TestSnapPreserveIndicesToPairs_OutOfRange(t *testing.T) {
	msgs := toolHeavyHistory()[1:10]
	out := agent.SnapPreserveIndicesToPairs(msgs, map[int]bool{-3: true, 99: true, 1: true})
	if !out[0] || !out[1] {
		t.Errorf("expected pair {0,1} to be selected, got %v", out)
	}
}

// --- ValidateHistory -------------------------------------------------------

// TestValidateHistory_NamesOffendingToolUseID asserts the validator produces a
// loud, actionable internal error rather than letting the request escape to
// the provider.
func TestValidateHistory_NamesOffendingToolUseID(t *testing.T) {
	bad := []providers.Message{
		{Role: "user", Content: "mission"},
		{Role: "assistant", Content: "ack"},
		toolResultMsg("toolu_017oK28QHiFcjQWLt9L4z7L8"),
		{Role: "assistant", Content: "done"},
	}
	err := agent.ValidateHistory(bad)
	if err == nil {
		t.Fatal("ValidateHistory() = nil, want error for orphan tool_result")
	}
	if !strings.Contains(err.Error(), "toolu_017oK28QHiFcjQWLt9L4z7L8") {
		t.Errorf("error must name the offending tool_use_id, got: %v", err)
	}
}

func TestValidateHistory_NamesUnansweredToolUse(t *testing.T) {
	bad := []providers.Message{
		{Role: "user", Content: "mission"},
		toolUseMsg("toolu_zzz"),
		{Role: "user", Content: "unrelated"},
		{Role: "assistant", Content: "done"},
	}
	err := agent.ValidateHistory(bad)
	if err == nil {
		t.Fatal("ValidateHistory() = nil, want error for unanswered tool_use")
	}
	if !strings.Contains(err.Error(), "toolu_zzz") {
		t.Errorf("error must name the offending tool_use id, got: %v", err)
	}
}

func TestValidateHistory_AcceptsValidHistory(t *testing.T) {
	ok := []providers.Message{
		{Role: "user", Content: "mission"},
		toolUseMsg("toolu_a"),
		toolResultMsg("toolu_a"),
		{Role: "assistant", Content: "done"},
	}
	if err := agent.ValidateHistory(ok); err != nil {
		t.Errorf("ValidateHistory() = %v, want nil", err)
	}
}

// TestValidateHistory_AllowsTrailingToolUse — the agent mid-turn case.
func TestValidateHistory_AllowsTrailingToolUse(t *testing.T) {
	ok := []providers.Message{
		{Role: "user", Content: "mission"},
		toolUseMsg("toolu_a"),
	}
	if err := agent.ValidateHistory(ok); err != nil {
		t.Errorf("ValidateHistory() = %v, want nil for trailing tool_use", err)
	}
}

// --- SanitizeToolPairs (belt and braces) -----------------------------------

func TestSanitizeToolPairs_DropsOrphanResultAndSynthesizesMissing(t *testing.T) {
	in := []providers.Message{
		{Role: "user", Content: "mission"},
		{Role: "assistant", Content: "ack"},
		toolResultMsg("toolu_orphan"),
		toolUseMsg("toolu_unanswered"),
		{Role: "user", Content: "next"},
		{Role: "assistant", Content: "done"},
	}
	out := agent.SanitizeToolPairs(in)
	assertToolPairing(t, "sanitized", out)
	if err := agent.ValidateHistory(out); err != nil {
		t.Errorf("ValidateHistory() after sanitize = %v, want nil\n%s", err, dumpHistory(out))
	}
}

// TestSanitizeToolPairs_HoistsResultsToFront — tool_result blocks must lead
// the user message that carries them.
func TestSanitizeToolPairs_HoistsResultsToFront(t *testing.T) {
	in := []providers.Message{
		{Role: "user", Content: "mission"},
		toolUseMsg("toolu_a", "toolu_b"),
		{Role: "user", Content: []providers.ContentBlock{
			{Type: "tool_result", ToolUseID: "toolu_a", Content: "A"},
			{Type: "text", Text: "an interjection"},
			{Type: "tool_result", ToolUseID: "toolu_b", Content: "B"},
		}},
		{Role: "assistant", Content: "done"},
	}
	out := agent.SanitizeToolPairs(in)
	blocks, ok := out[2].Content.([]providers.ContentBlock)
	if !ok {
		t.Fatalf("message 2 lost its blocks: %#v", out[2].Content)
	}
	if blocks[0].Type != "tool_result" || blocks[1].Type != "tool_result" {
		t.Errorf("tool_result blocks must lead the message, got %s,%s", blocks[0].Type, blocks[1].Type)
	}
	if err := agent.ValidateHistory(out); err != nil {
		t.Errorf("ValidateHistory() = %v, want nil", err)
	}
}

// TestSanitizeToolPairs_Idempotent — running it twice must not change output.
func TestSanitizeToolPairs_Idempotent(t *testing.T) {
	in := []providers.Message{
		{Role: "user", Content: "mission"},
		{Role: "assistant", Content: "ack"},
		toolResultMsg("toolu_orphan"),
		toolUseMsg("toolu_x"),
		{Role: "assistant", Content: "double assistant"},
	}
	once := agent.SanitizeToolPairs(in)
	twice := agent.SanitizeToolPairs(once)
	if dumpHistory(once) != dumpHistory(twice) {
		t.Errorf("SanitizeToolPairs not idempotent:\nonce:\n%stwice:\n%s", dumpHistory(once), dumpHistory(twice))
	}
}

// --- Provider-level pairing error detection & self-heal --------------------

func TestIsToolPairingError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "bedrock_unexpected_tool_use_id",
			err:  fmt.Errorf("API error (status 400)\nError: aws-bedrock error: messages.8.content.0: unexpected `tool_use_id` found in `tool_result` blocks: toolu_017oK28QHiFcjQWLt9L4z7L8."),
			want: true,
		},
		{
			name: "anthropic_ids_without_tool_result",
			err:  fmt.Errorf("API error (status 400)\nError: messages.4: `tool_use` ids were found without `tool_result` blocks immediately after: toolu_abc"),
			want: true,
		},
		{
			name: "unrelated_400",
			err:  fmt.Errorf("API error (status 400)\nError: max_tokens is too large"),
			want: false,
		},
		{"nil", nil, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := providers.IsToolPairingError(tc.err); got != tc.want {
				t.Errorf("IsToolPairingError() = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestPairingError_MentionsResume — the 400 is recoverable via `--resume`, and
// the user-facing error has to say so.
func TestPairingError_MentionsResume(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"error":{"type":"invalid_request_error","message":"aws-bedrock error: messages.8.content.0: unexpected `+"`tool_use_id`"+` found in `+"`tool_result`"+` blocks: toolu_017oK28QHiFcjQWLt9L4z7L8."}}`)
	}))
	defer ts.Close()

	client := providers.NewClient("fake-key", ts.URL, "m", 4096)
	_, err := client.Call("sys", []providers.Message{{Role: "user", Content: "hi"}}, nil)
	if err == nil {
		t.Fatal("expected error from 400 response")
	}
	if !strings.Contains(err.Error(), "--resume") {
		t.Errorf("pairing-error message must mention --resume, got:\n%v", err)
	}
}

// TestHandleMessage_ResanitizesAndRetriesOnce verifies the single automatic
// re-sanitize-and-retry: the first call fails with the pairing family, the
// history is repaired in place, and the retry succeeds.
func TestHandleMessage_ResanitizesAndRetriesOnce(t *testing.T) {
	var calls int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		calls++
		if calls == 1 {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"error":{"message":"unexpected `+"`tool_use_id`"+` found in `+"`tool_result`"+` blocks: toolu_orphan"}}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"content":[{"type":"text","text":"recovered"}],"usage":{"input_tokens":10,"output_tokens":5}}`)
	}))
	defer ts.Close()

	client := providers.NewClient("fake-key", ts.URL, "m", 4096)
	a := agent.NewAgent(client, "test")
	a.SetHistory([]providers.Message{
		{Role: "user", Content: "mission"},
		{Role: "assistant", Content: "ack"},
		toolResultMsg("toolu_orphan"),
	})

	out, err := a.HandleMessage("continue")
	if err != nil {
		t.Fatalf("HandleMessage() error = %v, want recovery", err)
	}
	if !strings.Contains(out, "recovered") {
		t.Errorf("HandleMessage() = %q, want the retried response", out)
	}
	if calls != 2 {
		t.Errorf("expected exactly 2 API calls (fail + one retry), got %d", calls)
	}
}

// TestHandleMessage_RetriesOnlyOnce — if the retry also fails, surface the
// error instead of looping.
func TestHandleMessage_RetriesOnlyOnce(t *testing.T) {
	var calls int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		calls++
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"error":{"message":"unexpected `+"`tool_use_id`"+` found in `+"`tool_result`"+` blocks: toolu_orphan"}}`)
	}))
	defer ts.Close()

	client := providers.NewClient("fake-key", ts.URL, "m", 4096)
	a := agent.NewAgent(client, "test")
	a.SetHistory([]providers.Message{
		{Role: "user", Content: "mission"},
		{Role: "assistant", Content: "ack"},
		toolResultMsg("toolu_orphan"),
	})

	if _, err := a.HandleMessage("continue"); err == nil {
		t.Fatal("expected the error to surface after the single retry")
	}
	if calls != 2 {
		t.Errorf("expected exactly 2 API calls, got %d", calls)
	}
}
