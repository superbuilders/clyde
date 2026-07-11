package main

import (
	"bufio"
	"embed"
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
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

//go:embed static/*
var staticFiles embed.FS

// ── Data types ──

type RunningProcess struct {
	PID       int    `json:"pid"`
	TTY       string `json:"tty"`
	StartTime string `json:"start_time"`
	Args      string `json:"args"`
	CWD       string `json:"cwd"`
}

type SessionInfo struct {
	ID           string `json:"id"`
	User         string `json:"user"`
	Timestamp    string `json:"timestamp"`
	MessageCount int    `json:"message_count"`
	LastModified string `json:"last_modified"`
	CWD          string `json:"cwd"`
	Project      string `json:"project"`
	Branch       string `json:"branch"`
	Running      bool   `json:"running"`
	PID          int    `json:"pid,omitempty"`
	Preview      string `json:"preview"`
	LastMsgType  string `json:"last_msg_type"`
	NeedsReply   bool   `json:"needs_reply"`
	ViewerAgent  bool   `json:"viewer_agent"`
	AgentBusy    bool   `json:"agent_busy"`
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

// ── Tmux management ──

// tmuxName returns a deterministic tmux session name for a viewer-managed agent.
func tmuxName(sessionID string) string {
	// Sanitize for tmux: replace dots/colons with dashes
	safe := strings.NewReplacer(".", "-", ":", "-").Replace(sessionID)
	return "sv_" + safe
}

// isTmuxRunning checks if a tmux session exists.
func isTmuxRunning(name string) bool {
	sessions := getTmuxSessions()
	_, ok := sessions[name]
	return ok
}

// getTmuxSessions returns all active tmux session names (cached per call-site via caller).
var _tmuxSessionsCache map[string]bool
var _tmuxSessionsCacheTime time.Time

func getTmuxSessions() map[string]bool {
	// Cache for 2 seconds to avoid hammering tmux during a single API call
	if time.Since(_tmuxSessionsCacheTime) < 2*time.Second && _tmuxSessionsCache != nil {
		return _tmuxSessionsCache
	}
	result := make(map[string]bool)
	out, err := exec.Command("tmux", "list-sessions", "-F", "#{session_name}").Output()
	if err != nil {
		_tmuxSessionsCache = result
		_tmuxSessionsCacheTime = time.Now()
		return result
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			result[line] = true
		}
	}
	_tmuxSessionsCache = result
	_tmuxSessionsCacheTime = time.Now()
	return result
}

// startClyde starts a clyde process in a tmux session with the correct CWD.
// For sessions with existing messages, uses -r to resume.
// For empty sessions, starts a fresh clyde REPL.
func startClyde(cwd, sessionID string) error {
	name := tmuxName(sessionID)
	if isTmuxRunning(name) {
		return nil // already running
	}

	// Check if session has existing messages (needs -r flag)
	sessDir := filepath.Join(cwd, ".clyde", "sessions", sessionID)
	entries, _ := os.ReadDir(sessDir)
	hasMessages := len(entries) > 0

	var cmd string
	if hasMessages {
		cmd = fmt.Sprintf("cd %s && clyde -r %s", shellQuote(cwd), shellQuote(sessionID))
	} else {
		cmd = fmt.Sprintf("cd %s && clyde", shellQuote(cwd))
	}

	return exec.Command("tmux", "new-session", "-d", "-s", name, "-x", "200", "-y", "50", cmd).Run()
}

// sendToClyde sends a user message to the clyde REPL via tmux.
func sendToClyde(sessionID, message string) error {
	name := tmuxName(sessionID)
	if !isTmuxRunning(name) {
		return fmt.Errorf("tmux session %s not running", name)
	}

	// Use send-keys with -l (literal) to avoid interpretation of special chars.
	// Then send Enter to submit the line.
	if err := exec.Command("tmux", "send-keys", "-t", name, "-l", message).Run(); err != nil {
		return err
	}
	return exec.Command("tmux", "send-keys", "-t", name, "Enter").Run()
}

// stopClyde kills a viewer-managed tmux session.
func stopClyde(sessionID string) error {
	name := tmuxName(sessionID)
	if !isTmuxRunning(name) {
		return nil
	}
	return exec.Command("tmux", "kill-session", "-t", name).Run()
}

// isAgentBusy checks if the clyde agent in a tmux session is actively processing.
// Heuristic: capture the last lines of the pane and look for the "You: " prompt.
// If visible, clyde is idle. If not, it's working.
func isViewerAgentBusy(sessionID string) bool {
	name := tmuxName(sessionID)
	if !isTmuxRunning(name) {
		return false
	}
	out, err := exec.Command("tmux", "capture-pane", "-t", name, "-p").Output()
	if err != nil {
		return false
	}
	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	// Check last 5 non-empty lines for the prompt
	count := 0
	for i := len(lines) - 1; i >= 0 && count < 5; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		count++
		// The prompt ends with "You: " (styled, but the text is there)
		// After ANSI stripping, look for "You:" at end of a line
		stripped := stripANSI(line)
		if strings.HasSuffix(stripped, "You: ") || strings.HasSuffix(stripped, "You:") {
			return false // prompt visible = idle
		}
	}
	return true // no prompt found = busy processing
}

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

func stripANSI(s string) string {
	return ansiRe.ReplaceAllString(s, "")
}

// ── Main ──

func main() {
	e := echo.New()
	e.Use(middleware.Logger())
	e.Use(middleware.CORS())

	api := e.Group("/api")
	api.GET("/sessions", getSessions)
	api.GET("/sessions/:id/messages", getSessionMessages)
	api.POST("/sessions/:id/messages", postSessionMessage)
	api.POST("/sessions/:id/stop", stopSession)
	api.GET("/sessions/:id/status", getSessionStatus)
	api.GET("/projects", getProjects)
	api.POST("/sessions/new", createSession)

	staticFS, _ := fs.Sub(staticFiles, "static")
	e.GET("/*", echo.WrapHandler(http.FileServer(http.FS(staticFS))))

	fmt.Println("🔍 Session Viewer running at http://localhost:8787")
	e.Logger.Fatal(e.Start(":8787"))
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

func getLastMsgTypes(entries []os.DirEntry) (string, string) {
	var last, lastMeaningful string
	for i := len(entries) - 1; i >= 0; i-- {
		m := messageTypeRe.FindStringSubmatch(entries[i].Name())
		if len(m) >= 2 {
			if last == "" {
				last = m[1]
			}
			if lastMeaningful == "" && m[1] != "diagnostic" {
				lastMeaningful = m[1]
			}
			if last != "" && lastMeaningful != "" {
				break
			}
		}
	}
	return last, lastMeaningful
}

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

// getUsername returns the normalized username (same logic as session package).
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

// formatTimestampDir formats a time for session directory names.
func formatTimestampDir(t time.Time) string {
	return t.Format("2006-01-02T15-04-05")
}

// ── Session list cache ──

var _sessionsCache []SessionInfo
var _sessionsCacheTime time.Time
const sessionsCacheTTL = 5 * time.Second

// ── API handlers ──

func getSessions(c echo.Context) error {
	// Age filter: default to last 30 days, 0 = no limit
	daysStr := c.QueryParam("days")
	days := 30
	if daysStr != "" {
		if d, err := strconv.Atoi(daysStr); err == nil {
			days = d
		}
	}

	// Check cache
	now := time.Now()
	if time.Since(_sessionsCacheTime) < sessionsCacheTTL && _sessionsCache != nil {
		filtered := filterByAge(_sessionsCache, days, now)
		return c.JSON(http.StatusOK, filtered)
	}

	processes := getRunningProcesses()
	cwdSet := discoverProjectDirs()
	for _, p := range processes {
		if p.CWD != "" {
			cwdSet[p.CWD] = true
		}
	}
	home, _ := os.UserHomeDir()
	bc := make(map[string]string)

	var all []SessionInfo
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
			sp := filepath.Join(sessDir, sid)
			user := ""
			parts := strings.SplitN(sid, "_", 2)
			if len(parts) == 2 {
				user = parts[1]
				if idx := strings.Index(user, "_from_"); idx >= 0 {
					user = user[:idx]
				}
			}
			me, _ := os.ReadDir(sp)
			sort.Slice(me, func(i, j int) bool { return me[i].Name() < me[j].Name() })
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

			// Terminal-based running detection
			running := false
			rpid := 0
			for _, proc := range processes {
				if proc.CWD == dir && matchProcessToSession(proc, sid) {
					running = true
					rpid = proc.PID
					break
				}
			}

			// Viewer-managed tmux agent
			viewerAgent := isTmuxRunning(tmuxName(sid))
			agentBusy := false
			if viewerAgent {
				agentBusy = isViewerAgentBusy(sid)
			}

			lt, lmt := getLastMsgTypes(me)
			needsReply := lmt == "assistant"

			all = append(all, SessionInfo{
				ID: sid, User: user, Timestamp: parts[0],
				MessageCount: len(me), LastModified: lms,
				CWD: dir, Project: project, Branch: branch,
				Running: running || viewerAgent, PID: rpid,
				Preview: getSessionPreview(sp), LastMsgType: lt,
				NeedsReply: needsReply,
				ViewerAgent: viewerAgent, AgentBusy: agentBusy,
			})
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].LastModified > all[j].LastModified })

	// Update cache
	_sessionsCache = all
	_sessionsCacheTime = time.Now()

	filtered := filterByAge(all, days, now)
	return c.JSON(http.StatusOK, filtered)
}

func filterByAge(sessions []SessionInfo, days int, now time.Time) []SessionInfo {
	if days <= 0 {
		return sessions
	}
	cutoff := now.AddDate(0, 0, -days)
	var result []SessionInfo
	for _, s := range sessions {
		if s.LastModified == "" {
			continue
		}
		t, err := time.Parse(time.RFC3339, s.LastModified)
		if err != nil {
			continue
		}
		if t.After(cutoff) {
			result = append(result, s)
		}
	}
	return result
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

	// Pagination: limit (default 100), before (for loading older)
	limit := 100
	if l := c.QueryParam("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}
	before := c.QueryParam("before")

	// First pass: collect all matching filenames
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

	// Apply limit: take the last N entries (most recent)
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

// postSessionMessage starts clyde in tmux (if needed) and sends the message
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

	// Check if agent is currently busy
	if isViewerAgentBusy(sid) {
		return c.JSON(http.StatusConflict, map[string]string{"error": "agent is busy processing"})
	}

	// Start clyde in tmux if not already running
	if err := startClyde(body.CWD, sid); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "failed to start clyde: " + err.Error(),
		})
	}

	// Wait for clyde to be ready (show prompt)
	// For resumed sessions, clyde needs a moment to load history
	ready := false
	for i := 0; i < 20; i++ { // up to 10 seconds
		time.Sleep(500 * time.Millisecond)
		if !isViewerAgentBusy(sid) {
			ready = true
			break
		}
	}
	if !ready {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{
			"error": "clyde not ready yet — try again in a moment",
		})
	}

	// Send the message
	if err := sendToClyde(sid, body.Content); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "failed to send message: " + err.Error(),
		})
	}

	return c.JSON(http.StatusAccepted, map[string]string{"status": "accepted"})
}

// stopSession kills a viewer-managed clyde tmux session
func stopSession(c echo.Context) error {
	sid := c.Param("id")
	if sid == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "id required"})
	}
	if !isTmuxRunning(tmuxName(sid)) {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "no viewer agent running for this session"})
	}
	if err := stopClyde(sid); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "stopped"})
}

func getSessionStatus(c echo.Context) error {
	sid := c.Param("id")
	name := tmuxName(sid)
	running := isTmuxRunning(name)
	busy := false
	if running {
		busy = isViewerAgentBusy(sid)
	}
	return c.JSON(http.StatusOK, map[string]interface{}{
		"viewer_agent": running,
		"busy":         busy,
	})
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

// createSession creates a new session directory and returns its ID
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
	if _, err := os.Stat(body.CWD); os.IsNotExist(err) {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "dir not found"})
	}

	sessRoot := filepath.Join(body.CWD, ".clyde", "sessions")
	if err := os.MkdirAll(sessRoot, 0755); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	now := time.Now()
	dirName := formatTimestampDir(now) + "_" + getUsername()
	sessDir := filepath.Join(sessRoot, dirName)
	if err := os.MkdirAll(sessDir, 0755); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]string{
		"status":     "ok",
		"session_id": dirName,
		"cwd":        body.CWD,
	})
}
