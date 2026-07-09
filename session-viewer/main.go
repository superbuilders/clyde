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
}

type MessageFile struct {
	Filename  string `json:"filename"`
	Timestamp string `json:"timestamp"`
	Type      string `json:"type"`
	Content   string `json:"content"`
}

type ProjectInfo struct {
	Path    string `json:"path"`
	Name    string `json:"name"`
	Branch  string `json:"branch"`
}

func main() {
	e := echo.New()
	e.Use(middleware.Logger())
	e.Use(middleware.CORS())

	api := e.Group("/api")
	api.GET("/sessions", getSessions)
	api.GET("/sessions/:id/messages", getSessionMessages)
	api.POST("/sessions/:id/messages", postSessionMessage)
	api.GET("/projects", getProjects)
	api.POST("/sessions/new", createSession)

	// Serve static files
	staticFS, _ := fs.Sub(staticFiles, "static")
	e.GET("/*", echo.WrapHandler(http.FileServer(http.FS(staticFS))))

	fmt.Println("🔍 Session Viewer running at http://localhost:8787")
	e.Logger.Fatal(e.Start(":8787"))
}

// ── process discovery ──

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
					continue
				}
				if foundCWD && strings.HasPrefix(line, "n") {
					proc.CWD = line[1:]
					break
				}
			}
		}
		processes = append(processes, proc)
	}
	return processes
}

// ── helpers ──

func parseSessionTimestamp(sessionID string) (time.Time, error) {
	parts := strings.SplitN(sessionID, "_", 2)
	if len(parts) < 1 {
		return time.Time{}, fmt.Errorf("invalid session ID")
	}
	tsStr := strings.Replace(parts[0], "T", " ", 1)
	spaceParts := strings.SplitN(tsStr, " ", 2)
	if len(spaceParts) == 2 {
		tsStr = spaceParts[0] + " " + strings.Replace(spaceParts[1], "-", ":", 2)
	}
	return time.Parse("2006-01-02 15:04:05", tsStr)
}

func matchProcessToSession(proc RunningProcess, sessionID string) bool {
	if strings.Contains(proc.Args, "-r ") {
		parts := strings.SplitN(proc.Args, "-r ", 2)
		if len(parts) == 2 {
			resumeID := strings.TrimSpace(parts[1])
			return strings.HasPrefix(sessionID, resumeID) || sessionID == resumeID
		}
	}
	procTime, err := time.Parse("Mon Jan _2 15:04:05 2006", strings.TrimSpace(proc.StartTime))
	if err != nil {
		return false
	}
	sessTime, err := parseSessionTimestamp(sessionID)
	if err != nil {
		return false
	}
	diff := procTime.Sub(sessTime)
	if diff < 0 {
		diff = -diff
	}
	return diff < 60*time.Second
}

func getSessionPreview(sessionPath string) string {
	entries, err := os.ReadDir(sessionPath)
	if err != nil {
		return ""
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), "_user.md") {
			data, err := readFileCapped(filepath.Join(sessionPath, entry.Name()), 1024)
			if err != nil {
				return ""
			}
			content := strings.TrimSpace(data)
			content = strings.TrimPrefix(content, "**You:**")
			content = strings.TrimPrefix(content, "**You:**\n")
			content = strings.TrimSpace(content)
			content = strings.Join(strings.Fields(content), " ")
			if len(content) > 140 {
				content = content[:140] + "…"
			}
			return content
		}
	}
	return ""
}

func cleanThinkingContent(content string) string {
	lines := strings.Split(content, "\n")
	var cleaned []string
	for _, line := range lines {
		if strings.HasPrefix(line, "signature:") {
			continue
		}
		cleaned = append(cleaned, line)
	}
	return strings.TrimSpace(strings.Join(cleaned, "\n"))
}

// getBranch returns the current git branch for a directory. Cached per request via the map.
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

// getLastMessageType returns the type of the last message file in a session
// For needs_reply detection, we also return the last "meaningful" type (ignoring diagnostic)
func getLastMessageType(entries []os.DirEntry) (lastType string, lastMeaningfulType string) {
	for i := len(entries) - 1; i >= 0; i-- {
		name := entries[i].Name()
		matches := messageTypeRe.FindStringSubmatch(name)
		if len(matches) >= 2 {
			t := matches[1]
			if lastType == "" {
				lastType = t
			}
			if lastMeaningfulType == "" && t != "diagnostic" {
				lastMeaningfulType = t
			}
			if lastType != "" && lastMeaningfulType != "" {
				break
			}
		}
	}
	return
}

// discoverProjectDirs finds all directories containing .clyde/sessions
func discoverProjectDirs() map[string]bool {
	cwdSet := make(map[string]bool)

	homeDir, _ := os.UserHomeDir()
	cwdSet[homeDir] = true

	cwd, _ := os.Getwd()
	cwdSet[cwd] = true

	discoverDirs := []string{
		filepath.Join(homeDir, "code"),
		filepath.Join(homeDir, "Downloads"),
	}
	for _, searchDir := range discoverDirs {
		findOut, err := exec.Command("find", searchDir, "-maxdepth", "4", "-name", ".clyde", "-type", "d").Output()
		if err == nil {
			for _, line := range strings.Split(strings.TrimSpace(string(findOut)), "\n") {
				line = strings.TrimSpace(line)
				if line != "" {
					cwdSet[filepath.Dir(line)] = true
				}
			}
		}
	}
	return cwdSet
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

	homeDir, _ := os.UserHomeDir()
	branchCache := make(map[string]string)

	var allSessions []SessionInfo

	for dir := range cwdSet {
		sessDir := filepath.Join(dir, ".clyde", "sessions")
		entries, err := os.ReadDir(sessDir)
		if err != nil {
			continue
		}

		project := filepath.Base(dir)
		if dir == homeDir {
			project = "~"
		}
		branch := getBranch(dir, branchCache)

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}

			sessionID := entry.Name()
			sessionPath := filepath.Join(sessDir, sessionID)

			user := ""
			parts := strings.SplitN(sessionID, "_", 2)
			if len(parts) == 2 {
				user = parts[1]
				if idx := strings.Index(user, "_from_"); idx >= 0 {
					user = user[:idx]
				}
			}

			msgEntries, _ := os.ReadDir(sessionPath)
			msgCount := len(msgEntries)

			// Sort for last-message detection
			sort.Slice(msgEntries, func(i, j int) bool {
				return msgEntries[i].Name() < msgEntries[j].Name()
			})

			var lastMod time.Time
			for _, me := range msgEntries {
				info, err := me.Info()
				if err == nil && info.ModTime().After(lastMod) {
					lastMod = info.ModTime()
				}
			}
			lastModStr := ""
			if !lastMod.IsZero() {
				lastModStr = lastMod.Format(time.RFC3339)
			}

			running := false
			runPID := 0
			for _, proc := range processes {
				if proc.CWD == dir && matchProcessToSession(proc, sessionID) {
					running = true
					runPID = proc.PID
					break
				}
			}

			lastMsgType, lastMeaningful := getLastMessageType(msgEntries)
			// "needs_reply" = the agent finished its turn and the ball is in the human's court
			needsReply := !running && lastMeaningful == "assistant"

			sess := SessionInfo{
				ID:           sessionID,
				User:         user,
				Timestamp:    parts[0],
				MessageCount: msgCount,
				LastModified: lastModStr,
				CWD:          dir,
				Project:      project,
				Branch:       branch,
				Running:      running,
				PID:          runPID,
				Preview:      getSessionPreview(sessionPath),
				LastMsgType:  lastMsgType,
				NeedsReply:   needsReply,
			}
			allSessions = append(allSessions, sess)
		}
	}

	sort.Slice(allSessions, func(i, j int) bool {
		return allSessions[i].LastModified > allSessions[j].LastModified
	})

	return c.JSON(http.StatusOK, allSessions)
}

var messageTypeRe = regexp.MustCompile(`_([a-z-]+)\.md$`)

func getSessionMessages(c echo.Context) error {
	sessionID := c.Param("id")
	cwdParam := c.QueryParam("cwd")
	if cwdParam == "" || sessionID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "cwd and session id required"})
	}

	sessionPath := filepath.Join(cwdParam, ".clyde", "sessions", sessionID)
	if _, err := os.Stat(sessionPath); os.IsNotExist(err) {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "session not found"})
	}

	entries, err := os.ReadDir(sessionPath)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

	typesFilter := c.QueryParam("types")
	allowedTypes := map[string]bool{"user": true, "assistant": true, "thinking": true}
	if typesFilter != "" {
		allowedTypes = make(map[string]bool)
		for _, t := range strings.Split(typesFilter, ",") {
			allowedTypes[strings.TrimSpace(t)] = true
		}
	}

	var messages []MessageFile
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		matches := messageTypeRe.FindStringSubmatch(name)
		if len(matches) < 2 {
			continue
		}
		msgType := matches[1]
		if !allowedTypes[msgType] {
			continue
		}

		tsStr := strings.TrimSuffix(name, "_"+msgType+".md")
		content, err := readFileCapped(filepath.Join(sessionPath, name), 50*1024)
		if err != nil {
			continue
		}
		if msgType == "thinking" {
			content = cleanThinkingContent(content)
		}

		messages = append(messages, MessageFile{
			Filename:  name,
			Timestamp: tsStr,
			Type:      msgType,
			Content:   content,
		})
	}
	return c.JSON(http.StatusOK, messages)
}

// postSessionMessage writes a new user message file into a session directory
func postSessionMessage(c echo.Context) error {
	sessionID := c.Param("id")

	var body struct {
		CWD     string `json:"cwd"`
		Content string `json:"content"`
	}
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid body"})
	}
	if body.CWD == "" || body.Content == "" || sessionID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "cwd, content, and session id required"})
	}

	sessionPath := filepath.Join(body.CWD, ".clyde", "sessions", sessionID)
	if _, err := os.Stat(sessionPath); os.IsNotExist(err) {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "session not found"})
	}

	// Generate timestamp matching clyde's format: 2026-07-09T14-11-29.681
	now := time.Now()
	ts := now.Format("2006-01-02T15-04-05") + fmt.Sprintf(".%03d", now.Nanosecond()/1e6)

	filename := ts + "_user.md"
	filePath := filepath.Join(sessionPath, filename)

	// Write in clyde's format
	fileContent := "**You:**\n\n" + body.Content + "\n"
	if err := os.WriteFile(filePath, []byte(fileContent), 0644); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]string{
		"status":   "ok",
		"filename": filename,
	})
}

// getProjects returns all known project directories that have .clyde
func getProjects(c echo.Context) error {
	cwdSet := discoverProjectDirs()
	homeDir, _ := os.UserHomeDir()
	branchCache := make(map[string]string)

	var projects []ProjectInfo
	for dir := range cwdSet {
		// Only include dirs that actually have .clyde/sessions
		sessDir := filepath.Join(dir, ".clyde", "sessions")
		if _, err := os.Stat(sessDir); os.IsNotExist(err) {
			continue
		}
		name := filepath.Base(dir)
		if dir == homeDir {
			name = "~"
		}
		projects = append(projects, ProjectInfo{
			Path:   dir,
			Name:   name,
			Branch: getBranch(dir, branchCache),
		})
	}

	sort.Slice(projects, func(i, j int) bool {
		return projects[i].Name < projects[j].Name
	})

	return c.JSON(http.StatusOK, projects)
}

// createSession opens a new terminal tab running clyde in the given directory
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

	// Verify the directory exists
	if _, err := os.Stat(body.CWD); os.IsNotExist(err) {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "directory not found"})
	}

	// Use osascript to open a new Terminal.app tab with clyde
	script := fmt.Sprintf(`
		tell application "Terminal"
			activate
			do script "cd %s && clyde"
		end tell
	`, shellQuote(body.CWD))

	cmd := exec.Command("osascript", "-e", script)
	if err := cmd.Run(); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to open terminal: " + err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "ok", "cwd": body.CWD})
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

func readFileCapped(path string, maxBytes int64) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return "", err
	}
	if info.Size() <= maxBytes {
		data, err := os.ReadFile(path)
		return string(data), err
	}

	buf := make([]byte, maxBytes)
	n, _ := f.Read(buf)
	return string(buf[:n]) + "\n\n… [truncated, " + fmt.Sprintf("%d", info.Size()) + " bytes total]", nil
}
