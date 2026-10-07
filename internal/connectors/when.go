package connectors

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Models are poor at turning raw timestamps into "when", so connectors hand them a readable
// time in the user's time zone and how long ago it was.

type locationKey struct{}

// WithLocation tells connectors the user's time zone.
func WithLocation(ctx context.Context, loc *time.Location) context.Context {
	return context.WithValue(ctx, locationKey{}, loc)
}

func locationFrom(ctx context.Context) *time.Location {
	if loc, ok := ctx.Value(locationKey{}).(*time.Location); ok && loc != nil {
		return loc
	}
	return time.Local
}

// now is replaceable in tests.
var now = time.Now

// when describes t for a model: "2026-10-07 16:50 -03" and "6 min ago".
func when(ctx context.Context, t time.Time) (string, string) {
	return t.In(locationFrom(ctx)).Format("2006-01-02 15:04 MST"), ago(now().Sub(t))
}

func ago(d time.Duration) string {
	switch {
	case d < 0:
		return "in the future"
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%d min ago", int(d.Minutes()))
	case d < 48*time.Hour:
		h := int(d.Hours())
		if m := int(d.Minutes()) % 60; h < 6 && m >= 5 {
			return fmt.Sprintf("%d h %d min ago", h, m)
		}
		return fmt.Sprintf("%d h ago", h)
	default:
		return fmt.Sprintf("%d days ago", int(d.Hours()/24))
	}
}

// slackTime parses a Slack ts ("1791402652.594349").
func slackTime(ts string) (time.Time, bool) {
	secs, frac, _ := strings.Cut(ts, ".")
	s, err := strconv.ParseInt(secs, 10, 64)
	if err != nil || s <= 0 {
		return time.Time{}, false
	}
	var ns int64
	if frac != "" {
		f, _ := strconv.ParseFloat("0."+frac, 64)
		ns = int64(f * 1e9)
	}
	return time.Unix(s, ns), true
}
