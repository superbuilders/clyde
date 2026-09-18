package main

// Bonnie M1 — authentication gate.
//
// Design constraints (PLAN.md §4 M1, §5):
//   - `auth = none` is the default and must be byte-for-byte today's solo
//     behaviour: no middleware, no routes, no cookies, nothing.
//   - No --dev-accept-token, no header-auth bypass, no impersonation grant, in
//     any build. There is exactly one way to obtain a session: complete the
//     OIDC code flow against the configured issuer.
//   - The frontend is upstream's and is never modified (A2). Auth is enforced
//     entirely in middleware, and the *page itself* is gated, so an
//     authenticated document implies authenticated XHRs.
//
// The session cookie is an HMAC-SHA256-signed JSON payload. We do not re-use
// the provider's id_token as the session: it is short-lived, large, and
// carries claims we have no business storing in a cookie.

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/labstack/echo/v4"
	"golang.org/x/oauth2"
)

const (
	sessionCookieName = "bonnie_session"
	oauthCookieName   = "bonnie_oauth"
	sessionTTL        = 12 * time.Hour
	oauthTTL          = 10 * time.Minute
)

// authConfig is the resolved auth configuration. Mode "none" means disabled.
type authConfig struct {
	Mode         string // "none" | "oidc"
	Issuer       string
	ClientID     string
	ClientSecret string
	RedirectURL  string
	Allowed      []string // domains ("superbuilders.school") and/or exact emails
	SessionKey   []byte
	CookieSecure bool

	provider *oidc.Provider
	verifier *oidc.IDTokenVerifier
	oauth    *oauth2.Config
}

// authFlags holds raw flag values; resolved by buildAuth after flag.Parse.
type authFlags struct {
	mode         string
	issuer       string
	clientID     string
	clientSecret string
	redirectURL  string
	allowed      string
	sessionKey   string
	cookieSecure string
}

var authFlagVals authFlags

// registerAuthFlags wires auth flags. Every one defaults from the environment
// so systemd can supply them without a growing ExecStart line.
func registerAuthFlags() {
	fs := func(name, env, def, usage string) *string {
		v := def
		if e := os.Getenv(env); e != "" {
			v = e
		}
		p := new(string)
		flag.StringVar(p, name, v, usage)
		return p
	}
	// Bind into the package-level struct after parse via closures.
	mode := fs("auth", "BONNIE_AUTH", "none", "auth mode: none|oidc")
	issuer := fs("oidc-issuer", "BONNIE_OIDC_ISSUER", "", "OIDC issuer URL")
	cid := fs("oidc-client-id", "BONNIE_OIDC_CLIENT_ID", "", "OIDC client id")
	csec := fs("oidc-client-secret", "BONNIE_OIDC_CLIENT_SECRET", "", "OIDC client secret")
	redir := fs("oidc-redirect-url", "BONNIE_OIDC_REDIRECT_URL", "", "OIDC redirect URL")
	allow := fs("allowed-emails", "BONNIE_ALLOWED_EMAILS", "", "comma-separated allowed domains and/or exact emails")
	skey := fs("session-key", "BONNIE_SESSION_KEY", "", "hex-encoded session signing key (>=32 bytes)")
	sec := fs("cookie-secure", "BONNIE_COOKIE_SECURE", "auto", "Secure cookie flag: auto|true|false")

	authFlagPtrs = []*string{mode, issuer, cid, csec, redir, allow, skey, sec}
}

var authFlagPtrs []*string

func collectAuthFlags() {
	if len(authFlagPtrs) != 8 {
		return
	}
	authFlagVals = authFlags{
		mode:         *authFlagPtrs[0],
		issuer:       *authFlagPtrs[1],
		clientID:     *authFlagPtrs[2],
		clientSecret: *authFlagPtrs[3],
		redirectURL:  *authFlagPtrs[4],
		allowed:      *authFlagPtrs[5],
		sessionKey:   *authFlagPtrs[6],
		cookieSecure: *authFlagPtrs[7],
	}
}

// buildAuth validates configuration and performs OIDC discovery. It returns a
// disabled config for mode "none". Any misconfiguration in mode "oidc" is a
// hard error: we refuse to start rather than serve an unauthenticated viewer
// that was *meant* to be authenticated.
func buildAuth(ctx context.Context) (*authConfig, error) {
	collectAuthFlags()
	f := authFlagVals

	mode := strings.TrimSpace(strings.ToLower(f.mode))
	switch mode {
	case "", "none":
		return &authConfig{Mode: "none"}, nil
	case "oidc":
	default:
		return nil, fmt.Errorf("unknown --auth mode %q (want none|oidc)", f.mode)
	}

	cfg := &authConfig{
		Mode:         "oidc",
		Issuer:       strings.TrimSpace(f.issuer),
		ClientID:     strings.TrimSpace(f.clientID),
		ClientSecret: f.clientSecret,
		RedirectURL:  strings.TrimSpace(f.redirectURL),
	}
	missing := []string{}
	if cfg.Issuer == "" {
		missing = append(missing, "--oidc-issuer")
	}
	if cfg.ClientID == "" {
		missing = append(missing, "--oidc-client-id")
	}
	if cfg.ClientSecret == "" {
		missing = append(missing, "--oidc-client-secret")
	}
	if cfg.RedirectURL == "" {
		missing = append(missing, "--oidc-redirect-url")
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("auth=oidc requires %s", strings.Join(missing, ", "))
	}

	// Allowlist. Empty is refused: an OIDC-gated viewer with an empty allowlist
	// admits every identity the issuer will mint, which for a federated pool is
	// far more than intended.
	for _, raw := range strings.Split(f.allowed, ",") {
		e := strings.TrimSpace(strings.ToLower(raw))
		if e == "" {
			continue
		}
		cfg.Allowed = append(cfg.Allowed, strings.TrimPrefix(e, "@"))
	}
	if len(cfg.Allowed) == 0 {
		return nil, errors.New("auth=oidc requires a non-empty --allowed-emails")
	}

	key, err := hex.DecodeString(strings.TrimSpace(f.sessionKey))
	if err != nil {
		return nil, fmt.Errorf("--session-key must be hex: %w", err)
	}
	if len(key) < 32 {
		return nil, fmt.Errorf("--session-key must decode to >=32 bytes, got %d", len(key))
	}
	cfg.SessionKey = key

	switch strings.ToLower(strings.TrimSpace(f.cookieSecure)) {
	case "true":
		cfg.CookieSecure = true
	case "false":
		cfg.CookieSecure = false
	default: // auto: secure unless the redirect URL is plain-http localhost dev
		cfg.CookieSecure = strings.HasPrefix(cfg.RedirectURL, "https://")
	}

	// Discovery. Done at startup so a bad issuer fails fast and loudly rather
	// than on the first login attempt.
	discoCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	prov, err := oidc.NewProvider(discoCtx, cfg.Issuer)
	if err != nil {
		return nil, fmt.Errorf("oidc discovery on %s: %w", cfg.Issuer, err)
	}
	cfg.provider = prov
	cfg.verifier = prov.Verifier(&oidc.Config{ClientID: cfg.ClientID})
	cfg.oauth = &oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  cfg.RedirectURL,
		Endpoint:     prov.Endpoint(),
		Scopes:       []string{oidc.ScopeOpenID, "email", "profile"},
	}
	return cfg, nil
}

// ── signing ──

func (a *authConfig) sign(payload []byte) string {
	mac := hmac.New(sha256.New, a.SessionKey)
	mac.Write(payload)
	return base64.RawURLEncoding.EncodeToString(payload) + "." +
		base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (a *authConfig) unsign(tok string) ([]byte, error) {
	parts := strings.Split(tok, ".")
	if len(parts) != 2 {
		return nil, errors.New("malformed token")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, errors.New("malformed payload")
	}
	got, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, errors.New("malformed signature")
	}
	mac := hmac.New(sha256.New, a.SessionKey)
	mac.Write(payload)
	if subtle.ConstantTimeCompare(got, mac.Sum(nil)) != 1 {
		return nil, errors.New("bad signature")
	}
	return payload, nil
}

type sessionClaims struct {
	Email string `json:"email"`
	Sub   string `json:"sub"`
	Exp   int64  `json:"exp"`
}

type oauthState struct {
	State    string `json:"state"`
	Verifier string `json:"verifier"`
	Next     string `json:"next"`
	Exp      int64  `json:"exp"`
}

func (a *authConfig) setCookie(c echo.Context, name, value string, ttl time.Duration) {
	ck := &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		Secure:   a.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	}
	if ttl > 0 {
		ck.Expires = time.Now().Add(ttl)
		ck.MaxAge = int(ttl.Seconds())
	} else {
		ck.MaxAge = -1
	}
	c.SetCookie(ck)
}

// currentSession returns the verified session, or an error.
func (a *authConfig) currentSession(c echo.Context) (*sessionClaims, error) {
	ck, err := c.Cookie(sessionCookieName)
	if err != nil {
		return nil, errors.New("no session cookie")
	}
	payload, err := a.unsign(ck.Value)
	if err != nil {
		return nil, err
	}
	var s sessionClaims
	if err := json.Unmarshal(payload, &s); err != nil {
		return nil, errors.New("malformed session")
	}
	if time.Now().Unix() > s.Exp {
		return nil, errors.New("session expired")
	}
	return &s, nil
}

// emailAllowed matches an exact address, or the domain part against a
// bare-domain entry. Comparison is lowercase.
func (a *authConfig) emailAllowed(email string) bool {
	email = strings.ToLower(strings.TrimSpace(email))
	at := strings.LastIndex(email, "@")
	if at < 0 {
		return false
	}
	domain := email[at+1:]
	for _, e := range a.Allowed {
		if strings.Contains(e, "@") {
			if e == email {
				return true
			}
			continue
		}
		if e == domain {
			return true
		}
	}
	return false
}

// safeNext confines post-login redirects to this origin. Anything else — absolute
// URLs, protocol-relative "//evil", backslash tricks — collapses to "/".
func safeNext(raw string) string {
	if raw == "" {
		return "/"
	}
	if strings.ContainsAny(raw, "\\\r\n") {
		return "/"
	}
	if !strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "//") {
		return "/"
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "" || u.Host != "" {
		return "/"
	}
	out := u.EscapedPath()
	if out == "" {
		out = "/"
	}
	if u.RawQuery != "" {
		out += "?" + u.RawQuery
	}
	if u.Fragment != "" {
		out += "#" + u.EscapedFragment()
	}
	return out
}

// ── wiring ──

// isAPIRequest distinguishes XHR/API calls from document navigations, so the
// former get a 401 they can detect and the latter get a redirect to login.
func isAPIRequest(c echo.Context) bool {
	if strings.HasPrefix(c.Request().URL.Path, "/api/") {
		return true
	}
	return strings.Contains(c.Request().Header.Get("Accept"), "application/json")
}

// install registers auth routes and the gate. For mode "none" it does nothing
// at all — that is the point.
func (a *authConfig) install(e *echo.Echo) {
	if a.Mode == "none" {
		return
	}

	e.GET("/healthz", func(c echo.Context) error {
		return c.String(http.StatusOK, "ok\n")
	})
	e.GET("/auth/login", a.handleLogin)
	e.GET("/auth/callback", a.handleCallback)
	e.GET("/auth/logout", a.handleLogout)
	e.POST("/auth/logout", a.handleLogout)
	// Minimal, self-contained denial page. Not part of the viewer UI (A2).
	e.GET("/auth/denied", func(c echo.Context) error {
		return c.HTML(http.StatusForbidden,
			"<!doctype html><meta charset=utf-8><title>Access denied</title>"+
				"<h1>Access denied</h1><p>Your account is not permitted to use this instance.</p>"+
				"<p><a href=\"/auth/logout\">Sign out and try another account</a></p>")
	})

	e.Use(a.middleware)
}

func (a *authConfig) middleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		p := c.Request().URL.Path
		if p == "/healthz" || strings.HasPrefix(p, "/auth/") {
			return next(c)
		}
		if _, err := a.currentSession(c); err != nil {
			if isAPIRequest(c) {
				return c.JSON(http.StatusUnauthorized, map[string]string{
					"error": "unauthenticated",
					"login": "/auth/login",
				})
			}
			nxt := c.Request().URL.RequestURI()
			return c.Redirect(http.StatusFound, "/auth/login?next="+url.QueryEscape(nxt))
		}
		return next(c)
	}
}

func (a *authConfig) handleLogin(c echo.Context) error {
	// Already signed in: go straight where they were headed.
	if _, err := a.currentSession(c); err == nil {
		return c.Redirect(http.StatusFound, safeNext(c.QueryParam("next")))
	}
	verifier := oauth2.GenerateVerifier()
	stateBytes := make([]byte, 32)
	if _, err := rand.Read(stateBytes); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "entropy failure")
	}
	st := base64.RawURLEncoding.EncodeToString(stateBytes)

	payload, err := json.Marshal(oauthState{
		State:    st,
		Verifier: verifier,
		Next:     safeNext(c.QueryParam("next")),
		Exp:      time.Now().Add(oauthTTL).Unix(),
	})
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "state encode")
	}
	a.setCookie(c, oauthCookieName, a.sign(payload), oauthTTL)

	redirect := a.oauth.AuthCodeURL(st, oauth2.S256ChallengeOption(verifier))
	return c.Redirect(http.StatusFound, redirect)
}

func (a *authConfig) handleCallback(c echo.Context) error {
	// Provider-side error (e.g. access_denied) — surface it, don't render blank.
	if e := c.QueryParam("error"); e != "" {
		desc := c.QueryParam("error_description")
		fmt.Printf("[auth] event=provider_error error=%s desc=%q\n", e, desc)
		return c.HTML(http.StatusForbidden, fmt.Sprintf(
			"<!doctype html><meta charset=utf-8><title>Login failed</title><h1>Login failed</h1>"+
				"<p>The identity provider returned <code>%s</code>.</p><p>%s</p>"+
				"<p><a href=\"/auth/login\">Try again</a></p>",
			echoHTMLEscape(e), echoHTMLEscape(desc)))
	}

	ck, err := c.Cookie(oauthCookieName)
	if err != nil {
		return a.authFail(c, "missing_oauth_cookie", "Your login attempt expired. Please try again.")
	}
	payload, err := a.unsign(ck.Value)
	if err != nil {
		return a.authFail(c, "bad_oauth_cookie", "Your login attempt could not be verified. Please try again.")
	}
	var st oauthState
	if err := json.Unmarshal(payload, &st); err != nil {
		return a.authFail(c, "malformed_oauth_cookie", "Your login attempt could not be read. Please try again.")
	}
	if time.Now().Unix() > st.Exp {
		return a.authFail(c, "oauth_expired", "Your login attempt expired. Please try again.")
	}
	if subtle.ConstantTimeCompare([]byte(st.State), []byte(c.QueryParam("state"))) != 1 {
		return a.authFail(c, "state_mismatch", "Login state mismatch. Please try again.")
	}
	code := c.QueryParam("code")
	if code == "" {
		return a.authFail(c, "missing_code", "The identity provider did not return an authorization code.")
	}

	ctx, cancel := context.WithTimeout(c.Request().Context(), 20*time.Second)
	defer cancel()

	tok, err := a.oauth.Exchange(ctx, code, oauth2.VerifierOption(st.Verifier))
	if err != nil {
		fmt.Printf("[auth] event=exchange_failed err=%v\n", err)
		return a.authFail(c, "exchange_failed", "Could not complete the token exchange with the identity provider.")
	}
	rawID, ok := tok.Extra("id_token").(string)
	if !ok || rawID == "" {
		return a.authFail(c, "no_id_token", "The identity provider did not return an ID token.")
	}
	idTok, err := a.verifier.Verify(ctx, rawID)
	if err != nil {
		fmt.Printf("[auth] event=verify_failed err=%v\n", err)
		return a.authFail(c, "verify_failed", "The ID token failed verification.")
	}

	var claims struct {
		Email         string `json:"email"`
		EmailVerified any    `json:"email_verified"`
		Sub           string `json:"sub"`
	}
	if err := idTok.Claims(&claims); err != nil {
		return a.authFail(c, "claims_failed", "Could not read identity claims.")
	}
	if claims.Email == "" {
		return a.authFail(c, "no_email", "The identity provider did not supply an email address.")
	}
	// email_verified is load-bearing, not cosmetic. The shared Cognito pool
	// permits self-signup, so a domain allowlist alone would be satisfiable by
	// registering an unverified address in an allowed domain.
	if !truthy(claims.EmailVerified) {
		fmt.Printf("[auth] event=denied reason=email_unverified email=%s\n", claims.Email)
		return c.Redirect(http.StatusFound, "/auth/denied")
	}
	if !a.emailAllowed(claims.Email) {
		fmt.Printf("[auth] event=denied reason=not_allowlisted email=%s\n", claims.Email)
		return c.Redirect(http.StatusFound, "/auth/denied")
	}

	sess, err := json.Marshal(sessionClaims{
		Email: strings.ToLower(claims.Email),
		Sub:   claims.Sub,
		Exp:   time.Now().Add(sessionTTL).Unix(),
	})
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "session encode")
	}
	a.setCookie(c, sessionCookieName, a.sign(sess), sessionTTL)
	a.setCookie(c, oauthCookieName, "", 0) // clear
	fmt.Printf("[auth] event=login email=%s\n", strings.ToLower(claims.Email))
	return c.Redirect(http.StatusFound, safeNext(st.Next))
}

func (a *authConfig) handleLogout(c echo.Context) error {
	a.setCookie(c, sessionCookieName, "", 0)
	a.setCookie(c, oauthCookieName, "", 0)
	return c.Redirect(http.StatusFound, "/auth/login")
}

// authFail logs and renders a visible failure. Never a blank page: a silent
// empty render is exactly how v1 shipped a login that showed nothing.
func (a *authConfig) authFail(c echo.Context, event, human string) error {
	fmt.Printf("[auth] event=%s path=%s\n", event, c.Request().URL.Path)
	a.setCookie(c, oauthCookieName, "", 0)
	return c.HTML(http.StatusBadRequest,
		"<!doctype html><meta charset=utf-8><title>Login failed</title><h1>Login failed</h1><p>"+
			echoHTMLEscape(human)+"</p><p><a href=\"/auth/login\">Try again</a></p>")
}

func truthy(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return t == "true" || t == "1"
	default:
		return false
	}
}

func echoHTMLEscape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&#39;")
	return r.Replace(s)
}
