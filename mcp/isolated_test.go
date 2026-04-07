package mcp

import (
	"context"
	"fmt"
	"os/exec"
	"sync"
	"testing"
	"time"
)

func TestTwoConcurrentPlaywrightServers(t *testing.T) {
	if _, err := exec.LookPath("npx"); err != nil {
		t.Skip("npx not available")
	}

	var wg sync.WaitGroup
	results := make([]string, 2)
	errors := make([]error, 2)

	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			server := NewPlaywrightServer("--headless")
			defer server.Close()

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			if err := server.EnsureRunning(ctx); err != nil {
				errors[id] = fmt.Errorf("start: %w", err)
				return
			}

			result, err := server.CallTool(ctx, "browser_navigate", map[string]interface{}{
				"url": fmt.Sprintf("data:text/html,<h1>Server %d</h1>", id),
			})
			if err != nil {
				errors[id] = fmt.Errorf("navigate: %w", err)
				return
			}
			results[id] = fmt.Sprintf("OK (%d parts)", len(result.Content))
		}(i)
	}

	wg.Wait()

	for i := 0; i < 2; i++ {
		if errors[i] != nil {
			t.Errorf("Server %d: %v", i, errors[i])
		} else {
			t.Logf("✅ Server %d: %s", i, results[i])
		}
	}
}
