package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/etchebarne/barn/internal/model"
	"github.com/etchebarne/barn/internal/sandbox"
	"github.com/etchebarne/barn/internal/store"
)

// Sandboxer runs commands in agents' sandboxes (see package sandbox).
type Sandboxer interface {
	Available() bool
	Exec(ctx context.Context, sandboxID, command, workdir string, stdin []byte, timeout time.Duration) (sandbox.Result, error)
	Status(ctx context.Context, sandboxID string) string
	Restart(ctx context.Context, sandboxID string) error
	Remove(ctx context.Context, sandboxID string) error
	CopyOut(ctx context.Context, sandboxID, path string, max int64) (io.ReadCloser, int64, error)
}

// SandboxState is an agent's sandbox status ("unavailable", "none", "stopped", "running") and
// the other agents sharing it.
func (m *Manager) SandboxState(ctx context.Context, agentID string) (string, []string, error) {
	if !m.sandboxesAvailable() {
		return "unavailable", nil, nil
	}
	agent, err := m.store.GetAgent(ctx, agentID)
	if err != nil {
		return "", nil, err
	}
	if agent.SandboxID == nil {
		return "none", nil, nil
	}
	members, err := m.store.SandboxMembers(ctx, *agent.SandboxID)
	if err != nil {
		return "", nil, err
	}
	var others []string
	for _, a := range members {
		if a.ID != agentID {
			others = append(others, a.ID)
		}
	}
	return m.Sandboxes.Status(ctx, *agent.SandboxID), others, nil
}

// RestartSandbox restarts (or creates) an agent's sandbox.
func (m *Manager) RestartSandbox(ctx context.Context, agentID string) error {
	if !m.sandboxesAvailable() {
		return sandbox.ErrUnavailable
	}
	id, err := m.store.SandboxFor(ctx, agentID)
	if err != nil {
		return err
	}
	return m.Sandboxes.Restart(ctx, id)
}

const (
	toolRunCommand = "run_command"
	toolReadFile   = "read_file"
	toolWriteFile  = "write_file"
	toolListFiles  = "list_files"

	defaultCommandTimeout = 2 * time.Minute
	maxCommandTimeout     = 15 * time.Minute
	maxReadBytes          = 200_000
)

var (
	runCommandTool = function(toolRunCommand,
		"Run a shell command (bash) on your own Linux computer: a Debian sandbox where you're root. "+
			"Install what you need (apt, pip, npm). Output (stdout and stderr) is returned; very long "+
			"output is clipped in the middle, so redirect to a file when you need all of it. For "+
			"long-running jobs, start them in the background (nohup … &) and check on them later.",
		`{
			"type": "object",
			"properties": {
				"command": {"type": "string"},
				"timeout_seconds": {"type": "integer", "description": "Kill the command after this long. Default 120, max 900."},
				"workdir": {"type": "string", "description": "Directory to run in. Default /home/agent."}
			},
			"required": ["command"],
			"additionalProperties": false
		}`)

	readFileTool = function(toolReadFile,
		"Read a text file from your sandbox (up to 200 KB).",
		`{
			"type": "object",
			"properties": {"path": {"type": "string", "description": "Absolute, or relative to /home/agent."}},
			"required": ["path"],
			"additionalProperties": false
		}`)

	writeFileTool = function(toolWriteFile,
		"Write a text file in your sandbox, creating parent folders. Replaces the file if it exists.",
		`{
			"type": "object",
			"properties": {
				"path": {"type": "string", "description": "Absolute, or relative to /home/agent."},
				"content": {"type": "string"}
			},
			"required": ["path", "content"],
			"additionalProperties": false
		}`)

	listFilesTool = function(toolListFiles,
		"List a folder in your sandbox.",
		`{
			"type": "object",
			"properties": {"path": {"type": "string", "description": "Default /home/agent."}},
			"additionalProperties": false
		}`)
)

func (m *Manager) sandboxesAvailable() bool {
	return m.Sandboxes != nil && m.Sandboxes.Available()
}

// shellQuote quotes s as a single bash word.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func sandboxPath(p string) string {
	if p == "" {
		return "/home/agent"
	}
	if !strings.HasPrefix(p, "/") {
		p = "/home/agent/" + p
	}
	return path.Clean(p)
}

func (l *loop) sandboxExec(ctx context.Context, agent store.Agent, command, workdir string, stdin []byte, timeout time.Duration) (sandbox.Result, error) {
	if !l.m.sandboxesAvailable() {
		return sandbox.Result{}, sandbox.ErrUnavailable
	}
	id, err := l.m.store.SandboxFor(ctx, agent.ID)
	if err != nil {
		return sandbox.Result{}, err
	}
	return l.m.Sandboxes.Exec(ctx, id, command, workdir, stdin, timeout)
}

func commandResult(res sandbox.Result) string {
	out := map[string]any{"exit_code": res.ExitCode, "output": res.Output}
	if res.TimedOut {
		out["timed_out"] = true
		out["note"] = "The command hit its timeout and was killed."
	}
	if res.Dropped > 0 {
		out["clipped_bytes"] = res.Dropped
	}
	return toolOK(out)
}

func (l *loop) runSandboxTool(ctx context.Context, agent store.Agent, name string, raw []byte) (string, bool) {
	switch name {
	case toolRunCommand:
		var a struct {
			Command        string `json:"command"`
			TimeoutSeconds int    `json:"timeout_seconds"`
			Workdir        string `json:"workdir"`
		}
		if err := json.Unmarshal(raw, &a); err != nil {
			return toolError("invalid arguments: %v", err), false
		}
		if strings.TrimSpace(a.Command) == "" {
			return toolError("command is empty"), false
		}
		timeout := defaultCommandTimeout
		if a.TimeoutSeconds > 0 {
			timeout = min(time.Duration(a.TimeoutSeconds)*time.Second, maxCommandTimeout)
		}
		res, err := l.sandboxExec(ctx, agent, a.Command, sandboxPath(a.Workdir), nil, timeout)
		if err != nil {
			return toolError("%v", err), false
		}
		return commandResult(res), true

	case toolReadFile:
		var a struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal(raw, &a); err != nil || a.Path == "" {
			return toolError("path is required"), false
		}
		p := sandboxPath(a.Path)
		res, err := l.sandboxExec(ctx, agent, fmt.Sprintf("head -c %d -- %s", maxReadBytes, shellQuote(p)), "", nil, 30*time.Second)
		if err != nil {
			return toolError("%v", err), false
		}
		if res.ExitCode != 0 {
			return toolError("couldn't read %s: %s", p, strings.TrimSpace(res.Output)), false
		}
		if !utf8.ValidString(res.Output) {
			return toolError("%s isn't a text file; inspect it with run_command (e.g. file, xxd)", p), false
		}
		return toolOK(map[string]any{"path": p, "content": res.Output}), true

	case toolWriteFile:
		var a struct {
			Path    string `json:"path"`
			Content string `json:"content"`
		}
		if err := json.Unmarshal(raw, &a); err != nil || a.Path == "" {
			return toolError("path is required"), false
		}
		p := sandboxPath(a.Path)
		cmd := fmt.Sprintf("mkdir -p -- %s && cat > %s", shellQuote(path.Dir(p)), shellQuote(p))
		res, err := l.sandboxExec(ctx, agent, cmd, "", []byte(a.Content), 30*time.Second)
		if err != nil {
			return toolError("%v", err), false
		}
		if res.ExitCode != 0 {
			return toolError("couldn't write %s: %s", p, strings.TrimSpace(res.Output)), false
		}
		return toolOK(map[string]any{"path": p, "bytes": len(a.Content)}), true

	case toolListFiles:
		var a struct {
			Path string `json:"path"`
		}
		_ = json.Unmarshal(raw, &a)
		p := sandboxPath(a.Path)
		res, err := l.sandboxExec(ctx, agent, "ls -la --group-directories-first -- "+shellQuote(p), "", nil, 30*time.Second)
		if err != nil {
			return toolError("%v", err), false
		}
		if res.ExitCode != 0 {
			return toolError("couldn't list %s: %s", p, strings.TrimSpace(res.Output)), false
		}
		return toolOK(map[string]any{"path": p, "listing": res.Output}), true
	}
	return toolError("unknown tool %q", name), false
}

// activityFor is the activity line shown while a tool runs.
func activityFor(call model.ToolCall) string {
	if call.Function.Name == toolRunCommand {
		var a struct {
			Command string `json:"command"`
		}
		if json.Unmarshal([]byte(call.Function.Arguments), &a) == nil && a.Command != "" {
			cmd := strings.Join(strings.Fields(a.Command), " ")
			if utf8.RuneCountInString(cmd) > 60 {
				cmd = string([]rune(cmd)[:57]) + "…"
			}
			return "running " + cmd
		}
	}
	return toolLabel(call.Function.Name)
}
