// Package ghauth connects a Bonnie user to their own GitHub account.
//
// PLAN.md §M6, second attempt. The first one minted a GitHub App installation
// token, which is scoped to an installation rather than to a person: every
// Bonnie user got identical access to whatever the App was installed on, and
// extending that to a second or third organisation meant installing the App
// there too. That is Bonnie deciding who can reach which repository, which is
// a decision GitHub already makes, correctly, and keeps up to date.
//
// So instead each user authorises Bonnie against their own account once, and
// the resulting token carries exactly their access — every repository, in every
// organisation, including ones Bonnie has never heard of, and nothing they were
// not already entitled to. When someone is removed from an org on GitHub, their
// agent loses that org. Bonnie maintains no list and cannot get one wrong.
//
// A classic OAuth App rather than a GitHub App. This is the one place the older
// mechanism is the right one: a GitHub App's user-to-server token is limited to
// the intersection of the user's access and the App's installations, so it
// cannot see an organisation nobody has installed the App on. The whole point
// here is not to enumerate organisations, which rules it out.
//
// The cost, stated plainly because it is a real regression from the installation
// token: a classic OAuth token does not expire. The mitigation is that it is the
// user's own credential with the user's own permissions, stored 0600 inside
// their own 0750 home — so the blast radius of a leak is exactly the access that
// user already had, rather than a shared org-wide identity nobody owns. Revoking
// one is the user's own settings page, not an operator task.
package ghauth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Scopes requested at authorisation.
//
//   - repo gives read/write on private repositories the user can already reach.
//     There is no read-only variant in the classic scope model: it is
//     all-or-nothing, and cloning a private repo requires it.
//   - read:org lets `gh` enumerate the user's organisations, which is what makes
//     `gh repo list <org>` work rather than fail confusingly.
//   - workflow is needed to push any commit that touches .github/workflows,
//     which is otherwise rejected at push time with an error that does not
//     mention scopes at all.
const Scopes = "repo read:org workflow"

// Config is the OAuth App registration, as stored in Secrets Manager.
type Config struct {
	ClientID     string `json:"GITHUB_OAUTH_CLIENT_ID"`
	ClientSecret string `json:"GITHUB_OAUTH_CLIENT_SECRET"`

	// Endpoint overrides, for tests only. Unexported so no caller outside this
	// package can point the flow at a host that is not GitHub — which would be
	// a way to exfiltrate the client secret.
	tokenURL string
	userURL  string
}

func (c Config) tokenEndpoint() string {
	if c.tokenURL != "" {
		return c.tokenURL
	}
	return "https://github.com/login/oauth/access_token"
}

func (c Config) userEndpoint() string {
	if c.userURL != "" {
		return c.userURL
	}
	return "https://api.github.com/user"
}

// Configured reports whether a connect flow can be attempted at all.
func (c Config) Configured() bool {
	return c.ClientID != "" && c.ClientSecret != ""
}

// AuthorizeURL is where the browser is sent to ask the user for consent.
//
// state is opaque here on purpose: binding it to the caller's session is the
// caller's job, and this package has no business knowing how Bonnie signs
// things.
func (c Config) AuthorizeURL(redirectURL, state string) string {
	q := url.Values{
		"client_id":    {c.ClientID},
		"redirect_uri": {redirectURL},
		"scope":        {Scopes},
		"state":        {state},
	}
	return "https://github.com/login/oauth/authorize?" + q.Encode()
}

// Identity is a connected account: the token and who it belongs to.
type Identity struct {
	Token string
	Login string
}

// Exchange turns an authorisation code into a token, and then asks GitHub who
// that token belongs to.
//
// The second call is not optional bookkeeping. git's credential store matches
// on username and `gh` records one in its hosts file; deriving it from the
// Bonnie email would be wrong for anyone whose GitHub handle differs from their
// work address, which is most people. Asking costs one request, once.
func (c Config) Exchange(ctx context.Context, redirectURL, code string) (Identity, error) {
	form := url.Values{
		"client_id":     {c.ClientID},
		"client_secret": {c.ClientSecret},
		"code":          {code},
		"redirect_uri":  {redirectURL},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.tokenEndpoint(), strings.NewReader(form.Encode()))
	if err != nil {
		return Identity{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := client().Do(req)
	if err != nil {
		return Identity{}, fmt.Errorf("exchanging the authorization code: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if err != nil {
		return Identity{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return Identity{}, fmt.Errorf("github returned %s exchanging the code", resp.Status)
	}

	// GitHub answers a *failed* exchange with 200 and an error document. Taking
	// the status code as the verdict would hand an empty string onward as though
	// it were a credential, and the first sign of trouble would be a clone
	// failing inside an agent much later.
	var parsed struct {
		AccessToken      string `json:"access_token"`
		Scope            string `json:"scope"`
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return Identity{}, fmt.Errorf("github's response was not JSON: %w", err)
	}
	if parsed.Error != "" {
		return Identity{}, fmt.Errorf("github refused the code: %s (%s)",
			parsed.ErrorDescription, parsed.Error)
	}
	if parsed.AccessToken == "" {
		return Identity{}, fmt.Errorf("github returned no access token")
	}

	login, err := c.login(ctx, parsed.AccessToken)
	if err != nil {
		return Identity{}, err
	}
	return Identity{Token: parsed.AccessToken, Login: login}, nil
}

// login asks which account a token belongs to.
func (c Config) login(ctx context.Context, token string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.userEndpoint(), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := client().Do(req)
	if err != nil {
		return "", fmt.Errorf("identifying the token: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("github returned %s identifying the token", resp.Status)
	}
	var who struct {
		Login string `json:"login"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<16)).Decode(&who); err != nil {
		return "", err
	}
	if who.Login == "" {
		return "", fmt.Errorf("github reported no login for the token")
	}
	return who.Login, nil
}

// client is a fresh client with a timeout rather than http.DefaultClient, which
// has none: a hung GitHub would otherwise pin a request goroutine indefinitely.
func client() *http.Client {
	return &http.Client{Timeout: 30 * time.Second}
}
