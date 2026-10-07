package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func uploadMultipart(t *testing.T, baseURL, cwd, filename string, content []byte) (int, []byte) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	w.WriteField("cwd", cwd)
	fw, err := w.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	fw.Write(content)
	w.Close()
	resp, err := http.Post(baseURL+"/api/upload", w.FormDataContentType(), &buf)
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	defer resp.Body.Close()
	var out bytes.Buffer
	out.ReadFrom(resp.Body)
	return resp.StatusCode, out.Bytes()
}

func decodeSaved(t *testing.T, body []byte) savedAttachment {
	t.Helper()
	var s savedAttachment
	if err := json.Unmarshal(body, &s); err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
	return s
}

func TestUpload_WritesIntoAttachmentsDir(t *testing.T) {
	home := t.TempDir()
	proj := filepath.Join(home, "code", "proj")
	os.MkdirAll(filepath.Join(proj, ".clyde", "sessions"), 0755)
	baseURL, cleanup := startTestServer(t, home)
	defer cleanup()

	status, body := uploadMultipart(t, baseURL, proj, "my notes.txt", []byte("hello"))
	if status != http.StatusOK {
		t.Fatalf("status %d: %s", status, body)
	}
	saved := decodeSaved(t, body)
	if saved.Path != filepath.Join(".clyde", "attachments", "my-notes.txt") {
		t.Fatalf("unexpected path %q", saved.Path)
	}
	got, err := os.ReadFile(filepath.Join(proj, saved.Path))
	if err != nil || string(got) != "hello" {
		t.Fatalf("file not written: %v %q", err, got)
	}
}

func TestUpload_CollisionGetsSuffix(t *testing.T) {
	home := t.TempDir()
	proj := filepath.Join(home, "code", "proj2")
	os.MkdirAll(filepath.Join(proj, ".clyde", "sessions"), 0755)
	baseURL, cleanup := startTestServer(t, home)
	defer cleanup()

	_, b1 := uploadMultipart(t, baseURL, proj, "a.txt", []byte("one"))
	status, b2 := uploadMultipart(t, baseURL, proj, "a.txt", []byte("two"))
	if status != http.StatusOK {
		t.Fatalf("status %d: %s", status, b2)
	}
	s1, s2 := decodeSaved(t, b1), decodeSaved(t, b2)
	if s1.Filename != "a.txt" || s2.Filename != "a-1.txt" {
		t.Fatalf("collision not handled: %q %q", s1.Filename, s2.Filename)
	}
	got, _ := os.ReadFile(filepath.Join(proj, s1.Path))
	if string(got) != "one" {
		t.Fatalf("first file clobbered: %q", got)
	}
}

func TestUpload_PathTraversalIsContained(t *testing.T) {
	home := t.TempDir()
	proj := filepath.Join(home, "code", "proj3")
	os.MkdirAll(filepath.Join(proj, ".clyde", "sessions"), 0755)
	baseURL, cleanup := startTestServer(t, home)
	defer cleanup()

	for _, name := range []string{"../../escaped.txt", "..\\..\\escaped.txt", "/etc/escaped.txt"} {
		status, body := uploadMultipart(t, baseURL, proj, name, []byte("x"))
		if status != http.StatusOK {
			t.Fatalf("status %d: %s", status, body)
		}
		saved := decodeSaved(t, body)
		if strings.Contains(saved.Path, "..") {
			t.Fatalf("path escaped: %q", saved.Path)
		}
		if !strings.HasPrefix(saved.AbsPath, filepath.Join(proj, ".clyde", "attachments")) {
			t.Fatalf("wrote outside project: %q", saved.AbsPath)
		}
	}
	if _, err := os.Stat(filepath.Join(home, "escaped.txt")); err == nil {
		t.Fatal("file escaped the project directory")
	}
}

func TestUpload_SizeLimitRejected(t *testing.T) {
	home := t.TempDir()
	proj := filepath.Join(home, "code", "proj4")
	os.MkdirAll(filepath.Join(proj, ".clyde", "sessions"), 0755)
	baseURL, cleanup := startTestServer(t, home)
	defer cleanup()

	big := bytes.Repeat([]byte("a"), int(maxAttachmentBytes)+1024)
	status, body := uploadMultipart(t, baseURL, proj, "big.bin", big)
	if status != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d: %s", status, body)
	}
	if !strings.Contains(string(body), "limit") {
		t.Fatalf("unclear error: %s", body)
	}
}

func TestUpload_Base64JSONPayload(t *testing.T) {
	home := t.TempDir()
	proj := filepath.Join(home, "code", "proj5")
	os.MkdirAll(filepath.Join(proj, ".clyde", "sessions"), 0755)
	baseURL, cleanup := startTestServer(t, home)
	defer cleanup()

	payload := fmt.Sprintf(`{"cwd":%q,"filename":"pic.png","content_base64":%q}`,
		proj, base64.StdEncoding.EncodeToString([]byte("PNGDATA")))
	status, body := httpPost(t, baseURL+"/api/upload", payload)
	if status != http.StatusOK {
		t.Fatalf("status %d: %s", status, body)
	}
	saved := decodeSaved(t, body)
	got, err := os.ReadFile(filepath.Join(proj, saved.Path))
	if err != nil || string(got) != "PNGDATA" {
		t.Fatalf("bytes not written: %v %q", err, got)
	}
}

func TestSanitizeAttachmentName(t *testing.T) {
	cases := map[string]string{
		"a b.txt":            "a-b.txt",
		"../../etc/passwd":   "passwd",
		"..\\..\\boot.ini":   "boot.ini",
		"":                   "attachment",
		"..":                 "attachment",
		"/absolute/path.png": "path.png",
	}
	for in, want := range cases {
		if got := sanitizeAttachmentName(in); got != want {
			t.Errorf("sanitize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAttachmentNoteReferencesOnDiskPath(t *testing.T) {
	note := attachmentNote([]*savedAttachment{{Path: filepath.Join(".clyde", "attachments", "a.txt")}})
	if !strings.Contains(note, ".clyde/attachments/a.txt") {
		t.Fatalf("note does not reference path: %q", note)
	}
}
