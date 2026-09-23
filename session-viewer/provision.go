package main

// `bonnie provision` — the only root-only subcommand (PLAN.md §M3).
//
// Creates the Unix user behind an email address and records the mapping. It is
// idempotent so it can be run from cloud-init or by hand without special
// cases, and it refuses on ambiguity rather than guessing: a wrong
// email→username edge means serving one person's sessions to another.
//
// Deliberately NOT a web endpoint. Creating users is an administrative act; it
// requires root and a shell on the box.

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// usernameRe is deliberately strict: this string reaches useradd and the
// filesystem. Lowercase, starts with a letter, no leading dash, no dots.
var usernameRe = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)

// emailRe is a sanity check, not RFC 5322. The address has already been
// asserted by the identity provider.
var emailRe = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

type provisionOpts struct {
	email    string
	username string
	mapPath  string
	group    string
	dryRun   bool
}

func runProvision(argv []string) int {
	fs := flag.NewFlagSet("provision", flag.ContinueOnError)
	var o provisionOpts
	fs.StringVar(&o.email, "email", "", "authenticated email address of the user (required)")
	fs.StringVar(&o.username, "username", "", "unix username to create (default: derived from email)")
	fs.StringVar(&o.mapPath, "map", userMapPath, "path to the email→username map")
	fs.StringVar(&o.group, "group", "bonnie-users", "supplementary group all bonnie users join")
	fs.BoolVar(&o.dryRun, "dry-run", false, "print what would happen, change nothing")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: bonnie provision --email <addr> [--username <name>]\n\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(argv); err != nil {
		return 2
	}
	if err := provision(o); err != nil {
		fmt.Fprintf(os.Stderr, "provision: %v\n", err)
		return 1
	}
	return 0
}

func provision(o provisionOpts) error {
	if o.email == "" {
		return errors.New("--email is required")
	}
	email := strings.ToLower(strings.TrimSpace(o.email))
	if !emailRe.MatchString(email) {
		return fmt.Errorf("%q does not look like an email address", email)
	}

	username := o.username
	if username == "" {
		var err error
		username, err = usernameForEmail(email)
		if err != nil {
			return err
		}
	}
	if !usernameRe.MatchString(username) {
		return fmt.Errorf("username %q is not acceptable (want ^[a-z][a-z0-9_-]{0,31}$)", username)
	}

	// Ambiguity check against the existing map, before touching the system.
	existing, err := loadUserMap(o.mapPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reading %s: %w", o.mapPath, err)
	}
	if existing != nil {
		if prev, ok := existing.byEmail[email]; ok && prev != username {
			return fmt.Errorf("%q is already mapped to %q; refusing to remap to %q",
				email, prev, username)
		}
		// The reverse edge matters too: two emails sharing one Unix user would
		// silently merge two people's sessions into one home directory.
		for e, u := range existing.byEmail {
			if u == username && e != email {
				return fmt.Errorf("username %q is already mapped to %q; refusing to also map %q",
					username, e, email)
			}
		}
	}

	if os.Geteuid() != 0 && !o.dryRun {
		return errors.New("must run as root (or use --dry-run)")
	}

	if o.dryRun {
		fmt.Printf("would ensure group   %s\n", o.group)
		fmt.Printf("would ensure user    %s (home /home/%s, shell /usr/sbin/nologin)\n", username, username)
		fmt.Printf("would map            %s -> %s in %s\n", email, username, o.mapPath)
		return nil
	}

	if err := ensureGroup(o.group); err != nil {
		return err
	}
	created, err := ensureUser(username, o.group)
	if err != nil {
		return err
	}
	if err := ensureHomeSkeleton(username); err != nil {
		return err
	}
	if err := appendMapping(o.mapPath, email, username); err != nil {
		return err
	}

	action := "already present"
	if created {
		action = "created"
	}
	fmt.Printf("user %s %s; %s -> %s recorded in %s\n", username, action, email, username, o.mapPath)
	return nil
}

// usernameForEmail derives a Unix name from the local part of an address.
// Conservative on purpose: anything it cannot render safely is an error the
// operator resolves with --username, not a silent mangling.
func usernameForEmail(email string) (string, error) {
	local := email[:strings.Index(email, "@")]
	s := strings.ToLower(local)
	s = strings.NewReplacer(".", "-", "+", "-", "_", "-").Replace(s)
	// Drop anything still outside the allowed set.
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		}
	}
	s = strings.Trim(b.String(), "-")
	if len(s) > 32 {
		s = strings.Trim(s[:32], "-")
	}
	if !usernameRe.MatchString(s) {
		return "", fmt.Errorf("cannot derive a safe username from %q; pass --username", email)
	}
	return s, nil
}

func ensureGroup(group string) error {
	if _, err := user.LookupGroup(group); err == nil {
		return nil
	}
	cmd := exec.Command("groupadd", "--system", group)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("groupadd %s: %v: %s", group, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// ensureUser creates the account if absent. Shell is nologin: these accounts
// exist to own files and run agents, not to be logged into. Idempotent.
func ensureUser(username, group string) (bool, error) {
	if u, err := user.Lookup(username); err == nil {
		uid, _ := strconv.Atoi(u.Uid)
		if uid < 1000 {
			return false, fmt.Errorf("refusing to manage system account %q (uid %d)", username, uid)
		}
		// Ensure group membership even for an existing user.
		if out, err := exec.Command("usermod", "-aG", group, username).CombinedOutput(); err != nil {
			return false, fmt.Errorf("usermod -aG %s %s: %v: %s", group, username, err, strings.TrimSpace(string(out)))
		}
		return false, nil
	}
	args := []string{
		"--create-home",
		"--home-dir", filepath.Join("/home", username),
		"--shell", "/usr/sbin/nologin",
		"--groups", group,
		username,
	}
	if out, err := exec.Command("useradd", args...).CombinedOutput(); err != nil {
		return false, fmt.Errorf("useradd %s: %v: %s", username, err, strings.TrimSpace(string(out)))
	}
	return true, nil
}

// ensureHomeSkeleton creates the tree the agent expects, owned by the user.
//
// Home is 0750, not 0755: the isolation assertion in PLAN.md M3 is that Bob's
// agent reading Alice's tree gets EACCES, and that is a permission bit, not a
// policy. Group is the user's own group, so bonnie-users membership does not
// grant cross-user reads.
func ensureHomeSkeleton(username string) error {
	u, err := user.Lookup(username)
	if err != nil {
		return err
	}
	uid, _ := strconv.Atoi(u.Uid)
	gid, _ := strconv.Atoi(u.Gid)

	dirs := []string{
		u.HomeDir,
		filepath.Join(u.HomeDir, "code"),
		filepath.Join(u.HomeDir, "code", "scratch"),
		filepath.Join(u.HomeDir, "code", "scratch", ".clyde", "sessions"),
		filepath.Join(u.HomeDir, ".clyde"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o750); err != nil {
			return err
		}
		if err := os.Chown(d, uid, gid); err != nil {
			return err
		}
	}
	if err := os.Chmod(u.HomeDir, 0o750); err != nil {
		return err
	}

	// A git identity, so commits the agent makes are attributable.
	gitconfig := filepath.Join(u.HomeDir, ".gitconfig")
	if _, err := os.Stat(gitconfig); os.IsNotExist(err) {
		content := fmt.Sprintf("[user]\n\tname = %s\n\temail = %s@bonnie.local\n", username, username)
		if err := os.WriteFile(gitconfig, []byte(content), 0o640); err != nil {
			return err
		}
		if err := os.Chown(gitconfig, uid, gid); err != nil {
			return err
		}
	}

	// The agent config, copied from the service's own so a provisioned user can
	// actually reach the model. 0600 and owned by them.
	src := filepath.Join(serviceHome(), ".clyde", "config")
	dst := filepath.Join(u.HomeDir, ".clyde", "config")
	if b, err := os.ReadFile(src); err == nil {
		if _, err := os.Stat(dst); os.IsNotExist(err) {
			if err := os.WriteFile(dst, b, 0o600); err != nil {
				return err
			}
			if err := os.Chown(dst, uid, gid); err != nil {
				return err
			}
		}
	}

	// Initialise the scratch repo so the project is discoverable.
	scratch := filepath.Join(u.HomeDir, "code", "scratch")
	if _, err := os.Stat(filepath.Join(scratch, ".git")); os.IsNotExist(err) {
		p, err := lookupPrincipal(username)
		if err != nil {
			return err
		}
		cmd := p.command("git", "init", "-q", scratch)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git init %s: %v: %s", scratch, err, strings.TrimSpace(string(out)))
		}
	}
	return nil
}

// serviceHome is the home of the account the service itself runs under, used
// as the template for provisioned users' agent config.
func serviceHome() string {
	if h := os.Getenv("BONNIE_SERVICE_HOME"); h != "" {
		return h
	}
	return "/srv/bonnie/home"
}

// appendMapping records email→username idempotently, preserving comments.
func appendMapping(path, email, username string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	lines := []string{}
	if len(b) > 0 {
		lines = strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	}
	for _, l := range lines {
		f := strings.Fields(strings.TrimSpace(l))
		if len(f) == 2 && strings.EqualFold(f[0], email) && f[1] == username {
			return nil // already recorded
		}
	}
	if len(lines) == 0 {
		lines = append(lines,
			"# bonnie email -> unix user. Written by `bonnie provision`.",
			"# An email mapping to two users is a hard error, not a guess.")
	}
	lines = append(lines, fmt.Sprintf("%s %s", email, username))
	sort.SliceStable(lines, func(i, j int) bool {
		ci := strings.HasPrefix(strings.TrimSpace(lines[i]), "#")
		cj := strings.HasPrefix(strings.TrimSpace(lines[j]), "#")
		return ci && !cj
	})

	// Validate before publishing: never leave an unparseable map on disk,
	// because that fails every login, not just this one.
	out := strings.Join(lines, "\n") + "\n"
	if _, err := parseUserMap(out); err != nil {
		return fmt.Errorf("refusing to write an invalid map: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(out), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
