package tools

import (
	"claude-repl/api"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

func init() {
	Register(webSearchTool, executeWebSearch, displayWebSearch)
}

var webSearchTool = api.Tool{
	Name:        "web_search",
	Description: "Search the internet using Brave Search API. Returns titles, URLs, and snippets for search results. Use for finding current documentation, error solutions, package versions, recent news, or any information beyond your training data.",
	InputSchema: map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"query": map[string]interface{}{
				"type":        "string",
				"description": "The search query to execute",
			},
			"num_results": map[string]interface{}{
				"type":        "integer",
				"description": "Number of results to return (1-10, default 5)",
				"default":     5,
			},
		},
		"required": []string{"query"},
	},
}

func executeWebSearch(input map[string]interface{}, apiClient *api.Client, conversationHistory []api.Message) (string, error) {
	query, queryOk := input["query"].(string)
	if !queryOk || query == "" {
		return "", fmt.Errorf("query is required. Example: web_search(\"golang http client\")")
	}

	// Default to 5 results if not specified
	numResults := 5
	if numVal, ok := input["num_results"].(float64); ok {
		numResults = int(numVal)
	}
	// Cap at 10 results
	if numResults > 10 {
		numResults = 10
	}

	// Get API key from environment
	apiKey := os.Getenv("BRAVE_SEARCH_API_KEY")
	if apiKey == "" {
		return "", fmt.Errorf("BRAVE_SEARCH_API_KEY not found in .env file.\n\nTo fix this:\n  1. Sign up for a free API key at https://brave.com/search/api/\n  2. Add to your .env file: BRAVE_SEARCH_API_KEY=your-key-here\n  3. Free tier includes 2,000 searches per month")
	}

	// Build API request
	apiURL := fmt.Sprintf("https://api.search.brave.com/res/v1/web/search?q=%s&count=%d",
		url.QueryEscape(query), numResults)

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create search request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Subscription-Token", apiKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("search request failed: %w\n\nCheck your internet connection", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read search response: %w", err)
	}

	// Handle HTTP errors
	if resp.StatusCode != http.StatusOK {
		switch resp.StatusCode {
		case 401:
			return "", fmt.Errorf("search API authentication failed (401)\n\nYour API key may be invalid:\n  - Verify BRAVE_SEARCH_API_KEY in .env file\n  - Try generating a new key at https://brave.com/search/api/")
		case 429:
			return "", fmt.Errorf("search rate limit exceeded (429)\n\nYou've reached your monthly search limit (2000 free searches).\n  - Wait until next month for limit reset\n  - Or upgrade at https://brave.com/search/api/ ($5/mo for 20K searches)")
		case 400:
			return "", fmt.Errorf("invalid search query (400): %s\n\nCheck your query syntax", string(body))
		default:
			return "", fmt.Errorf("search API error (status %d): %s", resp.StatusCode, string(body))
		}
	}

	// Parse JSON response
	var result struct {
		Web struct {
			Results []struct {
				Title       string `json:"title"`
				URL         string `json:"url"`
				Description string `json:"description"`
			} `json:"results"`
		} `json:"web"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("failed to parse search results: %w\n\nResponse: %s", err, string(body))
	}

	// Check if we got any results
	if len(result.Web.Results) == 0 {
		return fmt.Sprintf("No results found for '%s'.\n\nSuggestions:\n  - Try different keywords\n  - Check spelling\n  - Use more general terms\n  - Try removing quotes or special characters", query), nil
	}

	// Format results
	var output strings.Builder
	output.WriteString(fmt.Sprintf("Found %d results for \"%s\":\n\n", len(result.Web.Results), query))

	for i, res := range result.Web.Results {
		output.WriteString(fmt.Sprintf("%d. [%s] - %s\n", i+1, res.Title, res.URL))
		if res.Description != "" {
			// Truncate description if too long
			desc := res.Description
			if len(desc) > 200 {
				desc = desc[:197] + "..."
			}
			output.WriteString(fmt.Sprintf("   %s\n", desc))
		}
		output.WriteString("\n")
	}

	return output.String(), nil
}

func displayWebSearch(input map[string]interface{}) string {
	query, _ := input["query"].(string)
	
	// Truncate long queries for display
	displayQuery := query
	if len(displayQuery) > 50 {
		displayQuery = displayQuery[:47] + "..."
	}
	return fmt.Sprintf("→ Searching web: \"%s\"", displayQuery)
}
