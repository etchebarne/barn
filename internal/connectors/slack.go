package connectors

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coder/websocket"
)

// Slack: post and read messages through the Web API; receive mentions and DMs over Socket Mode
// (a websocket openbot opens to Slack, so no public URL is needed).
type Slack struct{}

func (Slack) Name() string        { return "slack" }
func (Slack) DisplayName() string { return "Slack" }
func (Slack) Description() string {
	return "Read channels and post messages as you or as the bot; get notified when the bot is mentioned or messaged."
}
func (Slack) CredentialFields() []Field {
	return []Field{
		{Key: "bot_token", Label: "Bot token", Secret: true, Help: "Starts with xoxb-. From Install App or OAuth & Permissions."},
		{Key: "user_token", Label: "User token", Secret: true, Optional: true,
			Help: "Starts with xoxp-. Lets agents post as you (Slack shows “Sent using” under it). Without it they post as the bot."},
		{Key: "app_token", Label: "App-level token", Secret: true, Optional: true,
			Help: "Starts with xapp-. Lets agents hear mentions and DMs."},
	}
}
func (Slack) ConfigFields() []Field { return nil }
func (Slack) SignalTypes() []SignalType {
	f := []string{"channel", "user", "user_name", "text", "ts", "thread_ts"}
	return []SignalType{
		{Type: "slack.app_mention", Description: "Someone mentioned the bot in a channel", Fields: f},
		{Type: "slack.message", Description: "A message (from a person, a bot or an integration such as alerts) in a DM with the bot or in a channel it's in", Fields: append(f, "channel_type")},
	}
}

func (Slack) Tools(_ context.Context, acct Account) ([]Tool, error) {
	post := Tool{Name: "post_message", Description: "Post a message to a channel (by name like #alerts or id), optionally in a thread.", External: true,
		Title: "Slack message", Verb: "Send message", Body: "text", Labels: map[string]string{"channel": "To", "thread_ts": "Thread"},
		Parameters: params(map[string]string{"channel": "#name or channel id", "text": "message (Slack mrkdwn)", "?thread_ts": "reply in this thread"})}
	if acct.Credentials["user_token"] != "" {
		// With the user's token, messages always go out as them ("Sent using" the app).
		post.Title = "Slack message as you"
		post.Description = "Post a message as the user to a channel (by name like #alerts or id, including the " +
			"user's own channels), optionally in a thread."
	}
	return []Tool{
		post,
		{Name: "read_channel", Description: "Read recent messages in a channel.",
			Parameters: params(map[string]string{"channel": "#name or channel id", "?limit": "int:how many (default 20, max 100)"})},
		{Name: "list_channels", Description: "List channels the bot can see.", Parameters: params(map[string]string{})},
		{Name: "get_user", Description: "Look up a user's name by id.", Parameters: params(map[string]string{"user": "user id, e.g. U123"})},
	}, nil
}

func (Slack) call(ctx context.Context, acct Account, token, method string, args map[string]any, out any) error {
	h := http.Header{}
	h.Set("Authorization", "Bearer "+token)
	var raw map[string]any
	if err := apiRequest(ctx, "POST", baseURL(acct, "https://slack.com/api")+"/"+method, h, args, &raw); err != nil {
		return err
	}
	if ok, _ := raw["ok"].(bool); !ok {
		return userErr("Slack %s: %s", method, str(raw, "error"))
	}
	b, _ := json.Marshal(raw)
	return json.Unmarshal(b, out)
}

// channelID resolves "#name" (or a bare name) to a channel id.
func (s Slack) channelID(ctx context.Context, acct Account, token, channel string) (string, error) {
	name := strings.TrimPrefix(channel, "#")
	if name == "" {
		return "", userErr("channel is required")
	}
	if !strings.HasPrefix(channel, "#") && (strings.HasPrefix(name, "C") || strings.HasPrefix(name, "G") || strings.HasPrefix(name, "D")) && strings.ToUpper(name) == name {
		return name, nil
	}
	var out struct {
		Channels []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"channels"`
	}
	if err := s.call(ctx, acct, token, "conversations.list",
		map[string]any{"types": "public_channel,private_channel", "limit": 1000, "exclude_archived": true}, &out); err != nil {
		return "", err
	}
	for _, c := range out.Channels {
		if strings.EqualFold(c.Name, name) {
			return c.ID, nil
		}
	}
	if token == acct.Credentials["user_token"] {
		return "", userErr("no channel #%s that the user is in", name)
	}
	return "", userErr("no channel #%s that the bot can see (invite it with /invite)", name)
}

func (s Slack) Call(ctx context.Context, acct Account, tool string, args json.RawMessage) (any, error) {
	var a struct {
		Channel  string `json:"channel"`
		Text     string `json:"text"`
		ThreadTS string `json:"thread_ts"`
		Limit    int    `json:"limit"`
		User     string `json:"user"`
	}
	if err := decodeArgs(args, &a); err != nil {
		return nil, err
	}
	token := acct.Credentials["bot_token"]
	switch tool {
	case "post_message":
		if strings.TrimSpace(a.Text) == "" {
			return nil, userErr("text is required")
		}
		if user := acct.Credentials["user_token"]; user != "" {
			token = user // post as the user when their token is set
		}
		id, err := s.channelID(ctx, acct, token, a.Channel)
		if err != nil {
			return nil, err
		}
		body := map[string]any{"channel": id, "text": a.Text}
		if a.ThreadTS != "" {
			body["thread_ts"] = a.ThreadTS
		}
		var out struct {
			TS string `json:"ts"`
		}
		if err := s.call(ctx, acct, token, "chat.postMessage", body, &out); err != nil {
			return nil, err
		}
		return map[string]string{"channel": id, "ts": out.TS}, nil
	case "read_channel":
		id, err := s.channelID(ctx, acct, token, a.Channel)
		if err != nil {
			return nil, err
		}
		limit := a.Limit
		if limit <= 0 || limit > 100 {
			limit = 20
		}
		var out struct {
			Messages []map[string]any `json:"messages"`
		}
		if err := s.call(ctx, acct, token, "conversations.history", map[string]any{"channel": id, "limit": limit}, &out); err != nil {
			return nil, err
		}
		var msgs []map[string]string
		for _, m := range out.Messages {
			from := firstNonEmpty(str(m, "bot_profile.name"), str(m, "username")) // bots and integrations
			if u := str(m, "user"); u != "" && str(m, "bot_id") == "" {
				from = s.userName(ctx, acct, u)
			}
			msg := map[string]string{"from": from, "user": str(m, "user"),
				"text": truncateStr(s.readable(ctx, acct, str(m, "text")), 2000), "ts": str(m, "ts"), "thread_ts": str(m, "thread_ts")}
			if t, ok := slackTime(str(m, "ts")); ok {
				msg["time"], msg["ago"] = when(ctx, t)
			}
			msgs = append(msgs, msg)
		}
		return msgs, nil
	case "list_channels":
		var out struct {
			Channels []map[string]any `json:"channels"`
		}
		if err := s.call(ctx, acct, token, "conversations.list", map[string]any{"types": "public_channel,private_channel", "limit": 200, "exclude_archived": true}, &out); err != nil {
			return nil, err
		}
		var chans []map[string]string
		for _, c := range out.Channels {
			chans = append(chans, map[string]string{"id": str(c, "id"), "name": "#" + str(c, "name"), "is_member": str(c, "is_member")})
		}
		return chans, nil
	case "get_user":
		var out struct {
			User map[string]any `json:"user"`
		}
		if err := s.call(ctx, acct, token, "users.info", map[string]any{"user": a.User}, &out); err != nil {
			return nil, err
		}
		return map[string]string{"id": str(out.User, "id"), "name": str(out.User, "real_name"), "handle": str(out.User, "name")}, nil
	}
	return nil, userErr("unknown tool %q", tool)
}

func (s Slack) Verify(ctx context.Context, acct Account) error {
	var out map[string]any
	if err := s.call(ctx, acct, acct.Credentials["bot_token"], "auth.test", nil, &out); err != nil {
		return err
	}
	if user := acct.Credentials["user_token"]; user != "" {
		if !strings.HasPrefix(user, "xoxp-") {
			return userErr("the user token should start with xoxp- (the bot token starts with xoxb-)")
		}
		var who map[string]any
		if err := s.call(ctx, acct, user, "auth.test", nil, &who); err != nil {
			return fmt.Errorf("user token: %w", err)
		}
	}
	if app := acct.Credentials["app_token"]; app != "" {
		var conn map[string]any
		if err := s.call(ctx, acct, app, "apps.connections.open", nil, &conn); err != nil {
			return fmt.Errorf("app-level token: %w", err)
		}
	}
	return nil
}

// Listen runs Socket Mode: open a connection URL, then read event envelopes, acknowledging
// each. Slack asks clients to reconnect from time to time; Listen returns and is restarted.
func (s Slack) Listen(ctx context.Context, acct Account, emit func(Signal)) error {
	app := acct.Credentials["app_token"]
	if app == "" {
		<-ctx.Done() // no Socket Mode configured; nothing to listen to
		return nil
	}
	var conn struct {
		URL string `json:"url"`
	}
	if err := s.call(ctx, acct, app, "apps.connections.open", nil, &conn); err != nil {
		return err
	}
	if _, err := url.Parse(conn.URL); err != nil || conn.URL == "" {
		return fmt.Errorf("Slack returned no socket URL")
	}
	// Who we are, so our own messages don't wake anyone (other bots' messages, like alerts
	// from integrations, do).
	var self struct {
		UserID string `json:"user_id"`
		BotID  string `json:"bot_id"`
	}
	_ = s.call(ctx, acct, acct.Credentials["bot_token"], "auth.test", nil, &self)
	ws, _, err := websocket.Dial(ctx, conn.URL, nil)
	if err != nil {
		return err
	}
	defer ws.CloseNow()
	ws.SetReadLimit(4 << 20)
	for {
		_, data, err := ws.Read(ctx)
		if err != nil {
			return err
		}
		var env struct {
			EnvelopeID string `json:"envelope_id"`
			Type       string `json:"type"`
			Payload    struct {
				Event map[string]any `json:"event"`
			} `json:"payload"`
		}
		if err := json.Unmarshal(data, &env); err != nil {
			continue
		}
		if env.EnvelopeID != "" {
			ack, _ := json.Marshal(map[string]string{"envelope_id": env.EnvelopeID})
			wctx, cancel := context.WithTimeout(ctx, 10*time.Second)
			err := ws.Write(wctx, websocket.MessageText, ack)
			cancel()
			if err != nil {
				return err
			}
		}
		switch env.Type {
		case "disconnect":
			return nil // Slack wants us to reconnect
		case "events_api":
			if sig, ok := s.slackSignal(ctx, acct, self.UserID, self.BotID, env.Payload.Event); ok {
				emit(sig)
			}
		}
	}
}

func (s Slack) slackSignal(ctx context.Context, acct Account, selfUser, selfBot string, ev map[string]any) (Signal, bool) {
	// Ignore our own messages (no loops) and edits, joins and the like (noise). Messages from
	// other bots and integrations count: that's how alerts arrive.
	switch str(ev, "subtype") {
	case "", "bot_message", "thread_broadcast", "file_share":
	default:
		return Signal{}, false
	}
	if (selfBot != "" && str(ev, "bot_id") == selfBot) || (selfUser != "" && str(ev, "user") == selfUser) {
		return Signal{}, false
	}
	name := firstNonEmpty(str(ev, "bot_profile.name"), str(ev, "username"))
	if str(ev, "bot_id") == "" {
		name = s.userName(ctx, acct, str(ev, "user"))
	}
	f := map[string]string{"channel": str(ev, "channel"), "user": str(ev, "user"),
		"user_name": name,
		"text":      truncateStr(s.readable(ctx, acct, str(ev, "text")), 4000),
		"ts":        str(ev, "ts"), "thread_ts": str(ev, "thread_ts")}
	switch str(ev, "type") {
	case "app_mention":
		return Signal{Type: "slack.app_mention", Fields: f}, true
	case "message":
		f["channel_type"] = str(ev, "channel_type")
		return Signal{Type: "slack.message", Fields: f}, true
	}
	return Signal{}, false
}
