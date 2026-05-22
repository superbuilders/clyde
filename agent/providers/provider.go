package providers

// Provider is the interface that all LLM backends must implement.
// The agent calls Provider.Call() and receives a normalized Response
// in Claude-canonical format.
//
// The existing *Client (Claude) satisfies this interface implicitly.
// New backends (Ollama, OpenAI, etc.) implement it by translating
// their wire format to/from the canonical types in this package.
type Provider interface {
	// Call sends a request to the LLM with the given system prompt,
	// conversation history, and available tools. Returns a normalized
	// Response in Claude-canonical format.
	Call(systemPrompt string, messages []Message, tools []Tool) (*Response, error)
}

// StreamingProvider extends Provider with token-by-token streaming.
// Providers that don't support streaming simply don't implement this
// interface, and the agent falls back to Provider.Call().
type StreamingProvider interface {
	Provider
	// StreamCall sends a request to the LLM and streams the response
	// token by token. The onText callback is called for each incremental
	// text token, and the onThinking callback is called for each
	// incremental thinking token. Either callback may be nil.
	//
	// Returns the final assembled Response (identical to what Call()
	// would return) with complete content blocks and usage statistics.
	// All downstream code (history, session persistence, compaction)
	// can use the Response identically regardless of whether it came
	// from Call() or StreamCall().
	StreamCall(
		systemPrompt string,
		messages []Message,
		tools []Tool,
		onText func(text string),
		onThinking func(text string),
	) (*Response, error)
}
