package main

// Principal — who a request acts as, and how to run processes as them.
//
// PLAN.md §1: the webserver is privileged and can see everything; it enforces
// authorization in application code. The kernel boundary exists for the
// *agent*, which is untrusted because it runs an LLM with run_bash. So the
// rule is narrow and absolute: anything that executes a user's code runs as
// that user's uid, and nothing about that depends on the webserver being
// unprivileged.
//
// Solo mode is the default and must be byte-for-byte today's behaviour: the
// principal is the invoking user, no credential is applied, and commands are
// constructed exactly as they were before this file existed.

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
)

// Principal is a resolved identity: a Unix user the server may act as.
type Principal struct {
	Username string
	Email    string
	Home     string

	// Solo means "run as the invoking user, applying no credential". It is not
	// the same as uid == our uid: it is an explicit statement that we are in
	// single-user mode and must not touch SysProcAttr at all.
	Solo bool

	UID    uint32
	GID    uint32
	Groups []uint32
}

// soloPrincipal is the single-user identity: whoever the server runs as.
func soloPrincipal() (*Principal, error) {
	u, err := user.Current()
	if err != nil {
		return nil, fmt.Errorf("resolving current user: %w", err)
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		home = u.HomeDir
	}
	return &Principal{
		Username: u.Username,
		Home:     home,
		Solo:     true,
	}, nil
}

// lookupPrincipal resolves a Unix username to a principal we can act as.
func lookupPrincipal(username string) (*Principal, error) {
	u, err := user.Lookup(username)
	if err != nil {
		return nil, fmt.Errorf("lookup user %q: %w", username, err)
	}
	uid, err := strconv.ParseUint(u.Uid, 10, 32)
	if err != nil {
		return nil, fmt.Errorf("uid %q: %w", u.Uid, err)
	}
	gid, err := strconv.ParseUint(u.Gid, 10, 32)
	if err != nil {
		return nil, fmt.Errorf("gid %q: %w", u.Gid, err)
	}
	// Refuse to act as a system account. A bug in the email map must not be
	// able to turn into "run the agent as root".
	if uid < 1000 {
		return nil, fmt.Errorf("refusing to act as system account %q (uid %d)", username, uid)
	}
	gidStrs, err := u.GroupIds()
	if err != nil {
		return nil, fmt.Errorf("groups for %q: %w", username, err)
	}
	var groups []uint32
	for _, g := range gidStrs {
		n, err := strconv.ParseUint(g, 10, 32)
		if err != nil {
			continue
		}
		groups = append(groups, uint32(n))
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i] < groups[j] })
	if u.HomeDir == "" {
		return nil, fmt.Errorf("user %q has no home directory", username)
	}
	return &Principal{
		Username: u.Username,
		Home:     u.HomeDir,
		UID:      uint32(uid),
		GID:      uint32(gid),
		Groups:   groups,
	}, nil
}

// ── email → username ────────────────────────────────────────────────────────

// ErrNoMapping means the email is authenticated but not provisioned.
var ErrNoMapping = errors.New("no unix user mapped to this email")

// userMapPath is the authoritative email→username map, written by `provision`.
const userMapPath = "/etc/bonnie/users.map"

type userMap struct {
	byEmail map[string]string
}

// parseUserMap reads "email username" lines. It refuses on ambiguity: PLAN.md
// M3 requires that an email resolving to two different users is a hard error,
// not a silent pick. Getting this wrong means serving one person's sessions to
// another, so a malformed map must fail closed for everyone rather than
// quietly mis-route one account.
func parseUserMap(r string) (*userMap, error) {
	m := &userMap{byEmail: map[string]string{}}
	for i, raw := range strings.Split(r, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 {
			return nil, fmt.Errorf("line %d: want 'email username', got %q", i+1, line)
		}
		email := strings.ToLower(fields[0])
		username := fields[1]
		if prev, ok := m.byEmail[email]; ok && prev != username {
			return nil, fmt.Errorf("line %d: %q maps to both %q and %q — refusing ambiguous map",
				i+1, email, prev, username)
		}
		m.byEmail[email] = username
	}
	return m, nil
}

func loadUserMap(path string) (*userMap, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parseUserMap(string(b))
}

func (m *userMap) username(email string) (string, error) {
	u, ok := m.byEmail[strings.ToLower(strings.TrimSpace(email))]
	if !ok {
		return "", ErrNoMapping
	}
	return u, nil
}

// ── resolver ────────────────────────────────────────────────────────────────

// principalResolver turns an authenticated email into a Principal. In solo
// mode it ignores the email entirely and always yields the invoking user,
// which is what keeps single-user behaviour unchanged.
type principalResolver struct {
	solo     bool
	mapPath  string
	mu       sync.Mutex
	cache    map[string]*Principal
	soloPrin *Principal
}

func newPrincipalResolver(multiUser bool, mapPath string) (*principalResolver, error) {
	r := &principalResolver{
		solo:    !multiUser,
		mapPath: mapPath,
		cache:   map[string]*Principal{},
	}
	if r.solo {
		p, err := soloPrincipal()
		if err != nil {
			return nil, err
		}
		r.soloPrin = p
	}
	return r, nil
}

func (r *principalResolver) forEmail(email string) (*Principal, error) {
	if r.solo {
		return r.soloPrin, nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if p, ok := r.cache[email]; ok {
		return p, nil
	}
	// Re-read the map each miss: provision can add users while we are running,
	// and a stale cache would mean a freshly provisioned user cannot log in
	// until the service restarts.
	m, err := loadUserMap(r.mapPath)
	if err != nil {
		return nil, fmt.Errorf("loading %s: %w", r.mapPath, err)
	}
	username, err := m.username(email)
	if err != nil {
		return nil, err
	}
	p, err := lookupPrincipal(username)
	if err != nil {
		return nil, err
	}
	p.Email = email
	r.cache[email] = p
	return p, nil
}

// ── running as the principal ────────────────────────────────────────────────

// command builds an exec.Cmd that runs as this principal.
//
// In solo mode it returns exactly what exec.Command returns, with no
// SysProcAttr and no environment rewriting, so single-user behaviour is
// unchanged. Otherwise credentials are applied via SysProcAttr.Credential,
// which the runtime applies in the child after fork and before exec — no
// per-thread credential juggling, nothing a goroutine can escape.
//
// Never route this through a shell: argv is explicit.
func (p *Principal) command(name string, args ...string) *exec.Cmd {
	if p == nil || p.Solo {
		return exec.Command(name, args...)
	}
	// Re-arm NoNewPrivileges in the child.
	//
	// The unit cannot set it: on systemd 255, NoNewPrivileges=yes combined with
	// any seccomp-installing sandbox directive silently removes CAP_SETUID from
	// the service's *permitted* set while leaving it in the bounding set, so
	// every spawn fails with EPERM. The service therefore runs without it — it
	// must change uid, which is precisely what NNP forbids.
	//
	// The agent has no such need, and it is the process actually running
	// untrusted output, so it gets NNP back here: setpriv sets the bit before
	// exec, and it is inherited by everything the agent spawns. Without this,
	// an agent could regain privilege through a setuid binary.
	argv := append([]string{"--no-new-privs", "--", name}, args...)
	cmd := exec.Command(setprivPath, argv...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Credential: &syscall.Credential{
			Uid:    p.UID,
			Gid:    p.GID,
			Groups: p.Groups,
		},
	}
	cmd.Dir = p.Home
	cmd.Env = p.environ()
	return cmd
}

// setprivPath is util-linux's setpriv. Absolute, because this runs with an
// inherited PATH and the whole point is that it is the real one.
const setprivPath = "/usr/bin/setpriv"

// environ is the child environment for a non-solo principal. HOME must point
// at the principal's home or the agent writes its sessions into the wrong
// tree, and TMUX_TMPDIR must be per-user or every user shares one tmux server
// — which would hand any user control of everyone else's agents.
func (p *Principal) environ() []string {
	keep := []string{"PATH", "LANG", "LC_ALL", "TERM"}
	env := []string{
		"HOME=" + p.Home,
		"USER=" + p.Username,
		"LOGNAME=" + p.Username,
		"TMUX_TMPDIR=" + p.runtimeDir(),
	}
	for _, k := range keep {
		if v := os.Getenv(k); v != "" {
			env = append(env, k+"="+v)
		}
	}
	return env
}

// runtimeDir is where this principal's tmux server socket lives. One directory
// per user, 0700 and owned by them: a shared socket would mean any user could
// attach to another user's tmux and drive their agent.
func (p *Principal) runtimeDir() string {
	if p.Solo {
		return os.Getenv("TMUX_TMPDIR")
	}
	return filepath.Join(runtimeRoot, p.Username)
}

// runtimeRoot is the systemd RuntimeDirectory for the service.
var runtimeRoot = "/run/bonnie"

// ensureRuntimeDir creates the principal's tmux directory, owned by them.
func (p *Principal) ensureRuntimeDir() error {
	if p == nil || p.Solo {
		return nil
	}
	d := p.runtimeDir()
	if err := os.MkdirAll(d, 0o700); err != nil {
		return err
	}
	if err := os.Chown(d, int(p.UID), int(p.GID)); err != nil {
		return err
	}
	return os.Chmod(d, 0o700)
}

// mkdirAs creates dir, and any missing parents, as the principal rather than
// as the service.
//
// The service runs as root but deliberately has no CAP_DAC_OVERRIDE, so
// os.MkdirAll into a user's 0750 home fails with EACCES — as it should. More
// importantly, a directory the service created would be owned by root, and the
// agent (which runs as the user) could not write into it. Every write into a
// user's tree happens as that user; the service only ever reads.
func (p *Principal) mkdirAs(dir string) error {
	if p == nil || p.Solo {
		return os.MkdirAll(dir, 0o750)
	}
	if !ownsPath(p, dir) {
		return fmt.Errorf("refusing to create %q outside %s's home", dir, p.Username)
	}
	// `mkdir -p` rather than a chain of syscalls: setting the credential is a
	// property of a child process, so the work has to happen in one.
	cmd := p.command("mkdir", "-p", "-m", "0750", dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("mkdir -p %s as %s: %v: %s", dir, p.Username, err, strings.TrimSpace(string(out)))
	}
	return nil
}
