package main

import (
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"session-viewer/internal/principal"
)

// maxAttachmentBytes caps a single uploaded attachment. The bytes travel over
// the wire from the browser (the session viewer frequently runs on a different
// machine than the project), so an unbounded upload is both a memory and a
// disk hazard for every other user on the box.
const maxAttachmentBytes int64 = 25 << 20 // 25 MiB

// attachmentsSubdir is where uploads land, relative to the project/worktree
// cwd. A dedicated directory keeps the project root clean and makes the files
// trivially greppable and ignorable, while still living inside the tree the
// agent runs in so plain relative paths work.
var attachmentsSubdir = filepath.Join(".clyde", "attachments")

// errAttachmentTooLarge is returned when an upload exceeds maxAttachmentBytes.
type errAttachmentTooLarge struct{ Limit int64 }

func (e errAttachmentTooLarge) Error() string {
	return fmt.Sprintf("file exceeds the %s upload limit", humanBytes(e.Limit))
}

func humanBytes(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.0fGB", float64(n)/float64(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.0fMB", float64(n)/float64(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.0fKB", float64(n)/float64(1<<10))
	}
	return fmt.Sprintf("%dB", n)
}

// sanitizeAttachmentName reduces a client-supplied filename to a single safe
// path component. The client controls this string completely, so anything that
// could climb out of the attachments directory (separators, "..", NUL, drive
// prefixes) has to be destroyed here rather than merely checked for.
func sanitizeAttachmentName(name string) string {
	// Windows-style separators survive filepath.Base on Linux, so normalize
	// them first — "..\\..\\etc\\passwd" must not stay one component.
	name = strings.ReplaceAll(name, "\\", "/")
	name = filepath.Base(filepath.Clean("/" + name))

	var b strings.Builder
	for _, r := range name {
		switch {
		case r == os.PathSeparator || r == '/' || r == 0:
			// dropped
		case unicode.IsControl(r):
			// dropped
		case r == ' ':
			b.WriteByte('-')
		default:
			b.WriteRune(r)
		}
	}
	out := strings.Trim(b.String(), ".-")
	if out == "" || out == "." || out == ".." {
		out = "attachment"
	}
	if len(out) > 128 {
		ext := filepath.Ext(out)
		if len(ext) > 16 {
			ext = ""
		}
		out = out[:128-len(ext)] + ext
	}
	return out
}

// attachmentDir returns the absolute attachments directory for a project cwd,
// creating it as the principal. The directory must be owned by the user: the
// agent reads these files back as that user, and the service itself has no
// CAP_DAC_OVERRIDE.
func attachmentDir(pr *principal.Principal, cwd string) (string, error) {
	dir := filepath.Join(cwd, attachmentsSubdir)
	if err := pr.MkdirAs(dir); err != nil {
		return "", err
	}
	return dir, nil
}

// uniqueAttachmentPath picks a non-colliding path inside dir for name.
func uniqueAttachmentPath(dir, name string) (string, string) {
	candidate := name
	dst := filepath.Join(dir, candidate)
	if _, err := os.Stat(dst); err != nil {
		return dst, candidate
	}
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	for i := 1; ; i++ {
		candidate = fmt.Sprintf("%s-%d%s", base, i, ext)
		dst = filepath.Join(dir, candidate)
		if _, err := os.Stat(dst); os.IsNotExist(err) {
			return dst, candidate
		}
	}
}

// withinDir reports whether path is inside dir after symlink resolution of the
// parts that exist. This is the last line of defence against escaping the
// worktree; sanitizeAttachmentName is the first.
func withinDir(dir, path string) bool {
	resolve := func(p string) string {
		if r, err := filepath.EvalSymlinks(p); err == nil {
			return r
		}
		return filepath.Clean(p)
	}
	d := resolve(dir)
	p := filepath.Clean(path)
	if r, err := filepath.EvalSymlinks(filepath.Dir(p)); err == nil {
		p = filepath.Join(r, filepath.Base(p))
	}
	rel, err := filepath.Rel(d, p)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}

// savedAttachment describes a file written into the project tree.
type savedAttachment struct {
	Filename string `json:"filename"` // base name as written
	Path     string `json:"path"`     // path relative to the project cwd
	AbsPath  string `json:"abs_path"` // absolute path on the agent's machine
	Size     int64  `json:"size"`
}

// saveAttachment streams src into the project's attachments directory as the
// principal and returns where it landed. declaredSize, when > 0, lets an
// oversized upload be rejected before a byte is written.
func saveAttachment(pr *principal.Principal, cwd, filename string, src io.Reader, declaredSize int64) (*savedAttachment, error) {
	if declaredSize > maxAttachmentBytes {
		return nil, errAttachmentTooLarge{Limit: maxAttachmentBytes}
	}
	dir, err := attachmentDir(pr, cwd)
	if err != nil {
		return nil, fmt.Errorf("create attachments directory: %w", err)
	}
	name := sanitizeAttachmentName(filename)
	dst, name := uniqueAttachmentPath(dir, name)
	if !withinDir(cwd, dst) || !withinDir(dir, dst) {
		return nil, fmt.Errorf("refusing to write outside the project directory")
	}

	// Count as we copy: a multipart part can lie about (or omit) its length,
	// so the limit has to hold on the actual stream too.
	counted := &countingReader{r: io.LimitReader(src, maxAttachmentBytes+1)}
	if err := pr.WriteFrom(dst, counted); err != nil {
		return nil, err
	}
	if counted.n > maxAttachmentBytes {
		_ = pr.RemoveAs(dst)
		return nil, errAttachmentTooLarge{Limit: maxAttachmentBytes}
	}
	rel := filepath.Join(attachmentsSubdir, name)
	return &savedAttachment{Filename: name, Path: rel, AbsPath: dst, Size: counted.n}, nil
}

// saveBase64Attachment decodes a base64 payload sent inline with a chat message
// and writes it into the project tree.
func saveBase64Attachment(pr *principal.Principal, cwd, filename, b64 string) (*savedAttachment, error) {
	// Accept data: URLs too — that is what FileReader.readAsDataURL produces.
	if i := strings.Index(b64, ";base64,"); i >= 0 {
		b64 = b64[i+len(";base64,"):]
	}
	b64 = strings.TrimSpace(b64)
	// 4 base64 chars per 3 bytes: reject before allocating the decode buffer.
	if int64(len(b64))/4*3 > maxAttachmentBytes {
		return nil, errAttachmentTooLarge{Limit: maxAttachmentBytes}
	}
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		raw, err = base64.URLEncoding.DecodeString(b64)
		if err != nil {
			return nil, fmt.Errorf("invalid base64 content")
		}
	}
	return saveAttachment(pr, cwd, filename, strings.NewReader(string(raw)), int64(len(raw)))
}

type countingReader struct {
	r io.Reader
	n int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}

// attachmentNote renders the line prepended to a chat message so the agent
// knows exactly where the uploaded bytes live on its own filesystem.
func attachmentNote(saved []*savedAttachment) string {
	if len(saved) == 0 {
		return ""
	}
	var b strings.Builder
	for _, s := range saved {
		fmt.Fprintf(&b, "include ./%s\n", filepath.ToSlash(s.Path))
	}
	return b.String()
}
