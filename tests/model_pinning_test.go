package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/superbuilders/clyde/agent"
	"github.com/superbuilders/clyde/agent/providers"
)

// These tests lock in the "pin everything explicitly" contract.
//
// Background: moving from Claude Opus 4.6 to Opus 5 silently changed three
// server-side defaults and broke harness behavior without any error:
//
//  1. thinking.display flipped to "omitted" -> all reasoning text vanished
//  2. thinking.budget_tokens was removed    -> hard 400 if sent
//  3. thinking defaults to ON               -> --no-think became a no-op
//
// Each of those has a regression test below. If a future model changes another
// default, the fix is to pin it here too.

// captureRequest spins up a stub API server, runs one Call through the real
// client, and returns the decoded request body that was sent on the wire.
func captureRequest(t *testing.T, build func(url string) *providers.Client, tools []providers.Tool) map[string]interface{} {
	t.Helper()

	var got map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("failed to decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"m","type":"message","role":"assistant",
			"content":[{"type":"text","text":"ok"}],"model":"m",
			"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`))
	}))
	defer srv.Close()

	client := build(srv.URL)
	if _, err := client.Call("system", []providers.Message{{Role: "user", Content: "hi"}}, tools); err != nil {
		t.Fatalf("Call failed: %v", err)
	}
	if got == nil {
		t.Fatal("no request captured")
	}
	return got
}

func thinkingOf(t *testing.T, req map[string]interface{}) map[string]interface{} {
	t.Helper()
	th, ok := req["thinking"].(map[string]interface{})
	if !ok {
		t.Fatalf("request has no thinking object; got keys %v", keysOf(req))
	}
	return th
}

func keysOf(m map[string]interface{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// --- Regression 1: thinking.display must always be pinned ---

func TestThinkingDisplayIsAlwaysPinned(t *testing.T) {
	// This is the bug that made reasoning invisible on Opus 5. The default
	// must be "summarized", never left to the model.
	req := captureRequest(t, func(url string) *providers.Client {
		return providers.NewClient("k", url, "claude-opus-5", 4096)
	}, nil)

	th := thinkingOf(t, req)
	if th["display"] != providers.ThinkingDisplaySummarized {
		t.Errorf("thinking.display = %v, want %q (omitting it makes reasoning invisible on Opus 5+)",
			th["display"], providers.ThinkingDisplaySummarized)
	}
	if th["type"] != providers.ThinkingTypeAdaptive {
		t.Errorf("thinking.type = %v, want %q", th["type"], providers.ThinkingTypeAdaptive)
	}
}

func TestWithThinkingBackfillsDisplay(t *testing.T) {
	// A caller that sets only Type must still get a pinned display mode,
	// otherwise reasoning silently disappears again.
	req := captureRequest(t, func(url string) *providers.Client {
		return providers.NewClient("k", url, "claude-opus-5", 4096).
			WithThinking(&providers.ThinkingConfig{Type: providers.ThinkingTypeAdaptive})
	}, nil)

	if got := thinkingOf(t, req)["display"]; got != providers.ThinkingDisplaySummarized {
		t.Errorf("display = %v, want %q", got, providers.ThinkingDisplaySummarized)
	}
}

// --- Regression 2: effort replaces budget_tokens ---

func TestOutputConfigEffortIsPinned(t *testing.T) {
	req := captureRequest(t, func(url string) *providers.Client {
		return providers.NewClient("k", url, "claude-opus-5", 4096)
	}, nil)

	oc, ok := req["output_config"].(map[string]interface{})
	if !ok {
		t.Fatalf("request has no output_config; got keys %v", keysOf(req))
	}
	if oc["effort"] != providers.DefaultEffort {
		t.Errorf("output_config.effort = %v, want %q", oc["effort"], providers.DefaultEffort)
	}
}

func TestLegacyBudgetTokensNeverReachesTheWire(t *testing.T) {
	// budget_tokens is a hard 400 on Opus 5+. A config that still sets
	// ThinkingBudget must be translated to effort, not passed through.
	req := captureRequest(t, func(url string) *providers.Client {
		c := providers.NewClient("k", url, "claude-opus-5", 4096)
		return c.WithEffort(providers.EffortForBudget(8192))
	}, nil)

	th := thinkingOf(t, req)
	if _, present := th["budget_tokens"]; present {
		t.Error("budget_tokens must never be sent — Opus 5+ rejects it with a 400")
	}
	if th["type"] == providers.ThinkingTypeEnabled {
		t.Error(`thinking.type must not be "enabled" — Opus 5+ rejects it with a 400`)
	}
}

func TestEffortForBudgetMapping(t *testing.T) {
	cases := []struct {
		budget int
		want   string
	}{
		{0, providers.DefaultEffort},
		{1024, providers.EffortLow},
		{4096, providers.EffortLow},
		{8192, providers.EffortMedium},
		{16384, providers.EffortMedium},
		{32768, providers.EffortHigh},
		{100000, providers.EffortXHigh},
	}
	for _, c := range cases {
		if got := providers.EffortForBudget(c.budget); got != c.want {
			t.Errorf("EffortForBudget(%d) = %q, want %q", c.budget, got, c.want)
		}
	}
}

func TestIsValidEffortRejectsUnknown(t *testing.T) {
	for _, e := range providers.ValidEfforts {
		if !providers.IsValidEffort(e) {
			t.Errorf("IsValidEffort(%q) = false, want true", e)
		}
	}
	for _, bad := range []string{"", "LOW", "ultra", "8192"} {
		if providers.IsValidEffort(bad) {
			t.Errorf("IsValidEffort(%q) = true, want false", bad)
		}
	}
}

// --- Regression 3: disabling thinking must be explicit ---

func TestNoThinkSendsExplicitDisabled(t *testing.T) {
	// Omitting the thinking field does NOT disable thinking on Opus 5 /
	// Sonnet 5 — they think by default. --no-think must send
	// {"type":"disabled"} explicitly or it is a silent no-op.
	req := captureRequest(t, func(url string) *providers.Client {
		return providers.NewClient("k", url, "claude-opus-5", 4096).WithThinking(nil)
	}, nil)

	th := thinkingOf(t, req)
	if th["type"] != providers.ThinkingTypeDisabled {
		t.Errorf("thinking.type = %v, want %q (omitting the field leaves thinking ON for Opus 5+)",
			th["type"], providers.ThinkingTypeDisabled)
	}
}

// --- Deprecated sampling params must never be sent ---

func TestDeprecatedSamplingParamsAreNeverSent(t *testing.T) {
	// temperature / top_p / top_k are hard 400s on Opus 5. They are not part
	// of Request at all; this test guards against someone re-adding them.
	req := captureRequest(t, func(url string) *providers.Client {
		return providers.NewClient("k", url, "claude-opus-5", 4096)
	}, nil)

	for _, banned := range []string{"temperature", "top_p", "top_k", "context_management"} {
		if _, present := req[banned]; present {
			t.Errorf("%q must not be sent — it is rejected with a 400 by Opus 5+", banned)
		}
	}
}

// --- tool_choice pinning ---

func TestToolChoicePinnedOnlyWhenToolsPresent(t *testing.T) {
	tools := []providers.Tool{{
		Name:        "t",
		Description: "d",
		InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}},
	}}

	withTools := captureRequest(t, func(url string) *providers.Client {
		return providers.NewClient("k", url, "claude-opus-5", 4096)
	}, tools)
	tc, ok := withTools["tool_choice"].(map[string]interface{})
	if !ok {
		t.Fatalf("tool_choice missing when tools are present; keys %v", keysOf(withTools))
	}
	if tc["type"] != providers.DefaultToolChoice {
		t.Errorf("tool_choice.type = %v, want %q", tc["type"], providers.DefaultToolChoice)
	}

	noTools := captureRequest(t, func(url string) *providers.Client {
		return providers.NewClient("k", url, "claude-opus-5", 4096)
	}, nil)
	if _, present := noTools["tool_choice"]; present {
		t.Error("tool_choice must be omitted when there are no tools")
	}
}

// --- Immutability of the With* builders ---

func TestWithBuildersDoNotMutateReceiver(t *testing.T) {
	base := providers.NewClient("k", "http://x", "m", 4096)
	baseEffort := base.Effort()

	_ = base.WithEffort(providers.EffortMax)
	if base.Effort() != baseEffort {
		t.Errorf("WithEffort mutated the receiver: %q -> %q", baseEffort, base.Effort())
	}

	_ = base.WithThinking(nil)
	if base.Thinking().Type != providers.DefaultThinkingType {
		t.Errorf("WithThinking mutated the receiver: type is now %q", base.Thinking().Type)
	}
}

// --- agent.New wiring ---

func TestAgentNewWiresThinkingConfig(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		a := agent.New(agent.Config{APIKey: "k", APIURL: "http://x", ModelID: "claude-opus-5", MaxTokens: 4096})
		if a == nil {
			t.Fatal("agent.New returned nil")
		}
	})

	t.Run("legacy_budget_translates_to_effort", func(t *testing.T) {
		if got := agent.EffortForBudget(8192); got != agent.EffortMedium {
			t.Errorf("agent.EffortForBudget(8192) = %q, want %q", got, agent.EffortMedium)
		}
	})

	t.Run("effort_reexports_match_providers", func(t *testing.T) {
		if agent.DefaultEffort != providers.DefaultEffort {
			t.Errorf("agent.DefaultEffort = %q, want %q", agent.DefaultEffort, providers.DefaultEffort)
		}
		if len(agent.ValidEfforts) != len(providers.ValidEfforts) {
			t.Error("agent.ValidEfforts is out of sync with providers.ValidEfforts")
		}
	})
}

// --- The serialized shape, as a readable contract ---

func TestPinnedRequestShape(t *testing.T) {
	req := captureRequest(t, func(url string) *providers.Client {
		return providers.NewClient("k", url, "claude-opus-5", 64000)
	}, nil)

	raw, _ := json.MarshalIndent(req, "", "  ")
	for _, want := range []string{`"thinking"`, `"display"`, `"output_config"`, `"effort"`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("pinned request is missing %s\nfull body:\n%s", want, raw)
		}
	}
}
