package main

import (
	"bufio"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"session-viewer/internal/auth"
	"session-viewer/internal/principal"
	"session-viewer/internal/provision"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

//go:embed static/*
var staticFiles embed.FS

// ── Types ──

// ViewerCache is the persistent JSON cache stored on disk.
type ViewerCache struct {
	Sessions    map[string]*CachedSession `json:"sessions"` // key: "cwd::session_id"
	Preferences Preferences               `json:"preferences"`
	LastScan    string                    `json:"last_scan"`
}

type CachedSession struct {
	ID            string `json:"id"`
	CWD           string `json:"cwd"`
	Project       string `json:"project"`
	Branch        string `json:"branch"`
	User          string `json:"user"`
	Name          string `json:"name"` // user-assigned display name
	MessageCount  int    `json:"message_count"`
	LastModified  string `json:"last_modified"`
	Preview       string `json:"preview"`
	Read          bool   `json:"read"`
	LastReadCount int    `json:"last_read_count"` // message count when last marked read

	WorktreeParent     string `json:"worktree_parent,omitempty"`      // parent folder path, if part of a worktree group
	WorktreeParentName string `json:"worktree_parent_name,omitempty"` // parent folder basename (for display)
}

type Preferences struct {
	HiddenProjects    map[string]bool `json:"hidden_projects"`
	CollapsedProjects map[string]bool `json:"collapsed_projects"`
}

// SessionResponse is what the API returns (cached data + live status).
type SessionResponse struct {
	ID           string `json:"id"`
	CWD          string `json:"cwd"`
	Project      string `json:"project"`
	Branch       string `json:"branch"`
	User         string `json:"user"`
	Name         string `json:"name"`
	MessageCount int    `json:"message_count"`
	LastModified string `json:"last_modified"`
	Preview      string `json:"preview"`
	Unread       bool   `json:"unread"`
	ProcessType  string `json:"process_type"` // "sh", "tmux", ""
	Busy         bool   `json:"busy"`

	WorktreeParent     string `json:"worktree_parent,omitempty"`      // parent folder path
	WorktreeParentName string `json:"worktree_parent_name,omitempty"` // parent folder basename (for display)
}

type MessageFile struct {
	Filename  string `json:"filename"`
	Timestamp string `json:"timestamp"`
	Type      string `json:"type"`
	Content   string `json:"content"`
}

type ProjectInfo struct {
	Path            string          `json:"path"`
	Name            string          `json:"name"`
	Branch          string          `json:"branch"`
	IsWorktreeGroup bool            `json:"is_worktree_group,omitempty"`
	ParentPath      string          `json:"parent_path,omitempty"`
	ParentName      string          `json:"parent_name,omitempty"`
	Worktrees       []WorktreeEntry `json:"worktrees,omitempty"`
}

// sortedKeys returns the keys of a string-keyed set in deterministic order.
// Used wherever a map is iterated to produce API output, or where the loop
// body mutates the same map.
func sortedKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// ── Worktree types and detection (WT-2) ──

// WorktreeGroup represents a set of sibling git worktrees sharing a parent folder.
type WorktreeGroup struct {
	ParentDir  string          // the container folder path
	ParentName string          // basename of container folder
	Worktrees  []WorktreeEntry // all worktrees in the group
}

// WorktreeEntry represents a single git worktree.
type WorktreeEntry struct {
	Path   string `json:"path"`   // worktree root path
	Branch string `json:"branch"` // branch name
	Name   string `json:"name"`   // basename of worktree dir
}

// parseWorktreeList parses the output of `git worktree list --porcelain` into entries.
func parseWorktreeList(output string) []WorktreeEntry {
	var entries []WorktreeEntry
	var current *WorktreeEntry

	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			if current != nil {
				entries = append(entries, *current)
				current = nil
			}
			continue
		}
		if strings.HasPrefix(line, "worktree ") {
			path := strings.TrimPrefix(line, "worktree ")
			current = &WorktreeEntry{
				Path: path,
				Name: filepath.Base(path),
			}
		} else if strings.HasPrefix(line, "branch ") && current != nil {
			branch := strings.TrimPrefix(line, "branch ")
			// Strip refs/heads/ prefix
			branch = strings.TrimPrefix(branch, "refs/heads/")
			current.Branch = branch
		}
		// Ignore HEAD, bare, detached, etc.
	}
	// Flush last entry if no trailing blank line
	if current != nil {
		entries = append(entries, *current)
	}
	return entries
}

// detectWorktreeGroup checks if dir is part of a git worktree group where all
// worktrees are siblings (same parent directory). Returns nil if not applicable.
func detectWorktreeGroup(dir string) *WorktreeGroup {
	// Resolve symlinks for consistent path comparison (macOS /var → /private/var)
	dir, _ = filepath.EvalSymlinks(dir)

	// Step 1: Is this inside a git repo?
	gitDirOut, err := exec.Command("git", "-C", dir, "rev-parse", "--git-dir").Output()
	if err != nil {
		return nil // not a git repo at all
	}
	gitDir := strings.TrimSpace(string(gitDirOut))

	commonDirOut, err := exec.Command("git", "-C", dir, "rev-parse", "--git-common-dir").Output()
	if err != nil {
		return nil
	}
	commonDir := strings.TrimSpace(string(commonDirOut))

	// Resolve relative paths against dir
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(dir, gitDir)
	}
	if !filepath.IsAbs(commonDir) {
		commonDir = filepath.Join(dir, commonDir)
	}
	gitDir = filepath.Clean(gitDir)
	commonDir = filepath.Clean(commonDir)
	// Resolve symlinks
	gitDir, _ = filepath.EvalSymlinks(gitDir)
	commonDir, _ = filepath.EvalSymlinks(commonDir)

	// Step 2: Get worktree list
	wtOut, err := exec.Command("git", "-C", dir, "worktree", "list", "--porcelain").Output()
	if err != nil {
		return nil
	}
	worktrees := parseWorktreeList(string(wtOut))
	if len(worktrees) <= 1 {
		return nil // no linked worktrees, normal repo
	}

	// Resolve symlinks in worktree paths
	for i := range worktrees {
		if resolved, err := filepath.EvalSymlinks(worktrees[i].Path); err == nil {
			worktrees[i].Path = resolved
			worktrees[i].Name = filepath.Base(resolved)
		}
	}

	// Step 3: Group the worktrees that are siblings of the main worktree root.
	// The main worktree root is the parent of commonDir (commonDir points to .git).
	//
	// Outliers — worktrees created outside the container folder, e.g. a scratch
	// worktree under /tmp — are deliberately EXCLUDED rather than treated as a
	// veto. Previously a single out-of-tree worktree returned nil here and
	// silently collapsed the entire group back into flat, ungrouped projects.
	mainWorktreeRoot := filepath.Dir(commonDir)
	parentFolder := filepath.Dir(mainWorktreeRoot)

	siblings := make([]WorktreeEntry, 0, len(worktrees))
	for _, wt := range worktrees {
		if filepath.Dir(wt.Path) == parentFolder {
			siblings = append(siblings, wt)
		}
	}

	// A group needs at least two co-located worktrees to be meaningful. If the
	// main worktree stands alone in its folder (all linked worktrees live
	// elsewhere), this is not a sibling layout — don't group.
	if len(siblings) <= 1 {
		return nil
	}

	// Stable order so the API response does not depend on git's output order.
	sort.Slice(siblings, func(i, j int) bool { return siblings[i].Path < siblings[j].Path })

	return &WorktreeGroup{
		ParentDir:  parentFolder,
		ParentName: filepath.Base(parentFolder),
		Worktrees:  siblings,
	}
}

type RunningProcess struct {
	PID       int
	TTY       string
	StartTime string
	Args      string
	CWD       string
}

// ── Globals ──

var (
	cache     ViewerCache
	cacheMu   sync.RWMutex
	cachePath string
	scanning  bool
	scanMu    sync.Mutex
)

func cacheKey(cwd, id string) string { return cwd + "::" + id }

// ── Cache management ──

func initCache() {
	home, _ := os.UserHomeDir()
	cacheDir := filepath.Join(home, ".clyde")
	os.MkdirAll(cacheDir, 0755)
	cachePath = filepath.Join(cacheDir, "viewer-cache.json")

	data, err := os.ReadFile(cachePath)
	if err != nil {
		cache = newEmptyCache()
		return
	}
	if err := json.Unmarshal(data, &cache); err != nil {
		cache = newEmptyCache()
		return
	}
	fixNilMaps(&cache)
	fmt.Printf("📂 Loaded %d cached sessions\n", len(cache.Sessions))
}

func newEmptyCache() ViewerCache {
	return ViewerCache{
		Sessions: make(map[string]*CachedSession),
		Preferences: Preferences{
			HiddenProjects:    make(map[string]bool),
			CollapsedProjects: make(map[string]bool),
		},
	}
}

func fixNilMaps(c *ViewerCache) {
	if c.Sessions == nil {
		c.Sessions = make(map[string]*CachedSession)
	}
	if c.Preferences.HiddenProjects == nil {
		c.Preferences.HiddenProjects = make(map[string]bool)
	}
	if c.Preferences.CollapsedProjects == nil {
		c.Preferences.CollapsedProjects = make(map[string]bool)
	}
}

func saveCache() {
	// cachePath is read under the same lock as the cache itself. saveCache runs
	// on goroutines that outlive the request that spawned them, so anything
	// that swaps the path out from under one of them — a test moving to a fresh
	// temp dir, say — is a data race, not a theoretical one.
	cacheMu.RLock()
	data, err := json.MarshalIndent(cache, "", "  ")
	path := cachePath
	cacheMu.RUnlock()
	if err != nil {
		return
	}
	os.WriteFile(path, data, 0644)
}

// ── Tmux helpers ──

func tmuxName(sessionID string) string {
	safe := strings.NewReplacer(".", "-", ":", "-").Replace(sessionID)
	return "sv_" + safe
}

var _tmuxCache map[string]bool
var _tmuxCacheTime time.Time

// principals resolves an authenticated email to the Unix user we act as. In
// solo mode it always yields the invoking user, so nothing below changes.
var principals *principal.Resolver

// tmuxCmd builds a tmux invocation for a principal.
//
// Every tmux call must be scoped to the owning user, for two reasons: the
// server must run as them so the agent it spawns inherits their uid, and the
// socket must be theirs so no other user can attach and drive their agent.
// Solo mode returns a plain exec.Command, identical to before.
func tmuxCmd(p *principal.Principal, args ...string) *exec.Cmd {
	if p == nil || p.Solo {
		return exec.Command("tmux", args...)
	}
	return p.Command("tmux", args...)
}

// tmuxCacheKey keeps per-user caches apart. A single shared cache would let
// one user's session list mask another's.
func tmuxCacheKey(p *principal.Principal) string {
	if p == nil || p.Solo {
		return ""
	}
	return p.Username
}

var _tmuxCacheByUser = map[string]map[string]bool{}
var _tmuxCacheTimeByUser = map[string]time.Time{}

func getTmuxSessions(p *principal.Principal) map[string]bool {
	key := tmuxCacheKey(p)
	if t, ok := _tmuxCacheTimeByUser[key]; ok && time.Since(t) < 2*time.Second {
		if c := _tmuxCacheByUser[key]; c != nil {
			return c
		}
	}
	result := make(map[string]bool)
	out, err := tmuxCmd(p, "list-sessions", "-F", "#{session_name}").Output()
	if err == nil {
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			if line = strings.TrimSpace(line); line != "" {
				result[line] = true
			}
		}
	}
	_tmuxCacheByUser[key] = result
	_tmuxCacheTimeByUser[key] = time.Now()
	return result
}

func invalidateTmuxCache(p *principal.Principal) {
	delete(_tmuxCacheTimeByUser, tmuxCacheKey(p))
}

func isTmuxRunning(p *principal.Principal, name string) bool {
	return getTmuxSessions(p)[name]
}

func startClyde(p *principal.Principal, cwd, sessionID string) error {
	name := tmuxName(sessionID)
	if isTmuxRunning(p, name) {
		return nil
	}
	if err := p.EnsureRuntimeDir(); err != nil {
		return fmt.Errorf("preparing runtime dir: %w", err)
	}
	// Always use -r to resume into the existing session directory,
	// even if it's empty. Without -r, clyde creates its own new session.
	cmd := fmt.Sprintf("cd %s && clyde -r %s", shellQuote(cwd), shellQuote(sessionID))
	err := tmuxCmd(p, "new-session", "-d", "-s", name, "-x", "200", "-y", "50", cmd).Run()
	// Invalidate tmux cache
	invalidateTmuxCache(p)
	return err
}

func sendToClyde(p *principal.Principal, sessionID, message string) error {
	name := tmuxName(sessionID)
	if !isTmuxRunning(p, name) {
		return fmt.Errorf("tmux session %s not running", name)
	}
	if err := tmuxCmd(p, "send-keys", "-t", name, "-l", message).Run(); err != nil {
		return err
	}
	return tmuxCmd(p, "send-keys", "-t", name, "Enter").Run()
}

func stopClyde(p *principal.Principal, sessionID string) error {
	name := tmuxName(sessionID)
	if !isTmuxRunning(p, name) {
		return nil
	}
	err := tmuxCmd(p, "kill-session", "-t", name).Run()
	invalidateTmuxCache(p)
	return err
}

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

func stripANSI(s string) string { return ansiRe.ReplaceAllString(s, "") }

func isTmuxBusy(p *principal.Principal, sessionID string) bool {
	name := tmuxName(sessionID)
	if !isTmuxRunning(p, name) {
		return false
	}
	out, err := exec.Command("tmux", "capture-pane", "-t", name, "-p").Output()
	if err != nil {
		return false
	}
	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	count := 0
	for i := len(lines) - 1; i >= 0 && count < 5; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		count++
		stripped := stripANSI(line)
		if strings.HasSuffix(stripped, "You: ") || strings.HasSuffix(stripped, "You:") {
			return false
		}
	}
	return true
}

// ── Process discovery ──

func getRunningProcesses() []RunningProcess {
	var processes []RunningProcess
	out, err := exec.Command("pgrep", "-x", "clyde").Output()
	if err != nil {
		return processes
	}
	for _, pidStr := range strings.Fields(strings.TrimSpace(string(out))) {
		pid, _ := strconv.Atoi(pidStr)
		if pid == 0 {
			continue
		}
		proc := RunningProcess{PID: pid}
		psOut, err := exec.Command("ps", "-o", "tty=,lstart=,args=", "-p", pidStr).Output()
		if err != nil {
			continue
		}
		fields := strings.Fields(strings.TrimSpace(string(psOut)))
		if len(fields) >= 6 {
			proc.TTY = fields[0]
			proc.StartTime = strings.Join(fields[1:6], " ")
			proc.Args = strings.Join(fields[6:], " ")
		}
		lsofOut, err := exec.Command("lsof", "-p", pidStr, "-Fn").Output()
		if err == nil {
			scanner := bufio.NewScanner(strings.NewReader(string(lsofOut)))
			foundCWD := false
			for scanner.Scan() {
				line := scanner.Text()
				if line == "fcwd" {
					foundCWD = true
				} else if foundCWD && strings.HasPrefix(line, "n") {
					proc.CWD = line[1:]
					break
				}
			}
		}
		processes = append(processes, proc)
	}
	return processes
}

func matchProcessToSession(proc RunningProcess, sessionID string) bool {
	if strings.Contains(proc.Args, "-r ") {
		parts := strings.SplitN(proc.Args, "-r ", 2)
		if len(parts) == 2 {
			rid := strings.TrimSpace(parts[1])
			return strings.HasPrefix(sessionID, rid) || sessionID == rid
		}
	}
	pt, err := time.Parse("Mon Jan _2 15:04:05 2006", strings.TrimSpace(proc.StartTime))
	if err != nil {
		return false
	}
	st, err := parseSessionTimestamp(sessionID)
	if err != nil {
		return false
	}
	d := pt.Sub(st)
	if d < 0 {
		d = -d
	}
	return d < 60*time.Second
}

// getLiveStatus computes process_type and busy for all sessions.
// Returns map[cacheKey] → {processType, busy}
type liveStatus struct {
	processType string
	busy        bool
}

func getLiveStatuses(p *principal.Principal, sessions map[string]*CachedSession) map[string]liveStatus {
	result := make(map[string]liveStatus)
	processes := getRunningProcesses()
	tmuxSessions := getTmuxSessions(p)

	for key, s := range sessions {
		st := liveStatus{}

		// Check tmux first (sv_ prefix)
		tName := tmuxName(s.ID)
		if tmuxSessions[tName] {
			st.processType = "tmux"
			st.busy = isTmuxBusy(p, s.ID)
			result[key] = st
			continue
		}

		// Check terminal processes
		for _, proc := range processes {
			if proc.CWD == s.CWD && matchProcessToSession(proc, s.ID) {
				st.processType = "sh"
				break
			}
		}

		result[key] = st
	}
	return result
}

// ── Helpers ──

func parseSessionTimestamp(sessionID string) (time.Time, error) {
	parts := strings.SplitN(sessionID, "_", 2)
	if len(parts) < 1 {
		return time.Time{}, fmt.Errorf("invalid")
	}
	tsStr := strings.Replace(parts[0], "T", " ", 1)
	sp := strings.SplitN(tsStr, " ", 2)
	if len(sp) == 2 {
		tsStr = sp[0] + " " + strings.Replace(sp[1], "-", ":", 2)
	}
	return time.Parse("2006-01-02 15:04:05", tsStr)
}

func getSessionPreview(sessionPath string) string {
	entries, _ := os.ReadDir(sessionPath)
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), "_user.md") {
			data, err := readFileCapped(filepath.Join(sessionPath, e.Name()), 1024)
			if err != nil {
				return ""
			}
			c := strings.TrimSpace(data)
			c = strings.TrimPrefix(c, "**You:**")
			c = strings.TrimSpace(c)
			c = strings.Join(strings.Fields(c), " ")
			if len(c) > 140 {
				c = c[:140] + "…"
			}
			return c
		}
	}
	return ""
}

func cleanThinkingContent(content string) string {
	var out []string
	for _, line := range strings.Split(content, "\n") {
		if !strings.HasPrefix(line, "signature:") {
			out = append(out, line)
		}
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

func getBranch(dir string, cache map[string]string) string {
	if b, ok := cache[dir]; ok {
		return b
	}
	out, err := exec.Command("git", "-C", dir, "branch", "--show-current").Output()
	b := ""
	if err == nil {
		b = strings.TrimSpace(string(out))
	}
	cache[dir] = b
	return b
}

var messageTypeRe = regexp.MustCompile(`_([a-z-]+)\.md$`)

// discoverProjectDirs is the background scanner's root set.
//
// In multi-user mode this is the union over every provisioned user, which is
// what the read-time filter in getSessions already assumes: it documents the
// cache as seeing "every user's sessions" and does isolation per request. That
// was only ever true of the comment. Passing nil here took the *solo* branch,
// so the scanner walked the service account's own home — /srv/bonnie/home —
// and no user's sessions were ever scanned at all. Every real user saw an
// empty sidebar while the cache quietly filled up with the service's projects,
// which canAccess then correctly hid from them.
//
// The service's own home is deliberately not scanned in multi-user mode. It
// belongs to no principal, so nothing in it could be shown to anyone.
func discoverProjectDirs() map[string]bool {
	if principals == nil || principals.IsSolo() {
		return discoverProjectDirsFor(nil)
	}

	m, err := principal.LoadUserMap(principals.MapPath())
	if err != nil {
		log.Printf("scan: cannot read user map %s: %v", principals.MapPath(), err)
		return map[string]bool{}
	}

	s := make(map[string]bool)
	for _, username := range m.ByEmail() {
		p, err := principal.Lookup(username)
		if err != nil {
			log.Printf("scan: skipping %s: %v", username, err)
			continue
		}
		for dir := range discoverProjectDirsFor(p) {
			s[dir] = true
		}
	}
	return s
}

// discoverProjectDirsFor finds project directories belonging to a principal.
//
// In solo mode this is exactly the old behaviour: the invoking user's home and
// cwd. In multi-user mode it is scoped to the principal's home, which is what
// makes "Alice and Bob see only their own sessions" true of the listing as
// well as of the filesystem.
func discoverProjectDirsFor(p *principal.Principal) map[string]bool {
	s := make(map[string]bool)

	home := ""
	if p != nil && !p.Solo {
		home = p.Home
	} else {
		home, _ = os.UserHomeDir()
		cwd, _ := os.Getwd()
		// Resolve symlinks for consistent path comparison (macOS /tmp → /private/tmp)
		if resolved, err := filepath.EvalSymlinks(cwd); err == nil {
			cwd = resolved
		}
		s[cwd] = true
	}
	if home == "" {
		return s
	}
	s[home] = true

	for _, d := range []string{filepath.Join(home, "code"), filepath.Join(home, "Downloads")} {
		out, err := exec.Command("find", d, "-maxdepth", "4", "-name", ".clyde", "-type", "d").Output()
		if err == nil {
			for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
				if line = strings.TrimSpace(line); line != "" {
					dir := filepath.Dir(line)
					// Resolve symlinks for consistent path comparison
					if resolved, err := filepath.EvalSymlinks(dir); err == nil {
						dir = resolved
					}
					s[dir] = true
				}
			}
		}
	}

	// Directories shared with this principal (PLAN.md A5: sharing changes
	// exactly one thing from the viewer's perspective — how many directories
	// are in its search path). This is that one thing.
	//
	// Searched separately from ~/code rather than by letting find follow the
	// symlinks in ~/shared: -maxdepth would then be measured from the sharee's
	// home, so a project nested deeper in the owner's tree would fall outside
	// the limit and vanish for the sharee while being visible to the owner.
	// Each share is its own root, so depth is measured from the shared
	// directory itself, exactly as it is for the owner.
	for _, root := range sharedRoots(p) {
		s[root] = true
		out, err := exec.Command("find", root, "-maxdepth", "4", "-name", ".clyde", "-type", "d").Output()
		if err != nil {
			continue
		}
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			if line = strings.TrimSpace(line); line != "" {
				dir := filepath.Dir(line)
				if resolved, err := filepath.EvalSymlinks(dir); err == nil {
					dir = resolved
				}
				s[dir] = true
			}
		}
	}
	return s
}

// principalFor resolves the Unix identity a request acts as.
//
// In solo mode this is the invoking user and the email is irrelevant. In
// multi-user mode an authenticated email with no mapping is an error, never a
// fallback to the service account — falling back would silently give an
// unprovisioned user access to the service's own files.
func principalFor(c echo.Context) (*principal.Principal, error) {
	if principals == nil {
		// Unconfigured means "nobody asked for multi-user", which is solo: the
		// only way to get a multi-user resolver is to pass --multi-user, and
		// main() sets the global before serving. Defaulting to solo keeps
		// in-process callers (tests) honest without inventing a privileged
		// fallback for real requests, which is the case that would matter.
		r, err := principal.NewResolver(false, "")
		if err != nil {
			return nil, err
		}
		principals = r
	}
	if principals.IsSolo() {
		return principals.ForEmail("")
	}
	email := auth.EmailFrom(c)
	if email == "" {
		return nil, errors.New("no authenticated email on request")
	}
	return principals.ForEmail(email)
}

func readFileCapped(path string, maxBytes int64) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	info, _ := f.Stat()
	if info.Size() <= maxBytes {
		data, err := os.ReadFile(path)
		return string(data), err
	}
	buf := make([]byte, maxBytes)
	n, _ := f.Read(buf)
	return string(buf[:n]) + fmt.Sprintf("\n\n… [truncated, %d bytes total]", info.Size()), nil
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

// principalUsername is the name to stamp on a session: the user the session
// belongs to, not whoever the service happens to run as. In solo mode it falls
// back to the old git-derived name so single-user transcripts are unchanged.
func principalUsername(p *principal.Principal) string {
	if p == nil || p.Solo {
		return getUsername()
	}
	return p.Username
}

// principalHome is the home to compare a project path against when deciding
// whether to display it as "~".
func principalHome(p *principal.Principal) string {
	if p != nil && !p.Solo {
		return p.Home
	}
	h, _ := os.UserHomeDir()
	return h
}

func getUsername() string {
	// Use --global to avoid picking up a local repo config from the SV's working directory.
	// The session viewer can be launched from any repo — we want the user's identity,
	// not whatever local config happens to be in that repo.
	out, err := exec.Command("git", "config", "--global", "user.name").Output()
	if err == nil {
		name := strings.TrimSpace(string(out))
		if name != "" {
			name = strings.ToLower(name)
			name = strings.ReplaceAll(name, " ", "-")
			re := regexp.MustCompile(`[^a-z0-9\-]`)
			name = re.ReplaceAllString(name, "")
			if name != "" {
				return name
			}
		}
	}
	if u := os.Getenv("USER"); u != "" {
		return strings.ToLower(u)
	}
	return "unknown"
}

func formatTimestampDir(t time.Time) string {
	return t.Format("2006-01-02T15-04-05")
}

// ── Background scanner ──

// scanDirs is the set of directories the background scanner walks. It is a
// variable so tests can point the scanner at a temp tree: a test that scans the
// developer's real home is slow, machine-dependent, and — because the scanner
// prunes cache entries it did not find — will silently delete sessions another
// test injected by hand. Guarded by cacheMu, like cachePath, for the same
// reason: scans outlive the requests that start them.
var scanDirs = discoverProjectDirs

// waitForScanIdle blocks until no background scan is in flight, or the deadline
// passes. Scans are fired with `go backgroundScan()` from several handlers, so
// one can outlive the test that triggered it and prune the next test's fixture
// out from under it.
func waitForScanIdle(d time.Duration) {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		scanMu.Lock()
		idle := !scanning
		scanMu.Unlock()
		if idle {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func backgroundScan() {
	scanMu.Lock()
	if scanning {
		scanMu.Unlock()
		return
	}
	scanning = true
	scanMu.Unlock()
	defer func() {
		scanMu.Lock()
		scanning = false
		scanMu.Unlock()
	}()

	start := time.Now()
	cacheMu.RLock()
	roots := scanDirs
	cacheMu.RUnlock()
	cwdSet := roots()
	home, _ := os.UserHomeDir()
	bc := make(map[string]string)

	// ── WT-2/3/4: Detect worktree groups and expand discovery set ──
	// Map from dir → *WorktreeGroup for annotation during session scan
	worktreeGroupForDir := make(map[string]*WorktreeGroup)
	// Track which commonDirs we've already processed to avoid redundant git calls
	processedCommonDirs := make(map[string]bool)

	// First pass: detect worktree groups from initially discovered dirs.
	// Iterate a sorted snapshot: this loop ADDS keys to cwdSet, and Go does not
	// guarantee whether keys added during a range are visited. Ranging the map
	// directly made discovery (and the group representative) nondeterministic.
	for _, dir := range sortedKeys(cwdSet) {
		group := detectWorktreeGroup(dir)
		if group == nil {
			continue
		}
		// Deduplicate: compute commonDir key for this group
		commonDirOut, err := exec.Command("git", "-C", dir, "rev-parse", "--git-common-dir").Output()
		commonKey := dir
		if err == nil {
			cd := strings.TrimSpace(string(commonDirOut))
			if !filepath.IsAbs(cd) {
				cd = filepath.Join(dir, cd)
			}
			commonKey = filepath.Clean(cd)
		}
		if processedCommonDirs[commonKey] {
			// Already found this group from another sibling — just record the mapping
			worktreeGroupForDir[dir] = group
			continue
		}
		processedCommonDirs[commonKey] = true

		// WT-4: Add ALL worktree paths from the group to the discovery set
		for _, wt := range group.Worktrees {
			cwdSet[wt.Path] = true
			worktreeGroupForDir[wt.Path] = group
		}
		// Also add the parent folder to the discovery set
		cwdSet[group.ParentDir] = true
		worktreeGroupForDir[group.ParentDir] = group
	}

	found := make(map[string]bool)

	// Sorted for reproducible scan order and stable logs.
	for _, dir := range sortedKeys(cwdSet) {
		sessDir := filepath.Join(dir, ".clyde", "sessions")
		entries, err := os.ReadDir(sessDir)
		if err != nil {
			// WT-4: If this dir is part of a worktree group but has no .clyde/sessions/,
			// we still want it discoverable via the API (0 sessions).
			// It will appear via getProjects; no sessions to add here.
			continue
		}
		project := filepath.Base(dir)
		if dir == home {
			project = "~"
		}
		branch := getBranch(dir, bc)

		// WT-3: Determine worktree parent info for this dir
		wtParent := ""
		wtParentName := ""
		if group, ok := worktreeGroupForDir[dir]; ok {
			wtParent = group.ParentDir
			wtParentName = group.ParentName
		}

		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			sid := e.Name()
			key := cacheKey(dir, sid)
			found[key] = true
			sp := filepath.Join(sessDir, sid)

			user := ""
			parts := strings.SplitN(sid, "_", 2)
			if len(parts) == 2 {
				user = parts[1]
			}

			me, _ := os.ReadDir(sp)
			var lm time.Time
			for _, m := range me {
				if info, err := m.Info(); err == nil && info.ModTime().After(lm) {
					lm = info.ModTime()
				}
			}
			lms := ""
			if !lm.IsZero() {
				lms = lm.Format(time.RFC3339)
			}

			cacheMu.Lock()
			existing := cache.Sessions[key]
			if existing != nil {
				existing.MessageCount = len(me)
				existing.LastModified = lms
				existing.Project = project
				existing.Branch = branch
				existing.User = user
				existing.WorktreeParent = wtParent
				existing.WorktreeParentName = wtParentName
				// Note: we do NOT mark sessions unread here based on message count.
				// The frontend detects busy→idle transitions and marks unread only
				// when the agent run is fully complete (not on intermediate files).
				if existing.Preview == "" {
					existing.Preview = getSessionPreview(sp)
				}
			} else {
				cache.Sessions[key] = &CachedSession{
					ID:                 sid,
					CWD:                dir,
					Project:            project,
					Branch:             branch,
					User:               user,
					MessageCount:       len(me),
					LastModified:       lms,
					Preview:            getSessionPreview(sp),
					Read:               true,
					LastReadCount:      len(me),
					WorktreeParent:     wtParent,
					WorktreeParentName: wtParentName,
				}
			}
			cacheMu.Unlock()
		}
	}

	// Prune deleted sessions
	cacheMu.Lock()
	for key := range cache.Sessions {
		if !found[key] {
			delete(cache.Sessions, key)
		}
	}
	cache.LastScan = time.Now().Format(time.RFC3339)
	cacheMu.Unlock()

	saveCache()
	fmt.Printf("🔍 Scan: %d sessions in %v\n", len(found), time.Since(start).Round(time.Millisecond))
}

func startBackgroundScanner() {
	// Initial scan
	go backgroundScan()
	// Periodic refresh
	go func() {
		for {
			time.Sleep(60 * time.Second)
			backgroundScan()
		}
	}()
}

// ── API handlers ──

func getSessions(c echo.Context) error {
	// Resolve the Unix identity this request acts as. Every tmux and agent
	// operation below runs as that user; in solo mode it is the invoking user
	// and nothing changes.
	pr, err := principalFor(c)
	if err != nil {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "no unix identity: " + err.Error()})
	}
	daysStr := c.QueryParam("days")
	days := 30
	if daysStr != "" {
		if d, err := strconv.Atoi(daysStr); err == nil {
			days = d
		}
	}

	cacheMu.RLock()
	sessions := make(map[string]*CachedSession, len(cache.Sessions))
	for k, v := range cache.Sessions {
		sessions[k] = v
	}
	prefs := cache.Preferences
	cacheMu.RUnlock()

	// Compute live statuses
	statuses := getLiveStatuses(pr, sessions)

	now := time.Now()
	cutoff := time.Time{}
	if days > 0 {
		cutoff = now.AddDate(0, 0, -days)
	}

	var result []SessionResponse
	for key, s := range sessions {
		// Ownership filter. The cache is populated by a single background
		// scanner that has no principal — it sees every user's sessions — so
		// isolation has to happen here, at read time, per request.
		//
		// canAccess, not ownsPath: a shared directory's sessions are part of
		// what was shared (M4.2 grants the tree, including .clyde/sessions).
		if !canAccess(pr, s.CWD) {
			continue
		}
		// Age filter
		if days > 0 {
			if s.LastModified == "" {
				continue
			}
			t, err := time.Parse(time.RFC3339, s.LastModified)
			if err != nil || t.Before(cutoff) {
				continue
			}
		}

		st := statuses[key]
		result = append(result, SessionResponse{
			ID:                 s.ID,
			CWD:                s.CWD,
			Project:            s.Project,
			Branch:             s.Branch,
			User:               s.User,
			Name:               s.Name,
			MessageCount:       s.MessageCount,
			LastModified:       s.LastModified,
			Preview:            s.Preview,
			Unread:             !s.Read,
			ProcessType:        st.processType,
			Busy:               st.busy,
			WorktreeParent:     s.WorktreeParent,
			WorktreeParentName: s.WorktreeParentName,
		})
	}

	sort.Slice(result, func(i, j int) bool { return result[i].LastModified > result[j].LastModified })

	return c.JSON(http.StatusOK, map[string]interface{}{
		"sessions":    result,
		"preferences": prefs,
		"last_scan":   cache.LastScan,
	})
}

// guardSession is the authorization choke point for every session-scoped
// route. It resolves the caller and refuses anything they may not read.
//
// This exists because the application-layer half of isolation was applied to
// the *listing* and nowhere else. PLAN.md §1 is explicit that both halves must
// hold — the kernel decides what the agent can touch, the webserver decides
// what to show — but the webserver runs as root and reads transcripts
// directly, so for read paths there is no kernel half to fall back on.
// getSessions filtered correctly while GET /sessions/:id/messages served any
// transcript on the box to anyone who could guess a session id, which is
// derivable from a timestamp and a username. Verified exploitable against the
// deployed box before this was written.
//
// Handlers that spawn a process as the principal are also guarded, even though
// the kernel would stop them: a 403 is the correct answer to "may I touch this
// at all", and relying on a downstream EACCES makes the security property an
// accident of implementation rather than a decision.
func guardSession(c echo.Context, cwd, sid string) (*principal.Principal, error) {
	pr, err := principalFor(c)
	if err != nil {
		return nil, echo.NewHTTPError(http.StatusForbidden,
			map[string]string{"error": "no unix identity: " + err.Error()})
	}
	if !canAccessSession(pr, cwd, sid) {
		// 404, not 403: a distinguishable 403 confirms the session exists,
		// which leaks the existence of other people's conversations to anyone
		// willing to enumerate.
		//
		// echo.NewHTTPError, not c.JSON: c.JSON *writes* the response and
		// returns nil, so a guard built on it hands its caller a nil error and
		// every `if err != nil { return err }` falls straight through. The
		// first version of this function did exactly that — it wrote the 404
		// and then served the transcript underneath it, so the status line was
		// correct and the body still leaked. Returning a real error is what
		// makes the guard a guard.
		return nil, echo.NewHTTPError(http.StatusNotFound,
			map[string]string{"error": "not found"})
	}
	return pr, nil
}

func getSessionMessages(c echo.Context) error {
	sid := c.Param("id")
	cwd := c.QueryParam("cwd")
	after := c.QueryParam("after")
	if cwd == "" || sid == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "cwd and id required"})
	}
	if _, err := guardSession(c, cwd, sid); err != nil {
		return err
	}
	sp := filepath.Join(cwd, ".clyde", "sessions", sid)
	if _, err := os.Stat(sp); os.IsNotExist(err) {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "not found"})
	}
	entries, _ := os.ReadDir(sp)
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

	typesFilter := c.QueryParam("types")
	allowed := map[string]bool{"user": true, "assistant": true, "thinking": true}
	if typesFilter != "" {
		allowed = make(map[string]bool)
		for _, t := range strings.Split(typesFilter, ",") {
			allowed[strings.TrimSpace(t)] = true
		}
	}

	limit := 100
	if l := c.QueryParam("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}
	before := c.QueryParam("before")

	var matched []os.DirEntry
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if after != "" && name <= after {
			continue
		}
		if before != "" && name >= before {
			continue
		}
		m := messageTypeRe.FindStringSubmatch(name)
		if len(m) < 2 || !allowed[m[1]] {
			continue
		}
		matched = append(matched, e)
	}

	hasOlder := false
	if len(matched) > limit && before == "" && after == "" {
		hasOlder = true
		matched = matched[len(matched)-limit:]
	}

	var msgs []MessageFile
	for _, e := range matched {
		name := e.Name()
		m := messageTypeRe.FindStringSubmatch(name)
		ts := strings.TrimSuffix(name, "_"+m[1]+".md")
		content, err := readFileCapped(filepath.Join(sp, name), 50*1024)
		if err != nil {
			continue
		}
		if m[1] == "thinking" {
			content = cleanThinkingContent(content)
		}
		msgs = append(msgs, MessageFile{Filename: name, Timestamp: ts, Type: m[1], Content: content})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"messages":  msgs,
		"has_older": hasOlder,
	})
}

// findTerminalProcess returns a non-tmux clyde process attached to this session, if any.
func findTerminalProcess(p *principal.Principal, cwd, sessionID string) *RunningProcess {
	// Don't bother scanning if there's already a tmux session for this ID
	if isTmuxRunning(p, tmuxName(sessionID)) {
		return nil
	}
	for _, proc := range getRunningProcesses() {
		if proc.CWD == cwd && matchProcessToSession(proc, sessionID) {
			return &proc
		}
	}
	return nil
}

// killTerminalProcess sends SIGTERM to a process and waits for it to exit.
func killTerminalProcess(pid int) error {
	// Check the process exists first
	if err := syscall.Kill(pid, 0); err != nil {
		return fmt.Errorf("process %d not found: %w", pid, err)
	}
	// SIGTERM for graceful shutdown
	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
		return fmt.Errorf("failed to signal process %d: %w", pid, err)
	}
	// Wait up to 5 seconds for it to die
	for i := 0; i < 50; i++ {
		time.Sleep(100 * time.Millisecond)
		if err := syscall.Kill(pid, 0); err != nil {
			return nil // process is gone
		}
	}
	// Force kill if still alive
	syscall.Kill(pid, syscall.SIGKILL)
	time.Sleep(200 * time.Millisecond)
	return nil
}

func postSessionMessage(c echo.Context) error {
	// Resolve the Unix identity this request acts as. Every tmux and agent
	// operation below runs as that user; in solo mode it is the invoking user
	// and nothing changes.
	pr, err := principalFor(c)
	if err != nil {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "no unix identity: " + err.Error()})
	}
	sid := c.Param("id")
	var body struct {
		CWD     string `json:"cwd"`
		Content string `json:"content"`
		Force   bool   `json:"force"`
	}
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid body"})
	}
	if body.CWD == "" || body.Content == "" || sid == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "cwd, content, and id required"})
	}
	// A shared conversation is read-only, so ownership — not access — is the
	// test. The kernel would refuse the write anyway (the agent runs as the
	// caller), but an explicit 403 beats an EACCES surfacing as a mysteriously
	// dead agent five seconds later.
	if !pr.Owns(body.CWD) {
		if !canAccessSession(pr, body.CWD, sid) {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "not found"})
		}
		return c.JSON(http.StatusForbidden, map[string]string{"error": "shared conversations are read-only"})
	}
	sessPath := filepath.Join(body.CWD, ".clyde", "sessions", sid)
	if _, err := os.Stat(sessPath); os.IsNotExist(err) {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "session not found"})
	}
	if isTmuxBusy(pr, sid) {
		return c.JSON(http.StatusConflict, map[string]string{"error": "agent is busy processing"})
	}

	// Check for a terminal clyde process on this session
	if proc := findTerminalProcess(pr, body.CWD, sid); proc != nil {
		if !body.Force {
			// Return details so the frontend can show a takeover confirmation
			return c.JSON(http.StatusConflict, map[string]interface{}{
				"error": "terminal_process_running",
				"pid":   proc.PID,
				"tty":   proc.TTY,
			})
		}
		// Force mode: kill the terminal process before proceeding
		fmt.Printf("⚠️  Killing terminal clyde (PID %d, TTY %s) for session %s\n", proc.PID, proc.TTY, sid)
		if err := killTerminalProcess(proc.PID); err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{
				"error": "failed to kill terminal process: " + err.Error(),
			})
		}
	}

	if err := startClyde(pr, body.CWD, sid); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to start clyde: " + err.Error()})
	}
	ready := false
	for i := 0; i < 20; i++ {
		time.Sleep(500 * time.Millisecond)
		if !isTmuxBusy(pr, sid) {
			ready = true
			break
		}
	}
	if !ready {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": "clyde not ready yet"})
	}
	if err := sendToClyde(pr, sid, body.Content); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to send: " + err.Error()})
	}
	return c.JSON(http.StatusAccepted, map[string]string{"status": "accepted"})
}

func stopSession(c echo.Context) error {
	// Resolve the Unix identity this request acts as. Every tmux and agent
	// operation below runs as that user; in solo mode it is the invoking user
	// and nothing changes.
	pr, err := principalFor(c)
	if err != nil {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "no unix identity: " + err.Error()})
	}
	sid := c.Param("id")
	if sid == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "id required"})
	}
	if !isTmuxRunning(pr, tmuxName(sid)) {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "no tmux session running"})
	}
	if err := stopClyde(pr, sid); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "stopped"})
}

func getSessionStatus(c echo.Context) error {
	// Resolve the Unix identity this request acts as. Every tmux and agent
	// operation below runs as that user; in solo mode it is the invoking user
	// and nothing changes.
	pr, err := principalFor(c)
	if err != nil {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "no unix identity: " + err.Error()})
	}
	sid := c.Param("id")
	name := tmuxName(sid)
	tmuxRunning := isTmuxRunning(pr, name)
	busy := false
	if tmuxRunning {
		busy = isTmuxBusy(pr, sid)
	}
	return c.JSON(http.StatusOK, map[string]interface{}{
		"process_type": map[bool]string{true: "tmux", false: ""}[tmuxRunning],
		"busy":         busy,
	})
}

// patchSession updates user-editable metadata: name, read state.
func patchSession(c echo.Context) error {
	sid := c.Param("id")
	cwd := c.QueryParam("cwd")
	if sid == "" || cwd == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "id and cwd required"})
	}
	if _, err := guardSession(c, cwd, sid); err != nil {
		return err
	}
	var body struct {
		Name *string `json:"name"`
		Read *bool   `json:"read"`
	}
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid body"})
	}

	key := cacheKey(cwd, sid)
	cacheMu.Lock()
	s := cache.Sessions[key]
	if s == nil {
		cacheMu.Unlock()
		return c.JSON(http.StatusNotFound, map[string]string{"error": "session not found in cache"})
	}
	if body.Name != nil {
		s.Name = *body.Name
	}
	if body.Read != nil {
		s.Read = *body.Read
		if *body.Read {
			s.LastReadCount = s.MessageCount
		}
	}
	cacheMu.Unlock()

	go saveCache()
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

func patchPreferences(c echo.Context) error {
	var body Preferences
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid body"})
	}
	cacheMu.Lock()
	if body.HiddenProjects != nil {
		cache.Preferences.HiddenProjects = body.HiddenProjects
	}
	if body.CollapsedProjects != nil {
		cache.Preferences.CollapsedProjects = body.CollapsedProjects
	}
	cacheMu.Unlock()
	go saveCache()
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

func triggerScan(c echo.Context) error {
	go backgroundScan()
	return c.JSON(http.StatusAccepted, map[string]string{"status": "scanning"})
}

func getProjects(c echo.Context) error {
	// Resolve the Unix identity this request acts as, so discovery is scoped
	// to that user's home rather than to the whole filesystem.
	pr, err := principalFor(c)
	if err != nil {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "no unix identity: " + err.Error()})
	}
	cwdSet := discoverProjectDirsFor(pr)
	home, _ := os.UserHomeDir()
	if pr != nil && !pr.Solo {
		home = pr.Home
	}
	bc := make(map[string]string)

	// WT-6: Detect worktree groups from discovered dirs.
	// Sorted snapshot — this loop adds keys to cwdSet (see scan()).
	worktreeGroupForDir := make(map[string]*WorktreeGroup)
	processedCommonDirs := make(map[string]bool)

	for _, dir := range sortedKeys(cwdSet) {
		group := detectWorktreeGroup(dir)
		if group == nil {
			continue
		}
		commonDirOut, err := exec.Command("git", "-C", dir, "rev-parse", "--git-common-dir").Output()
		commonKey := dir
		if err == nil {
			cd := strings.TrimSpace(string(commonDirOut))
			if !filepath.IsAbs(cd) {
				cd = filepath.Join(dir, cd)
			}
			commonKey = filepath.Clean(cd)
		}
		if processedCommonDirs[commonKey] {
			worktreeGroupForDir[dir] = group
			continue
		}
		processedCommonDirs[commonKey] = true
		for _, wt := range group.Worktrees {
			cwdSet[wt.Path] = true
			worktreeGroupForDir[wt.Path] = group
		}
		cwdSet[group.ParentDir] = true
		worktreeGroupForDir[group.ParentDir] = group
	}

	// Track which worktree groups have been fully emitted
	emittedGroups := make(map[string]bool)

	var projects []ProjectInfo
	// Sorted: which member of a group carries IsWorktreeGroup must not depend on
	// map iteration order.
	for _, dir := range sortedKeys(cwdSet) {
		// Worktree expansion above can pull in sibling checkouts outside the
		// principal's home; drop anything they don't own or have been shared.
		if !canAccess(pr, dir) {
			continue
		}
		hasSessions := true
		if _, err := os.Stat(filepath.Join(dir, ".clyde", "sessions")); os.IsNotExist(err) {
			// For worktree group members without .clyde/sessions/, still include them
			if _, ok := worktreeGroupForDir[dir]; !ok {
				continue
			}
			hasSessions = false
		}
		_ = hasSessions
		name := filepath.Base(dir)
		if dir == home {
			name = "~"
		}
		pi := ProjectInfo{Path: dir, Name: name, Branch: getBranch(dir, bc)}

		if group, ok := worktreeGroupForDir[dir]; ok {
			pi.ParentPath = group.ParentDir
			pi.ParentName = group.ParentName
			// Emit full worktree list once per group
			if !emittedGroups[group.ParentDir] {
				pi.IsWorktreeGroup = true
				pi.Worktrees = group.Worktrees
				emittedGroups[group.ParentDir] = true
			}
		}

		projects = append(projects, pi)
	}
	sort.Slice(projects, func(i, j int) bool {
		if projects[i].Name != projects[j].Name {
			return projects[i].Name < projects[j].Name
		}
		// Same basename in different folders (e.g. sibling worktrees vs an
		// unrelated repo) — break the tie on path so ordering is total.
		return projects[i].Path < projects[j].Path
	})
	return c.JSON(http.StatusOK, projects)
}

// ── WT-5: POST /api/worktrees — create a new git worktree ──

func createWorktree(c echo.Context) error {
	var body struct {
		ParentPath string `json:"parent_path"`
		BranchName string `json:"branch_name"`
	}
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	if body.ParentPath == "" || body.BranchName == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "parent_path and branch_name are required"})
	}

	// Validate branch name: no spaces, no .., no ~, no ^, no :, no backslash, no control chars
	branchInvalid := strings.ContainsAny(body.BranchName, " \t\n~^:\\*?[") || strings.Contains(body.BranchName, "..")
	if branchInvalid {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid branch name: contains forbidden characters"})
	}

	// Find an existing git worktree child dir to run commands against
	gitDir := ""
	entries, err := os.ReadDir(body.ParentPath)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "cannot read parent directory: " + err.Error()})
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		childDir := filepath.Join(body.ParentPath, e.Name())
		// Check for .git file or directory
		gitPath := filepath.Join(childDir, ".git")
		if _, err := os.Stat(gitPath); err == nil {
			gitDir = childDir
			break
		}
	}
	if gitDir == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "no git worktree found in parent directory"})
	}

	// Sanitize directory name: replace "/" with "-" so branch "feature/payments"
	// creates directory "feature-payments" as a flat sibling (not nested subdirs).
	// The git branch name is preserved as-is — only the filesystem path changes.
	dirName := strings.ReplaceAll(body.BranchName, "/", "-")

	// Create the worktree
	worktreePath := filepath.Join(body.ParentPath, dirName)
	cmd := exec.Command("git", "-C", gitDir, "worktree", "add", "-b", body.BranchName, worktreePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		errMsg := strings.TrimSpace(string(output))
		if errMsg == "" {
			errMsg = err.Error()
		}
		return c.JSON(http.StatusConflict, map[string]string{"error": errMsg})
	}

	// Create .clyde/sessions/ so the viewer discovers it
	os.MkdirAll(filepath.Join(worktreePath, ".clyde", "sessions"), 0755)

	// Trigger rescan
	go backgroundScan()

	return c.JSON(http.StatusOK, map[string]interface{}{
		"status": "ok",
		"path":   worktreePath,
		"branch": body.BranchName,
	})
}

// uploadFile saves an uploaded file to the project root (session CWD) and returns the filename.
func uploadFile(c echo.Context) error {
	cwd := c.FormValue("cwd")
	if cwd == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "cwd required"})
	}
	// Upload writes into the project, so it needs ownership. Without this an
	// authenticated user could drop a file anywhere another user's tree.
	if pr, err := principalFor(c); err != nil {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "no unix identity: " + err.Error()})
	} else if !pr.Owns(cwd) {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "not your project"})
	}
	file, err := c.FormFile("file")
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "file required"})
	}
	src, err := file.Open()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to open uploaded file"})
	}
	defer src.Close()

	// Sanitize filename — keep original name but make it safe
	safeName := filepath.Base(file.Filename)
	safeName = strings.ReplaceAll(safeName, " ", "-")
	dstPath := filepath.Join(cwd, safeName)

	// If file already exists, add a numeric suffix
	if _, err := os.Stat(dstPath); err == nil {
		ext := filepath.Ext(safeName)
		base := strings.TrimSuffix(safeName, ext)
		for i := 1; ; i++ {
			candidate := fmt.Sprintf("%s-%d%s", base, i, ext)
			dstPath = filepath.Join(cwd, candidate)
			if _, err := os.Stat(dstPath); os.IsNotExist(err) {
				safeName = candidate
				break
			}
		}
	}

	dst, err := os.Create(dstPath)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to create file: " + err.Error()})
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to write file: " + err.Error()})
	}

	fmt.Printf("📎 Uploaded file: %s → %s\n", file.Filename, dstPath)
	return c.JSON(http.StatusOK, map[string]string{"filename": safeName})
}

func createSession(c echo.Context) error {
	// Resolve the Unix identity this request acts as. Every tmux and agent
	// operation below runs as that user; in solo mode it is the invoking user
	// and nothing changes.
	pr, err := principalFor(c)
	if err != nil {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "no unix identity: " + err.Error()})
	}
	var body struct {
		CWD string `json:"cwd"`
	}
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid body"})
	}
	if body.CWD == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "cwd required"})
	}
	// The caller supplies the path, and the service runs as root — so this has
	// to be checked, not assumed. Without it a user could start a session in
	// someone else's project.
	//
	// ownsPath, not canAccess: a share is read-only, so an agent started in
	// one could not write its own transcript. Refusing here gives "not your
	// project" instead of a session that appears to start and then silently
	// produces nothing. Writing into a share arrives as forking (M5).
	if !pr.Owns(body.CWD) {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "not your project"})
	}
	// Created as the principal, not as the service: a root-owned session
	// directory is one the agent cannot write its transcript into.
	dirName := formatTimestampDir(time.Now()) + "_" + principalUsername(pr)
	sessDir := filepath.Join(body.CWD, ".clyde", "sessions", dirName)
	if err := pr.MkdirAs(sessDir); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	// Add to cache immediately
	key := cacheKey(body.CWD, dirName)
	home := principalHome(pr)
	project := filepath.Base(body.CWD)
	if body.CWD == home {
		project = "~"
	}
	// WT: Detect worktree group so the session is grouped correctly from the start
	wtParent := ""
	wtParentName := ""
	if group := detectWorktreeGroup(body.CWD); group != nil {
		wtParent = group.ParentDir
		wtParentName = group.ParentName
	}
	cacheMu.Lock()
	cache.Sessions[key] = &CachedSession{
		ID: dirName, CWD: body.CWD, Project: project, User: principalUsername(pr),
		LastModified:       time.Now().Format(time.RFC3339),
		WorktreeParent:     wtParent,
		WorktreeParentName: wtParentName,
	}
	cacheMu.Unlock()
	go saveCache()

	// Start clyde in tmux immediately so the session is live
	if err := startClyde(pr, body.CWD, dirName); err != nil {
		fmt.Printf("⚠️  Failed to start clyde for new session: %v\n", err)
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "ok", "session_id": dirName, "cwd": body.CWD})
}

// ── Message & session management handlers ──

// deleteSessionMessage hard-deletes a message file from disk.
// Only allowed for stopped sessions (no tmux or terminal process).
func deleteSessionMessage(c echo.Context) error {
	// Resolve the Unix identity this request acts as. Every tmux and agent
	// operation below runs as that user; in solo mode it is the invoking user
	// and nothing changes.
	pr, err := principalFor(c)
	if err != nil {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "no unix identity: " + err.Error()})
	}
	sid := c.Param("id")
	filename := c.Param("filename")
	cwd := c.QueryParam("cwd")
	if sid == "" || filename == "" || cwd == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "id, filename, and cwd required"})
	}
	// Deletion needs ownership, not read access: a conversation shared with
	// you is read-only, and canAccessSession would say yes to it.
	if pr, gerr := guardSession(c, cwd, sid); gerr != nil {
		return gerr
	} else if !pr.Owns(cwd) {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "shared conversations are read-only"})
	}

	// Validate filename (prevent path traversal)
	if strings.Contains(filename, "/") || strings.Contains(filename, "..") {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid filename"})
	}

	// Check session is not running
	tName := tmuxName(sid)
	if isTmuxRunning(pr, tName) {
		return c.JSON(http.StatusConflict, map[string]string{"error": "cannot delete messages from a running session"})
	}
	if proc := findTerminalProcess(pr, cwd, sid); proc != nil {
		return c.JSON(http.StatusConflict, map[string]string{"error": "cannot delete messages from a running session"})
	}

	filePath := filepath.Join(cwd, ".clyde", "sessions", sid, filename)
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "message not found"})
	}

	if err := os.Remove(filePath); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to delete: " + err.Error()})
	}

	// Update message count in cache
	key := cacheKey(cwd, sid)
	cacheMu.Lock()
	if s := cache.Sessions[key]; s != nil {
		if s.MessageCount > 0 {
			s.MessageCount--
		}
	}
	cacheMu.Unlock()

	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// markAllRead marks sessions as read at the given scope.
func markAllRead(c echo.Context) error {
	var body struct {
		Scope          string `json:"scope"`           // "all", "cwd", "worktree"
		CWD            string `json:"cwd"`             // for scope="cwd"
		WorktreeParent string `json:"worktree_parent"` // for scope="worktree"
	}
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid body"})
	}

	cacheMu.Lock()
	count := 0
	for _, s := range cache.Sessions {
		if s.Read {
			continue
		}
		switch body.Scope {
		case "cwd":
			if s.CWD != body.CWD {
				continue
			}
		case "worktree":
			if s.WorktreeParent != body.WorktreeParent {
				continue
			}
			// "all" or default: mark everything
		}
		s.Read = true
		s.LastReadCount = s.MessageCount
		count++
	}
	cacheMu.Unlock()

	go saveCache()
	return c.JSON(http.StatusOK, map[string]interface{}{"status": "ok", "count": count})
}

// deleteWorktreeHandler deletes a git worktree and moves its sessions to the primary worktree.
func deleteWorktreeHandler(c echo.Context) error {
	var body struct {
		WorktreePath string `json:"worktree_path"`
		ParentPath   string `json:"parent_path"`
	}
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid body"})
	}
	if body.WorktreePath == "" || body.ParentPath == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "worktree_path and parent_path required"})
	}

	// Detect the worktree group
	group := detectWorktreeGroup(body.WorktreePath)
	if group == nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "not part of a worktree group"})
	}
	if len(group.Worktrees) <= 1 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "cannot delete the last worktree"})
	}

	// Find the primary worktree (main/master, or first non-deleted)
	primaryDir := ""
	for _, wt := range group.Worktrees {
		if wt.Path == body.WorktreePath {
			continue
		}
		if wt.Branch == "main" || wt.Branch == "master" {
			primaryDir = wt.Path
			break
		}
	}
	if primaryDir == "" {
		for _, wt := range group.Worktrees {
			if wt.Path != body.WorktreePath {
				primaryDir = wt.Path
				break
			}
		}
	}
	if primaryDir == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "no target worktree to move sessions to"})
	}

	// Move .clyde/sessions from worktree to primary
	srcSessions := filepath.Join(body.WorktreePath, ".clyde", "sessions")
	dstSessions := filepath.Join(primaryDir, ".clyde", "sessions")
	movedCount := 0
	if entries, err := os.ReadDir(srcSessions); err == nil {
		os.MkdirAll(dstSessions, 0755)
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			src := filepath.Join(srcSessions, e.Name())
			dst := filepath.Join(dstSessions, e.Name())
			if err := os.Rename(src, dst); err == nil {
				movedCount++
				cacheMu.Lock()
				oldKey := cacheKey(body.WorktreePath, e.Name())
				newKey := cacheKey(primaryDir, e.Name())
				if s, ok := cache.Sessions[oldKey]; ok {
					s.CWD = primaryDir
					s.Project = filepath.Base(primaryDir)
					cache.Sessions[newKey] = s
					delete(cache.Sessions, oldKey)
				}
				cacheMu.Unlock()
			}
		}
	}

	// Remove the git worktree
	cmd := exec.Command("git", "-C", primaryDir, "worktree", "remove", body.WorktreePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Try force remove
		cmd = exec.Command("git", "-C", primaryDir, "worktree", "remove", "--force", body.WorktreePath)
		output, err = cmd.CombinedOutput()
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{
				"error": "failed to remove worktree: " + strings.TrimSpace(string(output)),
			})
		}
	}

	go backgroundScan()
	go saveCache()

	return c.JSON(http.StatusOK, map[string]interface{}{
		"status":         "ok",
		"moved_sessions": movedCount,
		"moved_to":       primaryDir,
	})
}

// openInTerminal opens a macOS Terminal window attached to the tmux session for a clyde session.
func openInTerminal(c echo.Context) error {
	sid := c.Param("id")
	cwd := c.QueryParam("cwd")
	if sid == "" || cwd == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "id and cwd required"})
	}

	pr, err := principalFor(c)
	if err != nil {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "no unix identity: " + err.Error()})
	}

	if !canAccessSession(pr, cwd, sid) {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "not found"})
	}

	name := tmuxName(sid)

	// Start tmux session if not running
	if !isTmuxRunning(pr, name) {
		if err := startClyde(pr, cwd, sid); err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to start clyde: " + err.Error()})
		}
		time.Sleep(500 * time.Millisecond)
		if !isTmuxRunning(pr, name) {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "tmux session failed to start"})
		}
	}

	// Open Terminal.app and attach to the tmux session
	script := fmt.Sprintf(`tell application "Terminal"
	activate
	do script "tmux attach-session -t %s"
end tell`, name)

	if err := exec.Command("osascript", "-e", script).Run(); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to open terminal: " + err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "ok", "tmux_session": name})
}

// ── Main ──

// envBool reads a boolean from the environment, defaulting to false.
func envBool(key string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

func main() {
	// `provision` is a root-only administrative subcommand, not a web
	// endpoint: creating Unix users requires root and a shell on the box.
	// Handled before flag parsing so it gets its own flag set.
	if len(os.Args) > 1 && os.Args[1] == "provision" {
		os.Exit(provision.Run(os.Args[2:]))
	}
	// Likewise for sharing: it acts as two different users, so it needs root
	// and a shell on the box (PLAN.md §4 M4.2, A5).
	if len(os.Args) > 1 && os.Args[1] == "share" {
		os.Exit(runShare(os.Args[2:]))
	}

	// Flags are parsed before anything binds or scans, so a misconfigured auth
	// setup fails before the viewer is reachable.
	addr := ":8787"
	if v := os.Getenv("CLYDE_VIEWER_LISTEN"); v != "" {
		addr = v
	}
	flag.StringVar(&addr, "listen", addr, "listen address, e.g. :8787")
	multiUser := flag.Bool("multi-user", envBool("BONNIE_MULTI_USER"),
		"run each user's agents as their own Unix user (M3). Off = today's solo behaviour.")
	userMapFlag := flag.String("user-map", principal.UserMapPath, "email→username map used with --multi-user")
	auth.RegisterFlags()
	flag.Parse()
	if !strings.Contains(addr, ":") {
		addr = ":" + addr
	}

	resolver, err := principal.NewResolver(*multiUser, *userMapFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "principal configuration error: %v\n", err)
		os.Exit(2)
	}
	principals = resolver
	if *multiUser {
		// Fail at startup, not on the first request: running as an
		// unprivileged user means every spawn would fail with EPERM, and the
		// symptom would be sessions that silently produce nothing.
		if os.Geteuid() != 0 {
			fmt.Fprintln(os.Stderr, "--multi-user requires root (CAP_SETUID/CAP_SETGID/CAP_CHOWN)")
			os.Exit(2)
		}
		log.Printf("multi-user mode: principals from %s", *userMapFlag)

		// Close the tree to `other` (PLAN.md §4 M4.1, §7).
		//
		// umask is inherited across fork and exec, so setting it once here
		// covers every agent the server spawns — and setting it here rather
		// than in the agent keeps §7 the only upstream change. 027 makes an
		// agent's directories 0750 and its files 0640.
		//
		// This has to happen before sharing can be safe. ACLs only ever *add*
		// access; they cannot take back what `other` already has, so on a
		// world-readable tree a share's traverse bit exposes everything below
		// the home and revoking the ACL does not take that away.
		//
		// Solo never reaches this branch and keeps the inherited umask, which
		// is what makes solo byte-identical (§8 item 2).
		syscall.Umask(0o027)
	}

	authCfg, err := auth.Build(context.Background())
	if err != nil {
		fmt.Fprintf(os.Stderr, "auth configuration error: %v\n", err)
		os.Exit(2)
	}

	initCache()
	startBackgroundScanner()

	e := echo.New()
	e.Use(middleware.Logger())
	e.Use(middleware.CORS())
	authCfg.Install(e)

	api := e.Group("/api")
	api.GET("/sessions", getSessions)
	api.GET("/sessions/:id/messages", getSessionMessages)
	api.POST("/sessions/:id/messages", postSessionMessage)
	api.POST("/sessions/:id/stop", stopSession)
	api.GET("/sessions/:id/status", getSessionStatus)
	api.PATCH("/sessions/:id", patchSession)
	api.POST("/sessions/scan", triggerScan)
	api.POST("/sessions/new", createSession)
	api.POST("/upload", uploadFile)
	api.GET("/projects", getProjects)
	// Sharing (PLAN.md §4 M4.2). Grant and revoke are the same body against
	// the same path, so they differ only by method.
	api.GET("/shares", getShares)
	api.POST("/shares", postShare)
	api.DELETE("/shares", deleteShare)
	api.GET("/shareable-users", getShareableUsers)
	api.POST("/worktrees", createWorktree)
	api.POST("/worktrees/delete", deleteWorktreeHandler)
	api.DELETE("/sessions/:id/messages/:filename", deleteSessionMessage)
	api.POST("/sessions/mark-all-read", markAllRead)
	api.POST("/sessions/:id/open-terminal", openInTerminal)
	api.GET("/preferences", func(c echo.Context) error {
		cacheMu.RLock()
		defer cacheMu.RUnlock()
		return c.JSON(http.StatusOK, cache.Preferences)
	})
	api.PATCH("/preferences", patchPreferences)

	staticFS, _ := fs.Sub(staticFiles, "static")
	e.GET("/*", echo.WrapHandler(http.FileServer(http.FS(staticFS))))

	if authCfg.Mode == "none" {
		fmt.Printf("🔍 Session Viewer at http://localhost%s\n", addr)
	} else {
		fmt.Printf("🔒 Session Viewer at %s (auth=%s, allowed=%s)\n",
			addr, authCfg.Mode, strings.Join(authCfg.Allowed, ","))
	}
	e.Logger.Fatal(e.Start(addr))
}
