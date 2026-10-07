package runtime

import (
	"context"
	"errors"
	"testing"

	"github.com/etchebarne/openbot/internal/store"
)

// Deleting an agent removes what's only its own and keeps group conversations readable.
func TestDeleteAgent(t *testing.T) {
	ctx := context.Background()
	f := setup(t)
	sbx := &fakeSandbox{}
	f.rt.Sandboxes = sbx
	other, otherDM, _ := f.store.CreateAgentWithDM(ctx, store.Agent{Name: "Tracker", Instructions: "x", Model: "m", Language: "auto", TrustMode: "ask"})
	mate, _, _ := f.store.CreateAgentWithDM(ctx, store.Agent{Name: "Mate", Instructions: "x", Model: "m", Language: "auto", TrustMode: "ask"})

	group, err := f.store.CreateGroup(ctx, "Launch", []string{f.agent.ID, other.ID})
	if err != nil {
		t.Fatal(err)
	}
	id := f.agent.ID
	said, _ := f.store.InsertMessage(ctx, group.ID, "agent", &id, "shipping today")
	if _, err := f.store.ToggleReaction(ctx, said.ID, "agent:"+other.ID, "🎉"); err != nil {
		t.Fatal(err)
	}
	theirs, _ := f.store.InsertMessage(ctx, otherDM, "user", nil, "hi")
	f.store.ToggleReaction(ctx, theirs.ID, "agent:"+id, "👍")
	f.store.AddMemory(ctx, id, "Martin likes short replies", nil)
	f.store.CreateTask(ctx, store.Task{AgentID: id, Name: "Daily", Purpose: "p", Kind: "cron", Cron: "0 9 * * *", Enabled: true})
	own, _ := f.store.SandboxFor(ctx, id)
	// Mate shares Tracker's sandbox, so that one must survive Tracker's deletion later.
	f.store.SandboxFor(ctx, other.ID)
	f.store.ShareSandbox(ctx, mate.ID, other.ID)

	if err := f.rt.DeleteAgent(ctx, id); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.GetAgent(ctx, id); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("agent still there: %v", err)
	}
	if _, err := f.store.GetChat(ctx, f.chatID); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("its DM should be gone")
	}
	if mems, _ := f.store.Memories(ctx, id); len(mems) != 0 {
		t.Fatal("memories should be gone")
	}
	if tasks, _ := f.store.Tasks(ctx, id); len(tasks) != 0 {
		t.Fatal("tasks should be gone")
	}
	msg, err := f.store.GetMessage(ctx, said.ID)
	if err != nil || msg.AuthorKind != "agent" || msg.AuthorAgentID != nil || len(msg.Reactions) != 1 {
		t.Fatalf("its group message should stay, without an author: %+v %v", msg, err)
	}
	if m, _ := f.store.GetMessage(ctx, theirs.ID); len(m.Reactions) != 0 {
		t.Fatal("its reactions elsewhere should be gone")
	}
	if len(sbx.removed) != 1 || sbx.removed[0] != own {
		t.Fatalf("its own sandbox should be removed: %v", sbx.removed)
	}

	// A shared sandbox stays while another agent uses it.
	if err := f.rt.DeleteAgent(ctx, other.ID); err != nil {
		t.Fatal(err)
	}
	if len(sbx.removed) != 1 {
		t.Fatalf("a shared sandbox must stay: %v", sbx.removed)
	}

	// The last admin can't be deleted.
	admin, _, _ := f.store.CreateAgentWithDM(ctx, store.Agent{Name: "openbot", Instructions: "x", Model: "m", Language: "auto", TrustMode: "ask", IsAdmin: true})
	if err := f.rt.DeleteAgent(ctx, admin.ID); !errors.Is(err, store.ErrLastAdmin) {
		t.Fatalf("deleting the last admin: %v", err)
	}
	if err := f.rt.DeleteAgent(ctx, "nope"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("unknown agent: %v", err)
	}
}
