package main

// `bonnie git-credentials` — put a working GitHub credential in every
// provisioned user's home.
//
// PLAN.md §M6. This closes a gap M3 opened and nobody noticed: cloud-init
// installs a token into the *service* account's home, which was correct when
// every agent ran as `bonnie`, and became wrong the moment agents started
// running as the user. M4.1 then closed those homes to `other`, so the file is
// not merely in the wrong place — it is unreadable by design. The result was
// that no user's agent could clone, fetch, or push anything private, and every
// gate passed anyway because they all run against pre-seeded fixtures that
// never touch a remote.
//
// Three decisions worth stating, because each had a tempting alternative.
//
// *Distribution, not minting on demand.* The obvious design is a git
// credential helper that calls a privileged local service to mint a token per
// invocation, so nothing is ever at rest. That service is unix-socket RPC with
// SO_PEERCRED, which §4 deleted by name as v1 over-engineering. Writing a
// short-lived token to a 0600 file inside a 0750 home — the same mechanism the
// box already uses, made per-user and rotating — needs no new concepts and no
// new process.
//
// *No fallback to the personal access token.* If the App is not configured
// this does nothing and says so. Copying the existing long-lived org PAT into
// five homes would make git work today and would be new exposure introduced
// silently, in the name of convenience. An installation token expires in an
// hour; that bound is the whole reason for the App.
//
// *Written as the user, never as root.* Same rule as MkdirAs and grantShare:
// every write into a user's tree happens as that user. The token goes over
// stdin rather than argv, because argv is world-readable in /proc for as long
// as the process lives.

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"session-viewer/internal/ghapp"
	"session-viewer/internal/principal"
	"strings"
	"time"
)

// gitCredentialsFile is where git's `store` helper looks. Matching the
// conventional name means the helper needs no configuration beyond being
// switched on.
const gitCredentialsFile = ".git-credentials"

// loadGitHubSecret reads the App configuration out of Secrets Manager.
//
// Shells out to the AWS CLI rather than linking the SDK. cloud-init already
// reads this same secret exactly this way, the box already has the CLI and an
// instance role, and adding the SDK would pull a large dependency tree into a
// module whose budget is measured in hundreds of lines (§4).
func loadGitHubSecret(secretID, region string) (ghapp.Config, error) {
	var cfg ghapp.Config
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

// writeCredentialsAs installs the token in one user's home, as that user.
func writeCredentialsAs(p *principal.Principal, token string) error {
	path := filepath.Join(p.Home, gitCredentialsFile)

	// `tee` rather than a redirect: PLAN.md §1 forbids running these through a
	// shell, and a redirect requires one. Content arrives on stdin, so the
	// token never appears in argv — which /proc exposes to every user on the
	// box for the lifetime of the process.
	//
	// x-access-token is the username GitHub requires for App installation
	// tokens; the token itself is the password.
	cmd := p.Command("tee", path)
	cmd.Stdin = strings.NewReader(
		fmt.Sprintf("https://x-access-token:%s@github.com\n", token))
	cmd.Stdout = nil
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("writing %s: %v: %s", path, err, strings.TrimSpace(string(out)))
	}

	// Explicitly, not by umask. Provision sets 027, which would leave this
	// 0640 and readable by the user's own group — and while that group has one
	// member today, a credential should not depend on that staying true.
	if out, err := p.Command("chmod", "0600", path).CombinedOutput(); err != nil {
		return fmt.Errorf("chmod %s: %v: %s", path, err, strings.TrimSpace(string(out)))
	}

	// Switch the helper on. Idempotent, and cheap enough to repeat every
	// refresh rather than tracking whether it was already done.
	if out, err := p.Command("git", "config", "--global",
		"credential.helper", "store").CombinedOutput(); err != nil {
		return fmt.Errorf("enabling credential helper for %s: %v: %s",
			p.Username, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func runGitCredentials(argv []string) int {
	fs := flag.NewFlagSet("git-credentials", flag.ContinueOnError)
	var (
		secretID = fs.String("secret", os.Getenv("BONNIE_GITHUB_SECRET"),
			"Secrets Manager id holding the GitHub App configuration")
		region = fs.String("region", envOr("AWS_REGION", "us-east-1"),
			"AWS region for the secret")
		mapPath = fs.String("user-map", principal.UserMapPath,
			"email→username map naming the users to credential")
	)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr,
			"usage: bonnie git-credentials --secret <id> [--region <r>]\n\n"+
				"Mints a GitHub App installation token and installs it in every\n"+
				"provisioned user's home. Run from a timer; tokens last an hour.\n\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(argv); err != nil {
		return 2
	}
	if *secretID == "" {
		fmt.Fprintln(os.Stderr, "git-credentials: --secret is required")
		return 2
	}
	if os.Geteuid() != 0 {
		// It writes into other users' homes as them, which needs root to
		// become them. Fail up front rather than part-way through a loop.
		fmt.Fprintln(os.Stderr, "git-credentials: must run as root (it writes as each user)")
		return 2
	}

	cfg, err := loadGitHubSecret(*secretID, *region)
	if err != nil {
		fmt.Fprintf(os.Stderr, "git-credentials: %v\n", err)
		return 1
	}
	if !cfg.Configured() {
		// Not an error: a box without the App configured should boot, and the
		// operator should be told exactly what is missing rather than left to
		// infer it from git failing inside an agent an hour later.
		fmt.Fprintf(os.Stderr,
			"git-credentials: secret %s has no GitHub App configuration "+
				"(need GITHUB_APP_ID, GITHUB_APP_INSTALLATION_ID, GITHUB_APP_PRIVATE_KEY).\n"+
				"No credentials installed; user agents cannot reach private repos.\n",
			*secretID)
		return 0
	}

	token, err := ghapp.Mint(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "git-credentials: %v\n", err)
		return 1
	}

	m, err := principal.LoadUserMap(*mapPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "git-credentials: reading %s: %v\n", *mapPath, err)
		return 1
	}

	// One token for every user. An installation token is scoped to the
	// installation, not to a person, so there is nothing per-user to mint —
	// attribution comes from the git identity provision writes into each
	// .gitconfig, not from the credential. Worth being explicit about: every
	// Bonnie user can reach every repo the App can. True per-user scoping
	// needs user-to-server OAuth, which is a much larger change and is
	// consistent to defer only because everyone here is one org.
	failures := 0
	installed := 0
	for _, username := range m.ByEmail() {
		p, err := principal.Lookup(username)
		if err != nil {
			fmt.Fprintf(os.Stderr, "git-credentials: skipping %s: %v\n", username, err)
			failures++
			continue
		}
		if err := writeCredentialsAs(p, token.Value); err != nil {
			fmt.Fprintf(os.Stderr, "git-credentials: %v\n", err)
			failures++
			continue
		}
		installed++
	}

	// Never print the token. The expiry is the useful part in a log: it says
	// how long the box has before the timer must have run again.
	fmt.Printf("installed credentials for %d user(s), expiring %s (in %s)\n",
		installed, token.ExpiresAt.UTC().Format(time.RFC3339),
		time.Until(token.ExpiresAt).Round(time.Minute))
	if failures > 0 {
		return 1
	}
	return 0
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
