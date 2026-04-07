package mcp

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/this-is-alpha-iota/clyde/providers"
	"github.com/this-is-alpha-iota/clyde/tools"
)

// RegisterPlaywrightTools registers the 21 Playwright MCP tools into clyde's
// tool registry. Each tool delegates to the given PlaywrightServer on invocation.
//
// Tools are registered from the embedded snapshot (no server needed at this point).
// The server is started lazily on first tool call via server.EnsureRunning().
func RegisterPlaywrightTools(server *PlaywrightServer) error {
	apiTools, err := PlaywrightTools()
	if err != nil {
		return fmt.Errorf("mcp: failed to load playwright tools: %w", err)
	}

	for _, tool := range apiTools {
		// Capture loop variable for the closure
		t := tool
		originalName := StripPrefix(t.Name)

		executor := func(input map[string]interface{}, apiClient *providers.Client, history []providers.Message) (string, error) {
			// Lazy-start the server on first tool call
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()

			if err := server.EnsureRunning(ctx); err != nil {
				return "", fmt.Errorf("Playwright MCP server failed to start: %w\n\n"+
					"Suggestions:\n"+
					"  - Ensure Node.js and npx are installed\n"+
					"  - Try running: npx @playwright/mcp@latest --headless\n"+
					"  - Check that Playwright browsers are installed: npx playwright install chromium", err)
			}

			result, err := server.CallTool(ctx, originalName, input)
			if err != nil {
				return "", fmt.Errorf("Playwright tool %q failed: %w", originalName, err)
			}

			if result.IsError {
				// Collect error text from content parts
				var errParts []string
				for _, part := range result.Content {
					if part.Text != "" {
						errParts = append(errParts, part.Text)
					}
				}
				return "", fmt.Errorf("Playwright error: %s", strings.Join(errParts, "\n"))
			}

			// Collect text output from content parts
			var parts []string
			for _, part := range result.Content {
				switch part.Type {
				case "text":
					parts = append(parts, part.Text)
				case "image":
					// Return as IMAGE_LOADED marker so the agent can include it
					parts = append(parts, fmt.Sprintf("IMAGE_LOADED:%s:0:%s", part.MimeType, part.Data))
				}
			}

			return strings.Join(parts, "\n"), nil
		}

		display := func(input map[string]interface{}) string {
			// Show a concise progress message
			detail := ""
			switch originalName {
			case "browser_navigate":
				if url, ok := input["url"].(string); ok {
					detail = url
				}
			case "browser_click":
				if el, ok := input["element"].(string); ok {
					detail = el
				} else if ref, ok := input["ref"].(string); ok {
					detail = ref
				}
			case "browser_fill_form":
				detail = "filling form fields"
			case "browser_type":
				if text, ok := input["text"].(string); ok {
					if len(text) > 40 {
						text = text[:40] + "..."
					}
					detail = fmt.Sprintf("%q", text)
				}
			case "browser_snapshot":
				detail = "capturing page"
			case "browser_take_screenshot":
				detail = "capturing screenshot"
			case "browser_evaluate":
				detail = "running JavaScript"
			case "browser_tabs":
				if action, ok := input["action"].(string); ok {
					detail = action
				}
			}

			displayName := strings.TrimPrefix(originalName, "browser_")
			if detail != "" {
				return fmt.Sprintf("→ Browser: %s %s", displayName, detail)
			}
			return fmt.Sprintf("→ Browser: %s", displayName)
		}

		tools.Register(t, executor, display)
	}

	return nil
}
