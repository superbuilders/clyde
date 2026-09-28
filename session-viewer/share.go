package main

// Sharing — granting one user read access to another user's directory.
//
// PLAN.md §4 (M4). Classic Unix permissions can express only owner/group/other,
// which cannot say "Alice and exactly Bob". POSIX ACLs can, so a share is:
//
//	1. a *grant*: recursive read+traverse for Bob on the shared directory, plus
//	   a default ACL so files created later are readable too;
//	2. a *corridor*: traverse-only (--x) on each directory between Alice's home
//	   and the shared one, because a grant Bob cannot walk to is useless;
//	3. a *symlink* in Bob's home, because a grant Bob cannot find is also
//	   useless — his agent has no reason to go looking in Alice's tree.
//
// Two invariants shape the code below.
//
// The filesystem is the database. There is no share registry to drift out of
// sync; a share exists iff the ACL exists. This works because the two kinds of
// entry are self-describing: a grant is r-x, a corridor bit is --x. That
// distinction is what lets revoke recompute the correct state from the tree
// alone (see reconcileCorridor).
//
// Sharing is two-sided and neither side is root. The grant modifies Alice's
// tree and runs as Alice; the symlink modifies Bob's tree and runs as Bob. The
// service performs neither. This is the same rule as mkdirAs: every write into
// a user's tree happens as that user. Alice can set ACLs on her own files
// without privilege because POSIX lets the owner do so.
//
// Shares are read-only. Write access will arrive as "forking" — a copy into
// the sharee's own tree — which touches only the sharee's home and so leaves
// this invariant intact. Nothing here should ever grant w.

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"session-viewer/internal/principal"
	"sort"
	"strings"
)

// sharedDirName is where a user's inbound shares appear, under their home.
const sharedDirName = "shared"

// ── pure logic ──────────────────────────────────────────────────────────────

// corridorFor returns the directories that need a traverse bit so that target
// is reachable from home: home itself and every directory between it and
// target, excluding target (which gets a full grant instead).
//
// Ordering is outermost-first, which is the order they must be applied in for
// the path to be walkable at every intermediate step.
//
// Everything above home is out of scope: /srv/bonnie/users is 0751 precisely
// so that it is traversable by anyone without per-share surgery.
func corridorFor(home, target string) ([]string, error) {
	home = filepath.Clean(home)
	target = filepath.Clean(target)
	if home == "" || home == "/" {
		return nil, fmt.Errorf("refusing to build a corridor from %q", home)
	}
	if target == home {
		// Sharing a whole home directory would make the corridor and the grant
		// the same directory, and the grant (r-x) would expose every project
		// Alice has. If we ever want this it needs its own design.
		return nil, fmt.Errorf("refusing to share an entire home directory (%s)", home)
	}
	if !strings.HasPrefix(target, home+string(filepath.Separator)) {
		return nil, fmt.Errorf("target %q is not inside %q", target, home)
	}

	var dirs []string
	for d := filepath.Dir(target); len(d) >= len(home); d = filepath.Dir(d) {
		dirs = append(dirs, d)
		if d == home {
			break
		}
	}
	// Collected innermost-first; the caller wants outermost-first.
	for i, j := 0, len(dirs)-1; i < j; i, j = i+1, j-1 {
		dirs[i], dirs[j] = dirs[j], dirs[i]
	}
	return dirs, nil
}

// aclEntry is one user's access to one path, as reported by getfacl.
type aclEntry struct {
	Path  string
	Perms string // as printed by getfacl, e.g. "r-x" or "--x"
}

// isGrant reports whether this entry is a share root rather than a corridor
// bit. Read access is only ever applied to the directory actually shared, so
// the presence of r is what distinguishes intent from scaffolding.
func (e aclEntry) isGrant() bool { return strings.Contains(e.Perms, "r") }

// parseGetfaclRecursive extracts one user's entries from `getfacl -R` output.
//
// The format is stanzas separated by blank lines, each beginning with
// "# file: <path>" followed by entry lines like "user:bob:r-x". Default-ACL
// entries are prefixed "default:" and are deliberately ignored here: they
// describe what future files inherit, not what is currently reachable, so they
// must not be mistaken for corridor bits.
//
// Paths are returned exactly as printed. Callers must pass --absolute-names,
// because getfacl otherwise strips the leading slash and the results cannot be
// compared against anything.
func parseGetfaclRecursive(out, username string) []aclEntry {
	var (
		entries []aclEntry
		cur     string
		want    = "user:" + username + ":"
	)
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		switch {
		case strings.HasPrefix(line, "# file:"):
			cur = strings.TrimSpace(strings.TrimPrefix(line, "# file:"))
		case strings.HasPrefix(line, "#"), line == "":
			// owner/group headers and stanza separators
		case strings.HasPrefix(line, "default:"):
			// inheritance, not current reachability
		case strings.HasPrefix(line, want):
			perms := strings.TrimPrefix(line, want)
			// getfacl appends "\t#effective:r--" when the mask clips an entry.
			if i := strings.IndexAny(perms, " \t"); i >= 0 {
				perms = perms[:i]
			}
			if cur != "" {
				entries = append(entries, aclEntry{Path: cur, Perms: perms})
			}
		}
	}
	return entries
}

// reconcileCorridor computes the corridor change needed after the set of
// grants has changed.
//
// This is the heart of revocation. Rather than trying to undo what a grant
// did — which breaks as soon as two shares overlap, because they share
// ancestors — it derives the corridor that *should* exist from the grants that
// *do* exist, and returns the difference.
//
// Being a pure function of the observed tree, it is also self-healing: a
// corridor bit left behind by an interrupted revoke is removed by the next one.
//
// Stale corridor bits are not cosmetic. They name a user, and anyone who can
// reach the directory can read that name with getfacl — so leaving them turns
// a user's home into a permanent public record of everyone they ever shared
// with, including people they revoked. They also leave genuine reachability
// for anything permissive deeper in the tree, and ext4 caps a directory at
// roughly 500 entries, which the home directory would hit first.
func reconcileCorridor(home string, entries []aclEntry) (add, remove []string, err error) {
	want := map[string]bool{}
	grants := map[string]bool{}
	for _, e := range entries {
		if !e.isGrant() {
			continue
		}
		grants[e.Path] = true
		dirs, cerr := corridorFor(home, e.Path)
		if cerr != nil {
			// A grant we cannot place is a grant we should not silently ignore:
			// it means the tree contains something we did not put there.
			return nil, nil, fmt.Errorf("reconciling %s: %w", e.Path, cerr)
		}
		for _, d := range dirs {
			want[d] = true
		}
	}

	have := map[string]bool{}
	for _, e := range entries {
		if !e.isGrant() {
			have[e.Path] = true
		}
	}

	for d := range want {
		if have[d] {
			continue
		}
		// Never turn a grant into a corridor bit.
		//
		// A share is granted recursively, so every directory inside it is also
		// a grant — and each one's ancestors include the share root itself.
		// The share root therefore appears in `want`, and applying a traverse
		// bit to it would rewrite r-x down to --x, silently revoking the share
		// while leaving an entry that still looks present in getfacl.
		//
		// Found by the overlapping-share gate: revoking one share downgraded
		// the other to traverse-only, so it stayed listed but unreadable.
		if grants[d] {
			continue
		}
		add = append(add, d)
	}
	for d := range have {
		if !want[d] {
			remove = append(remove, d)
		}
	}
	sort.Strings(add)
	sort.Strings(remove)
	return add, remove, nil
}

// linkNameFor is where a share from owner appears in the sharee's home.
//
// For a conversation that is ~/shared/<owner>/<project>/<session-id>, and for
// anything else ~/shared/<owner>/<basename>. Namespacing by owner means two
// people sharing directories with the same name do not collide, and the
// provenance of a shared directory is visible in its path.
//
// The project component exists because a session id is a uuid and says
// nothing. The sharee's agent finds these links by walking the filesystem, so
// the path is the only description it gets; ~/shared/alice/api-server/<id> can
// be reasoned about and ~/shared/alice/<id> cannot. It costs nothing in
// safety — the name is text in the sharee's own home, not a grant — and the
// owner's project name was already implied by the share.
//
// Must stay a pure function of (shareeHome, owner, target): revoke recomputes
// the link name to delete it, so any nondeterminism here strands links.
func linkNameFor(shareeHome, ownerUsername, target string) string {
	root := filepath.Join(shareeHome, sharedDirName, ownerUsername)
	if cwd, id, ok := splitSessionDir(target); ok {
		return filepath.Join(root, filepath.Base(cwd), id)
	}
	return filepath.Join(root, filepath.Base(target))
}

// ── visibility ──────────────────────────────────────────────────────────────

// sharedRoots returns the real directories a principal can reach through the
// shares in their home: the resolved targets of the symlinks under ~/shared.
//
// Resolved, because the symlink points into the owner's tree and every later
// comparison is against real paths. A dangling link — the owner deleted the
// conversation, or revoked by hand — is skipped rather than erroring, since
// one stale link must not make the whole session list fail.
//
// The tree is walked to whatever depth the links happen to sit at rather than
// at a fixed two levels, because linkNameFor inserts a project component for
// conversations (~/shared/<owner>/<project>/<id>) and does not for anything
// else. Recursion stops at the first symlink on each branch: a link *is* the
// share, and descending through it would enumerate the owner's tree.
func sharedRoots(p *principal.Principal) []string {
	if p == nil || p.Solo {
		return nil
	}
	var roots []string
	var walk func(dir string, depth int)
	walk = func(dir string, depth int) {
		// A bound, not a limit: the layouts above are two or three deep, and
		// an unbounded walk of a directory the sharee can write is a way to
		// make every request expensive.
		if depth > 4 {
			return
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, e := range entries {
			full := filepath.Join(dir, e.Name())
			if e.Type()&os.ModeSymlink != 0 {
				if target, err := filepath.EvalSymlinks(full); err == nil {
					roots = append(roots, target)
				}
				continue
			}
			if e.IsDir() {
				walk(full, depth+1)
			}
		}
	}
	walk(filepath.Join(p.Home, sharedDirName), 0)
	return roots
}

// sessionDir is where a conversation's transcript lives. A session is
// identified by (cwd, id) everywhere in the viewer; this is the one place that
// turns that pair into a path.
func sessionDir(cwd, id string) string {
	if cwd == "" || id == "" {
		return ""
	}
	return filepath.Join(cwd, ".clyde", "sessions", id)
}

// splitSessionDir is the inverse of sessionDir: given a grant path, recover
// the (cwd, id) pair the viewer identifies a conversation by.
//
// Needed because the filesystem is the database. getShares reads back raw ACL
// paths, and the only way to report them as conversations rather than as
// directories is to undo the construction. ok is false for any path that is
// not shaped like a session directory, which is how a grant made by some
// other means gets ignored rather than mis-reported.
func splitSessionDir(path string) (cwd, id string, ok bool) {
	path = filepath.Clean(path)
	id = filepath.Base(path)
	rest := filepath.Dir(path)
	if filepath.Base(rest) != "sessions" {
		return "", "", false
	}
	rest = filepath.Dir(rest)
	if filepath.Base(rest) != ".clyde" {
		return "", "", false
	}
	cwd = filepath.Dir(rest)
	if cwd == "" || cwd == "." || id == "" || id == "." {
		return "", "", false
	}
	return cwd, id, true
}

// canAccessSession reports whether a principal may read one conversation.
//
// Sharing is per-conversation, not per-project (AJ, M4.2 review). The
// filesystem was never the constraint — a session directory takes an ACL like
// any other, and the corridor simply runs two levels deeper through .clyde and
// sessions. Project-level sharing was inherited from the viewer's discovery
// model, which finds projects by looking for .clyde directories, and it gave
// away far more than intended: to share one conversation you had to hand over
// every file in the repository.
//
// Granting on the session directory alone means the sharee can read the
// transcript and cannot list the project, enumerate sibling conversations, or
// read a single line of source.
func canAccessSession(p *principal.Principal, cwd, id string) bool {
	if p == nil {
		return false
	}
	if p.Solo || p.Owns(cwd) {
		return true
	}
	return canAccess(p, sessionDir(cwd, id))
}

// canAccess reports whether a principal may see a path: their own tree, or
// something shared with them.
//
// This is deliberately a separate function from ownsPath rather than a
// loosening of it. ownsPath answers "is this yours", which is the question the
// share routes must ask — you may only grant access to what you own, and
// widening that check would let a sharee re-share someone else's directory.
// canAccess answers the weaker "may you look at this", which is the right
// question for listing projects and sessions.
//
// A5: sharing changes exactly one thing from the viewer's perspective — how
// many directories are in its search path. This is that one thing.
func canAccess(p *principal.Principal, path string) bool {
	if p.Owns(path) {
		return true
	}
	if p == nil || p.Solo || path == "" {
		return false
	}
	// Resolve before comparing: the caller's path may reach the share through
	// the symlink in the sharee's home, which is textually under their home but
	// really points into the owner's tree.
	//
	// The path need not exist — callers ask about directories that have not
	// been created yet — and EvalSymlinks fails outright on a missing path. So
	// resolve the longest existing ancestor and re-attach the remainder. Doing
	// only the whole-path form leaves a missing path unresolved while the share
	// roots are resolved, and on any system with a symlinked temp or home
	// directory the two can then never match.
	clean := resolveExisting(filepath.Clean(path))
	for _, root := range sharedRoots(p) {
		if clean == root || strings.HasPrefix(clean, root+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// resolveExisting resolves symlinks in the longest prefix of path that exists,
// leaving any non-existent tail untouched.
func resolveExisting(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved
	}
	parent := filepath.Dir(path)
	if parent == path || parent == string(filepath.Separator) {
		return path
	}
	return filepath.Join(resolveExisting(parent), filepath.Base(path))
}

// ── effects ─────────────────────────────────────────────────────────────────

// setfaclAs runs setfacl as the principal. Never through a shell (PLAN.md §1):
// argv is explicit, and a username that somehow contained a metacharacter
// would be an argument rather than syntax.
func setfaclAs(p *principal.Principal, args ...string) error {
	cmd := p.Command("setfacl", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("setfacl %s: %v: %s",
			strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

// currentACL reads every entry for username under home.
//
// This runs as the service rather than as the owner. Reading is the one thing
// the service is allowed to do unilaterally (it holds CAP_DAC_READ_SEARCH
// exactly so it can see everything), and doing it here means revocation still
// works if the owner's account is in a state where spawning fails.
func currentACL(home, username string) ([]aclEntry, error) {
	cmd := exec.Command("getfacl", "-R", "-s", "--absolute-names", home)
	out, err := cmd.CombinedOutput()
	if err != nil {
		// getfacl exits non-zero on unreadable subtrees but still prints what
		// it could read. Trusting partial output here would under-report
		// grants and so over-remove corridor bits, breaking live shares.
		return nil, fmt.Errorf("getfacl -R %s: %v: %s",
			home, err, strings.TrimSpace(string(out)))
	}
	return parseGetfaclRecursive(string(out), username), nil
}

// grantShare gives sharee read access to target inside owner's tree, and makes
// it visible in the sharee's home.
//
// Order matters: the grant is applied before the corridor, so there is never a
// moment where the sharee can walk to a directory that is not yet marked as
// deliberately shared. The symlink is last, because it is the only part that
// is purely cosmetic — if it fails, access still works and the next grant
// repairs it. The ACL is the source of truth; the link is derived.
func grantShare(owner, sharee *principal.Principal, target string) error {
	if owner == nil || sharee == nil {
		return fmt.Errorf("share requires two principals")
	}
	if owner.Username == sharee.Username {
		return fmt.Errorf("refusing to share %s with its own owner", target)
	}
	target = filepath.Clean(target)
	if !owner.Owns(target) {
		return fmt.Errorf("refusing to share %q: not inside %s's home", target, owner.Username)
	}
	fi, err := os.Stat(target)
	if err != nil {
		return fmt.Errorf("share target: %w", err)
	}
	if !fi.IsDir() {
		return fmt.Errorf("share target %q is not a directory", target)
	}
	corridor, err := corridorFor(owner.Home, target)
	if err != nil {
		return err
	}

	u := "u:" + sharee.Username

	// rX, not rx: capital X means "execute only where it is already set",
	// i.e. on directories. Lowercase would mark every shared file executable.
	if err := setfaclAs(owner, "-R", "-m", u+":rX", target); err != nil {
		return err
	}
	// The default ACL makes the share durable. Without it, files the owner
	// creates tomorrow are unreadable and the share appears to rot.
	if err := setfaclAs(owner, "-R", "-d", "-m", u+":rX", target); err != nil {
		return err
	}
	for _, dir := range corridor {
		// Traverse only. The sharee can pass through these directories but
		// cannot list them, so a share leaks the path it was given and nothing
		// about the owner's other projects.
		if err := setfaclAs(owner, "-m", u+":x", dir); err != nil {
			return err
		}
	}

	return linkShare(owner, sharee, target)
}

// linkShare creates the symlink in the sharee's home, as the sharee.
func linkShare(owner, sharee *principal.Principal, target string) error {
	link := linkNameFor(sharee.Home, owner.Username, target)
	if err := sharee.MkdirAs(filepath.Dir(link)); err != nil {
		return fmt.Errorf("creating share directory: %w", err)
	}
	// -n -f so that re-sharing replaces a stale link rather than creating one
	// inside the directory the old link points at.
	cmd := sharee.Command("ln", "-sfn", target, link)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("linking %s -> %s: %v: %s",
			link, target, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// revokeShare removes the sharee's access to target and repairs the corridor.
//
// The grant is removed first, then the corridor is recomputed from whatever
// grants remain. That ordering is what makes overlapping shares safe: if the
// owner shared two directories under the same parent and revokes one, the
// surviving grant keeps the shared ancestor's traverse bit alive.
func revokeShare(owner, sharee *principal.Principal, target string) error {
	if owner == nil || sharee == nil {
		return fmt.Errorf("revoke requires two principals")
	}
	target = filepath.Clean(target)
	if !owner.Owns(target) {
		return fmt.Errorf("refusing to revoke %q: not inside %s's home", target, owner.Username)
	}

	u := "u:" + sharee.Username
	if _, err := os.Stat(target); err == nil {
		if err := setfaclAs(owner, "-R", "-x", u, target); err != nil {
			return err
		}
		if err := setfaclAs(owner, "-R", "-d", "-x", u, target); err != nil {
			return err
		}
	}

	// Remove the link before repairing the corridor, so a failure part-way
	// leaves the sharee with no visible route rather than a dangling one.
	link := linkNameFor(sharee.Home, owner.Username, target)
	if out, err := sharee.Command("rm", "-f", link).CombinedOutput(); err != nil {
		return fmt.Errorf("removing %s: %v: %s", link, err, strings.TrimSpace(string(out)))
	}

	return repairCorridor(owner, sharee)
}

// repairCorridor brings the corridor into agreement with the surviving grants.
func repairCorridor(owner, sharee *principal.Principal) error {
	entries, err := currentACL(owner.Home, sharee.Username)
	if err != nil {
		return err
	}
	add, remove, err := reconcileCorridor(owner.Home, entries)
	if err != nil {
		return err
	}
	u := "u:" + sharee.Username
	for _, dir := range add {
		if err := setfaclAs(owner, "-m", u+":x", dir); err != nil {
			return err
		}
	}
	for _, dir := range remove {
		if err := setfaclAs(owner, "-x", u, dir); err != nil {
			return err
		}
	}
	return nil
}
