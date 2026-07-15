package mcp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/superbuilders/clyde/agent/providers"
	"github.com/superbuilders/clyde/agent/tools"
)

// ScreenshotDir is the default directory where Playwright screenshots are
// saved. This directory should be gitignored so screenshots don't get
// accidentally committed to repositories.
const ScreenshotDir = ".clyde/screenshots"

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

			// Redirect screenshot filenames to the gitignored screenshots dir
			// so they don't get accidentally committed to repositories.
			if originalName == "browser_take_screenshot" {
				input = redirectScreenshotFilename(input)
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

// RedirectScreenshotFilename ensures screenshot files are saved to the
// gitignored ScreenshotDir. If no filename is provided, it sets a default
// inside ScreenshotDir. If a filename is provided as a bare name or relative
// path, it's redirected into ScreenshotDir. Absolute paths are left as-is.
//
// The directory is created if it doesn't exist.
// Exported for testing.
func RedirectScreenshotFilename(input map[string]interface{}) map[string]interface{} {
	return redirectScreenshotFilename(input)
}

func redirectScreenshotFilename(input map[string]interface{}) map[string]interface{} {
	// Ensure the screenshots directory exists
	os.MkdirAll(ScreenshotDir, 0755)

	filename, hasFilename := input["filename"].(string)

	if !hasFilename || filename == "" {
		// No filename provided — Playwright will use its default naming
		// (page-{timestamp}.png) but we want it inside our dir.
		// Set the filename explicitly.
		ts := time.Now().Format("20060102-150405")
		ext := "png"
		if typ, ok := input["type"].(string); ok && typ == "jpeg" {
			ext = "jpeg"
		}
		input["filename"] = filepath.Join(ScreenshotDir, fmt.Sprintf("page-%s.%s", ts, ext))
		return input
	}

	// If it's already an absolute path, respect the user's intent
	if filepath.IsAbs(filename) {
		return input
	}

	// If already under ScreenshotDir, leave it alone
	if strings.HasPrefix(filename, ScreenshotDir) {
		return input
	}

	// Redirect relative filename into ScreenshotDir
	input["filename"] = filepath.Join(ScreenshotDir, filename)
	return input
}
