package runtime

import (
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/etchebarne/openbot/internal/model"
	"github.com/etchebarne/openbot/internal/skills"
)

func TestSkills(t *testing.T) {
	var mu sync.Mutex
	var system, readResult string
	f := setup(t)
	lib := &skills.Library{Dir: filepath.Join(t.TempDir(), "skills")}
	if _, err := lib.Save("weekly-report", "Write the Friday report.", "1. Pull the numbers.\n2. Post a table."); err != nil {
		t.Fatal(err)
	}
	f.rt.Skills = lib
	f.llm.handler = func(req model.Request) model.Message {
		mu.Lock()
		defer mu.Unlock()
		last := req.Messages[len(req.Messages)-1]
		switch {
		case last.Role == "user" && strings.Contains(last.Text(), "weekly report please"):
			system = req.Messages[0].Text()
			return toolCall(toolUseSkill, map[string]string{"name": "weekly-report"})
		case last.Role == "tool" && strings.Contains(last.Text(), "Pull the numbers"):
			readResult = last.Text()
		case last.Role == "user" && strings.Contains(last.Text(), "remember how to triage"):
			return toolCall(toolSaveSkill, map[string]string{"name": "triage", "description": "Sort new issues.",
				"instructions": "Label by area, then by urgency."})
		}
		return model.Text("assistant", "")
	}

	f.userSays(t, "weekly report please")
	f.waitIdle(t)
	mu.Lock()
	if !strings.Contains(system, "# Skills") || !strings.Contains(system, "- weekly-report: Write the Friday report.") {
		t.Errorf("the prompt should list skills:\n%s", system)
	}
	if !strings.Contains(readResult, "/shared/skills/weekly-report") {
		t.Errorf("use_skill result = %s", readResult)
	}
	mu.Unlock()

	// Saving a skill waits for the user's approval, then writes it.
	f.userSays(t, "remember how to triage issues")
	msgs := f.waitForMessages(t, 3)
	f.waitIdle(t)
	card := msgs[len(msgs)-1]
	if card.Prompt == nil || card.Prompt.Kind != "approval" || card.Prompt.Preview == nil || card.Prompt.Preview.Title != "Save skill" {
		t.Fatalf("expected a Save skill card, got %+v", card)
	}
	if _, err := lib.Get("triage"); err == nil {
		t.Fatal("the skill was saved before it was approved")
	}
	answerApproval(t, f, card, true)
	f.waitIdle(t)
	got, err := lib.Get("triage")
	if err != nil || got.Instructions != "Label by area, then by urgency.\n" {
		t.Fatalf("approved skill = %+v, %v", got, err)
	}
}
