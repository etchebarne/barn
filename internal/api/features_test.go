package api

import (
	"context"
	"net/http"
	"net/url"
	"testing"

	"github.com/etchebarne/openbot/internal/store"
)

func TestStopScheduleAndSearchAPI(t *testing.T) {
	c, st := setupWithKey(t)
	ctx := context.Background()
	agent, chat, err := st.CreateAgentWithDM(ctx, store.Agent{
		Name: "Ops", Instructions: "x", Model: "model-a", Language: "auto", TrustMode: "ask",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Stop: nothing to stop while idle; unknown agents are 404.
	if resp, _ := c.do("POST", "/api/agents/"+agent.ID+"/stop", "", true); resp.StatusCode != http.StatusConflict {
		t.Fatalf("stop idle agent: %d", resp.StatusCode)
	}
	if resp, _ := c.do("POST", "/api/agents/missing/stop", "", true); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("stop unknown agent: %d", resp.StatusCode)
	}

	// Schedule: the task, its next runs and (no) runs yet.
	resp, task := c.do("POST", "/api/agents/"+agent.ID+"/tasks",
		`{"name":"Morning digest","purpose":"Summarize deploys","cron":"0 9 * * *"}`, true)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create task: %d %v", resp.StatusCode, task)
	}
	taskID := task["id"].(string)
	resp, sched := c.do("GET", "/api/schedule?days=3", "", false)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("schedule: %d %v", resp.StatusCode, sched)
	}
	upcoming := sched["upcoming"].([]any)
	if len(sched["tasks"].([]any)) != 1 || len(upcoming) != 1 || len(upcoming[0].(map[string]any)["times"].([]any)) < 2 {
		t.Fatalf("schedule = %v", sched)
	}
	if resp, runs := c.doList("GET", "/api/tasks/"+taskID+"/runs"); resp.StatusCode != http.StatusOK || len(runs) != 0 {
		t.Fatalf("runs: %d %v", resp.StatusCode, runs)
	}
	if resp, _ := c.do("POST", "/api/tasks/"+taskID+"/run", "", true); resp.StatusCode != http.StatusAccepted {
		t.Fatalf("run now: %d", resp.StatusCode)
	}
	if resp, _ := c.do("POST", "/api/tasks/missing/run", "", true); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("run unknown task: %d", resp.StatusCode)
	}

	// Search: messages, memories and tasks.
	if _, err := st.InsertFull(ctx, store.NewMessage{ChatID: chat, AuthorKind: "user", Body: "Did the deploy go out?"}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.AddMemory(ctx, agent.ID, "Deploys happen on Tuesdays", nil); err != nil {
		t.Fatal(err)
	}
	resp, found := c.do("GET", "/api/search?q="+url.QueryEscape("deplo"), "", false)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("search: %d %v", resp.StatusCode, found)
	}
	if len(found["messages"].([]any)) != 1 || len(found["memories"].([]any)) != 1 || len(found["tasks"].([]any)) != 1 {
		t.Fatalf("search = %v", found)
	}
	if resp, _ := c.do("GET", "/api/search?q=%20", "", false); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("blank search: %d", resp.StatusCode)
	}
}
