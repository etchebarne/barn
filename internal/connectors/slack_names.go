package connectors

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Slack hands out user ids (U07…) and markup like <@U07…>; agents should see names.

var slackNames = struct {
	sync.Mutex
	m map[string]slackName // account id + user id
}{m: map[string]slackName{}}

type slackName struct {
	name    string
	expires time.Time
}

// userName returns a Slack user's display name (cached for an hour), or the id if unknown.
func (s Slack) userName(ctx context.Context, acct Account, id string) string {
	if id == "" {
		return ""
	}
	key := acct.ID + "|" + id
	slackNames.Lock()
	n, ok := slackNames.m[key]
	slackNames.Unlock()
	if ok && time.Now().Before(n.expires) {
		return n.name
	}
	var out struct {
		User map[string]any `json:"user"`
	}
	name := id
	lctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := s.get(lctx, acct, "users.info", url.Values{"user": {id}}, &out); err == nil {
		name = firstNonEmpty(str(out.User, "profile.display_name"), str(out.User, "real_name"), str(out.User, "name"), id)
	}
	slackNames.Lock()
	slackNames.m[key] = slackName{name: name, expires: time.Now().Add(time.Hour)}
	slackNames.Unlock()
	return name
}

var (
	slackUserRef    = regexp.MustCompile(`<@([UW][A-Z0-9]+)(?:\|[^>]*)?>`)
	slackChannelRef = regexp.MustCompile(`<#[CG][A-Z0-9]+\|([^>]*)>`)
	slackLinkRef    = regexp.MustCompile(`<((?:https?|mailto):[^|>]+)\|([^>]+)>`)
	slackBareLink   = regexp.MustCompile(`<((?:https?|mailto):[^>]+)>`)
	slackSpecial    = regexp.MustCompile(`<!(here|channel|everyone)[^>]*>`)
)

// get calls a read method with query parameters, which every Slack method accepts (JSON
// bodies aren't supported by all of them).
func (s Slack) get(ctx context.Context, acct Account, method string, q url.Values, out any) error {
	h := http.Header{}
	h.Set("Authorization", "Bearer "+acct.Credentials["bot_token"])
	var raw map[string]any
	if err := apiRequest(ctx, "GET", baseURL(acct, "https://slack.com/api")+"/"+method+"?"+q.Encode(), h, nil, &raw); err != nil {
		return err
	}
	if ok, _ := raw["ok"].(bool); !ok {
		return userErr("Slack %s: %s", method, str(raw, "error"))
	}
	b, _ := json.Marshal(raw)
	return json.Unmarshal(b, out)
}

// readable turns Slack message markup into plain text: <@U1> → @Name, <#C1|general> →
// #general, <https://x|label> → label (https://x), <!here> → @here.
func (s Slack) readable(ctx context.Context, acct Account, text string) string {
	text = slackUserRef.ReplaceAllStringFunc(text, func(m string) string {
		return "@" + s.userName(ctx, acct, slackUserRef.FindStringSubmatch(m)[1])
	})
	text = slackChannelRef.ReplaceAllString(text, "#$1")
	text = slackLinkRef.ReplaceAllString(text, "$2 ($1)")
	text = slackBareLink.ReplaceAllString(text, "$1")
	text = slackSpecial.ReplaceAllString(text, "@$1")
	return strings.NewReplacer("&lt;", "<", "&gt;", ">", "&amp;", "&").Replace(text)
}
