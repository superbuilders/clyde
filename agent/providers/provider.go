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
