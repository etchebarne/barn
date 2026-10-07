package sandbox

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// Process is a long-running command in a sandbox that talks over stdin and stdout, such as a
// local MCP server.
type Process struct {
	m         *Manager
	sandboxID string
	pidFile   string
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	stdout    io.ReadCloser
	stderr    *tail
	done      chan struct{}
	closeOnce sync.Once
}

// Start runs command (through bash, so pipes and quoting work) in the sandbox with env set,
// keeping its stdin and stdout open. Values in env are passed through the docker client's
// environment, so they don't show up in the host's process list.
func (m *Manager) Start(ctx context.Context, sandboxID, command string, env map[string]string) (*Process, error) {
	if err := m.Ensure(ctx, sandboxID); err != nil {
		return nil, err
	}
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	p := &Process{m: m, sandboxID: sandboxID, pidFile: "/tmp/openbot-proc-" + hex.EncodeToString(b) + ".pid",
		stderr: &tail{max: 8 << 10}, done: make(chan struct{})}

	args := []string{"exec", "-i", "-w", "/home/agent"}
	cmdEnv := os.Environ()
	for k, v := range env {
		args = append(args, "-e", k)
		cmdEnv = append(cmdEnv, k+"="+v)
	}
	// Record the pid so Close can stop the whole process tree: killing the docker client doesn't
	// stop what runs inside the container.
	args = append(args, containerName(sandboxID), "sh", "-c", `echo $$ > "$1"; exec bash -lc "$2"`, "sh", p.pidFile, command)
	p.cmd = exec.Command(m.docker, args...)
	p.cmd.Env = cmdEnv
	p.cmd.Stderr = p.stderr
	var err error
	if p.stdin, err = p.cmd.StdinPipe(); err != nil {
		return nil, err
	}
	if p.stdout, err = p.cmd.StdoutPipe(); err != nil {
		return nil, err
	}
	if err := p.cmd.Start(); err != nil {
		return nil, err
	}
	go func() {
		_ = p.cmd.Wait()
		close(p.done)
	}()
	return p, nil
}

func (p *Process) Write(b []byte) (int, error) { return p.stdin.Write(b) }
func (p *Process) Read(b []byte) (int, error)  { return p.stdout.Read(b) }

// Stderr returns the last few KB the process wrote to stderr (useful in error messages).
func (p *Process) Stderr() string { return p.stderr.String() }

// Close ends the process: first by closing its stdin (what well-behaved servers wait for), then
// by signalling its process tree inside the container.
func (p *Process) Close() error {
	p.closeOnce.Do(func() {
		_ = p.stdin.Close()
		select {
		case <-p.done:
		case <-time.After(2 * time.Second):
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			// Walk /proc for descendants; minimal images don't have pkill.
			_, _ = p.m.run(ctx, nil, "exec", containerName(p.sandboxID), "sh", "-c", `
k() {
  for s in /proc/[0-9]*/stat; do
    read -r pid _ _ ppid _ < "$s" 2>/dev/null || continue
    [ "$ppid" = "$1" ] && k "$pid"
  done
  kill -TERM "$1" 2>/dev/null
}
[ -f "$1" ] && k "$(cat "$1")"`, "sh", p.pidFile)
			select {
			case <-p.done:
			case <-time.After(3 * time.Second):
				_ = p.cmd.Process.Kill()
			}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = p.m.run(ctx, nil, "exec", containerName(p.sandboxID), "rm", "-f", p.pidFile)
	})
	return nil
}

// Done is closed once the process has exited.
func (p *Process) Done() <-chan struct{} { return p.done }

// tail keeps the last max bytes written to it.
type tail struct {
	mu  sync.Mutex
	max int
	buf []byte
}

func (t *tail) Write(b []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf = append(t.buf, b...)
	if len(t.buf) > t.max {
		t.buf = append([]byte(nil), t.buf[len(t.buf)-t.max:]...)
	}
	return len(b), nil
}

func (t *tail) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return string(t.buf)
}

// CopyOut streams a file out of the sandbox (binary-safe), refusing files over max bytes.
func (m *Manager) CopyOut(ctx context.Context, sandboxID, path string, max int64) (io.ReadCloser, int64, error) {
	if err := m.Ensure(ctx, sandboxID); err != nil {
		return nil, 0, err
	}
	out, err := m.run(ctx, nil, "exec", containerName(sandboxID), "stat", "-L", "-c", "%s %F", "--", path)
	if err != nil {
		return nil, 0, fmt.Errorf("no file at %s", path)
	}
	var size int64
	var kind string
	fmt.Sscanf(strings.TrimSpace(string(out)), "%d %s", &size, &kind)
	if kind != "regular" {
		return nil, 0, fmt.Errorf("%s isn't a regular file", path)
	}
	if size > max {
		return nil, size, fmt.Errorf("%s is %d bytes, over the %d byte limit", path, size, max)
	}
	cmd := exec.CommandContext(ctx, m.docker, "exec", containerName(sandboxID), "cat", "--", path)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, 0, err
	}
	if err := cmd.Start(); err != nil {
		return nil, 0, err
	}
	return &cmdReader{ReadCloser: stdout, cmd: cmd}, size, nil
}

type cmdReader struct {
	io.ReadCloser
	cmd *exec.Cmd
}

func (r *cmdReader) Close() error {
	r.ReadCloser.Close()
	return r.cmd.Wait()
}
