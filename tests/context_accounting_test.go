package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/superbuilders/clyde/agent"
	"github.com/superbuilders/clyde/agent/providers"
)

// --- Fix: "compacted, then immediately out of context again" ---
//
// Observed in session 2026-09-18T17-03-57: compaction ran, and the very next
// turn every tool call came back as
//
//	ERROR: Tool result too large for context window (estimated 36 tokens, limit 0 tokens)
//
// on results of 129 and 259 characters. The agent concluded the context window
// was exhausted and wrote a handoff instead of continuing. Two defects combined:
//
//  1. lastUsage still described the pre-compaction history, so the guard's
//     remaining budget stayed at zero after compaction had freed the window.
//  2. a zero budget discards results of *any* size, so the guard reported the
//     window as full for a 129-byte listing.
//
// A third defect in the same accounting: cache_creation_input_tokens was
// excluded from the input total, so a turn reporting input=2 /
// cache_creation=16816 metered as 2 tokens.

// TestUsageTotalIncludesCacheCreation verifies cache-creation tokens count
// toward the input total. They occupy window like any other input token.
func TestUsageTotalIncludesCacheCreation(t *testing.T) {
	// The exact usage line from the post-compaction turn in the session.
	u := providers.Usage{InputTokens: 2, OutputTokens: 921, CacheReadInputTokens: 0, CacheCreationInputTokens: 16816}

	if got := u.TotalInputTokens(); got != 16818 {
		t.Errorf("TotalInputTokens() = %d, want 16818 (2 input + 16816 cache_creation)", got)
	}

	// Output tokens are not input and must not leak into the total.
	if got := (providers.Usage{OutputTokens: 5000}).TotalInputTokens(); got != 0 {
		t.Errorf("output-only usage should total 0 input tokens, got %d", got)
	}
}

// TestShouldCompact_CountsCacheCreationTokens verifies compaction triggers when
// the window is filled by cache-creation tokens. Before the fix these were
// invisible, so a full window read as near-empty and compaction never fired —
// the request then failed at the provider with a context-length error.
func TestShouldCompact_CountsCacheCreationTokens(t *testing.T) {
	client := providers.NewClient("fake", "http://localhost", "m", 1000)
	a := agent.NewAgent(client, "test",
		agent.WithContextWindowSize(200000),
		agent.WithReserveTokens(16000),
	)

	// Threshold is 184000. Nothing but cache-creation tokens, well past it.
	a.SetLastUsage(providers.Usage{InputTokens: 10, CacheCreationInputTokens: 190000})

	if !a.ShouldCompact() {
		t.Error("cache-creation tokens should count toward the compaction threshold")
	}
}

// TestGuardOversizedToolResults_FullWindowKeepsSmallResults verifies that a
// full context window does not cause small tool results to be discarded.
// This is the literal failure from the session: a 129-character result
// rejected against a "limit of 0 tokens".
func TestGuardOversizedToolResults_FullWindowKeepsSmallResults(t *testing.T) {
	client := providers.NewClient("fake", "http://localhost", "m", 1000)
	a := agent.NewAgent(client, "test",
		agent.WithContextWindowSize(200000),
		agent.WithReserveTokens(16000),
	)

	// Window is over the reserve line: remaining budget computes to <= 0.
	a.SetLastUsage(providers.Usage{InputTokens: 199000})

	small := strings.Repeat("x", 129)
	result := a.GuardOversizedToolResults([]providers.ContentBlock{
		{Type: "tool_result", ToolUseID: "toolu_1", Content: small},
	})

	if result[0].IsError {
		t.Error("a 129-char tool result must not be discarded just because the window is full")
	}
	if got := result[0].Content.(string); got != small {
		t.Errorf("small result should pass through unchanged; got %q", shortPreview(got))
	}
}

// TestGuardOversizedToolResults_FullWindowStillBlocksHugeResults verifies the
// floor did not disable the guard. A genuinely unsurvivable result is still
// replaced, because that is the crash the guard exists to prevent.
func TestGuardOversizedToolResults_FullWindowStillBlocksHugeResults(t *testing.T) {
	client := providers.NewClient("fake", "http://localhost", "m", 1000)
	a := agent.NewAgent(client, "test",
		agent.WithContextWindowSize(200000),
		agent.WithReserveTokens(16000),
	)
	a.SetLastUsage(providers.Usage{InputTokens: 199000})

	// ~143k tokens: far above the floor.
	huge := strings.Repeat("x", 500000)
	result := a.GuardOversizedToolResults([]providers.ContentBlock{
		{Type: "tool_result", ToolUseID: "toolu_1", Content: huge},
	})

	if !result[0].IsError {
		t.Error("a 500k-char tool result must still be discarded")
	}
	if !strings.Contains(result[0].Content.(string), "too large for context window") {
		t.Error("discarded result should explain itself to the model")
	}
}

// TestGuardOversizedToolResults_FloorBoundary pins the floor's edges, so the
// guard can neither collapse to zero nor be silently widened.
func TestGuardOversizedToolResults_FloorBoundary(t *testing.T) {
	subtests := []struct {
		name      string
		chars     int
		wantError bool
	}{
		// MinToolResultTokenBudget is 4000 tokens ≈ 14000 chars at 3.5 chars/token.
		{name: "just_under_floor", chars: 13000, wantError: false},
		{name: "well_over_floor", chars: 40000, wantError: true},
	}

	for _, tc := range subtests {
		t.Run(tc.name, func(t *testing.T) {
			client := providers.NewClient("fake", "http://localhost", "m", 1000)
			a := agent.NewAgent(client, "test",
				agent.WithContextWindowSize(200000),
				agent.WithReserveTokens(16000),
			)
			a.SetLastUsage(providers.Usage{InputTokens: 200000}) // remaining is negative

			result := a.GuardOversizedToolResults([]providers.ContentBlock{
				{Type: "tool_result", ToolUseID: "t", Content: strings.Repeat("x", tc.chars)},
			})

			if result[0].IsError != tc.wantError {
				t.Errorf("%d chars: IsError = %v, want %v (estimated %d tokens, floor %d)",
					tc.chars, result[0].IsError, tc.wantError,
					agent.EstimateTokens(strings.Repeat("x", tc.chars)), agent.MinToolResultTokenBudget)
			}
		})
	}
}

// TestCompact_ReBaselinesUsage verifies that after a successful compaction the
// recorded usage describes the *new* history rather than the discarded one.
// Without this, ShouldCompact stays true and the oversize guard stays at zero
// budget until the next API response lands — which is the window in which the
// session died.
func TestCompact_ReBaselinesUsage(t *testing.T) {
	server, cleanup := mockCompactionServer(t)
	defer cleanup()

	client := providers.NewClient("fake", server.URL, "test-model", 4096)
	a := agent.NewAgent(client, "test",
		agent.WithContextWindowSize(200000),
		agent.WithReserveTokens(16000),
	)

	a.SetHistory(buildLongHistory(40))
	a.SetLastUsage(providers.Usage{InputTokens: 199000})

	if !a.ShouldCompact() {
		t.Fatal("precondition: agent should want to compact at 199000 tokens")
	}

	if err := a.Compact(); err != nil {
		t.Fatalf("Compact() failed: %v", err)
	}

	after := a.LastUsage().TotalInputTokens()
	if after >= 199000 {
		t.Errorf("usage not re-baselined after compaction: %d tokens (was 199000)", after)
	}
	if after == 0 {
		t.Error("usage should be an estimate of the new history, not zero")
	}
	if a.ShouldCompact() {
		t.Error("agent should not immediately want to compact again after compacting")
	}

	// And the guard must now hand out a real budget again.
	result := a.GuardOversizedToolResults([]providers.ContentBlock{
		{Type: "tool_result", ToolUseID: "t", Content: strings.Repeat("x", 30000)},
	})
	if result[0].IsError {
		t.Error("a 30k-char result should fit in a freshly compacted window")
	}
}

// mockCompactionServer answers the three compaction calls with a well-formed
// response. The content is irrelevant to these tests — only that the workflow
// completes so the re-baseline runs on the success path.
func mockCompactionServer(t *testing.T) (*httptest.Server, func()) {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"content":[{"type":"text","text":"## Current Objective\nrefactor\n\n## Preserve\n- Message 3: pivot\n"}],"usage":{"input_tokens":10,"output_tokens":5}}`)
	}))
	return ts, ts.Close
}

// buildLongHistory produces an alternating history long enough for Compact()
// to have something to summarize.
func buildLongHistory(n int) []providers.Message {
	msgs := make([]providers.Message, 0, n)
	msgs = append(msgs, providers.Message{Role: "user", Content: "Refactor the sharing model to be per-conversation."})
	for i := 1; i < n; i++ {
		role := "assistant"
		if i%2 == 0 {
			role = "user"
		}
		msgs = append(msgs, providers.Message{
			Role:    role,
			Content: fmt.Sprintf("turn %d: %s", i, strings.Repeat("detail ", 20)),
		})
	}
	return msgs
}

func shortPreview(s string) string {
	if len(s) > 80 {
		return s[:80] + "..."
	}
	return s
}
