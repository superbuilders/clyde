package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/superbuilders/clyde/agent/providers"
)

const okResponseBody = `{"id":"msg_1","type":"message","role":"assistant","content":[{"type":"text","text":"hi"}],"stop_reason":"end_turn"}`

// newTestClient builds a client pointed at srv with a near-zero retry delay so
// the retry tests stay fast.
func newTestClient(url string, attempts int) *providers.Client {
	return providers.NewClient("k", url, "m", 100).
		WithRetryPolicy(attempts, time.Millisecond)
}

func TestRetriesOn502ThenSucceeds(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) < 3 {
			w.WriteHeader(http.StatusBadGateway)
			fmt.Fprint(w, "<html>502 Bad Gateway</html>")
			return
		}
		fmt.Fprint(w, okResponseBody)
	}))
	defer srv.Close()

	var notices int32
	client := newTestClient(srv.URL, 5).
		WithRetryNotifier(func(attempt, maxAttempts, status int, delay time.Duration) {
			atomic.AddInt32(&notices, 1)
			if status != http.StatusBadGateway {
				t.Errorf("notifier status = %d, want 502", status)
			}
			if maxAttempts != 5 {
				t.Errorf("notifier maxAttempts = %d, want 5", maxAttempts)
			}
		})

	resp, err := client.Call("sys", nil, nil)
	if err != nil {
		t.Fatalf("Call returned error after retries: %v", err)
	}
	if resp == nil {
		t.Fatal("Call returned nil response")
	}
	if got := atomic.LoadInt32(&calls); got != 3 {
		t.Errorf("server calls = %d, want 3", got)
	}
	if got := atomic.LoadInt32(&notices); got != 2 {
		t.Errorf("retry notices = %d, want 2", got)
	}
}

func TestRetriesExhaustedSurfacesError(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	_, err := newTestClient(srv.URL, 3).Call("sys", nil, nil)
	if err == nil {
		t.Fatal("expected error after exhausting attempts")
	}
	if !strings.Contains(err.Error(), "status 502") {
		t.Errorf("error = %v, want it to mention status 502", err)
	}
	if got := atomic.LoadInt32(&calls); got != 3 {
		t.Errorf("server calls = %d, want 3 (bounded attempts)", got)
	}
}

func TestNoRetryOn4xx(t *testing.T) {
	for _, status := range []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusNotFound} {
		var calls int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&calls, 1)
			w.WriteHeader(status)
		}))

		_, err := newTestClient(srv.URL, 4).Call("sys", nil, nil)
		srv.Close()

		if err == nil {
			t.Fatalf("status %d: expected error", status)
		}
		if got := atomic.LoadInt32(&calls); got != 1 {
			t.Errorf("status %d: server calls = %d, want 1 (no retry)", status, got)
		}
	}
}

func TestHonorsRetryAfterHeader(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			w.Header().Set("Retry-After", "2")
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		fmt.Fprint(w, okResponseBody)
	}))
	defer srv.Close()

	var gotDelay time.Duration
	client := providers.NewClient("k", srv.URL, "m", 100).
		WithRetryPolicy(3, time.Millisecond).
		WithRetryNotifier(func(attempt, maxAttempts, status int, delay time.Duration) {
			gotDelay = delay
		})

	// The notifier reports the delay the client intends to honor; we only
	// assert the value so the test does not actually sleep 2s... but the
	// client does sleep, so keep the header small in future edits.
	done := make(chan error, 1)
	go func() {
		_, err := client.Call("sys", nil, nil)
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Call error: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Call did not complete")
	}

	if gotDelay != 2*time.Second {
		t.Errorf("retry delay = %v, want 2s from Retry-After header", gotDelay)
	}
}

func TestIsRetryableStatus(t *testing.T) {
	retryable := []int{408, 429, 500, 502, 503, 504, 529}
	for _, s := range retryable {
		if !providers.IsRetryableStatus(s) {
			t.Errorf("IsRetryableStatus(%d) = false, want true", s)
		}
	}
	for _, s := range []int{200, 400, 401, 403, 404, 409, 422} {
		if providers.IsRetryableStatus(s) {
			t.Errorf("IsRetryableStatus(%d) = true, want false", s)
		}
	}
}

func TestRetryDefaults(t *testing.T) {
	c := providers.NewClient("k", "http://localhost", "m", 100)
	if c.MaxAttempts() != providers.DefaultMaxAttempts {
		t.Errorf("MaxAttempts() = %d, want %d", c.MaxAttempts(), providers.DefaultMaxAttempts)
	}
	if c.RetryDelay() != providers.DefaultRetryDelay {
		t.Errorf("RetryDelay() = %v, want %v", c.RetryDelay(), providers.DefaultRetryDelay)
	}
}
