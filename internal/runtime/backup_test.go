package runtime

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/etchebarne/openbot/internal/store"
)

func TestBackupRoundTrip(t *testing.T) {
	ctx := context.Background()
	src := setup(t)
	slack, err := src.store.CreateAccount(ctx, store.ConnectorAccount{Type: "slack", Name: "Work Slack", Config: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	github, err := src.store.CreateAccount(ctx, store.ConnectorAccount{Type: "github", Name: "GitHub", Config: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	a := src.agent
	if err := src.rt.store.UpdateAgent(ctx, a.ID, store.AgentUpdate{Personality: ptr("Dry and brief.")}); err != nil {
		t.Fatal(err)
	}
	src.store.AddMemory(ctx, a.ID, "Martin's on call on Tuesdays.", nil)
	src.store.AddGrant(ctx, slack.ID, a.ID)
	src.store.AddGrant(ctx, github.ID, a.ID)
	mk := func(task store.Task) {
		task.AgentID, task.Purpose, task.Enabled = a.ID, "Do it.", true
		if _, err := src.store.CreateTask(ctx, task); err != nil {
			t.Fatal(err)
		}
	}
	mk(store.Task{Name: "Daily", Kind: "cron", Cron: "0 9 * * *"})
	mk(store.Task{Name: "Past", Kind: "once", At: time.Now().Add(-time.Hour).UnixMilli()})
	mk(store.Task{Name: "Soon", Kind: "once", At: time.Now().Add(48 * time.Hour).UnixMilli()})
	mk(store.Task{Name: "On mention", Kind: "signal", SignalAccountID: slack.ID, SignalType: "slack.app_mention", SignalMatch: map[string]string{"channel": "alerts"}})
	mk(store.Task{Name: "On PR", Kind: "signal", SignalAccountID: github.ID, SignalType: "pull_request.opened"})
	src.store.AllowAction(ctx, a.ID, "connector:"+slack.ID+":post_message", map[string]string{"channel": "alerts"}, "Post · Work Slack")
	src.store.AllowAction(ctx, a.ID, "connector:"+github.ID+":create_issue", nil, "Create issue · GitHub")
	src.store.AllowAction(ctx, a.ID, "tool:save_skill", nil, "Save skill")

	backup, err := src.rt.ExportAgents(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// Through JSON, as a file would.
	raw, _ := json.Marshal(backup)
	if strings.Contains(string(raw), slack.ID) {
		t.Fatal("the backup should refer to connections by name, not id")
	}
	if err := json.Unmarshal(raw, &backup); err != nil {
		t.Fatal(err)
	}

	// A fresh server with only Slack connected (under the same name, different case).
	dst := setup(t)
	if _, err := dst.store.CreateAccount(ctx, store.ConnectorAccount{Type: "slack", Name: "work slack", Config: json.RawMessage(`{}`)}); err != nil {
		t.Fatal(err)
	}
	dst.rt.Timezone = func(context.Context) *time.Location { return time.UTC }
	res, err := dst.rt.RestoreAgents(ctx, backup)
	if err != nil {
		t.Fatal(err)
	}
	// The fixture's own agent has the same name, so it's skipped: rename the backup's and retry.
	if len(res.Agents) != 1 || res.Agents[0].Restored || !strings.Contains(res.Agents[0].Notes[0], "already exists") {
		t.Fatalf("restore over the same name = %+v", res)
	}
	backup.Agents[0].Name = "Restored"
	if res, err = dst.rt.RestoreAgents(ctx, backup); err != nil {
		t.Fatal(err)
	}
	got := res.Agents[0]
	if !got.Restored || got.AgentId == nil {
		t.Fatalf("restore = %+v", got)
	}
	notes := strings.Join(got.Notes, "\n")
	for _, want := range []string{`"Past": its time has passed`, `"On PR": it runs on events from GitHub`, "No connection named GitHub", `"Create issue · GitHub"`} {
		if !strings.Contains(notes, want) {
			t.Errorf("notes lack %q:\n%s", want, notes)
		}
	}

	agent, _ := dst.store.GetAgent(ctx, *got.AgentId)
	if agent.Personality != "Dry and brief." || agent.Instructions != a.Instructions {
		t.Errorf("agent = %+v", agent)
	}
	mems, _ := dst.store.Memories(ctx, agent.ID)
	if len(mems) != 1 || mems[0].Text != "Martin's on call on Tuesdays." {
		t.Errorf("memories = %+v", mems)
	}
	tasks, _ := dst.store.Tasks(ctx, agent.ID)
	names := []string{}
	for _, task := range tasks {
		names = append(names, task.Name)
		if task.Kind != "signal" && task.NextFireAt == nil {
			t.Errorf("%s has no next run", task.Name)
		}
	}
	if strings.Join(names, ",") != "Daily,Soon,On mention" {
		t.Errorf("tasks = %v", names)
	}
	accts, _ := dst.store.AgentAccounts(ctx, agent.ID)
	if len(accts) != 1 || accts[0].Name != "work slack" {
		t.Errorf("connections = %+v", accts)
	}
	rules, _ := dst.store.ApprovalRules(ctx, agent.ID)
	if len(rules) != 2 {
		t.Fatalf("approvals = %+v", rules)
	}
	for _, r := range rules {
		if strings.HasPrefix(r.Action, "connector:") && (!strings.HasSuffix(r.Action, ":post_message") || r.Match["channel"] != "alerts") {
			t.Errorf("connector approval = %+v", r)
		}
	}

	// Not a backup at all.
	backup.Format = "something-else"
	if _, err := dst.rt.RestoreAgents(ctx, backup); err == nil {
		t.Error("a file of another format should be refused")
	}
}
