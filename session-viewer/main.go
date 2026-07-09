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
	Running      bool   `json:"running"`
	PID          int    `json:"pid,omitempty"`
	Preview      string `json:"preview"`
}

type MessageFile struct {
	Filename  string `json:"filename"`
	Timestamp string `json:"timestamp"`
	Type      string `json:"type"`
	Content   string `json:"content"`
}

func main() {
	e := echo.New()
	e.Use(middleware.Logger())
	e.Use(middleware.CORS())

	api := e.Group("/api")
	api.GET("/sessions", getSessions)
	api.GET("/sessions/:id/messages", getSessionMessages)

	// Serve static files
	staticFS, _ := fs.Sub(staticFiles, "static")
	e.GET("/*", echo.WrapHandler(http.FileServer(http.FS(staticFS))))

	fmt.Println("🔍 Session Viewer running at http://localhost:8787")
	e.Logger.Fatal(e.Start(":8787"))
}

func getRunningProcesses() []RunningProcess {
	var processes []RunningProcess

	out, err := exec.Command("pgrep", "-x", "clyde").Output()
	if err != nil {
		return processes
	}

	pids := strings.Fields(strings.TrimSpace(string(out)))

	for _, pidStr := range pids {
		pid, err := strconv.Atoi(pidStr)
		if err != nil {
			continue
		}

		proc := RunningProcess{PID: pid}

		psOut, err := exec.Command("ps", "-o", "tty=,lstart=,args=", "-p", pidStr).Output()
		if err != nil {
			continue
		}

		line := strings.TrimSpace(string(psOut))
		fields := strings.Fields(line)
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

func parseSessionTimestamp(sessionID string) (time.Time, error) {
	parts := strings.SplitN(sessionID, "_", 2)
	if len(parts) < 1 {
		return time.Time{}, fmt.Errorf("invalid session ID")
	}
	tsStr := parts[0]
	tsStr = strings.Replace(tsStr, "T", " ", 1)
	spaceParts := strings.SplitN(tsStr, " ", 2)
	if len(spaceParts) == 2 {
		timePart := strings.Replace(spaceParts[1], "-", ":", 2)
		tsStr = spaceParts[0] + " " + timePart
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

// getSessionPreview reads the first user message and returns a short preview
func getSessionPreview(sessionPath string) string {
	entries, err := os.ReadDir(sessionPath)
	if err != nil {
		return ""
	}

	// Sort by name (chronological)
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), "_user.md") {
			data, err := readFileCapped(filepath.Join(sessionPath, entry.Name()), 1024)
			if err != nil {
				return ""
			}
			// Strip the "**You:**\n" prefix
			content := strings.TrimSpace(data)
			content = strings.TrimPrefix(content, "**You:**")
			content = strings.TrimPrefix(content, "**You:**\n")
			content = strings.TrimSpace(content)
			// Collapse whitespace and truncate
			content = strings.Join(strings.Fields(content), " ")
			if len(content) > 140 {
				content = content[:140] + "…"
			}
			return content
		}
	}
	return ""
}

// cleanThinkingContent strips signature blocks from thinking messages
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

func getSessions(c echo.Context) error {
	processes := getRunningProcesses()

	cwdSet := make(map[string]bool)
	for _, p := range processes {
		if p.CWD != "" {
			cwdSet[p.CWD] = true
		}
	}

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
				if line == "" {
					continue
				}
				projectDir := filepath.Dir(line)
				cwdSet[projectDir] = true
			}
		}
	}

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

			preview := getSessionPreview(sessionPath)

			sess := SessionInfo{
				ID:           sessionID,
				User:         user,
				Timestamp:    parts[0],
				MessageCount: msgCount,
				LastModified: lastModStr,
				CWD:          dir,
				Project:      project,
				Running:      running,
				PID:          runPID,
				Preview:      preview,
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

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	var messages []MessageFile
	typesFilter := c.QueryParam("types")
	allowedTypes := map[string]bool{
		"user":      true,
		"assistant": true,
		"thinking":  true,
	}
	if typesFilter != "" {
		allowedTypes = make(map[string]bool)
		for _, t := range strings.Split(typesFilter, ",") {
			allowedTypes[strings.TrimSpace(t)] = true
		}
	}

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

		filePath := filepath.Join(sessionPath, name)
		content, err := readFileCapped(filePath, 50*1024)
		if err != nil {
			continue
		}

		// Strip signature blocks from thinking messages
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
		if err != nil {
			return "", err
		}
		return string(data), nil
	}

	buf := make([]byte, maxBytes)
	n, err := f.Read(buf)
	if err != nil {
		return "", err
	}
	return string(buf[:n]) + "\n\n… [truncated, " + fmt.Sprintf("%d", info.Size()) + " bytes total]", nil
}
