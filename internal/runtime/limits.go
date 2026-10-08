package runtime

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/etchebarne/openbot/internal/model"
	"github.com/etchebarne/openbot/internal/store"
)

// Tool results stay in the agent's context (and get resent with every step), so big ones are
// kept out of it: results over a limit are saved to a file in the agent's computer and replaced
// by a preview and the file's path, and files are read in pages.

const (
	maxToolResult  = 100_000 // characters, for openbot's own tools
	maxAppResult   = 50_000  // for connector and MCP tools, whose results are often bulky
	maxTurnResults = 200_000 // across one turn's tool results; past it, big results are saved to files
	resultPreview  = 1_500

	readLimitLines = 2_000   // per read_file call
	readMaxChars   = 100_000 // per read_file call
	readLineChars  = 2_000   // per line
)

const spillDir = "/tmp/openbot"

// spill saves content to a file in the agent's computer and returns its path.
func (l *loop) spill(ctx context.Context, agent store.Agent, kind, ext string, content []byte) (string, error) {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	p := fmt.Sprintf("%s/%s-%s-%s.%s", spillDir, kind, time.Now().Format("150405"), hex.EncodeToString(b), ext)
	res, err := l.sandboxExec(ctx, agent, fmt.Sprintf("mkdir -p %s && cat > %s", spillDir, shellQuote(p)), "", content, 30*time.Second, nil)
	if err != nil {
		return "", err
	}
	if res.ExitCode != 0 {
		return "", fmt.Errorf("%s", strings.TrimSpace(res.Output))
	}
	return p, nil
}

// limitResult keeps a tool result within its limit and the turn's (read_file and run_command
// manage their own size).
func (l *loop) limitResult(ctx context.Context, agent store.Agent, call model.ToolCall, result string) string {
	name := call.Function.Name
	if name == toolReadFile || name == toolRunCommand {
		l.turnChars += len(result)
		return result
	}
	limit := maxToolResult
	if _, ok := l.connectorTool(ctx, agent, name); ok {
		limit = maxAppResult
	}
	if l.turnChars+len(result) > maxTurnResults && len(result) > resultPreview*2 {
		limit = resultPreview * 2
	}
	if len(result) <= limit {
		l.turnChars += len(result)
		return result
	}
	out := map[string]any{"ok": true, "size_chars": len(result)}
	if l.m.sandboxesAvailable() {
		if p, err := l.spill(ctx, agent, strings.ReplaceAll(name, "__", "-"), "json", []byte(result)); err == nil {
			out["preview"] = cut(result, resultPreview)
			out["full_result"] = p
			out["note"] = "The result was too big for your context, so it's saved in that file in your computer: " +
				"search it (grep, jq) or read it in pages (read_file with offset and limit) instead of asking again."
		}
	}
	if _, saved := out["full_result"]; !saved {
		out["truncated_result"] = cut(result, limit)
		out["note"] = "The result was too big for your context and was cut; ask for less (filters, fewer items) if you need the rest."
	}
	b, _ := json.Marshal(out)
	l.turnChars += len(b)
	return string(b)
}

// cut keeps the first n bytes of s, at a character boundary.
func cut(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n] + "…"
}

type readState struct {
	stamp         string // mtime and size
	offset, limit int
}

// readFile reads a page of a text file: from line offset (1-based), up to limit lines, within
// readMaxChars. Rereading the same page of an unchanged file gets a short answer instead.
func (l *loop) readFile(ctx context.Context, agent store.Agent, p string, offset, limit int) (string, bool) {
	if offset < 1 {
		offset = 1
	}
	if limit < 1 || limit > readLimitLines {
		limit = readLimitLines
	}
	q := shellQuote(p)
	cmd := fmt.Sprintf(`[ -f %[1]s ] || { echo "no file at %[2]s" >&2; exit 2; }; stat -c '%%Y %%s' -- %[1]s; wc -l < %[1]s; echo '<<openbot>>'; awk -v s=%[3]d -v n=%[4]d 'NR>=s && NR<s+n' %[1]s`,
		q, strings.ReplaceAll(p, `"`, `'`), offset, limit)
	res, err := l.sandboxExec(ctx, agent, cmd, "", nil, 30*time.Second, nil)
	if err != nil {
		return toolError("%v", err), false
	}
	if res.ExitCode != 0 {
		return toolError("couldn't read %s: %s", p, strings.TrimSpace(res.Output)), false
	}
	head, body, ok := strings.Cut(res.Output, "<<openbot>>\n")
	if !ok {
		head, body, _ = strings.Cut(res.Output, "<<openbot>>")
	}
	lines := strings.SplitN(strings.TrimSpace(head), "\n", 2)
	stamp := strings.TrimSpace(lines[0])
	total := 0
	if len(lines) > 1 {
		total, _ = strconv.Atoi(strings.TrimSpace(lines[1]))
	}
	if !utf8.ValidString(body) {
		return toolError("%s isn't a text file; inspect it with run_command (e.g. file, xxd)", p), false
	}
	if prev, seen := l.reads[p]; seen && prev.stamp == stamp && prev.offset == offset && prev.limit == limit {
		return toolOK(map[string]any{"path": p, "unchanged": true,
			"note": "Unchanged since you read this part; it's earlier in your context."}), true
	}

	var b strings.Builder
	n := 0
	next := 0
	for i, line := range strings.SplitAfter(body, "\n") {
		if line == "" {
			continue
		}
		if len(line) > readLineChars {
			line = cut(line, readLineChars) + " [line cut]\n"
		}
		if b.Len()+len(line) > readMaxChars {
			next = offset + i
			break
		}
		b.WriteString(line)
		n++
	}
	if next == 0 && offset+n <= total {
		next = offset + n
	}
	out := map[string]any{"path": p, "content": b.String(), "from_line": offset, "lines": n, "total_lines": total}
	if next > 0 && next <= total {
		out["next_offset"] = next
		out["note"] = fmt.Sprintf("There's more: continue with offset %d.", next)
	}
	if l.reads == nil {
		l.reads = map[string]readState{}
	}
	l.reads[p] = readState{stamp: stamp, offset: offset, limit: limit}
	return toolOK(out), true
}

// saveFullOutput saves a clipped command's whole output and says where.
func (l *loop) saveFullOutput(ctx context.Context, agent store.Agent, full []byte) string {
	if len(full) == 0 {
		return ""
	}
	p, err := l.spill(ctx, agent, "out", "log", full)
	if err != nil {
		return ""
	}
	return p
}
