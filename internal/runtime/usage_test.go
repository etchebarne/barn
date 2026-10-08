package runtime

import (
	"context"
	"testing"

	"github.com/etchebarne/openbot/internal/model"
)

func TestUsageIsRecorded(t *testing.T) {
	var f fixture
	f = setup(t, func(model.Request) model.Message { return sendCall(f.chatID, "hi") })
	f.userSays(t, "hello")
	f.waitIdle(t)
	rows, err := f.store.UsageSince(context.Background(), f.agent.ID, 0)
	if err != nil || len(rows) != 2 || rows[0].Purpose != "turn" || rows[0].Model != "test-model" {
		t.Fatalf("usage rows: %+v %v", rows, err)
	}
	if got := f.rt.compactAt("unknown-model"); got != min(DefaultCompactAtTokens, DefaultContextWindow/2) {
		t.Fatalf("compactAt = %d", got)
	}
	f.rt.ContextWindow = func(string) int { return 32_000 }
	if got := f.rt.compactAt("small"); got != 16_000 {
		t.Fatalf("small window: compactAt = %d", got)
	}
}
