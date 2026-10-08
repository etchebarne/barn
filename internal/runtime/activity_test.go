package runtime

import (
	"testing"
	"time"

	"github.com/etchebarne/openbot/internal/view"
)

func TestActivitySinceSpansAWorkingStretch(t *testing.T) {
	f := setup(t)
	id := "agent-x"

	f.rt.setActivity(id, view.Working("thinking"))
	first := f.rt.Activity(id)
	if first.Since == nil {
		t.Fatal("working activity has no since")
	}

	time.Sleep(5 * time.Millisecond)
	f.rt.setActivity(id, view.Working("running npm test"))
	if got := f.rt.Activity(id); got.Since == nil || !got.Since.Equal(*first.Since) {
		t.Fatalf("since changed with the label: %v, want %v", got.Since, first.Since)
	}

	f.rt.setActivity(id, view.Idle())
	if got := f.rt.Activity(id); got.Since != nil {
		t.Fatalf("idle activity has since %v", got.Since)
	}

	time.Sleep(5 * time.Millisecond)
	f.rt.setActivity(id, view.Working("thinking"))
	if got := f.rt.Activity(id); got.Since == nil || !got.Since.After(*first.Since) {
		t.Fatalf("a new working stretch kept the old since: %v", got.Since)
	}
}
