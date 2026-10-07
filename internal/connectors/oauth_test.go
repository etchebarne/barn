package connectors

import (
	"context"
	"strings"
	"testing"

	"github.com/etchebarne/openbot/internal/connectors/oauthtest"
	"github.com/etchebarne/openbot/internal/secrets"
	"github.com/etchebarne/openbot/internal/store"
)

func TestOAuthSignIn(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	st, err := store.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	box, _ := secrets.Open(dir)
	m := NewManager(st, box)
	agent, _, _ := st.CreateAgentWithDM(ctx, store.Agent{Name: "a", Instructions: "x", Model: "m", Language: "auto", TrustMode: "ask"})

	// Without a sign-in, connecting with a key explains that this server uses sign-in.
	srv := oauthtest.New(t, true)
	_, err = m.Save(ctx, "", "mcp", "Linear", map[string]string{}, map[string]string{"url": srv.URL + "/mcp"})
	if ue, ok := err.(*UserError); !ok || ue.Code != "sign_in_required" {
		t.Fatalf("expected sign_in_required, got %v", err)
	}

	// The server refuses openbot's plain-http address, so openbot falls back to the loopback
	// redirect and the user pastes the address back.
	authURL, pasteBack, err := m.BeginOAuth(ctx, OAuthRequest{MCPURL: srv.URL + "/mcp", RedirectURI: "http://100.64.0.1:8080/oauth/callback",
		Name: "Linear", AgentIDs: []string{agent.ID}})
	if err != nil || !pasteBack {
		t.Fatalf("begin: paste back %v, err %v", pasteBack, err)
	}
	landed := srv.Approve(t, authURL)
	if !strings.HasPrefix(landed, LoopbackRedirect+"?") {
		t.Fatalf("landed on %s", landed)
	}
	code, state, err := ParseCallback(landed)
	if err != nil {
		t.Fatal(err)
	}
	acct, flow, err := m.CompleteOAuth(ctx, state, code)
	if err != nil || flow.Name != "Linear" {
		t.Fatalf("complete: %v", err)
	}
	if _, _, err := m.CompleteOAuth(ctx, state, code); err == nil {
		t.Fatal("a sign-in must be single use")
	}
	if strings.Contains(acct.Credentials, "access-") {
		t.Fatal("tokens must be stored encrypted")
	}

	// The agent gets the server's tools, and calls carry the access token.
	tools, _ := m.ToolsFor(ctx, agent.ID)
	if len(tools) != 1 || tools[0].Name != "linear__whoami" {
		t.Fatalf("tools = %+v", tools)
	}
	if out, err := m.Call(ctx, agent.ID, "linear__whoami", nil); err != nil || out.(map[string]string)["content"] != "you are martin" {
		t.Fatalf("call = %v %v", out, err)
	}

	// A token rejected early is refreshed once and the call retried.
	srv.ExpireAccess()
	if _, err := m.Call(ctx, agent.ID, "linear__whoami", nil); err != nil {
		t.Fatalf("after expiry: %v", err)
	}
	if srv.Refreshes != 1 {
		t.Fatalf("refreshes = %d", srv.Refreshes)
	}

	// When the refresh token is revoked, the connection asks to be reconnected, without
	// retrying on every call.
	srv.Revoke()
	_, err = m.Call(ctx, agent.ID, "linear__whoami", nil)
	if ue, ok := err.(*UserError); !ok || ue.Code != "reconnect" || !strings.Contains(ue.Message, "Reconnect") {
		t.Fatalf("expected reconnect, got %v", err)
	}
	a, _, _ := m.Account(ctx, acct.ID)
	if _, expired := SignedIn(a); !expired {
		t.Fatal("the connection should be marked as needing a new sign-in")
	}

	// Reconnecting signs in again and clears the mark, keeping the connection and its access.
	srv.RejectHTTP = false
	authURL, pasteBack, err = m.BeginOAuth(ctx, OAuthRequest{MCPURL: srv.URL + "/mcp", RedirectURI: "http://100.64.0.1:8080/oauth/callback", AccountID: acct.ID})
	if err != nil || pasteBack {
		t.Fatalf("reconnect begin: paste back %v, err %v", pasteBack, err)
	}
	code, state, _ = ParseCallback(srv.Approve(t, authURL))
	again, _, err := m.CompleteOAuth(ctx, state, code)
	if err != nil || again.ID != acct.ID || again.Name != "Linear" {
		t.Fatalf("reconnect: %+v %v", again, err)
	}
	a, _, _ = m.Account(ctx, acct.ID)
	if _, expired := SignedIn(a); expired {
		t.Fatal("reconnecting should clear the mark")
	}
	if _, err := m.Call(ctx, agent.ID, "linear__whoami", nil); err != nil {
		t.Fatalf("after reconnect: %v", err)
	}
}

func TestParseCallback(t *testing.T) {
	if _, _, err := ParseCallback("http://127.0.0.1:8789/oauth/callback?error=access_denied"); err == nil || !strings.Contains(err.Error(), "refused") {
		t.Fatalf("denied: %v", err)
	}
	if _, _, err := ParseCallback("http://127.0.0.1:8789/oauth/callback"); err == nil {
		t.Fatal("no code should fail")
	}
	code, state, err := ParseCallback(" http://127.0.0.1:8789/oauth/callback?code=c1&state=s1 ")
	if err != nil || code != "c1" || state != "s1" {
		t.Fatalf("got %q %q %v", code, state, err)
	}
	for host, want := range map[string]bool{"localhost:5173": true, "127.0.0.1": true, "[::1]:80": true, "100.114.128.31:8080": false, "openbot.ts.net": false} {
		if IsLoopback(host) != want {
			t.Errorf("IsLoopback(%q) != %v", host, want)
		}
	}
}

// A connection's tool names don't change when another one with the same name is unreachable.
func TestToolPrefixesStable(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	st, err := store.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	box, _ := secrets.Open(dir)
	m := NewManager(st, box)
	agent, _, _ := st.CreateAgentWithDM(ctx, store.Agent{Name: "a", Instructions: "x", Model: "m", Language: "auto", TrustMode: "ask"})
	signIn := func(srv *oauthtest.Server) string {
		authURL, _, err := m.BeginOAuth(ctx, OAuthRequest{MCPURL: srv.URL + "/mcp", RedirectURI: "http://localhost/oauth/callback", Name: "Docs", AgentIDs: []string{agent.ID}})
		if err != nil {
			t.Fatal(err)
		}
		code, state, _ := ParseCallback(srv.Approve(t, authURL))
		acct, _, err := m.CompleteOAuth(ctx, state, code)
		if err != nil {
			t.Fatal(err)
		}
		return acct.ID
	}
	first, second := oauthtest.New(t, false), oauthtest.New(t, false)
	signIn(first)
	secondID := signIn(second)
	first.Close()
	mcpCache.Lock()
	mcpCache.tools = map[string]mcpCacheEntry{}
	mcpCache.Unlock()
	tools, _ := m.ToolsFor(ctx, agent.ID)
	if len(tools) != 1 || tools[0].Name != "docs2__whoami" || tools[0].AccountID != secondID {
		t.Fatalf("tools = %+v", tools)
	}
}
