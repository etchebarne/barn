package runtime

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/etchebarne/barn/internal/attachments"
	"github.com/etchebarne/barn/internal/model"
)

func TestAgentsSeeAndSendAttachments(t *testing.T) {
	ctx := context.Background()
	f := setup(t)
	shared := t.TempDir()
	files := &attachments.Files{Dir: filepath.Join(shared, "attachments"), Store: f.store}
	f.rt.Files = files
	sbx := &fakeSandbox{files: map[string][]byte{"/home/agent/report.pdf": []byte("%PDF-1.4 fake")}}
	f.rt.Sandboxes = sbx
	os.MkdirAll(filepath.Join(shared, "out"), 0o755)
	os.WriteFile(filepath.Join(shared, "out", "chart.png"), pngBytes(), 0o644)

	upload := func(name string, data []byte) string {
		a, err := files.Save(ctx, f.chatID, name, bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		return a.ID
	}
	var mu sync.Mutex
	var seen []model.Request
	f.llm.handler = func(req model.Request) model.Message {
		mu.Lock()
		seen = append(seen, req)
		mu.Unlock()
		last := req.Messages[len(req.Messages)-1]
		if last.Role == "user" && strings.Contains(last.Text(), "send me the report") {
			return toolCall(toolSendMessage, map[string]any{"chat_id": f.chatID, "text": "Here you go",
				"files": []string{"/home/agent/report.pdf", "/shared/out/chart.png"}})
		}
		if last.Role == "user" && strings.Contains(last.Text(), "escape") {
			return toolCall(toolSendMessage, map[string]any{"chat_id": f.chatID, "text": "x", "files": []string{"/shared/../../etc/passwd"}})
		}
		return model.Text("assistant", "")
	}

	ids := []string{upload("screen.png", pngBytes()), upload("notes.md", []byte("# Plan\n- ship it")), upload("deck.pdf", []byte("%PDF-1.4 x"))}
	msg, err := f.store.InsertMessageWithAttachments(ctx, f.chatID, "user", nil, "look at these", ids)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.rt.DeliverUserMessage(ctx, msg); err != nil {
		t.Fatal(err)
	}
	f.waitIdle(t)
	mu.Lock()
	last := seen[len(seen)-1].Messages
	user := last[len(last)-1]
	mu.Unlock()
	text := user.Text()
	for _, want := range []string{"look at these", `name="screen.png" type="image/png"`, "(image, shown to you)",
		"# Plan\n- ship it", `name="deck.pdf" type="application/pdf"`, `path="/shared/attachments/`} {
		if !strings.Contains(text, want) {
			t.Errorf("rendered message lacks %q:\n%s", want, text)
		}
	}
	if len(user.Images) != 1 || user.Images[0].MediaType != "image/png" {
		t.Fatalf("the model should see the image: %+v", user.Images)
	}

	// Only the last few messages with images carry them.
	for i := 0; i < imagesInContext+1; i++ {
		m, _ := f.store.InsertMessageWithAttachments(ctx, f.chatID, "user", nil, "another", []string{upload("s.png", pngBytes())})
		f.rt.DeliverUserMessage(ctx, m)
		f.waitIdle(t)
	}
	mu.Lock()
	withImages := 0
	for _, m := range seen[len(seen)-1].Messages {
		if len(m.Images) > 0 {
			withImages++
		}
	}
	mu.Unlock()
	if withImages != imagesInContext {
		t.Fatalf("messages with images in the request = %d, want %d", withImages, imagesInContext)
	}

	// The agent attaches files from its computer and from /shared.
	f.userSays(t, "send me the report")
	f.waitIdle(t)
	sent, _ := f.store.LastMessage(ctx, f.chatID)
	got, _ := f.store.GetMessage(ctx, sent.ID)
	if got.Body != "Here you go" || len(got.Attachments) != 2 || got.Attachments[0].Name != "report.pdf" ||
		got.Attachments[0].Mime != "application/pdf" || got.Attachments[1].Mime != "image/png" {
		t.Fatalf("sent = %+v", got)
	}
	f.userSays(t, "try to escape")
	f.waitIdle(t)
	if last, _ := f.store.LastMessage(ctx, f.chatID); last.Body == "x" {
		t.Fatal("paths outside /shared must be refused")
	}
}

func pngBytes() []byte {
	// A 1x1 PNG.
	return []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01, 0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4, 0x89, 0x00,
		0x00, 0x00, 0x0d, 0x49, 0x44, 0x41, 0x54, 0x78, 0x9c, 0x63, 0x00, 0x01, 0x00, 0x00, 0x05, 0x00, 0x01, 0x0d,
		0x0a, 0x2d, 0xb4, 0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4e, 0x44, 0xae, 0x42, 0x60, 0x82}
}
