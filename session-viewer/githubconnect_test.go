package main

// Tests for the GitHub connect callback's state handling.
//
// This is the security-critical half of the flow. An unbound callback is a
// login CSRF: an attacker who completes an authorization with *their* GitHub
// account and then gets a victim to load the resulting callback URL would pin
// the attacker's token into the victim's home, after which the victim's agent
// reads and writes the attacker's repositories — and pushes to the victim's
// repositories as the attacker.
//
// Every test here asserts a *refusal*, and each one is checked to fail if the
// corresponding guard is removed.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"session-viewer/internal/auth"
	"session-viewer/internal/ghauth"

	"github.com/labstack/echo/v4"
)

// connectHarness wires up just enough server to drive the callback.
func connectHarness(t *testing.T) *echo.Echo {
	t.Helper()
	authCfg = &auth.Config{SessionKey: []byte("0123456789abcdef0123456789abcdef")}
	ghOAuth = ghauth.Config{ClientID: "cid", ClientSecret: "secret"}
	e := echo.New()
	e.GET("/auth/github/callback", finishGitHubConnect)
	return e
}

// stateCookie builds the signed cookie the start handler would have set.
func stateCookie(t *testing.T, st ghState) *http.Cookie {
	t.Helper()
	payload, err := json.Marshal(st)
	if err != nil {
		t.Fatalf("marshalling state: %v", err)
	}
	return &http.Cookie{Name: ghStateCookie, Value: authCfg.Sign(payload)}
}

// refused asserts that the callback declined *for the stated reason*.
//
// Asserting only on the status code would be vacuous, and demonstrably so:
// every one of these requests also fails later, at principal resolution, with
// the same 400. A test that accepts any refusal would keep passing with the
// state check deleted — which was verified, not assumed. Matching the specific
// message pins the refusal to the specific guard.
func refused(t *testing.T, rec *httptest.ResponseRecorder, because string) {
	t.Helper()
	if rec.Code == http.StatusFound {
		t.Fatalf("callback accepted the request (302) instead of refusing")
	}
	if !strings.Contains(rec.Body.String(), because) {
		t.Fatalf("callback refused for the wrong reason.\n  want message containing: %q\n  got body: %s",
			because, strings.TrimSpace(rec.Body.String()))
	}
}

func callback(t *testing.T, e *echo.Echo, query string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/auth/github/callback?"+query, nil)
	for _, ck := range cookies {
		req.AddCookie(ck)
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

// A callback with no state cookie at all must not proceed to an exchange.
func TestConnectCallbackRefusesWithoutStateCookie(t *testing.T) {
	e := connectHarness(t)
	rec := callback(t, e, "code=abc&state=nonce")
	refused(t, rec, "expired")
}

// A state parameter that does not match the cookie is the classic CSRF: the
// attacker cannot set the victim's cookie, so the two disagree.
func TestConnectCallbackRefusesMismatchedState(t *testing.T) {
	e := connectHarness(t)
	ck := stateCookie(t, ghState{
		Nonce: "the-real-nonce",
		Email: "user@superbuilders.school",
		Exp:   time.Now().Add(time.Minute).Unix(),
	})
	rec := callback(t, e, "code=abc&state=attacker-nonce", ck)
	refused(t, rec, "could not be verified")
}

// An empty state parameter must not be treated as matching, which it would be
// under a naive comparison against a cookie whose nonce failed to encode.
func TestConnectCallbackRefusesEmptyState(t *testing.T) {
	e := connectHarness(t)
	ck := stateCookie(t, ghState{
		Nonce: "",
		Email: "user@superbuilders.school",
		Exp:   time.Now().Add(time.Minute).Unix(),
	})
	rec := callback(t, e, "code=abc&state=", ck)
	refused(t, rec, "could not be verified")
}

// A cookie whose signature does not verify must be rejected outright, not
// parsed. Without this, the Email binding below is attacker-controlled.
func TestConnectCallbackRefusesForgedCookie(t *testing.T) {
	e := connectHarness(t)
	payload, err := json.Marshal(ghState{
		Nonce: "n", Email: "victim@superbuilders.school",
		Exp: time.Now().Add(time.Minute).Unix(),
	})
	if err != nil {
		t.Fatal(err)
	}
	// Correct shape, wrong signature.
	forged := &http.Cookie{
		Name:  ghStateCookie,
		Value: strings.Split(authCfg.Sign(payload), ".")[0] + ".AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
	}
	rec := callback(t, e, "code=abc&state=n", forged)
	refused(t, rec, "could not be verified")
}

// An expired attempt must not be resumable. Authorization codes are short
// lived, but the cookie is the thing that binds the session, and a stale one
// left in a shared browser should not still work.
func TestConnectCallbackRefusesExpiredState(t *testing.T) {
	e := connectHarness(t)
	ck := stateCookie(t, ghState{
		Nonce: "n",
		Email: "user@superbuilders.school",
		Exp:   time.Now().Add(-time.Minute).Unix(),
	})
	rec := callback(t, e, "code=abc&state=n", ck)
	refused(t, rec, "expired")
}

// The session completing the flow must be the session that started it. This
// is the guard that actually stops the CSRF: an attacker can replay a whole
// valid authorization of their own account — real code, real state, real
// signed cookie, all minted in their own browser — and the only thing that
// distinguishes it from the victim's own attempt is whose session presents it.
func TestConnectCallbackRefusesADifferentSession(t *testing.T) {
	e := connectHarness(t)
	ck := stateCookie(t, ghState{
		Nonce: "n",
		Email: "victim@superbuilders.school",
		Exp:   time.Now().Add(time.Minute).Unix(),
	})
	// The harness has no authenticated session, so the email does not match
	// the one the flow was started for.
	rec := callback(t, e, "code=abc&state=n", ck)
	refused(t, rec, "different user")
}

// GitHub reporting an error must be surfaced, not swallowed into a success.
// The org-policy-blocked case arrives this way and is otherwise
// indistinguishable from the user pressing Cancel.
func TestConnectCallbackSurfacesGitHubError(t *testing.T) {
	e := connectHarness(t)
	rec := callback(t, e, "error=access_denied&error_description=The+organization+blocked+it")
	refused(t, rec, "organization blocked it")
}

// The state that travels through GitHub must not carry the user's email.
// GitHub echoes it back in a URL that lands in server logs and Referer
// headers, and the cookie already carries the binding.
func TestConnectStateParameterCarriesNoEmail(t *testing.T) {
	cfg := ghauth.Config{ClientID: "cid", ClientSecret: "secret"}
	raw := cfg.AuthorizeURL("https://bonnie.example/cb", "opaque-nonce")
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(u.Query().Get("state"), "@") {
		t.Errorf("state parameter looks like it carries an email: %q", u.Query().Get("state"))
	}
}
