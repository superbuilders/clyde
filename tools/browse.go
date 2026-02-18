package tools

import (
	"github.com/this-is-alpha-iota/clyde/api"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	md "github.com/JohannesKaufmann/html-to-markdown"
)

func init() {
	Register(browseTool, executeBrowse, displayBrowse)
}

var browseTool = api.Tool{
	Name:        "browse",
	Description: "Fetch a URL and convert HTML to readable markdown. Optionally extract specific information using AI processing. Use for reading documentation pages, following up on search results, or extracting specific information from web pages.",
	InputSchema: map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"url": map[string]interface{}{
				"type":        "string",
				"description": "The URL to fetch (HTTP/HTTPS)",
			},
			"prompt": map[string]interface{}{
				"type":        "string",
				"description": "Optional: What to extract/summarize from the page. If not provided, returns the full converted markdown. Example: 'List all tutorial sections' or 'What are the main features?'",
			},
			"max_length": map[string]interface{}{
				"type":        "integer",
				"description": "Maximum content length in KB (default 500, max 1000)",
				"default":     500,
			},
		},
		"required": []string{"url"},
	},
}

func executeBrowse(input map[string]interface{}, apiClient *api.Client, conversationHistory []api.Message) (string, error) {
	urlStr, urlOk := input["url"].(string)
	if !urlOk || urlStr == "" {
		return "", fmt.Errorf("url is required. Example: browse(\"https://example.com\")")
	}

	// Default to 500 KB if not specified
	maxLength := 500
	if maxVal, ok := input["max_length"].(float64); ok {
		maxLength = int(maxVal)
	}
	// Cap at 1000 KB
	if maxLength > 1000 {
		maxLength = 1000
	}

	// Optional prompt for AI extraction
	prompt := ""
	if promptVal, ok := input["prompt"].(string); ok {
		prompt = promptVal
	}

	// Validate URL format
	parsedURL, err := url.Parse(urlStr)
	if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
		return "", fmt.Errorf("invalid URL format. Must start with http:// or https://\n\nProvided: %s", urlStr)
	}

	// Create HTTP client with timeout and redirect handling
	client := &http.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("stopped after 10 redirects")
			}
			return nil
		},
	}

	// Make request
	req, err := http.NewRequest("GET", urlStr, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", "claude-repl/1.0 (Go HTTP Client)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

	resp, err := client.Do(req)
	if err != nil {
		if strings.Contains(err.Error(), "no such host") {
			return "", fmt.Errorf("could not resolve domain '%s'. Check the URL.\n\nError: %w", parsedURL.Host, err)
		}
		if strings.Contains(err.Error(), "timeout") {
			return "", fmt.Errorf("request timed out after 30 seconds. The server may be slow or unreachable.\n\nURL: %s", urlStr)
		}
		return "", fmt.Errorf("network error: %w\n\nCheck your internet connection", err)
	}
	defer resp.Body.Close()

	// Handle HTTP errors
	if resp.StatusCode != http.StatusOK {
		switch resp.StatusCode {
		case 404:
			return "", fmt.Errorf("page not found (404): %s\n\nThe URL may be incorrect or the page may have been removed", urlStr)
		case 403:
			return "", fmt.Errorf("access denied (403). The page may require authentication or permissions.\n\nURL: %s", urlStr)
		case 401:
			return "", fmt.Errorf("authentication required (401). The page requires login credentials.\n\nURL: %s", urlStr)
		case 429:
			return "", fmt.Errorf("rate limit exceeded (429). The server is throttling requests.\n\nURL: %s\n\nTry again later", urlStr)
		case 500, 502, 503, 504:
			return "", fmt.Errorf("server error (%d). The server is experiencing problems.\n\nURL: %s\n\nTry again later or check https://downdetector.com", resp.StatusCode, urlStr)
		default:
			return "", fmt.Errorf("HTTP error %d\n\nURL: %s", resp.StatusCode, urlStr)
		}
	}

	// Check content length if provided
	maxBytes := int64(maxLength) * 1024
	if resp.ContentLength > maxBytes {
		sizeKB := resp.ContentLength / 1024
		return "", fmt.Errorf("page too large (%d KB). Max allowed: %d KB.\n\nIncrease max_length or try a different page.\n\nURL: %s", sizeKB, maxLength, urlStr)
	}

	// Read body with limit
	limitReader := io.LimitReader(resp.Body, maxBytes)
	body, err := io.ReadAll(limitReader)
	if err != nil {
		return "", fmt.Errorf("failed to read page content: %w", err)
	}

	// Check if we hit the limit
	if int64(len(body)) >= maxBytes {
		return "", fmt.Errorf("page content exceeds %d KB. Increase max_length or try a different page.\n\nURL: %s", maxLength, urlStr)
	}

	// Convert HTML to Markdown
	converter := md.NewConverter("", true, nil)
	markdown, err := converter.ConvertString(string(body))
	if err != nil {
		return "", fmt.Errorf("failed to convert HTML to markdown: %w\n\nThe page may have malformed HTML", err)
	}

	// Trim excessive whitespace
	markdown = strings.TrimSpace(markdown)

	// If markdown is empty, provide helpful message
	if markdown == "" {
		return "", fmt.Errorf("page returned no readable content. It may be:\n  - A JavaScript-heavy page (requires browser rendering)\n  - An empty page\n  - A redirect page\n\nURL: %s", urlStr)
	}

	// If no prompt provided, return the markdown
	if prompt == "" {
		// Truncate if still too long after conversion
		if len(markdown) > int(maxBytes) {
			markdown = markdown[:maxBytes-100] + "\n\n[Content truncated due to length]"
		}
		return markdown, nil
	}

	// AI Processing: Use Claude to extract specific information
	// Build a message asking Claude to process the content
	extractionPrompt := fmt.Sprintf("Given this webpage content:\n\n%s\n\nUser request: %s", markdown, prompt)

	// Truncate markdown if too long for Claude context
	if len(extractionPrompt) > 100000 {
		// Keep first 90KB of content
		truncatedMarkdown := markdown[:90000] + "\n\n[Content truncated to fit context]"
		extractionPrompt = fmt.Sprintf("Given this webpage content:\n\n%s\n\nUser request: %s", truncatedMarkdown, prompt)
	}

	// Create a new conversation with the extraction prompt
	extractionHistory := []api.Message{
		{
			Role:    "user",
			Content: extractionPrompt,
		},
	}

	// Call Claude to process the content
	// We need the system prompt here
	systemPrompt := "You are a helpful AI assistant. Extract the requested information from the webpage content provided."
	resp2, err := apiClient.Call(systemPrompt, extractionHistory, []api.Tool{})
	if err != nil {
		return "", fmt.Errorf("failed to process page with AI: %w", err)
	}

	// Extract text response
	var textResponses []string
	for _, block := range resp2.Content {
		if block.Type == "text" && block.Text != "" {
			textResponses = append(textResponses, block.Text)
		}
	}

	if len(textResponses) == 0 {
		return markdown, nil // Fallback to raw markdown
	}

	return strings.Join(textResponses, "\n"), nil
}

func displayBrowse(input map[string]interface{}) string {
	urlStr, _ := input["url"].(string)
	prompt := ""
	if promptVal, ok := input["prompt"].(string); ok {
		prompt = promptVal
	}

	// Format display message
	if prompt != "" {
		// Truncate prompt if too long
		displayPrompt := prompt
		if len(displayPrompt) > 40 {
			displayPrompt = displayPrompt[:37] + "..."
		}
		return fmt.Sprintf("→ Browsing: %s (extract: \"%s\")", urlStr, displayPrompt)
	}
	return fmt.Sprintf("→ Browsing: %s", urlStr)
}
