// Package ghapp mints GitHub App installation tokens.
//
// PLAN.md §M6. Bonnie needs to give each user's agent access to the org's
// repositories, and the credential it hands out is the thing that decides how
// bad a bad day is. An installation token expires in an hour; a personal
// access token expires when someone remembers to revoke it. That difference is
// the entire reason this package exists.
//
// The flow GitHub specifies is two steps, and both are here:
//
//  1. Sign a short-lived JWT with the App's private key. This authenticates
//     *the App*, and can do almost nothing by itself.
//  2. Exchange it for an installation token, which authenticates the App *as
//     installed on one account* and is what git actually uses.
//
// Implemented on the standard library rather than by adding a JWT dependency.
// The signing is forty lines of crypto/rsa and encoding/json, the budget in
// §4 is tight, and a JWT library's value is in the parsing and validation
// directions — neither of which we do. We only ever sign, with one algorithm,
// over a payload we construct ourselves.
package ghapp

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Config is everything needed to mint a token, as stored in Secrets Manager.
//
// InstallationID is required rather than discovered. It can be looked up from
// the App JWT, but that is one more network call on a value that never changes
// for a given deployment, and a wrong installation is a misconfiguration we
// want to fail loudly at boot rather than silently paper over by picking the
// first one GitHub returns.
type Config struct {
	AppID          string `json:"GITHUB_APP_ID"`
	InstallationID string `json:"GITHUB_APP_INSTALLATION_ID"`
	PrivateKeyPEM  string `json:"GITHUB_APP_PRIVATE_KEY"`
}

// Configured reports whether all three fields are present.
//
// Used to decide between minting and doing nothing at all. There is
// deliberately no fallback to the personal access token the box used before:
// distributing a long-lived org credential into every user's home would be new
// exposure, silently, in the name of convenience. If the App is not configured
// the right outcome is that user git does not work and says so.
func (c Config) Configured() bool {
	return c.AppID != "" && c.InstallationID != "" && c.PrivateKeyPEM != ""
}

// Validate reports which fields are absent, by name.
//
// Configured answers yes-or-no for control flow; this answers "why not" for a
// human staring at a secret they just edited. Listing every missing field at
// once matters more than it looks: fixing them one 401 at a time means one
// round trip through the AWS console per field.
func (c Config) Validate() error {
	var missing []string
	for _, f := range []struct {
		name, value string
	}{
		{"GITHUB_APP_ID", c.AppID},
		{"GITHUB_APP_INSTALLATION_ID", c.InstallationID},
		{"GITHUB_APP_PRIVATE_KEY", c.PrivateKeyPEM},
	} {
		if strings.TrimSpace(f.value) == "" {
			missing = append(missing, f.name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("the secret is missing %s", strings.Join(missing, ", "))
	}
	return nil
}

// Token is an installation token and the moment it stops working.
type Token struct {
	Value     string
	ExpiresAt time.Time
}

// appJWT builds the assertion that authenticates the App itself.
//
// iat is backdated sixty seconds. GitHub rejects a JWT whose iat is in the
// future, and it compares against *their* clock: a box whose time is a few
// seconds fast fails every mint with a 401 that says nothing about clocks.
// This is GitHub's own documented advice and it costs nothing.
//
// exp is nine minutes out, under GitHub's ten-minute ceiling. The token lives
// for one HTTP call, so the margin exists only to absorb the backdating.
func appJWT(appID string, key *rsa.PrivateKey, now time.Time) (string, error) {
	header := map[string]string{"alg": "RS256", "typ": "JWT"}
	claims := map[string]any{
		"iat": now.Add(-60 * time.Second).Unix(),
		"exp": now.Add(9 * time.Minute).Unix(),
		"iss": appID,
	}

	seg := func(v any) (string, error) {
		b, err := json.Marshal(v)
		if err != nil {
			return "", err
		}
		// RawURLEncoding: JWT is base64url with no padding. StdEncoding
		// produces '+' and '/', which are not valid in the token and which
		// GitHub rejects with an unhelpful 401.
		return base64.RawURLEncoding.EncodeToString(b), nil
	}

	h, err := seg(header)
	if err != nil {
		return "", err
	}
	c, err := seg(claims)
	if err != nil {
		return "", err
	}

	signing := h + "." + c
	digest := sha256.Sum256([]byte(signing))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		return "", fmt.Errorf("signing app jwt: %w", err)
	}
	return signing + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

// parseKey accepts either of the two PEM encodings GitHub has handed out.
//
// The App settings page downloads PKCS#1 ("BEGIN RSA PRIVATE KEY"). Anything
// that has been round-tripped through openssl is likely to be PKCS#8 ("BEGIN
// PRIVATE KEY"). Accepting only one produces a failure whose message is about
// ASN.1 and whose cause is which button someone clicked a month ago.
func parseKey(pemText string) (*rsa.PrivateKey, error) {
	// Secrets Manager values are JSON strings, and a PEM pasted into one
	// usually arrives with literal backslash-n rather than newlines. Repair it
	// rather than rejecting it: the alternative is a base64 error that gives
	// no hint that the key is intact and merely flattened.
	if !strings.Contains(pemText, "\n") && strings.Contains(pemText, `\n`) {
		pemText = strings.ReplaceAll(pemText, `\n`, "\n")
	}

	block, _ := pem.Decode([]byte(strings.TrimSpace(pemText)))
	if block == nil {
		return nil, fmt.Errorf("private key is not PEM (expected a BEGIN line)")
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("private key is neither PKCS#1 nor PKCS#8: %w", err)
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("private key is %T, want RSA", parsed)
	}
	return key, nil
}

// Mint exchanges the App's private key for an installation token.
func Mint(cfg Config) (Token, error) {
	if !cfg.Configured() {
		return Token{}, fmt.Errorf("github app is not configured")
	}
	key, err := parseKey(cfg.PrivateKeyPEM)
	if err != nil {
		return Token{}, err
	}
	assertion, err := appJWT(cfg.AppID, key, time.Now())
	if err != nil {
		return Token{}, err
	}

	url := "https://api.github.com/app/installations/" + cfg.InstallationID + "/access_tokens"
	req, err := http.NewRequest(http.MethodPost, url, nil)
	if err != nil {
		return Token{}, err
	}
	req.Header.Set("Authorization", "Bearer "+assertion)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return Token{}, fmt.Errorf("requesting installation token: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode != http.StatusCreated {
		// The body is GitHub's error message and is the only thing that
		// distinguishes a bad key from a wrong installation id. It contains no
		// secret — the assertion went in a header, not the body.
		return Token{}, fmt.Errorf("installation token: HTTP %d: %s",
			resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var out struct {
		Token     string    `json:"token"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return Token{}, fmt.Errorf("decoding installation token: %w", err)
	}
	if out.Token == "" {
		return Token{}, fmt.Errorf("installation token response contained no token")
	}
	return Token{Value: out.Token, ExpiresAt: out.ExpiresAt}, nil
}
