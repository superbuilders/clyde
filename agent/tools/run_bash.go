package tools

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/this-is-alpha-iota/clyde/agent/providers"
)

func init() {
	Register(runBashTool, executeRunBash, displayRunBash)
}

// defaultBashTimeout is the maximum time a run_bash command can execute.
// Commands exceeding this are killed and return a timeout error.
const defaultBashTimeout = 120 * time.Second

var runBashTool = providers.Tool{
	Name: "run_bash",
	Description: "Execute arbitrary bash commands and return the output. " +
		"Commands have a 120-second timeout. " +
		"IMPORTANT: Do NOT use '&' for background processes — it will hang and timeout. " +
		"For servers, daemons, or any long-running process, use tmux: " +
		"run_bash(\"tmux new-session -d -s myserver 'python3 -m http.server 8080'\") " +
		"then run_bash(\"curl http://localhost:8080\") to interact with it, " +
		"and run_bash(\"tmux kill-session -t myserver\") to stop it.",
	InputSchema: map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"command": map[string]interface{}{
				"type":        "string",
				"description": "The bash command to execute. For background/long-running processes use tmux instead of &.",
			},
		},
		"required": []string{"command"},
	},
}

func executeRunBash(input map[string]interface{}, apiClient providers.Provider, conversationHistory []providers.Message) (string, error) {
	command, ok := input["command"].(string)
	if !ok || command == "" {
		return "", fmt.Errorf("command is required. Example: run_bash(\"ls -la\")")
	}

	ctx, cancel := context.WithTimeout(context.Background(), defaultBashTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-c", command)

	// Put the command in its own process group so we can kill all children
	// (including processes spawned with &) on timeout.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Start()
	if err != nil {
		return "", fmt.Errorf("failed to start command '%s': %w", command, err)
	}

	// Wait for completion or timeout
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case <-ctx.Done():
		// Timeout — kill the entire process group
		if cmd.Process != nil {
			syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		// Drain the wait goroutine
		<-done

		output := stdout.String() + stderr.String()
		return "", fmt.Errorf("Command timed out after %s: %s\n\n"+
			"Output before timeout:\n%s\n\n"+
			"If this is a server or long-running process, use tmux instead:\n"+
			"  run_bash(\"tmux new-session -d -s myserver '%s'\")\n"+
			"  run_bash(\"tmux kill-session -t myserver\")  # when done",
			defaultBashTimeout, command, output, command)

	case err := <-done:
		output := stdout.String() + stderr.String()

		if err != nil {
			exitErr, ok := err.(*exec.ExitError)
			if ok {
				exitCode := exitErr.ExitCode()
				suggestions := []string{
					fmt.Sprintf("Command failed with exit code %d: %s", exitCode, command),
					"",
					"Output:",
					output,
				}

				if exitCode == 127 {
					suggestions = append(suggestions,
						"",
						"Exit code 127 typically means 'command not found'.",
						"Suggestions:",
						"  - Check if the command is installed",
						"  - Verify the command name is spelled correctly",
						"  - Try which <command> to see if it's in PATH",
					)
				} else if exitCode == 126 {
					suggestions = append(suggestions,
						"",
						"Exit code 126 typically means 'permission denied'.",
						"Suggestions:",
						"  - Check file/script permissions",
						"  - Try: chmod +x <script>",
					)
				} else if exitCode == 1 {
					if strings.Contains(command, "test") {
						suggestions = append(suggestions,
							"",
							"This may indicate test failures. Check the output above for details.",
						)
					} else if strings.Contains(command, "git") {
						suggestions = append(suggestions,
							"",
							"Git command failed. Check the output above for details.",
							"Common issues: uncommitted changes, merge conflicts, or invalid references.",
						)
					}
				}

				return "", fmt.Errorf("%s", strings.Join(suggestions, "\n"))
			}
			return "", fmt.Errorf("failed to execute command '%s': %w", command, err)
		}

		return output, nil
	}
}

func displayRunBash(input map[string]interface{}) string {
	command, _ := input["command"].(string)
	return fmt.Sprintf("→ Running bash: %s", command)
}
