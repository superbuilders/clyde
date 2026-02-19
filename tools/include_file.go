package tools

import (
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/this-is-alpha-iota/clyde/api"
)

func init() {
	Register(includeFileTool, executeIncludeFile, displayIncludeFile)
}

var includeFileTool = api.Tool{
	Name:        "include_file",
	Description: "Include a file's contents in the conversation. For images (jpg, png, gif, webp), this sends the image to Claude for vision analysis. Can load from local filesystem or remote URLs. Use this tool when the user asks you to look at, analyze, or work with a specific file.",
	InputSchema: map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "File path (local or URL). Examples: './screenshot.png', '/tmp/diagram.jpg', 'https://example.com/image.png'",
			},
		},
		"required": []string{"path"},
	},
}

func executeIncludeFile(input map[string]interface{}, apiClient *api.Client, history []api.Message) (string, error) {
	path, ok := input["path"].(string)
	if !ok || path == "" {
		return "", fmt.Errorf("path is required. Example: include_file(\"./screenshot.png\")")
	}

	// Determine if URL or local path
	isURL := strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://")

	// Check file type
	ext := strings.ToLower(filepath.Ext(path))
	isImage := ext == ".jpg" || ext == ".jpeg" || ext == ".png" ||
		ext == ".gif" || ext == ".webp"

	if isImage {
		return loadImage(path, isURL)
	}

	// For non-images, return error for now (future: support text files)
	return "", fmt.Errorf("only image files are currently supported (.jpg, .png, .gif, .webp). Got: %s", ext)
}

func loadImage(path string, isURL bool) (string, error) {
	var data []byte
	var err error
	var mediaType string

	if isURL {
		// Fetch from URL
		resp, err := http.Get(path)
		if err != nil {
			return "", fmt.Errorf("failed to fetch image from URL: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			return "", fmt.Errorf("URL returned status %d. Check if the URL is correct and accessible", resp.StatusCode)
		}

		mediaType = resp.Header.Get("Content-Type")
		if !isValidImageType(mediaType) {
			return "", fmt.Errorf("unsupported image type from URL: %s. Supported types: image/jpeg, image/png, image/webp, image/gif", mediaType)
		}

		data, err = io.ReadAll(resp.Body)
		if err != nil {
			return "", fmt.Errorf("failed to read image data from URL: %w", err)
		}
	} else {
		// Read local file
		data, err = os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				return "", fmt.Errorf("file '%s' not found. Use list_files or glob to find available files", path)
			}
			if os.IsPermission(err) {
				return "", fmt.Errorf("permission denied reading '%s'. Check file permissions", path)
			}
			return "", fmt.Errorf("failed to read file '%s': %w", path, err)
		}

		// Detect media type from extension
		ext := strings.ToLower(filepath.Ext(path))
		mediaType = detectMediaType(ext)
		if mediaType == "" {
			return "", fmt.Errorf("unsupported image format: %s. Supported formats: .jpg, .jpeg, .png, .gif, .webp", ext)
		}
	}

	// Check size (5MB limit per Claude API)
	sizeBytes := len(data)
	sizeMB := float64(sizeBytes) / (1024 * 1024)
	if sizeBytes > 5*1024*1024 {
		return "", fmt.Errorf("image too large (%.1f MB). Maximum is 5MB. Try resizing the image or using a different file", sizeMB)
	}

	// Encode to base64
	encoded := base64.StdEncoding.EncodeToString(data)

	// Return special marker that agent will recognize
	// Format: IMAGE_LOADED:<media_type>:<size_kb>:<base64_data>
	sizeKB := float64(sizeBytes) / 1024
	return fmt.Sprintf("IMAGE_LOADED:%s:%.1f:%s", mediaType, sizeKB, encoded), nil
}

func isValidImageType(mediaType string) bool {
	validTypes := []string{"image/jpeg", "image/png", "image/webp", "image/gif"}
	for _, valid := range validTypes {
		if mediaType == valid {
			return true
		}
	}
	return false
}

func detectMediaType(ext string) string {
	switch ext {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".webp":
		return "image/webp"
	case ".gif":
		return "image/gif"
	default:
		return ""
	}
}

func displayIncludeFile(input map[string]interface{}) string {
	path, _ := input["path"].(string)
	return fmt.Sprintf("→ Including file: %s", path)
}
