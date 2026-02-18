package tools

import (
	"github.com/this-is-alpha-iota/clyde/api"
	"fmt"
	"os/exec"
	"strings"
)

func init() {
	Register(runBashTool, executeRunBash, displayRunBash)
}

var runBashTool = api.Tool{
	Name:        "run_bash",
	Description: "Execute arbitrary bash commands and return the output. Use this for running shell commands, scripts, or any command-line operations.",
	InputSchema: map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"command": map[string]interface{}{
				"type":        "string",
				"description": "The bash command to execute. Can be any valid bash command or script.",
			},
		},
		"required": []string{"command"},
	},
}

func executeRunBash(input map[string]interface{}, apiClient *api.Client, conversationHistory []api.Message) (string, error) {
	command, ok := input["command"].(string)
	if !ok || command == "" {
		return "", fmt.Errorf("command is required. Example: run_bash(\"ls -la\")")
	}

	cmd := exec.Command("bash", "-c", command)
	output, err := cmd.CombinedOutput()

	if err != nil {
		exitErr, ok := err.(*exec.ExitError)
		if ok {
			exitCode := exitErr.ExitCode()
			suggestions := []string{
				fmt.Sprintf("Command failed with exit code %d: %s", exitCode, command),
				"",
				"Output:",
				string(output),
			}

			// Add context-specific suggestions
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
				// Common exit code, try to provide context based on command
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

	return string(output), nil
}

func displayRunBash(input map[string]interface{}) string {
	command, _ := input["command"].(string)
	
	// Truncate long commands for display
	displayCmd := command
	if len(displayCmd) > 60 {
		displayCmd = displayCmd[:57] + "..."
	}
	return fmt.Sprintf("→ Running bash: %s", displayCmd)
}
