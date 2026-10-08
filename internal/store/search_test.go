package store

import (
	"context"
	"strings"
	"testing"
)

func TestSearch(t *testing.T) {
	ctx := context.Background()
	st, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	agent, chat, err := st.CreateAgentWithDM(ctx, Agent{Name: "Ops", Instructions: "x", Model: "m", Language: "auto", TrustMode: "ask"})
	if err != nil {
		t.Fatal(err)
	}
	say := func(kind, body string) Message {
		n := NewMessage{ChatID: chat, AuthorKind: kind, Body: body}
		if kind == "agent" {
			n.AuthorAgentID = &agent.ID
		}
		m, err := st.InsertFull(ctx, n)
		if err != nil {
			t.Fatal(err)
		}
		return m
	}
	say("user", "Can you check the Render deploys tonight?")
	hit := say("agent", "The deploy of café-api failed at 21:04, I'll retry it.")
	say("user", "unrelated chatter")
	if _, err := st.InsertFailure(ctx, chat, "Ops couldn't deploy anything", Failure{AgentID: agent.ID, Reason: "provider_error"}); err != nil {
		t.Fatal(err)
	}

	// Prefixes and accents match; every word must; system notices don't.
	got, err := st.SearchMessages(ctx, "deplo cafe", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != hit.ID || got[0].AuthorKind != "agent" {
		t.Fatalf("hits = %+v", got)
	}
	if !strings.Contains(got[0].Snippet, MatchStart+"deploy"+MatchEnd) {
		t.Fatalf("snippet = %q", got[0].Snippet)
	}
	if got, _ := st.SearchMessages(ctx, "deploy", 10); len(got) != 2 {
		t.Fatalf("deploy hits = %d", len(got))
	}
	// FTS syntax is just text.
	if _, err := st.SearchMessages(ctx, `"NEAR( deploy* OR`, 10); err != nil {
		t.Fatalf("odd query: %v", err)
	}
	// Cleared messages leave the index.
	if _, err := st.ClearChat(ctx, chat); err != nil {
		t.Fatal(err)
	}
	if got, _ := st.SearchMessages(ctx, "deploy", 10); len(got) != 0 {
		t.Fatalf("hits after clearing = %+v", got)
	}

	if _, err := st.AddMemory(ctx, agent.ID, "Martin's on-call week is 100% in October", nil); err != nil {
		t.Fatal(err)
	}
	if mems, err := st.SearchMemories(ctx, "on-call OCTOBER", 10); err != nil || len(mems) != 1 {
		t.Fatalf("memories = %+v, %v", mems, err)
	}
	if mems, _ := st.SearchMemories(ctx, "100%", 10); len(mems) != 1 {
		t.Fatalf("a literal %% should match: %+v", mems)
	}
	if mems, _ := st.SearchMemories(ctx, "1_0", 10); len(mems) != 0 {
		t.Fatalf("_ should be literal: %+v", mems)
	}
	if _, err := st.CreateTask(ctx, Task{AgentID: agent.ID, Name: "Deploy watch", Purpose: "Tell Martin when a deploy fails",
		Kind: "cron", Cron: "*/5 * * * *", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if tasks, err := st.SearchTasks(ctx, "watch fails", 10); err != nil || len(tasks) != 1 {
		t.Fatalf("tasks = %+v, %v", tasks, err)
	}
}
