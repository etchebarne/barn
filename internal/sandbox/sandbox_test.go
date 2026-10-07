package sandbox

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Integration test against the local Docker daemon; skipped without Docker or with -short.
func TestSandbox(t *testing.T) {
	if testing.Short() {
		t.Skip("needs Docker")
	}
	shared := t.TempDir()
	m := New(Options{Image: "debian:bookworm-slim", SharedDir: shared, Memory: "256m", CPUs: "1"})
	if !m.Available() {
		t.Skip("Docker isn't available")
	}
	ctx := context.Background()
	id := "test" + strings.ToLower(strings.ReplaceAll(t.Name(), "/", "-")) + time.Now().Format("150405")
	t.Cleanup(func() { m.Remove(context.Background(), id) })

	res, err := m.Exec(ctx, id, "echo hello && echo oops >&2 && pwd", "", nil, 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if res.ExitCode != 0 || !strings.Contains(res.Output, "hello") || !strings.Contains(res.Output, "oops") ||
		!strings.Contains(res.Output, "/home/agent") {
		t.Fatalf("unexpected result: %+v", res)
	}
	if m.Status(ctx, id) != "running" {
		t.Fatal("expected a running sandbox")
	}

	// Exit codes come through.
	if res, _ := m.Exec(ctx, id, "exit 3", "", nil, 10*time.Second); res.ExitCode != 3 {
		t.Fatalf("exit code = %d", res.ExitCode)
	}

	// Files persist in /home/agent, stdin works, and /shared is the host folder.
	if _, err := m.Exec(ctx, id, "cat > notes.txt && cp notes.txt /shared/from-sandbox.txt", "", []byte("remember me"), 10*time.Second); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(filepath.Join(shared, "from-sandbox.txt")); err != nil || string(b) != "remember me" {
		t.Fatalf("shared folder: %q %v", b, err)
	}
	if err := m.Restart(ctx, id); err != nil {
		t.Fatal(err)
	}
	if res, _ := m.Exec(ctx, id, "cat notes.txt", "", nil, 10*time.Second); res.Output != "remember me" {
		t.Fatalf("home should persist across restarts, got %q", res.Output)
	}

	// Timeouts kill the command inside the container.
	start := time.Now()
	res, err = m.Exec(ctx, id, "sleep 30", "", nil, 2*time.Second)
	if err != nil || !res.TimedOut || time.Since(start) > 15*time.Second {
		t.Fatalf("expected a timeout, got %+v err=%v after %s", res, err, time.Since(start))
	}
	// (The slim image has no pgrep; read /proc directly.)
	alive := `for p in /proc/[0-9]*; do tr '\0' ' ' < $p/cmdline 2>/dev/null; echo; done | grep -c '^sleep 30' || true`
	if res, _ := m.Exec(ctx, id, alive, "", nil, 10*time.Second); strings.TrimSpace(res.Output) != "0" {
		t.Fatalf("the timed-out command should be dead, found %q", res.Output)
	}

	// Huge output is clipped in the middle.
	res, _ = m.Exec(ctx, id, "seq 1 20000", "", nil, 10*time.Second)
	if res.Dropped == 0 || !strings.HasPrefix(res.Output, "1\n") || !strings.HasSuffix(strings.TrimSpace(res.Output), "20000") {
		t.Fatalf("expected clipped output with head and tail, dropped=%d", res.Dropped)
	}

	// Long-running processes talk over stdin/stdout and see their env; secrets stay out of the
	// docker command line.
	proc, err := m.Start(ctx, id, `echo starting >&2; while read -r l; do echo "$SECRET:$l"; done`, map[string]string{"SECRET": "s3cret"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.Join(proc.cmd.Args, " "), "s3cret") {
		t.Fatal("the secret must not be in the docker arguments")
	}
	proc.Write([]byte("hi\n"))
	line, err := bufio.NewReader(proc).ReadString('\n')
	if err != nil || line != "s3cret:hi\n" {
		t.Fatalf("line = %q %v", line, err)
	}
	for i := 0; i < 40 && !strings.Contains(proc.Stderr(), "starting"); i++ {
		time.Sleep(50 * time.Millisecond) // stderr is copied separately
	}
	if !strings.Contains(proc.Stderr(), "starting") {
		t.Fatalf("stderr = %q", proc.Stderr())
	}
	proc.Close()
	<-proc.Done()

	// A process that ignores stdin is killed with its children on Close.
	proc, err = m.Start(ctx, id, `sh -c 'sleep 300' & sleep 301; wait`, nil)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(500 * time.Millisecond)
	proc.Close()
	select {
	case <-proc.Done():
	case <-time.After(10 * time.Second):
		t.Fatal("the process should have exited")
	}
	sleeping := `for p in /proc/[0-9]*; do tr '\0' ' ' < $p/cmdline 2>/dev/null; echo; done | grep -c '^sleep 30[01]' || true`
	if res, _ := m.Exec(ctx, id, sleeping, "", nil, 10*time.Second); strings.TrimSpace(res.Output) != "0" {
		t.Fatalf("the process tree should be gone, found %q", res.Output)
	}
}
