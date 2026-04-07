package mcp

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/this-is-alpha-iota/clyde/api"
)

//go:embed playwright_tools.json
var embeddedPlaywrightTools []byte

// playwrightToolPrefix is prepended to all Playwright tool names to avoid
// collisions with clyde's built-in tools (e.g. our "browse" vs their "browser_*").
const playwrightToolPrefix = "mcp_playwright_"

// PlaywrightTools parses the embedded tool snapshot and returns Anthropic-
// formatted tool definitions with the "mcp_playwright_" prefix.
func PlaywrightTools() ([]api.Tool, error) {
	var mcpTools []Tool
	if err := json.Unmarshal(embeddedPlaywrightTools, &mcpTools); err != nil {
		return nil, fmt.Errorf("mcp: parse embedded playwright tools: %w", err)
	}

	apiTools := make([]api.Tool, 0, len(mcpTools))
	for _, t := range mcpTools {
		// Convert inputSchema from json.RawMessage to interface{}
		var schema interface{}
		if err := json.Unmarshal(t.InputSchema, &schema); err != nil {
			return nil, fmt.Errorf("mcp: parse schema for %q: %w", t.Name, err)
		}

		apiTools = append(apiTools, api.Tool{
			Name:        playwrightToolPrefix + t.Name,
			Description: t.Description,
			InputSchema: schema,
		})
	}

	return apiTools, nil
}

// StripPrefix removes the "mcp_playwright_" prefix from a tool name,
// returning the original MCP tool name (e.g. "browser_navigate").
func StripPrefix(prefixedName string) string {
	return strings.TrimPrefix(prefixedName, playwrightToolPrefix)
}

// HasPrefix reports whether the tool name has the "mcp_playwright_" prefix.
func HasPrefix(name string) bool {
	return strings.HasPrefix(name, playwrightToolPrefix)
}
