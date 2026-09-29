package ghauth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestAuthorizeURLCarriesScopesAndState(t *testing.T) {
	c := Config{ClientID: "cid", ClientSecret: "secret"}
	raw := c.AuthorizeURL("https://bonnie.example/auth/github/callback", "nonce-123")

	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parsing authorize URL: %v", err)
	}
	if u.Host != "github.com" || u.Path != "/login/oauth/authorize" {
		t.Errorf("authorize URL points at %s%s", u.Host, u.Path)
	}
	q := u.Query()
	if got := q.Get("state"); got != "nonce-123" {
		t.Errorf("state = %q, want nonce-123", got)
	}
	if got := q.Get("client_id"); got != "cid" {
		t.Errorf("client_id = %q", got)
	}
	// The scope set is the whole reason this works across organisations; a
	// silent narrowing of it would show up as private clones failing.
	for _, want := range []string{"repo", "read:org", "workflow"} {
		if !strings.Contains(q.Get("scope"), want) {
			t.Errorf("scope %q is missing %q", q.Get("scope"), want)
		}
	}
	// The client secret must never reach the browser.
	if strings.Contains(raw, "secret") {
		t.Errorf("authorize URL leaks the client secret: %s", raw)
	}
}

// fakeGitHub stands in for both github.com and api.github.com.
func fakeGitHub(t *testing.T, tokenBody, userBody string, tokenStatus int) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/login/oauth/access_token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(tokenStatus)
		_, _ = w.Write([]byte(tokenBody))
	})
	mux.HandleFunc("/user", func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer tok-abc" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(userBody))
	})
	return httptest.NewServer(mux)
}

// TestExchangeRejectsErrorDocumentWith200 is the case that motivated parsing
// the body at all. GitHub answers a bad or reused code with HTTP 200 and an
// error document; a handler that trusted the status code would store an empty
// string as a credential and only find out when a clone failed.
func TestExchangeRejectsErrorDocumentWith200(t *testing.T) {
	srv := fakeGitHub(t,
		`{"error":"bad_verification_code","error_description":"The code passed is incorrect or expired."}`,
		`{"login":"nobody"}`, http.StatusOK)
	defer srv.Close()

	c := Config{ClientID: "cid", ClientSecret: "secret", tokenURL: srv.URL + "/login/oauth/access_token"}
	_, err := c.Exchange(context.Background(), "https://bonnie.example/cb", "code")
	if err == nil {
		t.Fatal("Exchange accepted an error document returned with HTTP 200")
	}
	if !strings.Contains(err.Error(), "incorrect or expired") {
		t.Errorf("error does not relay GitHub's description: %v", err)
	}
}

// TestExchangeRejectsEmptyToken covers the other shape of the same trap: a 200
// with neither an error nor a token.
func TestExchangeRejectsEmptyToken(t *testing.T) {
	srv := fakeGitHub(t, `{"scope":"repo"}`, `{"login":"nobody"}`, http.StatusOK)
	defer srv.Close()

	c := Config{ClientID: "cid", ClientSecret: "secret", tokenURL: srv.URL + "/login/oauth/access_token"}
	if _, err := c.Exchange(context.Background(), "https://bonnie.example/cb", "code"); err == nil {
		t.Fatal("Exchange accepted a response with no access token")
	}
}

func TestExchangeReturnsTokenAndLogin(t *testing.T) {
	srv := fakeGitHub(t, `{"access_token":"tok-abc","scope":"repo,read:org"}`,
		`{"login":"octocat"}`, http.StatusOK)
	defer srv.Close()

	c := Config{
		ClientID:     "cid",
		ClientSecret: "secret",
		tokenURL:     srv.URL + "/login/oauth/access_token",
		userURL:      srv.URL + "/user",
	}
	id, err := c.Exchange(context.Background(), "https://bonnie.example/cb", "code")
	if err != nil {
		t.Fatalf("Exchange: %v", err)
	}
	if id.Token != "tok-abc" {
		t.Errorf("token = %q", id.Token)
	}
	// The login must come from GitHub, not be guessed: git's credential store
	// matches on username, and a wrong one silently fails to match.
	if id.Login != "octocat" {
		t.Errorf("login = %q, want octocat", id.Login)
	}
}

func TestConfigured(t *testing.T) {
	for _, tc := range []struct {
		name string
		cfg  Config
		want bool
	}{
		{"both", Config{ClientID: "a", ClientSecret: "b"}, true},
		{"no secret", Config{ClientID: "a"}, false},
		{"no id", Config{ClientSecret: "b"}, false},
		{"empty", Config{}, false},
	} {
		if got := tc.cfg.Configured(); got != tc.want {
			t.Errorf("%s: Configured() = %v, want %v", tc.name, got, tc.want)
		}
	}
}
