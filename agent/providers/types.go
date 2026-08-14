package providers

import "encoding/json"

// Message represents a single message in the conversation
type Message struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"`
}

// Tool represents a Claude API tool definition
type Tool struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema interface{} `json:"input_schema"`
}

// CacheControl represents prompt caching control
type CacheControl struct {
	Type string `json:"type"` // "ephemeral"
}

// ─────────────────────────────────────────────────────────────────────────────
// Pinned request defaults
//
// Every value below is sent EXPLICITLY on every request. The goal is that
// swapping TS_AGENT_MODEL_ID must never silently change harness behavior.
//
// Baseline is Claude Opus 4.6 — the model this harness was developed and
// tuned against. Relying on server-side defaults has already bitten us three
// times when moving 4.6 -> Opus 5:
//
//  1. thinking.display flipped from "summarized" to "omitted", so every
//     thinking block arrived with empty text and the TUI/session log showed
//     no reasoning at all (the model was still thinking — we just threw it away).
//  2. thinking.budget_tokens was replaced by output_config.effort; sending the
//     old field is a hard 400.
//  3. thinking defaults to ON, so omitting the field no longer disables
//     thinking — --no-think became a silent no-op.
//
// Rule of thumb: if the API has a knob that affects behavior and the model
// accepts it, pin it here rather than inheriting a default.
// ─────────────────────────────────────────────────────────────────────────────

// Thinking modes.
const (
	// ThinkingTypeAdaptive lets the model decide how much to think per turn.
	// The only mode supported by the Opus 5 / Sonnet 5 family.
	ThinkingTypeAdaptive = "adaptive"
	// ThinkingTypeDisabled turns thinking off. Must be sent explicitly —
	// omitting the thinking field does NOT disable thinking on models that
	// default it on (Opus 5, Sonnet 5).
	ThinkingTypeDisabled = "disabled"
	// ThinkingTypeEnabled is the legacy budget-based mode. Rejected with a 400
	// by Opus 5 and newer; retained only for older models.
	ThinkingTypeEnabled = "enabled"
)

// Thinking display modes — controls whether thinking text is returned.
const (
	// ThinkingDisplaySummarized returns human-readable thinking text.
	// This matches Opus 4.6 behavior and is what the TUI and session logs need.
	ThinkingDisplaySummarized = "summarized"
	// ThinkingDisplayOmitted returns thinking blocks with an empty thinking
	// field (signature only). This is the Opus 5 default and the reason
	// reasoning silently vanished from sessions.
	ThinkingDisplayOmitted = "omitted"
)

// Reasoning effort levels, in ascending order of thinking tokens spent.
// Replaces the deprecated thinking.budget_tokens knob.
const (
	EffortLow    = "low"
	EffortMedium = "medium"
	EffortHigh   = "high"
	EffortXHigh  = "xhigh"
	EffortMax    = "max"
)

// ValidEfforts lists every accepted output_config.effort value, as reported
// by the API itself when given an invalid variant.
var ValidEfforts = []string{EffortLow, EffortMedium, EffortHigh, EffortXHigh, EffortMax}

// IsValidEffort reports whether e is an accepted effort level.
func IsValidEffort(e string) bool {
	for _, v := range ValidEfforts {
		if v == e {
			return true
		}
	}
	return false
}

// Pinned defaults applied to every request.
const (
	// DefaultThinkingType — adaptive is the only mode the 5-family accepts and
	// is also valid on 4.6, so it is portable across both.
	DefaultThinkingType = ThinkingTypeAdaptive

	// DefaultThinkingDisplay — restores visible reasoning, matching 4.6.
	DefaultThinkingDisplay = ThinkingDisplaySummarized

	// DefaultEffort is calibrated against Opus 4.6's observed default thinking
	// volume. Measured median thinking tokens over an identical prompt set:
	//
	//	Opus 4.6 default (our baseline)  median 210   total 1926
	//	Opus 5 effort=low                median 118   total 1297   ← closest
	//	Opus 5 effort=medium             median 637   total 6539   (3.4x baseline)
	//	Opus 5 no output_config          median 723   total 7309   (3.8x baseline)
	//
	// "low" also preserves adaptive scaling (7 thinking tokens on a trivial
	// prompt vs 3662 on a hard concurrency design question), so cheap turns
	// stay cheap and hard turns still get depth. Override with
	// TS_AGENT_THINKING_EFFORT.
	DefaultEffort = EffortLow

	// DefaultToolChoice — pinned so tool-calling behavior cannot drift.
	DefaultToolChoice = "auto"
)

// EffortForBudget translates a legacy thinking.budget_tokens value into the
// nearest effort level, so pre-existing THINKING_BUDGET_TOKENS configs keep
// working on models that reject budget_tokens outright.
//
// Thresholds are approximate by design; budget_tokens was a hard cap while
// effort is an adaptive target, so there is no exact mapping.
func EffortForBudget(budget int) string {
	switch {
	case budget <= 0:
		return DefaultEffort
	case budget <= 4096:
		return EffortLow
	case budget <= 16384:
		return EffortMedium
	case budget <= 32768:
		return EffortHigh
	default:
		return EffortXHigh
	}
}

// ThinkingConfig configures thinking for the Claude API.
//
// Preferred form (Opus 4.6, Opus 5, Sonnet 5):
//
//	{Type: "adaptive", Display: "summarized"}
//
// To disable thinking, send {Type: "disabled"} explicitly. Do not rely on
// omitting the field — newer models think by default.
//
// BudgetTokens is legacy and rejected by Opus 5+; prefer OutputConfig.Effort.
type ThinkingConfig struct {
	Type string `json:"type"` // "adaptive", "disabled", or legacy "enabled"
	// Display controls whether thinking text is returned. Always set this —
	// the server-side default varies by model.
	Display string `json:"display,omitempty"` // "summarized" or "omitted"
	// BudgetTokens is only valid with Type=="enabled" (pre-Opus-5 models).
	BudgetTokens int `json:"budget_tokens,omitempty"`
}

// OutputConfig controls reasoning depth. Replaces thinking.budget_tokens on
// Opus 5 and newer; also accepted by Opus 4.6.
type OutputConfig struct {
	Effort string `json:"effort,omitempty"` // low | medium | high | xhigh | max
}

// ToolChoice pins how the model decides to call tools.
type ToolChoice struct {
	Type string `json:"type"` // "auto", "any", "none", or "tool"
	Name string `json:"name,omitempty"`
}

// Request represents a Claude API request.
//
// Deliberately NOT included, and why:
//
//	temperature     Deprecated on Opus 5 — any value other than 1.0 is a hard
//	                400. Thinking already forces temperature to 1, so omitting
//	                it is behaviorally identical to the 4.6 default (1.0) while
//	                staying forward-compatible.
//	top_p / top_k   Deprecated on Opus 5 (hard 400). Unset on 4.6 by default,
//	                so omitting matches baseline.
//	stop_sequences  Baseline is none; the harness relies on stop_reason instead.
//	service_tier    Left to the account/gateway default so routing policy is
//	                not hardcoded into the harness.
//	context_management  Not supported on this route ("Extra inputs are not
//	                permitted"); compaction is handled harness-side.
type Request struct {
	Model        string          `json:"model"`
	MaxTokens    int             `json:"max_tokens"`
	CacheControl *CacheControl   `json:"cache_control,omitempty"`
	System       string          `json:"system"`
	Messages     []Message       `json:"messages"`
	Tools        []Tool          `json:"tools,omitempty"`
	ToolChoice   *ToolChoice     `json:"tool_choice,omitempty"`
	Thinking     *ThinkingConfig `json:"thinking,omitempty"`
	OutputConfig *OutputConfig   `json:"output_config,omitempty"`
}

// ImageSource represents the source of an image in a content block
type ImageSource struct {
	Type      string `json:"type"`                // "base64" or "url"
	MediaType string `json:"media_type"`          // "image/jpeg", "image/png", "image/webp", "image/gif"
	Data      string `json:"data,omitempty"`      // Base64 data (for type="base64")
	URL       string `json:"url,omitempty"`       // URL (for type="url")
}

// ContentBlock represents a block of content in a Claude response.
//
// Block types:
//   - "text":              Text content (Text field populated)
//   - "thinking":          Thinking trace (Thinking + Signature fields populated)
//   - "redacted_thinking": Redacted thinking (Data field populated)
//   - "tool_use":          Tool call (ID, Name, Input fields populated)
//   - "tool_result":       Tool result (ToolUseID, Content fields populated)
//   - "image":             Image content (Source field populated)
type ContentBlock struct {
	Type      string                 `json:"type"`
	Text      string                 `json:"text,omitempty"`
	ID        string                 `json:"id,omitempty"`
	Name      string                 `json:"name,omitempty"`
	Input     map[string]interface{} `json:"input,omitempty"`
	Content   interface{}            `json:"content,omitempty"`
	ToolUseID string                 `json:"tool_use_id,omitempty"`
	IsError   bool                   `json:"is_error,omitempty"`
	Source    *ImageSource           `json:"source,omitempty"`  // For type="image"

	// Thinking block fields
	Thinking  string `json:"thinking,omitempty"`  // Thinking trace text (type="thinking")
	Signature string `json:"signature,omitempty"` // Signature for verification (type="thinking")
	Data      string `json:"data,omitempty"`      // Encrypted data (type="redacted_thinking")
}

// MarshalJSON implements custom JSON marshaling for ContentBlock.
// For tool_use blocks, the "input" field is always included (even when empty),
// because the Claude API requires it. For other block types, input is omitted
// when nil/empty (standard omitempty behavior).
func (b ContentBlock) MarshalJSON() ([]byte, error) {
	// Use an alias to avoid infinite recursion
	type Alias ContentBlock
	if b.Type == "tool_use" {
		// For tool_use: ensure input is always present
		inputVal := b.Input
		if inputVal == nil {
			inputVal = map[string]interface{}{}
		}
		return json.Marshal(&struct {
			Alias
			Input map[string]interface{} `json:"input"` // no omitempty
		}{
			Alias: Alias(b),
			Input: inputVal,
		})
	}
	if b.Type == "thinking" {
		// For thinking: always include the "thinking" field, even when empty.
		// The API may return thinking blocks with empty text but a valid
		// signature; omitting the field on round-trip causes strict
		// endpoints to reject the request.
		return json.Marshal(&struct {
			Alias
			Thinking string `json:"thinking"` // no omitempty
		}{
			Alias:    Alias(b),
			Thinking: b.Thinking,
		})
	}
	// For all other types: use default serialization (with omitempty on input)
	return json.Marshal(&struct {
		Alias
	}{
		Alias: Alias(b),
	})
}

// Usage represents token usage information in a response
type Usage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
}

// Response represents a Claude API response
type Response struct {
	ID         string         `json:"id"`
	Type       string         `json:"type"`
	Role       string         `json:"role"`
	Content    []ContentBlock `json:"content"`
	Model      string         `json:"model"`
	StopReason string         `json:"stop_reason"`
	Usage      Usage          `json:"usage"`
}
