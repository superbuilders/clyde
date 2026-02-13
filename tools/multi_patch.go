package tools

import (
	"claude-repl/api"
	"fmt"
	"os/exec"
	"strings"
)

func init() {
	Register(multiPatchTool, executeMultiPatch, displayMultiPatch)
}

var multiPatchTool = api.Tool{
	Name:        "multi_patch",
	Description: "Apply coordinated changes to multiple files atomically. If any patch fails, all previous changes are rolled back using git. Best for refactoring function names, updating imports, or applying consistent changes across files.",
	InputSchema: map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"patches": map[string]interface{}{
				"type":        "array",
				"description": "Array of patches to apply to different files",
				"items": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"path": map[string]interface{}{
							"type":        "string",
							"description": "The file path to patch",
						},
						"old_text": map[string]interface{}{
							"type":        "string",
							"description": "The exact text to find and replace in this file",
						},
						"new_text": map[string]interface{}{
							"type":        "string",
							"description": "The new text to replace old_text with",
						},
					},
					"required": []string{"path", "old_text", "new_text"},
				},
			},
		},
		"required": []string{"patches"},
	},
}

type patchInfo struct {
	Path    string
	OldText string
	NewText string
}

func executeMultiPatch(input map[string]interface{}, apiClient *api.Client, conversationHistory []api.Message) (string, error) {
	patches, ok := input["patches"].([]interface{})
	if !ok || len(patches) == 0 {
		return "", fmt.Errorf("multi_patch requires at least one patch. Example: {\"patches\": [{\"path\": \"file.go\", \"old_text\": \"...\", \"new_text\": \"...\"}]}")
	}

	// Parse patches
	var parsedPatches []patchInfo
	for i, p := range patches {
		patchMap, ok := p.(map[string]interface{})
		if !ok {
			return "", fmt.Errorf("patch %d is not a valid object", i+1)
		}

		path, pathOk := patchMap["path"].(string)
		oldText, oldOk := patchMap["old_text"].(string)
		newText, newOk := patchMap["new_text"].(string)

		if !pathOk || path == "" {
			return "", fmt.Errorf("patch %d is missing 'path' parameter", i+1)
		}
		if !oldOk {
			return "", fmt.Errorf("patch %d is missing 'old_text' parameter", i+1)
		}
		if !newOk {
			return "", fmt.Errorf("patch %d is missing 'new_text' parameter", i+1)
		}

		parsedPatches = append(parsedPatches, patchInfo{
			Path:    path,
			OldText: oldText,
			NewText: newText,
		})
	}

	// Check if git is available and we're in a git repo
	gitAvailable := false
	if cmd := exec.Command("git", "rev-parse", "--git-dir"); cmd.Run() == nil {
		gitAvailable = true
	}

	// Check for uncommitted changes and suggest commit if git available
	if gitAvailable {
		cmd := exec.Command("git", "status", "--porcelain")
		output, err := cmd.Output()
		if err == nil && len(output) > 0 {
			// There are uncommitted changes - suggest committing first
			suggestions := []string{
				"⚠️  You have uncommitted changes.",
				"",
				"It's recommended to commit your changes before applying multiple patches.",
				"This allows you to easily undo if something goes wrong.",
				"",
				"To commit first:",
				"  1. Review changes: run_bash(\"git status\")",
				"  2. Commit changes: run_bash(\"git add -A && git commit -m 'Before multi-patch'\")",
				"  3. Then run multi_patch again",
				"",
				"Or, if you're sure, I can proceed anyway.",
				"",
				fmt.Sprintf("Ready to apply %d patches to:", len(parsedPatches)),
			}
			for i, patch := range parsedPatches {
				suggestions = append(suggestions, fmt.Sprintf("  %d. %s", i+1, patch.Path))
			}

			// For now, we'll proceed but with a warning in the output
			// In a future version, we could add confirmation logic
			return strings.Join(suggestions, "\n"), nil
		}
	}

	// Apply patches
	var results []string
	var appliedPatches []patchInfo

	for i, patch := range parsedPatches {
		// Create input for executePatchFile
		patchInput := map[string]interface{}{
			"path":     patch.Path,
			"old_text": patch.OldText,
			"new_text": patch.NewText,
		}
		
		result, err := executePatchFile(patchInput, apiClient, conversationHistory)
		if err != nil {
			// Patch failed - attempt rollback if git available
			failureMsg := []string{
				fmt.Sprintf("❌ Patch %d/%d FAILED: %s", i+1, len(parsedPatches), patch.Path),
				fmt.Sprintf("Error: %v", err),
				"",
			}

			if gitAvailable && len(appliedPatches) > 0 {
				failureMsg = append(failureMsg,
					fmt.Sprintf("Rolling back %d successful patches...", len(appliedPatches)),
				)

				// Attempt to restore each file that was changed
				var rollbackErrors []string
				for _, applied := range appliedPatches {
					if restoreErr := exec.Command("git", "checkout", "--", applied.Path).Run(); restoreErr != nil {
						rollbackErrors = append(rollbackErrors, fmt.Sprintf("  - Failed to restore %s: %v", applied.Path, restoreErr))
					}
				}

				if len(rollbackErrors) > 0 {
					failureMsg = append(failureMsg,
						"⚠️  Some rollback operations failed:",
					)
					failureMsg = append(failureMsg, rollbackErrors...)
					failureMsg = append(failureMsg,
						"",
						"You may need to manually restore these files with:",
						"  git checkout -- <file>",
					)
				} else {
					failureMsg = append(failureMsg,
						"✓ Successfully rolled back all changes",
					)
				}
			} else if len(appliedPatches) > 0 {
				failureMsg = append(failureMsg,
					fmt.Sprintf("⚠️  %d patches were applied before this failure:", len(appliedPatches)),
				)
				for _, applied := range appliedPatches {
					failureMsg = append(failureMsg, fmt.Sprintf("  - %s", applied.Path))
				}
				if gitAvailable {
					failureMsg = append(failureMsg,
						"",
						"To undo these changes, run:",
						"  git checkout -- <files>",
					)
				} else {
					failureMsg = append(failureMsg,
						"",
						"You may need to manually undo these changes",
					)
				}
			}

			return "", fmt.Errorf("%s", strings.Join(failureMsg, "\n"))
		}

		appliedPatches = append(appliedPatches, patch)
		results = append(results, fmt.Sprintf("✓ Patch %d/%d: %s", i+1, len(parsedPatches), result))
	}

	// All patches succeeded
	summary := []string{
		fmt.Sprintf("✅ Successfully applied all %d patches:", len(parsedPatches)),
		"",
	}
	summary = append(summary, results...)

	if gitAvailable {
		summary = append(summary,
			"",
			"Next steps:",
			"  - Review changes: run_bash(\"git diff\")",
			"  - Commit changes: run_bash(\"git add -A && git commit -m 'Applied multi-patch'\")",
		)
	}

	return strings.Join(summary, "\n"), nil
}

func displayMultiPatch(input map[string]interface{}) string {
	patches, ok := input["patches"].([]interface{})
	if !ok {
		return "→ Applying multi-patch"
	}
	return fmt.Sprintf("→ Applying multi-patch: %d files", len(patches))
}
