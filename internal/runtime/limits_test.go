package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/etchebarne/openbot/internal/model"
	"github.com/etchebarne/openbot/internal/sandbox"
)

// fileSandbox answers read_file's command for a 3,000-line file and records saved files.
func fileSandbox(saved *sync.Map) *fakeSandbox {
	var lines []string
	for i := 1; i <= 3000; i++ {
		lines = append(lines, fmt.Sprintf("line %d", i))
	}
	return &fakeSandbox{exec: func(command string, env map[string]string) sandbox.Result {
		if strings.Contains(command, "<<openbot>>") {
			var s, n int
			fmt.Sscanf(command[strings.Index(command, "-v s="):], "-v s=%d -v n=%d", &s, &n)
			end := min(s+n-1, len(lines))
			return sandbox.Result{Output: "1700000000 99999\n3000\n<<openbot>>\n" + strings.Join(lines[s-1:end], "\n") + "\n"}
		}
		return sandbox.Result{Output: ""}
	}}
}

func TestReadFileInPages(t *testing.T) {
	ctx := context.Background()
	f := setup(t)
	f.rt.Sandboxes = fileSandbox(nil)
	agent, _ := f.store.GetAgent(ctx, f.agent.ID)
	l := &loop{m: f.rt, agentID: agent.ID}

	res, ok := l.readFile(ctx, agent, "/home/agent/big.txt", 0, 0)
	var page struct {
		Result struct {
			Content    string `json:"content"`
			Lines      int    `json:"lines"`
			Total      int    `json:"total_lines"`
			NextOffset int    `json:"next_offset"`
			Unchanged  bool   `json:"unchanged"`
		} `json:"result"`
	}
	json.Unmarshal([]byte(res), &page)
	if !ok || page.Result.Lines != 2000 || page.Result.Total != 3000 || page.Result.NextOffset != 2001 ||
		!strings.HasPrefix(page.Result.Content, "line 1\n") {
		t.Fatalf("first page: %s", cut(res, 300))
	}
	res, _ = l.readFile(ctx, agent, "/home/agent/big.txt", 2001, 0)
	page.Result.NextOffset = 0
	json.Unmarshal([]byte(res), &page)
	if page.Result.Lines != 1000 || page.Result.NextOffset != 0 || !strings.HasPrefix(page.Result.Content, "line 2001\n") {
		t.Fatalf("second page: %s", cut(res, 300))
	}
	// The same part of an unchanged file: a short answer.
	res, _ = l.readFile(ctx, agent, "/home/agent/big.txt", 2001, 0)
	if !strings.Contains(res, `"unchanged":true`) {
		t.Fatalf("reread: %s", cut(res, 300))
	}
}

func TestBigResultsAreSavedToFiles(t *testing.T) {
	ctx := context.Background()
	f := setup(t)
	sbx := &fakeSandbox{}
	f.rt.Sandboxes = sbx
	agent, _ := f.store.GetAgent(ctx, f.agent.ID)
	l := &loop{m: f.rt, agentID: agent.ID}

	// openbot's own tools: up to 100k characters stay.
	small := toolOK(map[string]string{"data": strings.Repeat("x", 50_000)})
	if got := l.limitResult(ctx, agent, model.ToolCall{Function: model.FunctionCall{Name: toolListAgents}}, small); got != small {
		t.Fatal("a result within its limit should be left alone")
	}
	big := toolOK(map[string]string{"data": strings.Repeat("y", 150_000)})
	got := l.limitResult(ctx, agent, model.ToolCall{Function: model.FunctionCall{Name: toolListAgents}}, big)
	if len(got) > 5_000 || !strings.Contains(got, `"full_result":"/tmp/openbot/`) || !strings.Contains(got, "preview") {
		t.Fatalf("a big result should be saved to a file: %s", cut(got, 400))
	}
	sbx.mu.Lock()
	if len(sbx.stdin) == 0 || len(sbx.stdin[len(sbx.stdin)-1]) != len(big) {
		t.Fatal("the whole result should be written to the file")
	}
	sbx.mu.Unlock()

	// Past the turn's budget, even mid-sized results go to files.
	l.turnChars = maxTurnResults
	got = l.limitResult(ctx, agent, model.ToolCall{Function: model.FunctionCall{Name: toolListAgents}}, small)
	if !strings.Contains(got, "full_result") {
		t.Fatalf("over the turn's budget: %s", cut(got, 300))
	}

	// Clipped command output: the whole of it is saved.
	res := commandResult(sandbox.Result{Output: "head…tail", Dropped: 10}, "/tmp/openbot/out.log")
	if !strings.Contains(res, `"full_output":"/tmp/openbot/out.log"`) {
		t.Fatalf("command result: %s", res)
	}
}
