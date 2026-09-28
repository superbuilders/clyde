package ghapp

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"strings"
	"testing"
	"time"
)

func testKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	// 1024 is too weak for production and ideal for a test: key generation is
	// the slowest thing in this file and nothing here is protecting anything.
	k, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func pkcs1(t *testing.T, k *rsa.PrivateKey) string {
	t.Helper()
	return string(pem.EncodeToMemory(&pem.Block{
		Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(k),
	}))
}

func pkcs8(t *testing.T, k *rsa.PrivateKey) string {
	t.Helper()
	b, err := x509.MarshalPKCS8PrivateKey(k)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: b}))
}

// Both PEM encodings must work. GitHub's settings page emits PKCS#1; anything
// round-tripped through openssl is likely PKCS#8. Supporting one and not the
// other fails with an ASN.1 error whose cause is a button someone clicked a
// month ago.
func TestParseKeyAcceptsBothEncodings(t *testing.T) {
	k := testKey(t)
	for name, text := range map[string]string{
		"pkcs1": pkcs1(t, k),
		"pkcs8": pkcs8(t, k),
	} {
		t.Run(name, func(t *testing.T) {
			got, err := parseKey(text)
			if err != nil {
				t.Fatalf("parseKey: %v", err)
			}
			if got.N.Cmp(k.N) != 0 {
				t.Error("parsed a different key than was encoded")
			}
		})
	}
}

// A PEM pasted into a Secrets Manager JSON value usually arrives with literal
// backslash-n instead of newlines. That is the single most likely way this is
// misconfigured, and the raw failure (a base64 error) gives no hint that the
// key is intact and merely flattened.
func TestParseKeyRepairsFlattenedPEM(t *testing.T) {
	k := testKey(t)
	flat := strings.ReplaceAll(pkcs1(t, k), "\n", `\n`)
	if strings.Contains(flat, "\n") {
		t.Fatal("test fixture is not actually flattened")
	}
	got, err := parseKey(flat)
	if err != nil {
		t.Fatalf("parseKey on a flattened PEM: %v", err)
	}
	if got.N.Cmp(k.N) != 0 {
		t.Error("parsed a different key than was encoded")
	}
}

func TestParseKeyRejectsGarbage(t *testing.T) {
	for _, s := range []string{"", "not a key", "-----BEGIN RSA PRIVATE KEY-----\nzzz\n-----END RSA PRIVATE KEY-----"} {
		if _, err := parseKey(s); err == nil {
			t.Errorf("parseKey(%q) accepted a non-key", s)
		}
	}
}

// The assertion must be base64url with no padding, and iat must be in the
// past. GitHub rejects both mistakes with a bare 401 that says nothing about
// which one it was, so they are asserted here instead.
func TestAppJWTShape(t *testing.T) {
	k := testKey(t)
	now := time.Now()
	tok, err := appJWT("12345", k, now)
	if err != nil {
		t.Fatal(err)
	}

	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		t.Fatalf("jwt has %d segments, want 3", len(parts))
	}
	for i, p := range parts {
		if strings.ContainsAny(p, "+/=") {
			t.Errorf("segment %d contains standard-base64 characters; must be base64url unpadded", i)
		}
	}

	var claims struct {
		Iat int64  `json:"iat"`
		Exp int64  `json:"exp"`
		Iss string `json:"iss"`
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("claims are not base64url: %v", err)
	}
	if err := json.Unmarshal(raw, &claims); err != nil {
		t.Fatal(err)
	}
	if claims.Iss != "12345" {
		t.Errorf("iss = %q, want the app id", claims.Iss)
	}
	if claims.Iat >= now.Unix() {
		t.Error("iat must be backdated: GitHub compares it against their clock, and a box a few seconds fast fails every mint")
	}
	if d := time.Unix(claims.Exp, 0).Sub(now); d > 10*time.Minute {
		t.Errorf("exp is %v out, over GitHub's 10 minute ceiling", d)
	}
}

// Configured is the switch between minting and doing nothing. It must never
// report true on a partial config: the fallback we deliberately do not have is
// distributing a long-lived PAT, and a half-configured App that silently does
// nothing is much better than one that silently does that.
func TestConfiguredRequiresEveryField(t *testing.T) {
	full := Config{AppID: "1", InstallationID: "2", PrivateKeyPEM: "3"}
	if !full.Configured() {
		t.Fatal("a complete config must be Configured")
	}
	for name, c := range map[string]Config{
		"no app id":          {InstallationID: "2", PrivateKeyPEM: "3"},
		"no installation id": {AppID: "1", PrivateKeyPEM: "3"},
		"no key":             {AppID: "1", InstallationID: "2"},
		"empty":              {},
	} {
		if c.Configured() {
			t.Errorf("%s: reported as configured", name)
		}
	}
}
