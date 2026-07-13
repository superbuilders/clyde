package main

import (
	"bufio"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

//go:embed static/*
var staticFiles embed.FS

// ── Types ──

// ViewerCache is the persistent JSON cache stored on disk.
type ViewerCache struct {
	Sessions    map[string]*CachedSession `json:"sessions"`    // key: "cwd::session_id"
	Preferences Preferences               `json:"preferences"`
	LastScan    string                     `json:"last_scan"`
}

type CachedSession struct {
	ID            string `json:"id"`
	CWD           string `json:"cwd"`
	Project       string `json:"project"`
	Branch        string `json:"branch"`
	User          string `json:"user"`
	Name          string `json:"name"`           // user-assigned display name
	MessageCount  int    `json:"message_count"`
	LastModified  string `json:"last_modified"`
	Preview       string `json:"preview"`
	Read          bool   `json:"read"`
	LastReadCount int    `json:"last_read_count"` // message count when last marked read
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
}

type MessageFile struct {
	Filename  string `json:"filename"`
	Timestamp string `json:"timestamp"`
	Type      string `json:"type"`
	Content   string `json:"content"`
}

type ProjectInfo struct {
	Path   string `json:"path"`
	Name   string `json:"name"`
	Branch string `json:"branch"`
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
	cacheMu.RLock()
	data, err := json.MarshalIndent(cache, "", "  ")
	cacheMu.RUnlock()
	if err != nil {
		return
	}
	os.WriteFile(cachePath, data, 0644)
}

// ── Tmux helpers ──

func tmuxName(sessionID string) string {
	safe := strings.NewReplacer(".", "-", ":", "-").Replace(sessionID)
	return "sv_" + safe
}

var _tmuxCache map[string]bool
var _tmuxCacheTime time.Time

func getTmuxSessions() map[string]bool {
	if time.Since(_tmuxCacheTime) < 2*time.Second && _tmuxCache != nil {
		return _tmuxCache
	}
	result := make(map[string]bool)
	out, err := exec.Command("tmux", "list-sessions", "-F", "#{session_name}").Output()
	if err == nil {
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			if line = strings.TrimSpace(line); line != "" {
				result[line] = true
			}
		}
	}
	_tmuxCache = result
	_tmuxCacheTime = time.Now()
	return result
}

func isTmuxRunning(name string) bool {
	return getTmuxSessions()[name]
}

func startClyde(cwd, sessionID string) error {
	name := tmuxName(sessionID)
	if isTmuxRunning(name) {
		return nil
	}
	sessDir := filepath.Join(cwd, ".clyde", "sessions", sessionID)
	entries, _ := os.ReadDir(sessDir)
	var cmd string
	if len(entries) > 0 {
		cmd = fmt.Sprintf("cd %s && clyde -r %s", shellQuote(cwd), shellQuote(sessionID))
	} else {
		cmd = fmt.Sprintf("cd %s && clyde", shellQuote(cwd))
	}
	err := exec.Command("tmux", "new-session", "-d", "-s", name, "-x", "200", "-y", "50", cmd).Run()
	// Invalidate tmux cache
	_tmuxCacheTime = time.Time{}
	return err
}

func sendToClyde(sessionID, message string) error {
	name := tmuxName(sessionID)
	if !isTmuxRunning(name) {
		return fmt.Errorf("tmux session %s not running", name)
	}
	if err := exec.Command("tmux", "send-keys", "-t", name, "-l", message).Run(); err != nil {
		return err
	}
	return exec.Command("tmux", "send-keys", "-t", name, "Enter").Run()
}

func stopClyde(sessionID string) error {
	name := tmuxName(sessionID)
	if !isTmuxRunning(name) {
		return nil
	}
	err := exec.Command("tmux", "kill-session", "-t", name).Run()
	_tmuxCacheTime = time.Time{}
	return err
}

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

func stripANSI(s string) string { return ansiRe.ReplaceAllString(s, "") }

func isTmuxBusy(sessionID string) bool {
	name := tmuxName(sessionID)
	if !isTmuxRunning(name) {
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

func getLiveStatuses(sessions map[string]*CachedSession) map[string]liveStatus {
	result := make(map[string]liveStatus)
	processes := getRunningProcesses()
	tmuxSessions := getTmuxSessions()

	for key, s := range sessions {
		st := liveStatus{}

		// Check tmux first (sv_ prefix)
		tName := tmuxName(s.ID)
		if tmuxSessions[tName] {
			st.processType = "tmux"
			st.busy = isTmuxBusy(s.ID)
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

func discoverProjectDirs() map[string]bool {
	s := make(map[string]bool)
	home, _ := os.UserHomeDir()
	s[home] = true
	cwd, _ := os.Getwd()
	s[cwd] = true
	for _, d := range []string{filepath.Join(home, "code"), filepath.Join(home, "Downloads")} {
		out, err := exec.Command("find", d, "-maxdepth", "4", "-name", ".clyde", "-type", "d").Output()
		if err == nil {
			for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
				if line = strings.TrimSpace(line); line != "" {
					s[filepath.Dir(line)] = true
				}
			}
		}
	}
	return s
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

func getUsername() string {
	out, err := exec.Command("git", "config", "user.name").Output()
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
	cwdSet := discoverProjectDirs()
	home, _ := os.UserHomeDir()
	bc := make(map[string]string)

	found := make(map[string]bool)

	for dir := range cwdSet {
		sessDir := filepath.Join(dir, ".clyde", "sessions")
		entries, err := os.ReadDir(sessDir)
		if err != nil {
			continue
		}
		project := filepath.Base(dir)
		if dir == home {
			project = "~"
		}
		branch := getBranch(dir, bc)

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
				if len(me) > existing.LastReadCount && existing.Read {
					existing.Read = false
				}
				if existing.Preview == "" {
					existing.Preview = getSessionPreview(sp)
				}
			} else {
				cache.Sessions[key] = &CachedSession{
					ID:            sid,
					CWD:           dir,
					Project:       project,
					Branch:        branch,
					User:          user,
					MessageCount:  len(me),
					LastModified:  lms,
					Preview:       getSessionPreview(sp),
					Read:          false,
					LastReadCount: 0,
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
	statuses := getLiveStatuses(sessions)

	now := time.Now()
	cutoff := time.Time{}
	if days > 0 {
		cutoff = now.AddDate(0, 0, -days)
	}

	var result []SessionResponse
	for key, s := range sessions {
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
			ID:           s.ID,
			CWD:          s.CWD,
			Project:      s.Project,
			Branch:       s.Branch,
			User:         s.User,
			Name:         s.Name,
			MessageCount: s.MessageCount,
			LastModified: s.LastModified,
			Preview:      s.Preview,
			Unread:       !s.Read,
			ProcessType:  st.processType,
			Busy:         st.busy,
		})
	}

	sort.Slice(result, func(i, j int) bool { return result[i].LastModified > result[j].LastModified })

	return c.JSON(http.StatusOK, map[string]interface{}{
		"sessions":    result,
		"preferences": prefs,
		"last_scan":   cache.LastScan,
	})
}

func getSessionMessages(c echo.Context) error {
	sid := c.Param("id")
	cwd := c.QueryParam("cwd")
	after := c.QueryParam("after")
	if cwd == "" || sid == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "cwd and id required"})
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

func postSessionMessage(c echo.Context) error {
	sid := c.Param("id")
	var body struct {
		CWD     string `json:"cwd"`
		Content string `json:"content"`
	}
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid body"})
	}
	if body.CWD == "" || body.Content == "" || sid == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "cwd, content, and id required"})
	}
	sessPath := filepath.Join(body.CWD, ".clyde", "sessions", sid)
	if _, err := os.Stat(sessPath); os.IsNotExist(err) {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "session not found"})
	}
	if isTmuxBusy(sid) {
		return c.JSON(http.StatusConflict, map[string]string{"error": "agent is busy processing"})
	}
	if err := startClyde(body.CWD, sid); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to start clyde: " + err.Error()})
	}
	ready := false
	for i := 0; i < 20; i++ {
		time.Sleep(500 * time.Millisecond)
		if !isTmuxBusy(sid) {
			ready = true
			break
		}
	}
	if !ready {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": "clyde not ready yet"})
	}
	if err := sendToClyde(sid, body.Content); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to send: " + err.Error()})
	}
	return c.JSON(http.StatusAccepted, map[string]string{"status": "accepted"})
}

func stopSession(c echo.Context) error {
	sid := c.Param("id")
	if sid == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "id required"})
	}
	if !isTmuxRunning(tmuxName(sid)) {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "no tmux session running"})
	}
	if err := stopClyde(sid); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "stopped"})
}

func getSessionStatus(c echo.Context) error {
	sid := c.Param("id")
	name := tmuxName(sid)
	tmuxRunning := isTmuxRunning(name)
	busy := false
	if tmuxRunning {
		busy = isTmuxBusy(sid)
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
	cwdSet := discoverProjectDirs()
	home, _ := os.UserHomeDir()
	bc := make(map[string]string)
	var projects []ProjectInfo
	for dir := range cwdSet {
		if _, err := os.Stat(filepath.Join(dir, ".clyde", "sessions")); os.IsNotExist(err) {
			continue
		}
		name := filepath.Base(dir)
		if dir == home {
			name = "~"
		}
		projects = append(projects, ProjectInfo{Path: dir, Name: name, Branch: getBranch(dir, bc)})
	}
	sort.Slice(projects, func(i, j int) bool { return projects[i].Name < projects[j].Name })
	return c.JSON(http.StatusOK, projects)
}

func createSession(c echo.Context) error {
	var body struct {
		CWD string `json:"cwd"`
	}
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid body"})
	}
	if body.CWD == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "cwd required"})
	}
	sessRoot := filepath.Join(body.CWD, ".clyde", "sessions")
	os.MkdirAll(sessRoot, 0755)
	dirName := formatTimestampDir(time.Now()) + "_" + getUsername()
	if err := os.MkdirAll(filepath.Join(sessRoot, dirName), 0755); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	// Add to cache immediately
	key := cacheKey(body.CWD, dirName)
	home, _ := os.UserHomeDir()
	project := filepath.Base(body.CWD)
	if body.CWD == home {
		project = "~"
	}
	cacheMu.Lock()
	cache.Sessions[key] = &CachedSession{
		ID: dirName, CWD: body.CWD, Project: project, User: getUsername(),
		LastModified: time.Now().Format(time.RFC3339),
	}
	cacheMu.Unlock()
	go saveCache()

	return c.JSON(http.StatusOK, map[string]string{"status": "ok", "session_id": dirName, "cwd": body.CWD})
}

// ── Main ──

func main() {
	initCache()
	startBackgroundScanner()

	e := echo.New()
	e.Use(middleware.Logger())
	e.Use(middleware.CORS())

	api := e.Group("/api")
	api.GET("/sessions", getSessions)
	api.GET("/sessions/:id/messages", getSessionMessages)
	api.POST("/sessions/:id/messages", postSessionMessage)
	api.POST("/sessions/:id/stop", stopSession)
	api.GET("/sessions/:id/status", getSessionStatus)
	api.PATCH("/sessions/:id", patchSession)
	api.POST("/sessions/scan", triggerScan)
	api.POST("/sessions/new", createSession)
	api.GET("/projects", getProjects)
	api.GET("/preferences", func(c echo.Context) error {
		cacheMu.RLock()
		defer cacheMu.RUnlock()
		return c.JSON(http.StatusOK, cache.Preferences)
	})
	api.PATCH("/preferences", patchPreferences)

	staticFS, _ := fs.Sub(staticFiles, "static")
	e.GET("/*", echo.WrapHandler(http.FileServer(http.FS(staticFS))))

	fmt.Println("🔍 Session Viewer at http://localhost:8787")
	e.Logger.Fatal(e.Start(":8787"))
}
