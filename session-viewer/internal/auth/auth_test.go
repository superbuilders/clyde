package auth

import (
	"encoding/hex"
	"strings"
	"testing"
)

func testAuth(t *testing.T, allowed ...string) *Config {
	t.Helper()
	key, err := hex.DecodeString(strings.Repeat("ab", 32))
	if err != nil {
		t.Fatal(err)
	}
	return &Config{Mode: "oidc", SessionKey: key, Allowed: allowed}
}

func TestSafeNextBlocksOpenRedirect(t *testing.T) {
	// Anything that could leave this origin must collapse to "/".
	hostile := []string{
		"https://evil.example/",
		"http://evil.example/",
		"//evil.example/",
		"///evil.example",
		"\\\\evil.example",
		"/\\evil.example",
		"javascript:alert(1)",
		"/path\r\nLocation: https://evil.example",
		"/path\nSet-Cookie: x=1",
		"",
	}
	for _, in := range hostile {
		if got := safeNext(in); got != "/" {
			t.Errorf("safeNext(%q) = %q, want \"/\"", in, got)
		}
	}
}

func TestSafeNextPreservesLocalPaths(t *testing.T) {
	cases := map[string]string{
		"/":                        "/",
		"/sessions":                "/sessions",
		"/sessions?id=abc":         "/sessions?id=abc",
		"/sessions?id=abc&tab=raw": "/sessions?id=abc&tab=raw",
		"/a/b/c":                   "/a/b/c",
	}
	for in, want := range cases {
		if got := safeNext(in); got != want {
			t.Errorf("safeNext(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestEmailAllowlist(t *testing.T) {
	a := testAuth(t, "superbuilders.school", "special@elsewhere.org")

	allow := []string{
		"anthony.beckner@superbuilders.school",
		"ANTHONY.BECKNER@SUPERBUILDERS.SCHOOL",
		"bonnie-e2e@superbuilders.school",
		"special@elsewhere.org",
	}
	for _, e := range allow {
		if !a.emailAllowed(e) {
			t.Errorf("emailAllowed(%q) = false, want true", e)
		}
	}

	deny := []string{
		"attacker@gmail.com",
		"attacker@notsuperbuilders.school",
		// substring / suffix confusion must not pass
		"attacker@evil-superbuilders.school",
		"attacker@superbuilders.school.evil.com",
		"other@elsewhere.org",
		"no-at-sign",
		"",
		// domain-only entry must not admit the bare domain as an address
		"superbuilders.school",
	}
	for _, e := range deny {
		if a.emailAllowed(e) {
			t.Errorf("emailAllowed(%q) = true, want false", e)
		}
	}
}

// TestDeployedAllowlistM2 pins the exact allowlist the bonnie-dev box runs with.
//
// The Cognito pool backing it (us-east-1_3uhuoRM3R) is a shared production pool
// holding thousands of federated identities from several schools, including
// other people at superbuilders.school. So M2 uses exact addresses, not a bare
// domain, and this test fails if anyone widens it back to a domain.
func TestDeployedAllowlistM2(t *testing.T) {
	a := testAuth(t,
		"anthony.beckner@superbuilders.school",
		"bonnie-e2e@superbuilders.school",
	)

	for _, e := range []string{
		"anthony.beckner@superbuilders.school",
		"bonnie-e2e@superbuilders.school",
	} {
		if !a.emailAllowed(e) {
			t.Errorf("deployed allowlist rejects %q, want allowed", e)
		}
	}

	for _, e := range []string{
		// A real, different person in the same pool. This is the case a bare
		// "superbuilders.school" entry would wrongly admit.
		"anthony.harley@superbuilders.school",
		// Other identities that exist in the pool and are not AJ.
		"ajbeckner@gmail.com",
		"ajbecknerapps@gmail.com",
		// Near-misses on the allowed addresses.
		"anthony.beckner@superbuilers.school",
		"anthony.beckner@superbuilders.school.evil.com",
		"superbuilders.school",
	} {
		if a.emailAllowed(e) {
			t.Errorf("deployed allowlist admits %q, want denied", e)
		}
	}
}

// TestEmailVerifiedFederated covers the Google-federation case that denied AJ.
//
// The shared pool maps Google's `email` but not `email_verified`, so a real
// Google login arrives with the claim absent and an `identities` entry instead.
func TestEmailVerifiedFederated(t *testing.T) {
	a := testAuth(t, "anthony.beckner@superbuilders.school")
	a.TrustedProviders = []string{"Google"}

	googleIdentities := []any{
		map[string]any{"providerName": "Google", "userId": "1234"},
	}

	// The exact shape that was being denied: no claim, Google identity.
	if !a.emailVerified(nil, googleIdentities) {
		t.Error("Google-federated login with absent email_verified was denied")
	}
	// Cognito has also shipped `identities` as a JSON string.
	raw := `[{"providerName":"Google","userId":"1234"}]`
	if !a.emailVerified(nil, raw) {
		t.Error("string-encoded identities claim was not parsed")
	}
	// A native verified user still passes, in both bool and string forms.
	if !a.emailVerified(true, nil) {
		t.Error("email_verified=true was denied")
	}
	if !a.emailVerified("true", nil) {
		t.Error(`email_verified="true" was denied`)
	}

	// --- the security property: unverified and untrusted must still fail ---

	// A Cognito-native self-signup with an allowlisted address it does not own.
	// This is the attack email_verified exists to stop, so it must still fail.
	if a.emailVerified(false, nil) {
		t.Error("unverified native user was admitted")
	}
	if a.emailVerified(nil, nil) {
		t.Error("user with no claim and no identities was admitted")
	}
	// Clever is wired into the same pool but is not a trusted provider.
	clever := []any{map[string]any{"providerName": "Clever", "userId": "x"}}
	if a.emailVerified(nil, clever) {
		t.Error("untrusted provider Clever was admitted")
	}
	// Malformed identities must not become a bypass.
	for _, bad := range []any{"not json", `{"providerName":"Google"}`, []any{"Google"}, 42} {
		if a.emailVerified(nil, bad) {
			t.Errorf("malformed identities %#v was admitted", bad)
		}
	}
	// With no trusted providers configured, federation grants nothing.
	a.TrustedProviders = nil
	if a.emailVerified(nil, googleIdentities) {
		t.Error("federation was trusted with an empty TrustedProviders list")
	}
}

func TestSignUnsignRoundTrip(t *testing.T) {
	a := testAuth(t, "example.com")
	payload := []byte(`{"email":"x@example.com","exp":123}`)
	tok := a.sign(payload)
	got, err := a.unsign(tok)
	if err != nil {
		t.Fatalf("unsign: %v", err)
	}
	if string(got) != string(payload) {
		t.Fatalf("round trip = %q, want %q", got, payload)
	}
}

func TestUnsignRejectsTampering(t *testing.T) {
	a := testAuth(t, "example.com")
	tok := a.sign([]byte(`{"email":"user@example.com"}`))

	bad := []string{
		"",
		"garbage",
		"only-one-part",
		tok + "x",                              // corrupt signature
		strings.SplitN(tok, ".", 2)[0] + ".AA", // wrong signature
	}
	for _, b := range bad {
		if _, err := a.unsign(b); err == nil {
			t.Errorf("unsign(%q) succeeded, want error", b)
		}
	}

	// A payload swapped for a different identity, keeping the old signature,
	// must not verify.
	forged := strings.SplitN(a.sign([]byte(`{"email":"attacker@example.com"}`)), ".", 2)[0] +
		"." + strings.SplitN(tok, ".", 2)[1]
	if _, err := a.unsign(forged); err == nil {
		t.Error("forged payload verified, want error")
	}
}

func TestUnsignRejectsForeignKey(t *testing.T) {
	a := testAuth(t, "example.com")
	other := testAuth(t, "example.com")
	k, _ := hex.DecodeString(strings.Repeat("cd", 32))
	other.SessionKey = k

	tok := other.sign([]byte(`{"email":"user@example.com"}`))
	if _, err := a.unsign(tok); err == nil {
		t.Error("token signed with a different key verified, want error")
	}
}

func TestTruthy(t *testing.T) {
	for _, v := range []any{true, "true", "1"} {
		if !truthy(v) {
			t.Errorf("truthy(%#v) = false, want true", v)
		}
	}
	// email_verified must be strictly true; absent/false/odd types deny.
	for _, v := range []any{false, "false", "", nil, 0, 1, "yes"} {
		if truthy(v) {
			t.Errorf("truthy(%#v) = true, want false", v)
		}
	}
}

func TestBuildAuthDefaultsToNone(t *testing.T) {
	authFlagPtrs = nil
	authFlagVals = authFlags{}
	cfg, err := Build(t.Context())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if cfg.Mode != "none" {
		t.Fatalf("default mode = %q, want none", cfg.Mode)
	}
}

func TestBuildAuthRefusesBadConfig(t *testing.T) {
	cases := map[string]authFlags{
		"unknown mode":   {mode: "basic"},
		"missing issuer": {mode: "oidc", clientID: "c", clientSecret: "s", redirectURL: "https://x/cb", allowed: "d.com", sessionKey: strings.Repeat("ab", 32)},
		"empty allowlist": {mode: "oidc", issuer: "https://issuer.invalid", clientID: "c", clientSecret: "s",
			redirectURL: "https://x/cb", allowed: "", sessionKey: strings.Repeat("ab", 32)},
		"short key": {mode: "oidc", issuer: "https://issuer.invalid", clientID: "c", clientSecret: "s",
			redirectURL: "https://x/cb", allowed: "d.com", sessionKey: "abcd"},
		"non-hex key": {mode: "oidc", issuer: "https://issuer.invalid", clientID: "c", clientSecret: "s",
			redirectURL: "https://x/cb", allowed: "d.com", sessionKey: "zzzz"},
	}
	for name, f := range cases {
		t.Run(name, func(t *testing.T) {
			authFlagPtrs = nil
			authFlagVals = f
			if _, err := Build(t.Context()); err == nil {
				t.Error("Build succeeded, want error")
			}
		})
	}
}
