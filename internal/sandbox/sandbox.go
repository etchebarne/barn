// Package sandbox runs agents' commands in Docker containers: one long-lived container per
// sandbox, with a persistent home volume and a /shared folder mounted in every sandbox.
package sandbox

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

//go:embed Dockerfile
var dockerfile []byte

// DefaultImage is built from the embedded Dockerfile when it's missing. Bump the tag when the
// Dockerfile changes.
const DefaultImage = "openbot-sandbox:2"

// ErrUnavailable means Docker isn't usable on this server.
var ErrUnavailable = errors.New("sandboxes aren't available on this server (Docker isn't installed or openbot can't use it)")

type Options struct {
	Image     string // defaults to DefaultImage
	SharedDir string // host folder mounted at /shared in every sandbox
	Memory    string // e.g. "2g"
	CPUs      string // e.g. "2"
	PIDs      int
}

type Manager struct {
	opts   Options
	docker string

	mu       sync.Mutex
	imageOK  bool
	starting map[string]*sync.Mutex // per-sandbox, so concurrent first uses don't race
}

// New finds the docker CLI. If Docker isn't usable, every call returns ErrUnavailable.
func New(opts Options) *Manager {
	if opts.Image == "" {
		opts.Image = DefaultImage
	}
	if opts.Memory == "" {
		opts.Memory = "2g"
	}
	if opts.CPUs == "" {
		opts.CPUs = "2"
	}
	if opts.PIDs == 0 {
		opts.PIDs = 512
	}
	m := &Manager{opts: opts, starting: map[string]*sync.Mutex{}}
	if path, err := exec.LookPath("docker"); err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if exec.CommandContext(ctx, path, "version", "--format", "{{.Server.Version}}").Run() == nil {
			m.docker = path
		}
	}
	return m
}

// Available reports whether Docker is usable.
func (m *Manager) Available() bool { return m.docker != "" }

func containerName(sandboxID string) string { return "openbot-sbx-" + strings.ToLower(sandboxID) }

func (m *Manager) run(ctx context.Context, stdin []byte, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, m.docker, args...)
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return out, fmt.Errorf("docker %s: %w: %s", args[0], err, strings.TrimSpace(string(out)))
	}
	return out, nil
}

// ensureImage builds the default image if it's missing. Custom images must already exist or
// be pullable.
func (m *Manager) ensureImage(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.imageOK {
		return nil
	}
	if _, err := m.run(ctx, nil, "image", "inspect", m.opts.Image); err != nil {
		if m.opts.Image != DefaultImage {
			if _, err := m.run(ctx, nil, "pull", m.opts.Image); err != nil {
				return err
			}
		} else if _, err := m.run(ctx, dockerfile, "build", "-t", DefaultImage, "-"); err != nil {
			return fmt.Errorf("building the sandbox image: %w", err)
		}
	}
	m.imageOK = true
	return nil
}

// Prepare builds or pulls the image ahead of the first use, so that use isn't slowed down by it.
func (m *Manager) Prepare(ctx context.Context) error {
	if !m.Available() {
		return ErrUnavailable
	}
	return m.ensureImage(ctx)
}

func (m *Manager) lockFor(sandboxID string) *sync.Mutex {
	m.mu.Lock()
	defer m.mu.Unlock()
	l, ok := m.starting[sandboxID]
	if !ok {
		l = &sync.Mutex{}
		m.starting[sandboxID] = l
	}
	return l
}

// Ensure makes sure the sandbox's container exists and is running.
func (m *Manager) Ensure(ctx context.Context, sandboxID string) error {
	if !m.Available() {
		return ErrUnavailable
	}
	l := m.lockFor(sandboxID)
	l.Lock()
	defer l.Unlock()

	name := containerName(sandboxID)
	out, err := m.run(ctx, nil, "container", "inspect", "--format", "{{.State.Running}}", name)
	if err == nil {
		if strings.TrimSpace(string(out)) == "true" {
			return nil
		}
		_, err = m.run(ctx, nil, "start", name)
		return err
	}
	if err := m.ensureImage(ctx); err != nil {
		return err
	}
	args := []string{"run", "-d", "--name", name,
		"--hostname", "sandbox",
		"--label", "openbot.sandbox=" + sandboxID,
		"--restart", "unless-stopped",
		"--memory", m.opts.Memory, "--cpus", m.opts.CPUs, "--pids-limit", strconv.Itoa(m.opts.PIDs),
		"-v", name + ":/home/agent",
	}
	if m.opts.SharedDir != "" {
		if err := os.MkdirAll(m.opts.SharedDir, 0o777); err != nil {
			return err
		}
		abs, err := filepath.Abs(m.opts.SharedDir)
		if err != nil {
			return err
		}
		args = append(args, "-v", abs+":/shared")
	}
	args = append(args, m.opts.Image, "sleep", "infinity")
	_, err = m.run(ctx, nil, args...)
	return err
}

// Result is the outcome of a command.
type Result struct {
	ExitCode int
	Output   string // stdout and stderr, interleaved
	TimedOut bool
	Dropped  int // bytes of output dropped from the middle to fit maxOutput
	// Full is the whole output (up to maxFullOutput) when some was dropped, to save elsewhere.
	Full []byte
}

// maxFullOutput bounds how much output a command keeps in memory.
const maxFullOutput = 10 << 20

// maxOutput is how much command output goes back to the agent (head and tail are kept).
const maxOutput = 16 << 10

// Exec runs a shell command in the sandbox, with env set. The timeout is enforced inside the
// container, so a runaway command is killed rather than left running. Values in env are passed
// through the docker client's environment, so they don't show up in the host's process list.
func (m *Manager) Exec(ctx context.Context, sandboxID, command, workdir string, stdin []byte, timeout time.Duration, env map[string]string) (Result, error) {
	if err := m.Ensure(ctx, sandboxID); err != nil {
		return Result{}, err
	}
	if workdir == "" {
		workdir = "/home/agent"
	}
	secs := int(timeout.Seconds())
	args := []string{"exec"}
	if stdin != nil {
		args = append(args, "-i")
	}
	cmdEnv := os.Environ()
	for k, v := range env {
		args = append(args, "-e", k)
		cmdEnv = append(cmdEnv, k+"="+v)
	}
	args = append(args, "-w", workdir, containerName(sandboxID),
		"timeout", "--signal=KILL", strconv.Itoa(secs), "bash", "-lc", command)
	// Leave the in-container timeout room to fire first.
	cctx, cancel := context.WithTimeout(ctx, timeout+15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(cctx, m.docker, args...)
	cmd.Env = cmdEnv
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var out bytes.Buffer
	capped := &limitWriter{w: &out, max: maxFullOutput}
	cmd.Stdout, cmd.Stderr = capped, capped
	start := time.Now()
	err := cmd.Run()
	elapsed := time.Since(start)

	res := Result{}
	var exitErr *exec.ExitError
	switch {
	case err == nil:
	case errors.As(err, &exitErr):
		res.ExitCode = exitErr.ExitCode()
		// timeout(1) KILLs the command (exit 137) when time runs out; an OOM kill also exits 137
		// but usually earlier.
		res.TimedOut = res.ExitCode == 137 && elapsed >= timeout-time.Second
	case cctx.Err() != nil:
		res.ExitCode, res.TimedOut = -1, true
	default:
		return res, err
	}
	res.Output, res.Dropped = clip(out.Bytes())
	if res.Dropped > 0 {
		res.Full = out.Bytes()
	}
	return res, nil
}

// limitWriter keeps the first max bytes written to it and discards the rest.
type limitWriter struct {
	w   *bytes.Buffer
	max int
}

func (l *limitWriter) Write(p []byte) (int, error) {
	if room := l.max - l.w.Len(); room > 0 {
		l.w.Write(p[:min(len(p), room)])
	}
	return len(p), nil
}

func clip(b []byte) (string, int) {
	if len(b) <= maxOutput {
		return string(b), 0
	}
	half := maxOutput / 2
	return string(b[:half]) + "\n…\n" + string(b[len(b)-half:]), len(b) - maxOutput
}

// Status reports a sandbox's container state: "running", "stopped", or "none".
func (m *Manager) Status(ctx context.Context, sandboxID string) string {
	if !m.Available() {
		return "unavailable"
	}
	out, err := m.run(ctx, nil, "container", "inspect", "--format", "{{.State.Running}}", containerName(sandboxID))
	switch {
	case err != nil:
		return "none"
	case strings.TrimSpace(string(out)) == "true":
		return "running"
	default:
		return "stopped"
	}
}

// Restart restarts a sandbox's container (processes stop; files in /home/agent stay).
func (m *Manager) Restart(ctx context.Context, sandboxID string) error {
	if !m.Available() {
		return ErrUnavailable
	}
	if m.Status(ctx, sandboxID) == "none" {
		return m.Ensure(ctx, sandboxID)
	}
	_, err := m.run(ctx, nil, "restart", "-t", "2", containerName(sandboxID))
	return err
}

// Remove deletes a sandbox's container and its home volume.
func (m *Manager) Remove(ctx context.Context, sandboxID string) error {
	if !m.Available() {
		return nil
	}
	name := containerName(sandboxID)
	_, _ = m.run(ctx, nil, "rm", "-f", name)
	_, _ = m.run(ctx, nil, "volume", "rm", "-f", name)
	return nil
}
