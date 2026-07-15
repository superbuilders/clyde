package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/superbuilders/clyde/agent/mcp"
)

// --- Fix: .png files should get saved to gitignored folder by default ---

// TestScreenshotDir_Constant verifies the screenshot directory constant.
func TestScreenshotDir_Constant(t *testing.T) {
	if mcp.ScreenshotDir != ".clyde/screenshots" {
		t.Errorf("ScreenshotDir = %q, want %q", mcp.ScreenshotDir, ".clyde/screenshots")
	}
}

// TestRedirectScreenshotFilename_NoFilename verifies that when no filename
// is provided, one is generated inside the screenshots directory.
func TestRedirectScreenshotFilename_NoFilename(t *testing.T) {
	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(origDir)

	input := map[string]interface{}{
		"type": "png",
	}
	result := mcp.RedirectScreenshotFilename(input)

	filename, ok := result["filename"].(string)
	if !ok || filename == "" {
		t.Fatal("expected filename to be set")
	}
	if !strings.HasPrefix(filename, mcp.ScreenshotDir) {
		t.Errorf("filename %q should start with %q", filename, mcp.ScreenshotDir)
	}
	if !strings.HasSuffix(filename, ".png") {
		t.Errorf("filename %q should end with .png", filename)
	}
}

// TestRedirectScreenshotFilename_NoFilenameJpeg verifies jpeg extension
// is used when type is "jpeg".
func TestRedirectScreenshotFilename_NoFilenameJpeg(t *testing.T) {
	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(origDir)

	input := map[string]interface{}{
		"type": "jpeg",
	}
	result := mcp.RedirectScreenshotFilename(input)

	filename, _ := result["filename"].(string)
	if !strings.HasSuffix(filename, ".jpeg") {
		t.Errorf("filename %q should end with .jpeg for jpeg type", filename)
	}
}

// TestRedirectScreenshotFilename_RelativePath verifies that a relative
// filename is redirected into the screenshots directory.
func TestRedirectScreenshotFilename_RelativePath(t *testing.T) {
	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(origDir)

	input := map[string]interface{}{
		"filename": "my-screenshot.png",
	}
	result := mcp.RedirectScreenshotFilename(input)

	filename, _ := result["filename"].(string)
	expected := filepath.Join(mcp.ScreenshotDir, "my-screenshot.png")
	if filename != expected {
		t.Errorf("filename = %q, want %q", filename, expected)
	}
}

// TestRedirectScreenshotFilename_AbsolutePath verifies that absolute
// paths are left unchanged (respecting explicit user intent).
func TestRedirectScreenshotFilename_AbsolutePath(t *testing.T) {
	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(origDir)

	absPath := "/tmp/my-screenshot.png"
	input := map[string]interface{}{
		"filename": absPath,
	}
	result := mcp.RedirectScreenshotFilename(input)

	filename, _ := result["filename"].(string)
	if filename != absPath {
		t.Errorf("absolute path should be unchanged: got %q, want %q", filename, absPath)
	}
}

// TestRedirectScreenshotFilename_AlreadyInDir verifies that files already
// targeted at the screenshots directory are not double-prefixed.
func TestRedirectScreenshotFilename_AlreadyInDir(t *testing.T) {
	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(origDir)

	path := filepath.Join(mcp.ScreenshotDir, "already-there.png")
	input := map[string]interface{}{
		"filename": path,
	}
	result := mcp.RedirectScreenshotFilename(input)

	filename, _ := result["filename"].(string)
	if filename != path {
		t.Errorf("should not double-prefix: got %q, want %q", filename, path)
	}
}

// TestRedirectScreenshotFilename_CreatesDirectory verifies that the
// screenshots directory is created if it doesn't exist.
func TestRedirectScreenshotFilename_CreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(origDir)

	input := map[string]interface{}{}
	mcp.RedirectScreenshotFilename(input)

	if _, err := os.Stat(mcp.ScreenshotDir); os.IsNotExist(err) {
		t.Error("screenshots directory should be created by redirect")
	}
}

// TestGitignoreContainsScreenshots verifies that .gitignore includes
// the screenshots directory pattern.
func TestGitignoreContainsScreenshots(t *testing.T) {
	content, err := os.ReadFile(".gitignore")
	if err != nil {
		t.Skipf("no .gitignore found: %v", err)
	}
	if !strings.Contains(string(content), ".clyde/screenshots/") {
		t.Error(".gitignore should contain .clyde/screenshots/")
	}
}
