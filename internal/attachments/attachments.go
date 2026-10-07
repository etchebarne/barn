// Package attachments stores the files attached to messages. They live under the shared
// folder (<data>/shared/attachments/<id>/<name>), which every sandbox mounts at /shared, so
// agents can open them with their tools.
package attachments

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/gif" // register decoders for DecodeConfig
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	_ "golang.org/x/image/webp"

	"github.com/etchebarne/openbot/internal/ids"
	"github.com/etchebarne/openbot/internal/store"
)

// MaxSize is the largest file openbot accepts.
const MaxSize = 25 << 20

// ErrTooLarge means a file is over MaxSize.
var ErrTooLarge = errors.New("the file is over 25 MB")

// Files stores attachment bytes on disk and their metadata in the store.
type Files struct {
	Dir   string // <data>/shared/attachments
	Store *store.Store
}

// Path is where an attachment's bytes are on this machine.
func (f *Files) Path(a store.Attachment) string { return filepath.Join(f.Dir, a.ID, a.Name) }

// SandboxPath is where agents find it in their sandbox.
func SandboxPath(a store.Attachment) string { return "/shared/attachments/" + a.ID + "/" + a.Name }

// Save stores r as a new, unsent attachment in chatID.
func (f *Files) Save(ctx context.Context, chatID, name string, r io.Reader) (store.Attachment, error) {
	a := store.Attachment{ID: ids.New(), ChatID: chatID, Name: SafeName(name)}
	dir := filepath.Join(f.Dir, a.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return a, err
	}
	out, err := os.Create(filepath.Join(dir, a.Name))
	if err != nil {
		return a, err
	}
	// Keep the start for sniffing the type and reading image dimensions.
	var head bytes.Buffer
	n, err := io.Copy(io.MultiWriter(out, &limitedBuffer{buf: &head, max: 1 << 20}), io.LimitReader(r, MaxSize+1))
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err == nil && n > MaxSize {
		err = ErrTooLarge
	}
	if err != nil {
		os.RemoveAll(dir)
		return a, err
	}
	a.Size = n
	a.Mime = detectType(a.Name, head.Bytes())
	if strings.HasPrefix(a.Mime, "image/") {
		if cfg, _, err := image.DecodeConfig(bytes.NewReader(head.Bytes())); err == nil {
			a.Width, a.Height = &cfg.Width, &cfg.Height
		}
	}
	saved, err := f.Store.CreateAttachment(ctx, a)
	if err != nil {
		os.RemoveAll(dir)
	}
	return saved, err
}

// detectType sniffs the content, falling back to the extension for formats sniffing doesn't
// know (text files of many kinds, office documents).
func detectType(name string, head []byte) string {
	sniffed := http.DetectContentType(head)
	if base, _, _ := strings.Cut(sniffed, ";"); base != "application/octet-stream" && base != "text/plain" {
		return base
	}
	if byExt := mime.TypeByExtension(strings.ToLower(filepath.Ext(name))); byExt != "" {
		base, _, _ := strings.Cut(byExt, ";")
		return base
	}
	base, _, _ := strings.Cut(sniffed, ";")
	return base
}

// SafeName keeps a file name usable on disk and in a sandbox: no directories, no control or
// path characters, at most 120 characters.
func SafeName(name string) string {
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	var b strings.Builder
	for _, r := range name {
		switch {
		case unicode.IsControl(r), strings.ContainsRune(`/\:*?"<>|`, r):
			b.WriteRune('_')
		default:
			b.WriteRune(r)
		}
	}
	s := strings.TrimSpace(b.String())
	if s == "" || s == "." || s == ".." {
		s = "file"
	}
	if r := []rune(s); len(r) > 120 {
		ext := filepath.Ext(s)
		if len([]rune(ext)) > 20 {
			ext = ""
		}
		s = string(r[:120-len([]rune(ext))]) + ext
	}
	return s
}

// Inline reports whether a browser may show the file in a tab: images it can't run code in,
// and PDFs. Everything else (HTML, SVG, scripts…) is served as a download.
func Inline(mimeType string) bool {
	switch mimeType {
	case "image/png", "image/jpeg", "image/gif", "image/webp", "application/pdf":
		return true
	}
	return false
}

// IsImage reports whether models can be shown the file as an image.
func IsImage(mimeType string) bool {
	switch mimeType {
	case "image/png", "image/jpeg", "image/gif", "image/webp":
		return true
	}
	return false
}

// Clean removes uploads never sent within a day and files left without a record (e.g. after
// a chat was deleted). Run it now and then.
func (f *Files) Clean(ctx context.Context) {
	stale, err := f.Store.StaleUploads(ctx, time.Now().Add(-24*time.Hour).UnixMilli())
	if err != nil {
		slog.Warn("attachments cleanup", "err", err)
		return
	}
	for _, a := range stale {
		_ = f.Store.DeleteAttachment(ctx, a.ID)
		os.RemoveAll(filepath.Join(f.Dir, a.ID))
	}
	known, err := f.Store.AttachmentIDs(ctx)
	if err != nil {
		return
	}
	entries, _ := os.ReadDir(f.Dir)
	for _, e := range entries {
		if !e.IsDir() || known[e.Name()] {
			continue
		}
		// Leave very new folders alone: an upload may be between writing and recording.
		if info, err := e.Info(); err == nil && time.Since(info.ModTime()) < time.Hour {
			continue
		}
		os.RemoveAll(filepath.Join(f.Dir, e.Name()))
	}
}

// Read returns an attachment's bytes (for images shown to models).
func (f *Files) Read(a store.Attachment) ([]byte, error) {
	if a.Size > MaxSize {
		return nil, fmt.Errorf("%s is too large", a.Name)
	}
	return os.ReadFile(f.Path(a))
}

// limitedBuffer keeps the first max bytes written to it.
type limitedBuffer struct {
	buf *bytes.Buffer
	max int
}

func (l *limitedBuffer) Write(p []byte) (int, error) {
	if room := l.max - l.buf.Len(); room > 0 {
		if len(p) > room {
			l.buf.Write(p[:room])
		} else {
			l.buf.Write(p)
		}
	}
	return len(p), nil
}
