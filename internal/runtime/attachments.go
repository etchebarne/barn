package runtime

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/etchebarne/barn/internal/attachments"
	"github.com/etchebarne/barn/internal/model"
	"github.com/etchebarne/barn/internal/store"
)

// How attachments reach agents: every file is listed with its sandbox path; small text files
// are included; images are shown to the model (the last few only, see loadImages).

const (
	inlineTextLimit = 32 << 10
	imagesInContext = 3 // most recent user messages whose images the model sees
)

// renderAttachments describes a message's attachments for the model and collects the images.
func (l *loop) renderAttachments(msg store.Message) (string, []string) {
	if len(msg.Attachments) == 0 || l.m.Files == nil {
		return "", nil
	}
	var b strings.Builder
	var images []string
	sandboxes := l.m.sandboxesAvailable()
	for _, a := range msg.Attachments {
		where := ""
		if sandboxes {
			where = fmt.Sprintf(" path=%q", attachments.SandboxPath(a))
		}
		attrs := fmt.Sprintf("name=%q type=%q size=%q%s", a.Name, a.Mime, humanSize(a.Size), where)
		switch {
		case attachments.IsImage(a.Mime):
			images = append(images, a.ID)
			fmt.Fprintf(&b, "\n<attachment %s/> (image, shown to you)", attrs)
		case isText(a) && a.Size <= inlineTextLimit:
			content, err := os.ReadFile(l.m.Files.Path(a))
			if err == nil && utf8.Valid(content) {
				fmt.Fprintf(&b, "\n<attachment %s>\n%s\n</attachment>", attrs, content)
				continue
			}
			fmt.Fprintf(&b, "\n<attachment %s/>", attrs)
		case sandboxes:
			fmt.Fprintf(&b, "\n<attachment %s/> (open it with your tools, e.g. pdftotext, python)", attrs)
		default:
			fmt.Fprintf(&b, "\n<attachment %s/> (you can't open this file: there's no sandbox on this server)", attrs)
		}
	}
	return b.String(), images
}

func isText(a store.Attachment) bool {
	if strings.HasPrefix(a.Mime, "text/") {
		return true
	}
	switch a.Mime {
	case "application/json", "application/xml", "application/yaml", "application/x-yaml", "application/toml",
		"application/javascript", "application/x-sh", "application/sql":
		return true
	}
	switch strings.ToLower(path.Ext(a.Name)) {
	case ".md", ".txt", ".csv", ".tsv", ".json", ".yaml", ".yml", ".toml", ".xml", ".log", ".go", ".py", ".js", ".ts",
		".tsx", ".jsx", ".rs", ".java", ".rb", ".sh", ".sql", ".html", ".css", ".ini", ".env":
		return true
	}
	return false
}

func humanSize(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%d KB", (n+512)>>10)
	}
	return fmt.Sprintf("%d bytes", n)
}

// loadImages fills in the image bytes of the most recent user messages that have images.
func (l *loop) loadImages(ctx context.Context, msgs []model.Message) {
	if l.m.Files == nil {
		return
	}
	left := imagesInContext
	for i := len(msgs) - 1; i >= 0 && left > 0; i-- {
		if len(msgs[i].ImageRefs) == 0 {
			continue
		}
		left--
		for _, id := range msgs[i].ImageRefs {
			a, err := l.m.store.GetAttachment(ctx, id)
			if err != nil {
				continue // deleted since
			}
			data, err := l.m.Files.Read(a)
			if err != nil {
				continue
			}
			msgs[i].Images = append(msgs[i].Images, model.Image{MediaType: a.Mime, Data: data})
		}
	}
}

// attachFiles stores files from the agent's computer as attachments in chatID, for
// send_message. Paths under /shared are read directly; others come out of the sandbox.
func (l *loop) attachFiles(ctx context.Context, agent store.Agent, chatID string, paths []string) ([]string, error) {
	if len(paths) == 0 {
		return nil, nil
	}
	if l.m.Files == nil {
		return nil, fmt.Errorf("attachments aren't available on this server")
	}
	if len(paths) > 10 {
		return nil, fmt.Errorf("at most 10 files per message")
	}
	var ids []string
	for _, p := range paths {
		p = sandboxPath(strings.TrimSpace(p))
		a, err := l.saveFile(ctx, agent, chatID, p)
		if err != nil {
			return nil, err
		}
		ids = append(ids, a.ID)
	}
	return ids, nil
}

func (l *loop) saveFile(ctx context.Context, agent store.Agent, chatID, p string) (store.Attachment, error) {
	name := path.Base(p)
	shared := filepath.Dir(l.m.Files.Dir) // <data>/shared
	if rel, ok := strings.CutPrefix(path.Clean(p), "/shared/"); ok {
		host := filepath.Join(shared, filepath.FromSlash(rel))
		if !strings.HasPrefix(host, shared+string(filepath.Separator)) {
			return store.Attachment{}, fmt.Errorf("%s is outside /shared", p)
		}
		f, err := os.Open(host)
		if err != nil {
			return store.Attachment{}, fmt.Errorf("no file at %s", p)
		}
		defer f.Close()
		return l.m.Files.Save(ctx, chatID, name, f)
	}
	if !l.m.sandboxesAvailable() {
		return store.Attachment{}, fmt.Errorf("no file at %s (only /shared files can be attached on this server)", p)
	}
	box, err := l.m.store.SandboxFor(ctx, agent.ID)
	if err != nil {
		return store.Attachment{}, err
	}
	r, _, err := l.m.Sandboxes.CopyOut(ctx, box, p, attachments.MaxSize)
	if err != nil {
		return store.Attachment{}, err
	}
	a, err := l.m.Files.Save(ctx, chatID, name, r)
	if cerr := r.Close(); err == nil && cerr != nil {
		err = fmt.Errorf("reading %s: %w", p, cerr)
	}
	return a, err
}
