package connectors

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/etchebarne/openbot/internal/secrets"
	"github.com/etchebarne/openbot/internal/store"
)

func hexHMAC(secret string, body []byte) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write(body)
	return hex.EncodeToString(m.Sum(nil))
}

func call(t *testing.T, typ Type, acct Account, tool string, args string) any {
	t.Helper()
	out, err := typ.Call(context.Background(), acct, tool, json.RawMessage(args))
	if err != nil {
		t.Fatalf("%s: %v", tool, err)
	}
	return out
}

// ---------------------------------------------------------------- GitHub

func TestGitHub(t *testing.T) {
	var mu sync.Mutex
	var created map[string]any
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer gh-token" {
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprint(w, `{"message":"Bad credentials"}`)
			return
		}
		switch {
		case r.Method == "GET" && r.URL.Path == "/user":
			fmt.Fprint(w, `{"login":"martin"}`)
		case r.Method == "GET" && r.URL.Path == "/repos/acme/web/issues":
			if r.URL.Query().Get("state") != "open" {
				t.Errorf("state = %q", r.URL.Query().Get("state"))
			}
			fmt.Fprint(w, `[{"number":7,"title":"Fix login","state":"open","user":{"login":"ana"},"html_url":"u7"},
				{"number":8,"title":"A PR","pull_request":{},"user":{"login":"bo"}}]`)
		case r.Method == "POST" && r.URL.Path == "/repos/acme/web/issues":
			mu.Lock()
			json.NewDecoder(r.Body).Decode(&created)
			mu.Unlock()
			fmt.Fprint(w, `{"number":9,"html_url":"https://github.com/acme/web/issues/9"}`)
		default:
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `{"message":"Not Found"}`)
		}
	}))
	defer api.Close()
	gh := GitHub{}
	acct := Account{Credentials: map[string]string{"token": "gh-token", "webhook_secret": "s3cret"}, Config: map[string]string{"base_url": api.URL}}

	if err := gh.Verify(context.Background(), acct); err != nil {
		t.Fatal(err)
	}
	bad := acct
	bad.Credentials = map[string]string{"token": "nope"}
	var ue *UserError
	if err := gh.Verify(context.Background(), bad); err == nil || !asUserErr(err, &ue) || !strings.Contains(ue.Message, "Bad credentials") {
		t.Fatalf("bad token should be a readable user error, got %v", err)
	}

	issues := call(t, gh, acct, "list_issues", `{"repo":"acme/web","state":"open"}`).([]map[string]any)
	if len(issues) != 1 || issues[0]["title"] != "Fix login" || issues[0]["author"] != "ana" {
		t.Fatalf("PRs should be filtered out of issues: %+v", issues)
	}
	out := call(t, gh, acct, "create_issue", `{"repo":"acme/web","title":"Broken","body":"details","labels":"bug,urgent"}`).(map[string]any)
	if out["url"] != "https://github.com/acme/web/issues/9" || created["title"] != "Broken" || len(created["labels"].([]any)) != 2 {
		t.Fatalf("create: %+v sent %+v", out, created)
	}
	if _, err := gh.Call(context.Background(), acct, "list_issues", json.RawMessage(`{"repo":"../../etc"}`)); err == nil {
		t.Fatal("malformed repo names must be rejected")
	}

	// Webhooks: valid signature → normalized signal; bad signature → rejected.
	body := []byte(`{"action":"opened","issue":{"number":12,"title":"Site down","html_url":"u12","body":"500s","labels":[{"name":"incident"}]},
		"repository":{"full_name":"acme/web"},"sender":{"login":"ana"}}`)
	req := httptest.NewRequest("POST", "/hooks/x", nil)
	req.Header.Set("X-GitHub-Event", "issues")
	req.Header.Set("X-Hub-Signature-256", "sha256="+hexHMAC("s3cret", body))
	sigs, err := gh.HandleWebhook(acct, req, body)
	if err != nil || len(sigs) != 1 || sigs[0].Type != "github.issues" {
		t.Fatalf("webhook: %v %+v", err, sigs)
	}
	f := sigs[0].Fields
	if f["repo"] != "acme/web" || f["action"] != "opened" || f["number"] != "12" || f["labels"] != "incident" || f["author"] != "ana" {
		t.Fatalf("fields = %+v", f)
	}
	req.Header.Set("X-Hub-Signature-256", "sha256="+hexHMAC("wrong", body))
	if _, err := gh.HandleWebhook(acct, req, body); err != ErrUnauthorized {
		t.Fatalf("expected ErrUnauthorized, got %v", err)
	}
}

func asUserErr(err error, target **UserError) bool {
	ue, ok := err.(*UserError)
	if ok {
		*target = ue
	}
	return ok
}

// ---------------------------------------------------------------- Linear

func TestLinear(t *testing.T) {
	var mu sync.Mutex
	var queries []string
	var vars []map[string]any
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "lin_api_key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		var req struct {
			Query     string         `json:"query"`
			Variables map[string]any `json:"variables"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		mu.Lock()
		queries = append(queries, req.Query)
		vars = append(vars, req.Variables)
		mu.Unlock()
		q := req.Query
		switch {
		case strings.Contains(q, "viewer"):
			fmt.Fprint(w, `{"data":{"viewer":{"id":"u1"}}}`)
		case strings.Contains(q, "issue(id: $id)") && !strings.Contains(q, "comments"):
			fmt.Fprint(w, `{"data":{"issue":{"id":"iid","identifier":"ENG-1","title":"Ship it","url":"u","state":{"name":"Todo"},"team":{"id":"tid","key":"ENG"}}}}`)
		case strings.Contains(q, "workflowStates"):
			if req.Variables["team"] != "tid" || req.Variables["name"] != "done" {
				t.Errorf("state lookup vars = %+v", req.Variables)
			}
			fmt.Fprint(w, `{"data":{"workflowStates":{"nodes":[{"id":"sid-done"}]}}}`)
		case strings.Contains(q, "issueUpdate"):
			input := req.Variables["input"].(map[string]any)
			if input["stateId"] != "sid-done" || req.Variables["id"] != "iid" {
				t.Errorf("update vars = %+v", req.Variables)
			}
			fmt.Fprint(w, `{"data":{"issueUpdate":{"issue":{"identifier":"ENG-1","title":"Ship it","state":{"name":"Done"},"team":{"key":"ENG"}}}}}`)
		case strings.Contains(q, "issues("):
			fmt.Fprint(w, `{"data":{"issues":{"nodes":[{"identifier":"ENG-1","title":"Ship it","state":{"name":"Todo"},"team":{"key":"ENG"},"assignee":{"name":"Ana"}}]}}}`)
		default:
			fmt.Fprint(w, `{"errors":[{"message":"unexpected query"}]}`)
		}
	}))
	defer api.Close()
	lin := Linear{}
	acct := Account{Credentials: map[string]string{"api_key": "lin_api_key", "webhook_secret": "whs"}, Config: map[string]string{"base_url": api.URL}}

	if err := lin.Verify(context.Background(), acct); err != nil {
		t.Fatal(err)
	}
	found := call(t, lin, acct, "search_issues", `{"query":"ship","team":"ENG"}`).([]map[string]any)
	if len(found) != 1 || found[0]["id"] != "ENG-1" || found[0]["assignee"] != "Ana" {
		t.Fatalf("search = %+v", found)
	}
	mu.Lock()
	filter := vars[len(vars)-1]["filter"].(map[string]any)
	mu.Unlock()
	if filter["title"].(map[string]any)["containsIgnoreCase"] != "ship" || filter["team"] == nil {
		t.Fatalf("search filter = %+v", filter)
	}
	updated := call(t, lin, acct, "update_issue", `{"id":"ENG-1","state":"done"}`).(map[string]any)
	if updated["state"] != "Done" {
		t.Fatalf("update = %+v", updated)
	}

	body := []byte(`{"action":"update","type":"Issue","url":"https://linear.app/x/ENG-1",
		"data":{"identifier":"ENG-1","title":"Ship it","state":{"name":"Done"},"team":{"key":"ENG"},"assignee":{"name":"Ana"}},
		"updatedFrom":{"stateId":"old","state":{"name":"In Progress"}}}`)
	req := httptest.NewRequest("POST", "/hooks/x", nil)
	req.Header.Set("Linear-Signature", hexHMAC("whs", body))
	sigs, err := lin.HandleWebhook(acct, req, body)
	if err != nil || len(sigs) != 1 {
		t.Fatalf("webhook: %v %+v", err, sigs)
	}
	f := sigs[0].Fields
	if sigs[0].Type != "linear.issue" || f["state"] != "Done" || f["state_changed"] != "true" || f["previous_state"] != "In Progress" {
		t.Fatalf("signal = %+v", sigs[0])
	}
	req.Header.Set("Linear-Signature", "deadbeef")
	if _, err := lin.HandleWebhook(acct, req, body); err != ErrUnauthorized {
		t.Fatalf("expected ErrUnauthorized, got %v", err)
	}
}

// ---------------------------------------------------------------- Render

func TestRender(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer rnd" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch {
		case r.URL.Path == "/owners":
			fmt.Fprint(w, `[{"owner":{"id":"own"}}]`)
		case r.URL.Path == "/services":
			fmt.Fprint(w, `[{"service":{"id":"srv-1","name":"web","type":"web_service","serviceDetails":{"url":"https://web.onrender.com"}}}]`)
		case r.Method == "POST" && r.URL.Path == "/services/srv-1/deploys":
			fmt.Fprint(w, `{"id":"dep-1","status":"created"}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer api.Close()
	rnd := Render{}
	key := []byte("render-webhook-key")
	acct := Account{Credentials: map[string]string{"api_key": "rnd", "webhook_secret": "whsec_" + base64.StdEncoding.EncodeToString(key)},
		Config: map[string]string{"base_url": api.URL}}
	if err := rnd.Verify(context.Background(), acct); err != nil {
		t.Fatal(err)
	}
	svcs := call(t, rnd, acct, "list_services", `{}`).([]map[string]string)
	if len(svcs) != 1 || svcs[0]["url"] != "https://web.onrender.com" {
		t.Fatalf("services = %+v", svcs)
	}
	if dep := call(t, rnd, acct, "trigger_deploy", `{"service_id":"srv-1"}`).(map[string]string); dep["deploy_id"] != "dep-1" {
		t.Fatalf("deploy = %+v", dep)
	}
	if _, err := rnd.Call(context.Background(), acct, "trigger_deploy", json.RawMessage(`{"service_id":"../owners"}`)); err == nil {
		t.Fatal("bad service ids must be rejected")
	}

	body := []byte(`{"type":"deploy_ended","data":{"id":"dep-1","serviceId":"srv-1","serviceName":"web","status":"failed"}}`)
	sign := func(ts int64) *http.Request {
		id, tsStr := "msg_1", strconv.FormatInt(ts, 10)
		m := hmac.New(sha256.New, key)
		m.Write([]byte(id + "." + tsStr + "."))
		m.Write(body)
		req := httptest.NewRequest("POST", "/hooks/x", nil)
		req.Header.Set("webhook-id", id)
		req.Header.Set("webhook-timestamp", tsStr)
		req.Header.Set("webhook-signature", "v1,bogus v1,"+base64.StdEncoding.EncodeToString(m.Sum(nil)))
		return req
	}
	sigs, err := rnd.HandleWebhook(acct, sign(time.Now().Unix()), body)
	if err != nil || sigs[0].Fields["status"] != "failed" || sigs[0].Fields["event"] != "deploy_ended" || sigs[0].Fields["service"] != "web" {
		t.Fatalf("webhook: %v %+v", err, sigs)
	}
	if _, err := rnd.HandleWebhook(acct, sign(time.Now().Add(-time.Hour).Unix()), body); err != ErrUnauthorized {
		t.Fatalf("old timestamps must be rejected (replay), got %v", err)
	}
}

// ---------------------------------------------------------------- Slack

func TestSlack(t *testing.T) {
	var mu sync.Mutex
	var posted map[string]any
	acks := make(chan string, 8)
	var wsURL string
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		switch r.URL.Path {
		case "/auth.test":
			fmt.Fprint(w, `{"ok":true,"user_id":"UBOT","bot_id":"B1"}`)
		case "/apps.connections.open":
			if auth != "Bearer xapp-1" {
				fmt.Fprint(w, `{"ok":false,"error":"invalid_auth"}`)
				return
			}
			fmt.Fprintf(w, `{"ok":true,"url":%q}`, wsURL)
		case "/conversations.list":
			fmt.Fprint(w, `{"ok":true,"channels":[{"id":"C111","name":"alerts","is_member":true}]}`)
		case "/users.info":
			id := r.URL.Query().Get("user")
			names := map[string]string{"U1": "Steve J", "UBOT": "openbot"}
			fmt.Fprintf(w, `{"ok":true,"user":{"id":%q,"name":"x","profile":{"display_name":%q}}}`, id, names[id])
		case "/conversations.history":
			fmt.Fprint(w, `{"ok":true,"messages":[{"user":"U1","text":"<@UBOT> see <#C111|alerts> and <https://x.dev|the doc> &amp; <!here>","ts":"3.0"},{"bot_id":"B9","username":"Cursor","text":"done","ts":"2.0"}]}`)
		case "/chat.postMessage":
			mu.Lock()
			json.NewDecoder(r.Body).Decode(&posted)
			mu.Unlock()
			fmt.Fprint(w, `{"ok":true,"ts":"1700.1"}`)
		case "/socket":
			ws, err := websocket.Accept(w, r, nil)
			if err != nil {
				return
			}
			defer ws.CloseNow()
			ctx := r.Context()
			send := func(v string) { ws.Write(ctx, websocket.MessageText, []byte(v)) }
			send(`{"type":"hello"}`)
			send(`{"envelope_id":"e1","type":"events_api","payload":{"event":{"type":"app_mention","channel":"C111","user":"U1","text":"<@UBOT> prod is down","ts":"1.0"}}}`)
			send(`{"envelope_id":"e2","type":"events_api","payload":{"event":{"type":"message","channel":"C111","bot_id":"B1","text":"bot echo"}}}`)
			send(`{"envelope_id":"e3","type":"events_api","payload":{"event":{"type":"message","channel":"D9","channel_type":"im","user":"U1","text":"hi bot","ts":"2.0"}}}`)
			send(`{"envelope_id":"e4","type":"events_api","payload":{"event":{"type":"message","subtype":"bot_message","channel":"C222","channel_type":"channel","bot_id":"B2","bot_profile":{"name":"Render"},"text":"Server unhealthy for api-production","ts":"3.0"}}}`)
			send(`{"envelope_id":"e5","type":"events_api","payload":{"event":{"type":"message","subtype":"channel_join","channel":"C222","user":"U1","text":"joined"}}}`)
			for range 5 {
				_, data, err := ws.Read(ctx)
				if err != nil {
					return
				}
				var ack struct {
					EnvelopeID string `json:"envelope_id"`
				}
				json.Unmarshal(data, &ack)
				acks <- ack.EnvelopeID
			}
			send(`{"type":"disconnect"}`)
		default:
			fmt.Fprint(w, `{"ok":false,"error":"unknown_method"}`)
		}
	}))
	defer api.Close()
	wsURL = "ws" + strings.TrimPrefix(api.URL, "http") + "/socket"
	sl := Slack{}
	acct := Account{Credentials: map[string]string{"bot_token": "xoxb-1", "app_token": "xapp-1"}, Config: map[string]string{"base_url": api.URL}}

	if err := sl.Verify(context.Background(), acct); err != nil {
		t.Fatal(err)
	}
	res := call(t, sl, acct, "post_message", `{"channel":"#alerts","text":"on it"}`).(map[string]string)
	if res["channel"] != "C111" || posted["channel"] != "C111" || posted["text"] != "on it" {
		t.Fatalf("#name should resolve to the channel id: %+v sent %+v", res, posted)
	}

	// Messages read back with names and readable text instead of Slack ids and markup.
	// …with when they were sent, in the user's time zone and as an age.
	now = func() time.Time { return time.Unix(3, 0).Add(6 * time.Minute) }
	defer func() { now = time.Now }()
	loc := time.FixedZone("-03", -3*3600)
	res2, err := sl.Call(WithLocation(context.Background(), loc), acct, "read_channel", json.RawMessage(`{"channel":"#alerts"}`))
	if err != nil {
		t.Fatal(err)
	}
	read := res2.([]map[string]string)
	if len(read) != 2 || read[0]["from"] != "Steve J" || read[0]["text"] != "@openbot see #alerts and the doc (https://x.dev) & @here" ||
		read[1]["from"] != "Cursor" || read[0]["time"] != "1969-12-31 21:00 -03" || read[0]["ago"] != "6 min ago" {
		t.Fatalf("read_channel = %+v", read)
	}

	var got []Signal
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := sl.Listen(ctx, acct, func(s Signal) { got = append(got, s) }); err != nil {
		t.Fatalf("listen: %v", err)
	}
	// Our own messages and joins are ignored; other bots' messages (alerts) come through, named.
	if len(got) != 3 || got[0].Type != "slack.app_mention" || got[0].Fields["text"] != "@openbot prod is down" ||
		got[0].Fields["user_name"] != "Steve J" ||
		got[1].Type != "slack.message" || got[1].Fields["channel_type"] != "im" ||
		got[2].Fields["user_name"] != "Render" || got[2].Fields["text"] != "Server unhealthy for api-production" {
		t.Fatalf("signals = %+v", got)
	}
	for _, want := range []string{"e1", "e2", "e3", "e4", "e5"} {
		if id := <-acks; id != want {
			t.Fatalf("ack %q, want %q", id, want)
		}
	}
}

// ---------------------------------------------------------------- MCP

func TestMCP(t *testing.T) {
	for _, sse := range []bool{false, true} {
		t.Run(map[bool]string{false: "json", true: "sse"}[sse], func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer mcp-token" {
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
				var msg struct {
					ID     *int            `json:"id"`
					Method string          `json:"method"`
					Params json.RawMessage `json:"params"`
				}
				json.NewDecoder(r.Body).Decode(&msg)
				if msg.ID == nil { // notification
					w.WriteHeader(http.StatusAccepted)
					return
				}
				if msg.Method != "initialize" && r.Header.Get("Mcp-Session-Id") != "sess-1" {
					t.Errorf("%s without the session id", msg.Method)
				}
				var result string
				switch msg.Method {
				case "initialize":
					w.Header().Set("Mcp-Session-Id", "sess-1")
					result = `{"protocolVersion":"2025-06-18","capabilities":{"tools":{}},"serverInfo":{"name":"fake"}}`
				case "tools/list":
					result = `{"tools":[{"name":"search_docs","description":"Search","inputSchema":{"type":"object","properties":{"q":{"type":"string"}}},"annotations":{"readOnlyHint":true}},
						{"name":"delete_doc","description":"Delete","inputSchema":{"type":"object"}}]}`
				case "tools/call":
					var p struct {
						Name      string            `json:"name"`
						Arguments map[string]string `json:"arguments"`
					}
					json.Unmarshal(msg.Params, &p)
					result = fmt.Sprintf(`{"content":[{"type":"text","text":"results for %s"}]}`, p.Arguments["q"])
				}
				resp := fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"result":%s}`, *msg.ID, result)
				if sse {
					// A progress notification first, then the response split over several data lines.
					w.Header().Set("Content-Type", "text/event-stream")
					fmt.Fprint(w, "event: message\ndata: {\"jsonrpc\":\"2.0\",\"method\":\"notifications/progress\"}\n\n")
					fmt.Fprint(w, "event: message\n")
					for _, line := range strings.Split(resp, "\n") {
						fmt.Fprintf(w, "data: %s\n", line)
					}
					fmt.Fprint(w, "\n")
					return
				}
				w.Header().Set("Content-Type", "application/json")
				io.WriteString(w, resp)
			}))
			defer srv.Close()
			m := MCP{}
			acct := Account{ID: "mcp-" + fmt.Sprint(sse), Credentials: map[string]string{"authorization": "Bearer mcp-token"}, Config: map[string]string{"url": srv.URL}}
			if err := m.Verify(context.Background(), acct); err != nil {
				t.Fatal(err)
			}
			tools, err := m.Tools(context.Background(), acct)
			if err != nil || len(tools) != 2 || tools[0].External || !tools[1].External {
				t.Fatalf("tools = %+v %v (read-only tools run directly; others need approval)", tools, err)
			}
			out := call(t, m, acct, "search_docs", `{"q":"deploys"}`).(map[string]string)
			if out["content"] != "results for deploys" {
				t.Fatalf("call = %+v", out)
			}

			// A recent successful check must not let wrong or missing credentials through.
			for _, auth := range []string{"Bearer wrong", ""} {
				bad := Account{ID: acct.ID, Credentials: map[string]string{"authorization": auth}, Config: acct.Config}
				if err := m.Verify(context.Background(), bad); err == nil {
					t.Fatalf("verify with %q should fail", auth)
				}
				if _, err := m.Tools(context.Background(), bad); err == nil {
					t.Fatalf("tools with %q should not come from the cache", auth)
				}
			}
		})
	}
}

// ---------------------------------------------------------------- Manager

func TestManagerAccountsToolsAndSignals(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	st, err := store.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	box, _ := secrets.Open(dir)
	m := NewManager(st, box)
	var got []map[string]string
	m.OnSignal = func(_ context.Context, sig store.Signal, fields map[string]string) { got = append(got, fields) }

	agent, _, _ := st.CreateAgentWithDM(ctx, store.Agent{Name: "a", Instructions: "x", Model: "m", Language: "auto", TrustMode: "ask"})
	hook, err := m.Save(ctx, "", "webhook", "Uptime pings", map[string]string{}, map[string]string{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(hook.Credentials, "token") {
		t.Fatal("credentials must be stored encrypted")
	}
	token, _ := m.Secret(ctx, hook.ID, "token")
	if len(token) < 32 {
		t.Fatalf("a webhook token should be generated, got %q", token)
	}

	// Tools are namespaced by account and only visible with a grant.
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{"login":"x"}`) }))
	defer api.Close()
	gh, err := m.Save(ctx, "", "github", "GitHub — work", map[string]string{"token": "t"}, map[string]string{"base_url": api.URL})
	if err != nil {
		t.Fatal(err)
	}
	if tools, _ := m.ToolsFor(ctx, agent.ID); len(tools) != 0 {
		t.Fatalf("no grant, no tools: %+v", tools)
	}
	st.SetGrants(ctx, gh.ID, []string{agent.ID})
	tools, _ := m.ToolsFor(ctx, agent.ID)
	if len(tools) == 0 || tools[0].Name != "github_work__list_issues" {
		t.Fatalf("tools = %+v", tools)
	}
	if _, err := m.Call(ctx, agent.ID, "github_work__nope", nil); err == nil {
		t.Fatal("unknown tools must fail")
	}

	// Accounts with the same name get numbered prefixes.
	for range 2 {
		dup, _ := m.Save(ctx, "", "github", "GitHub — work", map[string]string{"token": "t"}, map[string]string{"base_url": api.URL})
		st.SetGrants(ctx, dup.ID, []string{agent.ID})
	}
	prefixes := map[string]bool{}
	all, _ := m.ToolsFor(ctx, agent.ID)
	for _, tool := range all {
		prefixes[strings.Split(tool.Name, "__")[0]] = true
	}
	if len(prefixes) != 3 || !prefixes["github_work"] || !prefixes["github_work2"] || !prefixes["github_work3"] {
		t.Fatalf("prefixes = %v", prefixes)
	}

	// Updating with an empty secret keeps the saved one.
	if _, err := m.Save(ctx, gh.ID, "github", "GitHub — work", map[string]string{"token": ""}, map[string]string{"base_url": api.URL}); err != nil {
		t.Fatal(err)
	}
	if tok, _ := m.Secret(ctx, gh.ID, "token"); tok != "t" {
		t.Fatalf("token = %q", tok)
	}

	// Webhook endpoint: wrong token rejected, right token emits a signal.
	mux := http.NewServeMux()
	mux.HandleFunc("POST /hooks/{accountID}", m.ServeWebhook)
	srv := httptest.NewServer(mux)
	defer srv.Close()
	post := func(q string) int {
		resp, err := http.Post(srv.URL+"/hooks/"+hook.ID+q, "application/json", strings.NewReader(`{"status":"down","site":"openbot.dev"}`))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	if code := post("?token=wrong"); code != http.StatusUnauthorized {
		t.Fatalf("wrong token: %d", code)
	}
	if code := post("?token=" + token); code != http.StatusOK || len(got) != 1 || got[0]["status"] != "down" || got[0]["site"] != "openbot.dev" {
		t.Fatalf("signal: %d %+v", code, got)
	}
	if !Match(got[0], map[string]string{"status": "DOWN"}) || Match(got[0], map[string]string{"status": "up"}) {
		t.Fatal("matching is a case-insensitive contains")
	}
}

// With a user token, messages always go out as the user; without one, as the bot.
func TestSlackPostsAsUser(t *testing.T) {
	var mu sync.Mutex
	var postedWith, listedWith string
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch r.URL.Path {
		case "/auth.test":
			fmt.Fprint(w, `{"ok":true,"user_id":"U1"}`)
		case "/conversations.list":
			listedWith = r.Header.Get("Authorization")
			fmt.Fprint(w, `{"ok":true,"channels":[{"id":"C9","name":"bot-ception"}]}`)
		case "/chat.postMessage":
			postedWith = r.Header.Get("Authorization")
			fmt.Fprint(w, `{"ok":true,"ts":"1.0"}`)
		}
	}))
	defer api.Close()
	sl := Slack{}
	acct := Account{Credentials: map[string]string{"bot_token": "xoxb-1", "user_token": "xoxp-1"}, Config: map[string]string{"base_url": api.URL}}
	if err := sl.Verify(context.Background(), acct); err != nil {
		t.Fatal(err)
	}
	tools, _ := sl.Tools(context.Background(), acct)
	if strings.Contains(string(tools[0].Parameters), `"as"`) || tools[0].Title != "Slack message as you" {
		t.Fatalf("no choice of sender: %s %q", tools[0].Parameters, tools[0].Title)
	}
	call(t, sl, acct, "post_message", `{"channel":"#bot-ception","text":"hi"}`)
	if postedWith != "Bearer xoxp-1" || listedWith != "Bearer xoxp-1" {
		t.Fatalf("posted with %q, looked up with %q", postedWith, listedWith)
	}

	noUser := Account{Credentials: map[string]string{"bot_token": "xoxb-1"}, Config: acct.Config}
	call(t, sl, noUser, "post_message", `{"channel":"#bot-ception","text":"hi"}`)
	if postedWith != "Bearer xoxb-1" {
		t.Fatalf("without a user token: posted with %q", postedWith)
	}
	bad := Account{Credentials: map[string]string{"bot_token": "xoxb-1", "user_token": "xoxb-2"}, Config: acct.Config}
	if err := sl.Verify(context.Background(), bad); err == nil || !strings.Contains(err.Error(), "xoxp-") {
		t.Fatalf("a bot token in the user field should be caught: %v", err)
	}
}
