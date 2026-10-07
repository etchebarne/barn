package sandbox

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/creack/pty"
)

// The user's own access to a sandbox ("the agent's computer"): browsing and changing files,
// and an interactive shell. Everything runs as the same user the agents use.

// ErrNotFound means there's nothing at a path.
var ErrNotFound = errors.New("no such file or folder")

// ErrExists means the destination of a move is taken.
var ErrExists = errors.New("something with that name is already there")

// maxEntries caps how many items a folder listing returns.
const maxEntries = 5000

// Entry is one item in a folder.
type Entry struct {
	Name     string
	Kind     string // "file", "dir", "link" (to a file), "dirlink" (to a folder), or "other"
	Size     int64
	Modified time.Time
}

// cleanPath makes p an absolute, clean path inside the container.
func cleanPath(p string) (string, error) {
	if !strings.HasPrefix(p, "/") {
		return "", fmt.Errorf("path must be absolute: %q", p)
	}
	return path.Clean(p), nil
}

// List returns a folder's items, sorted by the caller. It reports whether the list was cut at
// maxEntries.
func (m *Manager) List(ctx context.Context, sandboxID, dir string) ([]Entry, bool, error) {
	dir, err := cleanPath(dir)
	if err != nil {
		return nil, false, err
	}
	if err := m.Ensure(ctx, sandboxID); err != nil {
		return nil, false, err
	}
	// type, link target type, size, mtime, name; NUL-terminated so names may hold anything.
	cmd := exec.CommandContext(ctx, m.docker, "exec", containerName(sandboxID), "sh", "-c",
		`cd -- "$1" 2>/dev/null || exit 3; find . -mindepth 1 -maxdepth 1 -printf '%y\t%Y\t%s\t%T@\t%f\0' | head -z -n "$2"`,
		"sh", dir, strconv.Itoa(maxEntries+1))
	var out, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	if err := cmd.Run(); err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) && exit.ExitCode() == 3 {
			return nil, false, ErrNotFound
		}
		return nil, false, fmt.Errorf("listing %s: %w: %s", dir, err, strings.TrimSpace(stderr.String()))
	}
	var entries []Entry
	for _, rec := range bytes.Split(out.Bytes(), []byte{0}) {
		f := strings.SplitN(string(rec), "\t", 5)
		if len(f) != 5 {
			continue
		}
		e := Entry{Name: f[4], Kind: "other"}
		switch f[0] {
		case "f":
			e.Kind = "file"
		case "d":
			e.Kind = "dir"
		case "l":
			e.Kind = "link"
			if f[1] == "d" {
				e.Kind = "dirlink"
			}
		}
		e.Size, _ = strconv.ParseInt(f[2], 10, 64)
		if secs, err := strconv.ParseFloat(f[3], 64); err == nil {
			e.Modified = time.Unix(0, int64(secs*float64(time.Second)))
		}
		entries = append(entries, e)
	}
	cut := len(entries) > maxEntries
	if cut {
		entries = entries[:maxEntries]
	}
	return entries, cut, nil
}

// CopyIn writes r to a file in the sandbox (creating its folder), replacing what's there.
func (m *Manager) CopyIn(ctx context.Context, sandboxID, file string, r io.Reader) error {
	file, err := cleanPath(file)
	if err != nil {
		return err
	}
	if err := m.Ensure(ctx, sandboxID); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, m.docker, "exec", "-i", containerName(sandboxID), "sh", "-c",
		`[ -d "$1" ] && { echo "$1 is a folder" >&2; exit 4; }; mkdir -p -- "$(dirname -- "$1")" && cat > "$1"`, "sh", file)
	cmd.Stdin = r
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("writing %s: %s", file, firstLine(stderr.String(), err))
	}
	return nil
}

// Mkdir creates a folder (and its parents).
func (m *Manager) Mkdir(ctx context.Context, sandboxID, dir string) error {
	dir, err := cleanPath(dir)
	if err != nil {
		return err
	}
	return m.fileOp(ctx, sandboxID, `mkdir -p -- "$1"`, dir)
}

// Move renames or moves a file or folder. It won't replace something already at to.
func (m *Manager) Move(ctx context.Context, sandboxID, from, to string) error {
	from, err := cleanPath(from)
	if err != nil {
		return err
	}
	if to, err = cleanPath(to); err != nil {
		return err
	}
	if from == "/" {
		return fmt.Errorf("can't move /")
	}
	return m.fileOp(ctx, sandboxID, `[ -e "$1" ] || [ -L "$1" ] || exit 3; [ -e "$2" ] && exit 5; mv -- "$1" "$2"`, from, to)
}

// Delete removes a file or a folder with everything in it.
func (m *Manager) Delete(ctx context.Context, sandboxID, target string) error {
	target, err := cleanPath(target)
	if err != nil {
		return err
	}
	if target == "/" {
		return fmt.Errorf("can't delete /")
	}
	return m.fileOp(ctx, sandboxID, `[ -e "$1" ] || [ -L "$1" ] || exit 3; rm -rf -- "$1"`, target)
}

// fileOp runs a short shell script on paths: exit 3 means not found, 5 already exists.
func (m *Manager) fileOp(ctx context.Context, sandboxID, script string, args ...string) error {
	if err := m.Ensure(ctx, sandboxID); err != nil {
		return err
	}
	cctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(cctx, m.docker, append([]string{"exec", containerName(sandboxID), "sh", "-c", script, "sh"}, args...)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err := cmd.Run()
	var exit *exec.ExitError
	switch {
	case err == nil:
		return nil
	case errors.As(err, &exit) && exit.ExitCode() == 3:
		return ErrNotFound
	case errors.As(err, &exit) && exit.ExitCode() == 5:
		return ErrExists
	}
	return errors.New(firstLine(stderr.String(), err))
}

func firstLine(s string, err error) string {
	if line, _, _ := strings.Cut(strings.TrimSpace(s), "\n"); line != "" {
		return line
	}
	return err.Error()
}

// Terminal is an interactive shell in a sandbox, on a pseudo-terminal.
type Terminal struct {
	m         *Manager
	sandboxID string
	pidFile   string
	cmd       *exec.Cmd
	pty       *os.File
	done      chan struct{}
	closeOnce sync.Once
}

// Shell starts a login shell in the sandbox's home folder, sized cols × rows.
func (m *Manager) Shell(ctx context.Context, sandboxID string, cols, rows uint16) (*Terminal, error) {
	if err := m.Ensure(ctx, sandboxID); err != nil {
		return nil, err
	}
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	t := &Terminal{m: m, sandboxID: sandboxID, pidFile: "/tmp/openbot-shell-" + hex.EncodeToString(b) + ".pid", done: make(chan struct{})}
	t.cmd = exec.Command(m.docker, "exec", "-it", "-w", "/home/agent", "-e", "TERM=xterm-256color", "-e", "COLORTERM=truecolor",
		containerName(sandboxID), "sh", "-c", `echo $$ > "$1"; exec bash -l`, "sh", t.pidFile)
	f, err := pty.StartWithSize(t.cmd, &pty.Winsize{Cols: cols, Rows: rows})
	if err != nil {
		return nil, err
	}
	t.pty = f
	go func() {
		_ = t.cmd.Wait()
		close(t.done)
	}()
	return t, nil
}

func (t *Terminal) Read(b []byte) (int, error)  { return t.pty.Read(b) }
func (t *Terminal) Write(b []byte) (int, error) { return t.pty.Write(b) }

// Resize changes the terminal's size.
func (t *Terminal) Resize(cols, rows uint16) error {
	return pty.Setsize(t.pty, &pty.Winsize{Cols: cols, Rows: rows})
}

// Done is closed once the shell has exited.
func (t *Terminal) Done() <-chan struct{} { return t.done }

// Close ends the shell and everything it started.
func (t *Terminal) Close() error {
	t.closeOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		select {
		case <-t.done:
		default:
			t.m.killTree(ctx, t.sandboxID, t.pidFile, "HUP")
			select {
			case <-t.done:
			case <-time.After(3 * time.Second):
				t.m.killTree(ctx, t.sandboxID, t.pidFile, "KILL")
				_ = t.cmd.Process.Kill()
			}
		}
		_ = t.pty.Close()
		_, _ = t.m.run(ctx, nil, "exec", containerName(t.sandboxID), "rm", "-f", t.pidFile)
	})
	return nil
}
