package runtime

import (
	"context"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/etchebarne/barn/internal/model"
	"github.com/etchebarne/barn/internal/store"
)

func TestFindMentions(t *testing.T) {
	agents := []store.Agent{{ID: "a", Name: "Weekly Digest"}, {ID: "b", Name: "Tracker"}, {ID: "c", Name: "Track"}}
	cases := map[string][]string{
		"@Tracker can you check?":            {"b"},
		"@track this":                        {"c"},
		"@Weekly Digest and @tracker please": {"a", "b"},
		"hey @weekly digest, @Weekly Digest": {"a"},
		"email me at martin@tracker.com":     nil,
		"@Trackers isn't anyone":             nil,
		"(@Tracker) look":                    {"b"},
		"no mentions here":                   nil,
	}
	for body, want := range cases {
		if got := findMentions(body, agents); !slices.Equal(got, want) {
			t.Errorf("%q: got %v, want %v", body, got, want)
		}
	}
}

func TestTurnOrder(t *testing.T) {
	members := []store.ChatMember{{AgentID: "a", Position: 0}, {AgentID: "b", Position: 1}, {AgentID: "c", Position: 2}}
	if got := turnOrder(members, nil, ""); !slices.Equal(got, []string{"a", "b", "c"}) {
		t.Errorf("plain order: %v", got)
	}
	if got := turnOrder(members, []string{"c", "zz"}, ""); !slices.Equal(got, []string{"c", "a", "b"}) {
		t.Errorf("mentioned first: %v", got)
	}
	if got := turnOrder(members, []string{"b"}, "b"); !slices.Equal(got, []string{"a", "c"}) {
		t.Errorf("the author is skipped: %v", got)
	}
}

type groupFixture struct {
	fixture
	a, b store.Agent
	chat store.Chat
	mu   sync.Mutex
	log  []string // "<agent>: <what it saw>"
}

// setupGroup creates agents A and B (besides the fixture's barn) in a group, all answered by
// script(agentName, lastUserText) which returns what to post ("" to stay silent).
func setupGroup(t *testing.T, script func(name, seen string) string) *groupFixture {
	t.Helper()
	g := &groupFixture{fixture: setup(t)}
	ctx := context.Background()
	mk := func(name string) store.Agent {
		a, _, err := g.store.CreateAgentWithDM(ctx, store.Agent{Name: name, Instructions: "x", Model: "test-model", Language: "auto", TrustMode: "ask"})
		if err != nil {
			t.Fatal(err)
		}
		g.rt.AddAgent(a.ID)
		return a
	}
	g.a, g.b = mk("Alpha"), mk("Beta")
	chatRe := regexp.MustCompile(`<group_turn chat_id="([^"]+)"`)
	g.llm.handler = func(req model.Request) model.Message {
		name := strings.TrimPrefix(strings.SplitN(req.Messages[0].Text(), ",", 2)[0], "You are ")
		last := req.Messages[len(req.Messages)-1]
		if last.Role != "user" || !strings.Contains(last.Text(), "<group_turn") {
			return model.Text("assistant", "")
		}
		g.mu.Lock()
		g.log = append(g.log, name+": "+last.Text())
		g.mu.Unlock()
		if say := script(name, last.Text()); say != "" {
			return sendCall(chatRe.FindStringSubmatch(last.Text())[1], say)
		}
		return model.Text("assistant", "")
	}
	chat, err := g.rt.CreateGroup(ctx, "Crew", []string{g.a.ID, g.b.ID}, nil)
	if err != nil {
		t.Fatal(err)
	}
	g.chat = chat
	// Let the "you were added" notices settle.
	g.waitAllIdle(t)
	g.mu.Lock()
	g.log = nil
	g.mu.Unlock()
	return g
}

func (g *groupFixture) waitAllIdle(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		busy := false
		for _, id := range []string{g.agent.ID, g.a.ID, g.b.ID} {
			pending, _ := g.store.PendingEvents(context.Background(), id)
			if len(pending) > 0 || g.rt.Activity(id).State != "idle" {
				busy = true
			}
		}
		g.rt.mu.Lock()
		cycling := len(g.rt.cycles) > 0
		g.rt.mu.Unlock()
		if !busy && !cycling {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("timed out waiting for the group to settle")
}

func (g *groupFixture) userPosts(t *testing.T, body string) {
	t.Helper()
	ctx := context.Background()
	msg, err := g.store.InsertMessage(ctx, g.chat.ID, "user", nil, body, g.rt.MentionsIn(ctx, g.chat.ID, body)...)
	if err != nil {
		t.Fatal(err)
	}
	if err := g.rt.DeliverUserMessage(ctx, msg); err != nil {
		t.Fatal(err)
	}
}

func (g *groupFixture) turns() []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	out := make([]string, len(g.log))
	for i, l := range g.log {
		out[i] = strings.SplitN(l, ":", 2)[0]
	}
	return out
}

func TestGroupTurnTaking(t *testing.T) {
	g := setupGroup(t, func(name, seen string) string {
		switch {
		case name == "Beta" && strings.Contains(seen, "@Beta can you start"):
			return "Started. @Alpha your turn to review."
		case name == "Alpha" && strings.Contains(seen, "your turn to review"):
			return "Reviewed, looks good."
		}
		return ""
	})
	g.userPosts(t, "@Beta can you start the report?")
	g.waitAllIdle(t)

	// Beta was mentioned, so it goes first; Alpha then sees both messages and replies; Beta gets a
	// turn for Alpha's reply and stays quiet; a silent round ends the cycle.
	if got := g.turns(); !slices.Equal(got, []string{"Beta", "Alpha", "Beta"}) {
		t.Fatalf("turns = %v", got)
	}
	g.mu.Lock()
	alphaSaw := g.log[1]
	lastBeta := g.log[2]
	g.mu.Unlock()
	if !strings.Contains(alphaSaw, "can you start the report") || !strings.Contains(alphaSaw, `from="Beta" sent_at`) ||
		!strings.Contains(alphaSaw, `mentions_you="true"`) {
		t.Fatalf("Alpha should see the user's message and Beta's mention of it: %s", alphaSaw)
	}
	if strings.Contains(lastBeta, "can you start the report") || !strings.Contains(lastBeta, "Reviewed, looks good") {
		t.Fatalf("Beta should only see what's new since its last turn: %s", lastBeta)
	}
	msgs, _, _ := g.store.ListMessages(context.Background(), g.chat.ID, "", 50)
	var bodies []string
	for _, m := range msgs {
		bodies = append(bodies, m.Body)
	}
	if !slices.Contains(bodies, "Started. @Alpha your turn to review.") || !slices.Contains(bodies, "Reviewed, looks good.") {
		t.Fatalf("unexpected group messages: %v", bodies)
	}
	// Beta's message mentioned Alpha.
	for _, m := range msgs {
		if strings.HasPrefix(m.Body, "Started.") && !slices.Equal(m.Mentions, []string{g.a.ID}) {
			t.Fatalf("mentions = %v", m.Mentions)
		}
	}
}

func TestGroupSilenceEndsQuickly(t *testing.T) {
	g := setupGroup(t, func(string, string) string { return "" })
	g.userPosts(t, "fyi, the deploy is done")
	g.waitAllIdle(t)
	if got := g.turns(); !slices.Equal(got, []string{"Alpha", "Beta"}) {
		t.Fatalf("each agent should get exactly one turn, got %v", got)
	}
}

func TestGroupRoundsAreCapped(t *testing.T) {
	n := 0
	var mu sync.Mutex
	g := setupGroup(t, func(name, _ string) string {
		mu.Lock()
		defer mu.Unlock()
		n++
		return "and another thing #" + name + strings.Repeat("!", n%3)
	})
	g.userPosts(t, "discuss")
	g.waitAllIdle(t)
	if turns := len(g.turns()); turns > 2*maxGroupRounds {
		t.Fatalf("expected at most %d turns, got %d", 2*maxGroupRounds, turns)
	}
	last, _ := g.store.LastMessage(context.Background(), g.chat.ID)
	if last.AuthorKind != "system" || !strings.Contains(last.Body, "pausing here") {
		t.Fatalf("expected a pause notice, got %+v", last)
	}
}
