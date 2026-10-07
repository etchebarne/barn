package sandbox

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

// Integration test against the local Docker daemon; skipped without Docker or with -short.
func TestComputer(t *testing.T) {
	if testing.Short() {
		t.Skip("needs Docker")
	}
	m := New(Options{Image: "debian:bookworm-slim", Memory: "256m", CPUs: "1"})
	if !m.Available() {
		t.Skip("Docker isn't available")
	}
	ctx := context.Background()
	id := "testcomputer" + time.Now().Format("150405")
	t.Cleanup(func() { m.Remove(context.Background(), id) })

	// Write a file in a new folder, with an awkward name.
	name := "/home/agent/notes dir/a\tb.txt"
	if err := m.CopyIn(ctx, id, name, strings.NewReader("hello")); err != nil {
		t.Fatal(err)
	}
	if err := m.Mkdir(ctx, id, "/home/agent/empty"); err != nil {
		t.Fatal(err)
	}
	entries, cut, err := m.List(ctx, id, "/home/agent/")
	if err != nil || cut {
		t.Fatal(err, cut)
	}
	kinds := map[string]string{}
	for _, e := range entries {
		kinds[e.Name] = e.Kind
	}
	if kinds["notes dir"] != "dir" || kinds["empty"] != "dir" {
		t.Fatalf("listing: %+v", entries)
	}
	inner, _, _ := m.List(ctx, id, "/home/agent/notes dir")
	if len(inner) != 1 || inner[0].Name != "a\tb.txt" || inner[0].Kind != "file" || inner[0].Size != 5 || inner[0].Modified.IsZero() {
		t.Fatalf("inner: %+v", inner)
	}
	if _, _, err := m.List(ctx, id, "/nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing folder: %v", err)
	}
	if _, _, err := m.List(ctx, id, "relative"); err == nil {
		t.Fatal("relative paths must be refused")
	}

	// Move: no overwriting, missing source reported.
	if err := m.Move(ctx, id, name, "/home/agent/empty/b.txt"); err != nil {
		t.Fatal(err)
	}
	if err := m.CopyIn(ctx, id, "/home/agent/c.txt", strings.NewReader("c")); err != nil {
		t.Fatal(err)
	}
	if err := m.Move(ctx, id, "/home/agent/c.txt", "/home/agent/empty/b.txt"); !errors.Is(err, ErrExists) {
		t.Fatalf("move over a file: %v", err)
	}
	if err := m.Move(ctx, id, "/home/agent/gone", "/home/agent/x"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("move missing: %v", err)
	}
	r, _, err := m.CopyOut(ctx, id, "/home/agent/empty/b.txt", 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(r)
	r.Close()
	if string(got) != "hello" {
		t.Fatalf("content: %q", got)
	}
	if err := m.CopyIn(ctx, id, "/home/agent/empty", strings.NewReader("x")); err == nil {
		t.Fatal("writing over a folder must fail")
	}

	// Delete.
	if err := m.Delete(ctx, id, "/home/agent/empty"); err != nil {
		t.Fatal(err)
	}
	if err := m.Delete(ctx, id, "/home/agent/empty"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete twice: %v", err)
	}
	if err := m.Delete(ctx, id, "/"); err == nil {
		t.Fatal("deleting / must be refused")
	}

	// A shell: runs commands, resizes, and goes away on Close.
	term, err := m.Shell(ctx, id, 80, 24)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := term.Write([]byte("stty size; echo marker-$((40+2))\n")); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	buf := make([]byte, 4096)
	deadline := time.Now().Add(15 * time.Second)
	for !strings.Contains(out.String(), "marker-42") && time.Now().Before(deadline) {
		n, err := term.Read(buf)
		out.Write(buf[:n])
		if err != nil {
			break
		}
	}
	if !strings.Contains(out.String(), "24 80") || !strings.Contains(out.String(), "marker-42") {
		t.Fatalf("shell output: %q", out.String())
	}
	if err := term.Resize(100, 30); err != nil {
		t.Fatal(err)
	}
	term.Close()
	select {
	case <-term.Done():
	case <-time.After(10 * time.Second):
		t.Fatal("the shell didn't exit")
	}
	res, _ := m.Exec(ctx, id, "ls /tmp | grep -c openbot-shell || true", "", nil, 10*time.Second)
	if strings.TrimSpace(res.Output) != "0" {
		t.Fatalf("pid file left behind: %q", res.Output)
	}
}
