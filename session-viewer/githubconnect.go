package main

// GitHub connect — give each user's agent the user's own GitHub access.
//
// PLAN.md §M6. The user clicks Connect GitHub, authorises Bonnie on their own
// account, and the token that comes back is written into their Unix home so
// that both `git` and `gh` work inside their agent sessions. Nothing about
// which repositories or which organisations is configured anywhere: the token
// carries the user's access, so GitHub remains the only thing that decides it.
//
// Two mechanics worth stating, because both were tempting to do the easy way.
//
// *Written as the user, never as root.* Same rule as MkdirAs and grantShare.
// The token goes over stdin rather than argv, because argv is world-readable in
// /proc for as long as the process lives, and a credential in argv would be
// readable by every other Bonnie user on the box.
//
// *State is signed and bound to the session.* An unbound callback is a login
// CSRF: an attacker who gets a victim to load a crafted /auth/github/callback
// pins the attacker's GitHub token into the victim's home, and thereafter the
// victim's agent pushes as the attacker. The state cookie carries the email the
// flow started for, and the callback refuses if it no longer matches.

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"session-viewer/internal/auth"
	"session-viewer/internal/ghauth"
	"session-viewer/internal/principal"

	"github.com/labstack/echo/v4"
)

const (
	// gitCredentialsFile is where git's `store` helper looks. Matching the
	// conventional name means the helper needs no configuration beyond being
	// switched on.
	gitCredentialsFile = ".git-credentials"

	// ghHostsFile is where the gh CLI keeps its token. Writing it directly
	// rather than driving `gh auth login` is deliberate: the interactive flow
	// wants a TTY and a browser, and `gh auth login --with-token` still needs
	// gh installed at the moment of connection. A file is inspectable, has no
	// failure modes we do not control, and is what gh itself writes.
	ghHostsFile = ".config/gh/hosts.yml"

	ghStateCookie = "bonnie_gh_state"
	ghStateTTL    = 10 * time.Minute
)

// ghOAuth is the OAuth App registration, loaded once at startup.
var ghOAuth ghauth.Config

// authCfg is the live auth configuration. The connect flow needs it to sign
// state and to name the user the flow began for, and both must be the same
// configuration the rest of the server is using.
var authCfg *auth.Config

// ghState is what the signed state cookie carries.
type ghState struct {
	Nonce string `json:"nonce"`
	Email string `json:"email"`
	Exp   int64  `json:"exp"`
}

// githubRedirectURL derives the callback from the request.
//
// Derived rather than configured because it must match GitHub's registration
// byte for byte, and a second place to write the hostname is a second place for
// it to drift. X-Forwarded-Proto is honoured because the box sits behind a load
// balancer that terminates TLS, so the scheme Echo sees is http even though the
// registered callback is https.
func githubRedirectURL(c echo.Context) string {
	scheme := "https"
	if fwd := c.Request().Header.Get("X-Forwarded-Proto"); fwd != "" {
		scheme = fwd
	} else if c.Request().TLS == nil {
		scheme = "http"
	}
	return scheme + "://" + c.Request().Host + "/auth/github/callback"
}

// getGitHubStatus reports whether the caller has connected their account.
//
// Reads the file rather than remembering the last connect: the file is the
// thing that makes git work, so anything else can disagree with reality. It
// reports the login, never the token.
func getGitHubStatus(c echo.Context) error {
	type status struct {
		Configured bool   `json:"configured"`
		Connected  bool   `json:"connected"`
		Login      string `json:"login,omitempty"`
	}
	if !ghOAuth.Configured() {
		return c.JSON(http.StatusOK, status{})
	}
	pr, err := principalFor(c)
	if err != nil {
		return c.JSON(http.StatusOK, status{Configured: true})
	}
	login := connectedLogin(pr)
	return c.JSON(http.StatusOK, status{
		Configured: true,
		Connected:  login != "",
		Login:      login,
	})
}

// connectedLogin returns the GitHub login recorded for a user, or "".
func connectedLogin(pr *principal.Principal) string {
	out, err := pr.Command("cat", filepath.Join(pr.Home, gitCredentialsFile)).Output()
	if err != nil {
		return ""
	}
	// https://<login>:<token>@github.com — parse it rather than splitting on
	// punctuation, since a token is opaque and could contain anything.
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		u, err := url.Parse(strings.TrimSpace(line))
		if err != nil || u.Host != "github.com" || u.User == nil {
			continue
		}
		if name := u.User.Username(); name != "" && name != "x-access-token" {
			return name
		}
	}
	return ""
}

// startGitHubConnect sends the user to GitHub for consent.
func startGitHubConnect(c echo.Context) error {
	if !ghOAuth.Configured() {
		return c.String(http.StatusServiceUnavailable,
			"GitHub connect is not configured on this server.")
	}
	// Resolved before redirecting: a user with no Unix account has nowhere to
	// put a token, and discovering that after they have authorised means asking
	// them to do it twice.
	pr, err := principalFor(c)
	if err != nil {
		return c.String(http.StatusForbidden,
			"No Unix account for this login, so there is nowhere to store a credential.")
	}
	if pr.Solo {
		return c.String(http.StatusBadRequest,
			"GitHub connect applies to multi-user deployments; run `gh auth login` instead.")
	}

	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return c.String(http.StatusInternalServerError, "could not generate state")
	}
	st := ghState{
		Nonce: base64.RawURLEncoding.EncodeToString(nonce),
		Email: auth.EmailFrom(c),
		Exp:   time.Now().Add(ghStateTTL).Unix(),
	}
	payload, err := json.Marshal(st)
	if err != nil {
		return c.String(http.StatusInternalServerError, "could not encode state")
	}
	signed := authCfg.Sign(payload)
	authCfg.SetTempCookie(c, ghStateCookie, signed, ghStateTTL)

	// The nonce alone travels as `state`; the cookie holds the whole claim.
	// GitHub echoes state back in a URL that lands in logs and Referer headers,
	// and there is no reason for the user's email to be in either.
	return c.Redirect(http.StatusFound,
		ghOAuth.AuthorizeURL(githubRedirectURL(c), st.Nonce))
}

// finishGitHubConnect exchanges the code and installs the credential.
func finishGitHubConnect(c echo.Context) error {
	if !ghOAuth.Configured() {
		return c.String(http.StatusServiceUnavailable, "GitHub connect is not configured.")
	}
	if e := c.QueryParam("error"); e != "" {
		// User pressed Cancel, or an org policy blocked the App. Surfacing
		// GitHub's own description matters for the second case, which is
		// otherwise indistinguishable from the first.
		return ghConnectFail(c, fmt.Sprintf("GitHub declined: %s",
			firstNonEmpty(c.QueryParam("error_description"), e)))
	}

	ck, err := c.Cookie(ghStateCookie)
	if err != nil {
		return ghConnectFail(c, "This connect attempt expired. Please try again.")
	}
	authCfg.ClearCookie(c, ghStateCookie)

	payload, err := authCfg.Unsign(ck.Value)
	if err != nil {
		return ghConnectFail(c, "This connect attempt could not be verified. Please try again.")
	}
	var st ghState
	if err := json.Unmarshal(payload, &st); err != nil {
		return ghConnectFail(c, "This connect attempt could not be read. Please try again.")
	}
	if time.Now().Unix() > st.Exp {
		return ghConnectFail(c, "This connect attempt expired. Please try again.")
	}
	if got := c.QueryParam("state"); got == "" || got != st.Nonce {
		return ghConnectFail(c, "This connect attempt could not be verified. Please try again.")
	}
	// The session must still be the one that started the flow. Without this the
	// callback would install whichever GitHub account completed the flow into
	// whichever Bonnie account happens to be logged in now.
	if email := auth.EmailFrom(c); email == "" || email != st.Email {
		return ghConnectFail(c, "You are signed in as a different user than when this started.")
	}

	pr, err := principalFor(c)
	if err != nil {
		return ghConnectFail(c, "No Unix account for this login.")
	}

	id, err := ghOAuth.Exchange(c.Request().Context(), githubRedirectURL(c), c.QueryParam("code"))
	if err != nil {
		return ghConnectFail(c, fmt.Sprintf("Could not complete the connection: %v", err))
	}
	if err := installGitHubCredential(pr, id); err != nil {
		return ghConnectFail(c, fmt.Sprintf("Could not store the credential: %v", err))
	}
	return c.Redirect(http.StatusFound, "/?github=connected")
}

// disconnectGitHub removes the credential from the caller's home.
//
// Local only: it deliberately does not revoke the token at GitHub, because
// doing so requires the App's client secret and would revoke the grant for the
// *account*, which the user may also be using elsewhere. Revocation lives on
// the user's own GitHub settings page, and the message says so.
func disconnectGitHub(c echo.Context) error {
	pr, err := principalFor(c)
	if err != nil {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "no unix account"})
	}
	for _, rel := range []string{gitCredentialsFile, ghHostsFile} {
		if out, err := pr.Command("rm", "-f", filepath.Join(pr.Home, rel)).CombinedOutput(); err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{
				"error": fmt.Sprintf("removing %s: %v: %s", rel, err, strings.TrimSpace(string(out))),
			})
		}
	}
	return c.JSON(http.StatusOK, map[string]string{
		"status": "disconnected",
		"note":   "To revoke the authorization itself, visit github.com/settings/applications",
	})
}

// installGitHubCredential writes the token into one user's home, as that user.
func installGitHubCredential(pr *principal.Principal, id ghauth.Identity) error {
	credPath := filepath.Join(pr.Home, gitCredentialsFile)

	// `tee` rather than a shell redirect: §1 forbids running these through a
	// shell, and a redirect requires one. Content arrives on stdin, so the
	// token never appears in argv.
	cmd := pr.Command("tee", credPath)
	cmd.Stdin = strings.NewReader(
		fmt.Sprintf("https://%s:%s@github.com\n", id.Login, id.Token))
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("writing %s: %v: %s", credPath, err, strings.TrimSpace(string(out)))
	}
	// Explicitly, not by umask. Provision sets 027, which would leave this 0640
	// and readable by the user's own group — and while that group has one member
	// today, a credential should not depend on that staying true.
	if err := runAs(pr, "chmod", "0600", credPath); err != nil {
		return err
	}
	if err := runAs(pr, "git", "config", "--global", "credential.helper", "store"); err != nil {
		return err
	}

	// gh needs its own copy; it does not read .git-credentials.
	hostsPath := filepath.Join(pr.Home, ghHostsFile)
	if err := runAs(pr, "mkdir", "-p", filepath.Dir(hostsPath)); err != nil {
		return err
	}
	hosts := pr.Command("tee", hostsPath)
	hosts.Stdin = strings.NewReader(fmt.Sprintf(
		"github.com:\n    oauth_token: %s\n    user: %s\n    git_protocol: https\n",
		id.Token, id.Login))
	if out, err := hosts.CombinedOutput(); err != nil {
		return fmt.Errorf("writing %s: %v: %s", hostsPath, err, strings.TrimSpace(string(out)))
	}
	if err := runAs(pr, "chmod", "0600", hostsPath); err != nil {
		return err
	}

	// Attribution. Without user.email, commits the agent makes are authored by
	// whatever git guesses from the hostname, which GitHub attributes to nobody.
	// <login>@users.noreply.github.com is the address GitHub itself accepts for
	// an account whose real address is private.
	if err := runAs(pr, "git", "config", "--global", "user.name", id.Login); err != nil {
		return err
	}
	return runAs(pr, "git", "config", "--global", "user.email",
		id.Login+"@users.noreply.github.com")
}

// runAs runs one command as the user, folding output into any error.
func runAs(pr *principal.Principal, name string, args ...string) error {
	out, err := pr.Command(name, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %v: %s", name, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// ghConnectFail renders a failure without leaking the query string back into
// the page, and without redirecting to a URL that still carries `code`.
func ghConnectFail(c echo.Context, human string) error {
	return c.HTML(http.StatusBadRequest,
		"<!doctype html><meta charset=utf-8><title>GitHub connect failed</title>"+
			"<body style=\"font:16px/1.5 system-ui;margin:4rem auto;max-width:34rem\">"+
			"<h1>GitHub connect failed</h1><p>"+html.EscapeString(human)+"</p>"+
			"<p><a href=\"/\">Back to Bonnie</a></p>")
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// loadGitHubOAuth reads the OAuth App registration out of Secrets Manager.
//
// Shells out to the AWS CLI rather than linking the SDK: cloud-init already
// reads this same secret exactly this way, the box has the CLI and an instance
// role, and the SDK would pull a large dependency tree into a module whose
// budget is measured in hundreds of lines (§4).
func loadGitHubOAuth(secretID, region string) (ghauth.Config, error) {
	var cfg ghauth.Config
	if secretID == "" {
		return cfg, nil
	}
	out, err := exec.Command("aws", "secretsmanager", "get-secret-value",
		"--secret-id", secretID, "--query", "SecretString", "--output", "text",
		"--region", region).Output()
	if err != nil {
		return cfg, fmt.Errorf("reading secret %s: %w", secretID, err)
	}
	if err := json.Unmarshal(out, &cfg); err != nil {
		return cfg, fmt.Errorf("secret %s is not JSON: %w", secretID, err)
	}
	return cfg, nil
}
