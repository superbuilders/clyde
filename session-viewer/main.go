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

	"github.com/joho/godotenv"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	"github.com/superbuilders/clyde/agent"
	"github.com/superbuilders/clyde/agent/session"
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

// ── Agent runner state ──

type agentRun struct {
	mu      sync.Mutex
	busy    bool
	ag      *agent.Agent
	sess    *session.Session
	history bool // true if history was loaded
}

var (
	agentRuns   = make(map[string]*agentRun) // key: cwd + "|" + sessionID
	agentRunsMu sync.RWMutex
)

func agentKey(cwd, sessionID string) string { return cwd + "|" + sessionID }

func getOrCreateRun(cwd, sessionID string) *agentRun {
	key := agentKey(cwd, sessionID)
	agentRunsMu.Lock()
	defer agentRunsMu.Unlock()
	if r, ok := agentRuns[key]; ok {
		return r
	}
	r := &agentRun{}
	agentRuns[key] = r
	return r
}

func isAgentBusy(cwd, sessionID string) bool {
	key := agentKey(cwd, sessionID)
	agentRunsMu.RLock()
	r, ok := agentRuns[key]
	agentRunsMu.RUnlock()
	if !ok {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.busy
}

// ── Main ──

func main() {
	// Load .env from CWD for API keys
	loadEnv()

	e := echo.New()
	e.Use(middleware.Logger())
	e.Use(middleware.CORS())

	api := e.Group("/api")
	api.GET("/sessions", getSessions)
	api.GET("/sessions/:id/messages", getSessionMessages)
	api.POST("/sessions/:id/messages", postSessionMessage)
	api.GET("/sessions/:id/status", getSessionStatus)
	api.GET("/projects", getProjects)
	api.POST("/sessions/new", createSession)

	staticFS, _ := fs.Sub(staticFiles, "static")
	e.GET("/*", echo.WrapHandler(http.FileServer(http.FS(staticFS))))

	fmt.Println("🔍 Session Viewer running at http://localhost:8787")
	e.Logger.Fatal(e.Start(":8787"))
}

func loadEnv() {
	// Try .env in CWD, then parent dirs, then home
	paths := []string{".env"}
	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(home, ".env"))
	}
	// Walk up from CWD
	cwd, _ := os.Getwd()
	for dir := cwd; dir != "/" && dir != "."; dir = filepath.Dir(dir) {
		p := filepath.Join(dir, ".env")
		if _, err := os.Stat(p); err == nil {
			paths = append([]string{p}, paths...)
			break
		}
	}
	for _, p := range paths {
		_ = godotenv.Load(p)
	}
}

func buildAgentConfig() agent.Config {
	apiKey := os.Getenv("TS_AGENT_API_KEY")
	return agent.Config{
		APIKey:            apiKey,
		APIURL:            "https://api.anthropic.com/v1/messages",
		ModelID:           "claude-opus-4-6",
		MaxTokens:         64000,
		ContextWindowSize: 200000,
		BraveSearchAPIKey: os.Getenv("BRAVE_SEARCH_API_KEY"),
		MCPPlaywright:     false, // no browser in viewer agent
	}
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

// ── API handlers ──

func getSessions(c echo.Context) error {
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
			running := false
			rpid := 0
			for _, proc := range processes {
				if proc.CWD == dir && matchProcessToSession(proc, sid) {
					running = true
					rpid = proc.PID
					break
				}
			}
			lt, lmt := getLastMsgTypes(me)
			// needs_reply: ball is in the user's court (for both running and past)
			needsReply := lmt == "assistant"
			busy := isAgentBusy(dir, sid)

			all = append(all, SessionInfo{
				ID: sid, User: user, Timestamp: parts[0],
				MessageCount: len(me), LastModified: lms,
				CWD: dir, Project: project, Branch: branch,
				Running: running, PID: rpid,
				Preview: getSessionPreview(sp), LastMsgType: lt,
				NeedsReply: needsReply, AgentBusy: busy,
			})
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].LastModified > all[j].LastModified })
	return c.JSON(http.StatusOK, all)
}

func getSessionMessages(c echo.Context) error {
	sid := c.Param("id")
	cwd := c.QueryParam("cwd")
	after := c.QueryParam("after") // only return messages after this filename
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

	var msgs []MessageFile
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if after != "" && name <= after {
			continue
		}
		m := messageTypeRe.FindStringSubmatch(name)
		if len(m) < 2 || !allowed[m[1]] {
			continue
		}
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
	return c.JSON(http.StatusOK, msgs)
}

// postSessionMessage writes a user message and starts an agent run
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

	run := getOrCreateRun(body.CWD, sid)
	run.mu.Lock()
	if run.busy {
		run.mu.Unlock()
		return c.JSON(http.StatusConflict, map[string]string{"error": "agent is busy processing"})
	}
	run.busy = true
	run.mu.Unlock()

	// Start agent in background
	go runAgent(run, body.CWD, sid, sessPath, body.Content)

	return c.JSON(http.StatusAccepted, map[string]string{"status": "accepted"})
}

func runAgent(run *agentRun, cwd, sid, sessPath, userInput string) {
	defer func() {
		run.mu.Lock()
		run.busy = false
		run.mu.Unlock()
	}()

	// Open session for writing
	sess, err := session.Open(sessPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[viewer] session open error: %v\n", err)
		return
	}

	cfg := buildAgentConfig()
	if cfg.APIKey == "" {
		fmt.Fprintln(os.Stderr, "[viewer] TS_AGENT_API_KEY not set, cannot run agent")
		// Write an error as a diagnostic
		sess.WriteMessage(session.TypeDiagnostic, "⚠️ Agent error: TS_AGENT_API_KEY not configured\n")
		return
	}

	// Create or reuse agent
	run.mu.Lock()
	needsHistory := run.ag == nil
	run.mu.Unlock()

	if needsHistory {
		// Create fresh agent with callbacks
		ag := agent.New(cfg,
			agent.WithToolUseCallback(func(displayMsg, toolName, toolUseID string, toolInput map[string]interface{}) {
				msgWithID := session.FormatToolUseID(displayMsg, toolUseID)
				inputJSON, _ := json.Marshal(toolInput)
				content := fmt.Sprintf("%s\nname: %s\ninput: %s\n",
					session.StripANSI(msgWithID), toolName, string(inputJSON))
				sess.WriteMessage(session.TypeToolUse, content)
			}),
			agent.WithOutputCallback(func(output string, toolUseID string) {
				content := fmt.Sprintf("[%s]\n```\n%s\n```\n", toolUseID, output)
				sess.WriteMessage(session.TypeToolResult, content)
			}),
			agent.WithThinkingCallback(func(text string, signature string) {
				content := "💭 " + text
				if signature != "" {
					content += "\nsignature: " + signature
				}
				sess.WriteMessage(session.TypeThinking, content+"\n")
			}),
			agent.WithDiagnosticCallback(func(msg string) {
				sess.WriteMessage(session.TypeDiagnostic, msg+"\n")
			}),
			agent.WithUserMessageCallback(func(text string) {
				sess.WriteMessage(session.TypeUser, "**You:**\n\n"+text+"\n")
			}),
			agent.WithAssistantMessageCallback(func(text string) {
				sess.WriteMessage(session.TypeAssistant, "**Claude:**\n\n"+text+"\n")
			}),
			agent.WithProgressCallback(func(msg string, toolUseID string) {
				// Progress lines are already captured by ToolUseCallback
			}),
			agent.WithCompactionCallback(func(marker string, summary string) {
				if marker != "" {
					sess.WriteMessage(session.TypeCompaction, marker+"\n")
				}
				if summary != "" {
					sess.WriteMessage(session.TypeSystem, "**System:**\n\n"+summary+"\n")
				}
			}),
			agent.WithErrorCallback(func(err error) {
				fmt.Fprintf(os.Stderr, "[viewer] agent error: %v\n", err)
			}),
		)

		// Reconstruct history from existing session files
		history, warnings, err := session.ReconstructHistory(sessPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[viewer] history reconstruction error: %v\n", err)
		}
		for _, w := range warnings {
			fmt.Fprintf(os.Stderr, "[viewer] history warning: %s\n", w)
		}
		if len(history) > 0 {
			ag.SetHistory(history)
		}

		run.mu.Lock()
		run.ag = ag
		run.sess = sess
		run.history = true
		run.mu.Unlock()
	} else {
		// Reuse existing agent - just update the session writer
		run.mu.Lock()
		run.sess = sess
		run.mu.Unlock()
	}

	// Run the agent loop (blocking - this does thinking, tool calls, everything)
	_, err = run.ag.HandleMessage(userInput)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[viewer] HandleMessage error: %v\n", err)
		sess.WriteMessage(session.TypeDiagnostic, fmt.Sprintf("⚠️ Agent error: %v\n", err))
	}
}

func getSessionStatus(c echo.Context) error {
	sid := c.Param("id")
	cwd := c.QueryParam("cwd")
	return c.JSON(http.StatusOK, map[string]interface{}{
		"busy": isAgentBusy(cwd, sid),
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

	// Create session directory
	sessRoot := filepath.Join(body.CWD, ".clyde", "sessions")
	if err := os.MkdirAll(sessRoot, 0755); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	now := time.Now()
	username := session.GetUsername()
	dirName := session.FormatTimestampDir(now) + "_" + username
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
