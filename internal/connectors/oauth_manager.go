package connectors

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/etchebarne/barn/internal/store"
)

// Sign-ins in progress live in memory: they're short (minutes) and single-use.
const flowTTL = 15 * time.Minute

type oauthState struct {
	mu        sync.Mutex
	flows     map[string]*OAuthFlow
	refreshMu map[string]*sync.Mutex // per account, so concurrent calls refresh once
}

var oauth = oauthState{flows: map[string]*OAuthFlow{}, refreshMu: map[string]*sync.Mutex{}}

// OAuthRequest starts a sign-in to an MCP server.
type OAuthRequest struct {
	MCPURL      string
	RedirectURI string // barn's callback as the browser reaches it
	Name        string
	AgentIDs    []string
	MessageID   string // a connect card this answers
	AccountID   string // a connection this reconnects
}

// BeginOAuth registers barn with the server's authorization server and returns the URL to
// send the user to. pasteBack means the server wouldn't accept barn's address, so the user
// will land on an error page whose address they paste back (see CompleteOAuth).
func (m *Manager) BeginOAuth(ctx context.Context, req OAuthRequest) (authURL string, pasteBack bool, err error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	flow, authURL, err := beginOAuth(ctx, req.MCPURL, req.RedirectURI)
	if err != nil {
		return "", false, err
	}
	flow.Name, flow.AgentIDs, flow.MessageID, flow.AccountID = strings.TrimSpace(req.Name), req.AgentIDs, req.MessageID, req.AccountID
	if flow.Name == "" {
		flow.Name = hostOf(req.MCPURL)
	}
	oauth.mu.Lock()
	for state, f := range oauth.flows {
		if time.Since(f.Created) > flowTTL {
			delete(oauth.flows, state)
		}
	}
	oauth.flows[flow.State] = flow
	oauth.mu.Unlock()
	return authURL, flow.PasteBack, nil
}

// CompleteOAuth finishes a sign-in: it exchanges the code, then creates the connection (with
// the requested agents' access) or updates the one being reconnected. It returns the flow so
// the caller can answer a connect card.
func (m *Manager) CompleteOAuth(ctx context.Context, state, code string) (store.ConnectorAccount, *OAuthFlow, error) {
	oauth.mu.Lock()
	flow, ok := oauth.flows[state]
	delete(oauth.flows, state) // single use
	oauth.mu.Unlock()
	if !ok || time.Since(flow.Created) > flowTTL {
		return store.ConnectorAccount{}, nil, userErr("this sign-in expired or was already used; start it again")
	}
	tokens, err := exchangeCode(ctx, flow, code)
	if errors.Is(err, errReconnect) {
		return store.ConnectorAccount{}, nil, userErr("%s refused the sign-in code; start it again", hostOf(flow.MCPURL))
	}
	if err != nil {
		return store.ConnectorAccount{}, nil, err
	}
	raw, _ := json.Marshal(tokens)
	creds := map[string]string{"oauth": string(raw)}
	config := map[string]string{"url": flow.MCPURL}
	name := flow.Name
	if flow.AccountID != "" {
		old, _, err := m.account(ctx, flow.AccountID)
		if err != nil {
			return store.ConnectorAccount{}, nil, err
		}
		name = old.Name
		// A reconnect replaces the sign-in and clears the "expired" mark, keeping the rest.
		for k, v := range old.Credentials {
			if k != "oauth" && k != "oauth_error" {
				creds[k] = v
			}
		}
	}
	acct, err := m.Save(ctx, flow.AccountID, "mcp", name, creds, config)
	if err != nil {
		return store.ConnectorAccount{}, nil, err
	}
	if flow.AccountID != "" {
		// Save keeps stored values for empty ones, so clear the expired mark directly.
		saved, _, err := m.account(ctx, acct.ID)
		if err != nil {
			return acct, flow, err
		}
		delete(saved.Credentials, "oauth_error")
		if err := m.storeCredentials(ctx, saved); err != nil {
			return acct, flow, err
		}
	}
	if flow.AccountID == "" {
		if err := m.store.SetGrants(ctx, acct.ID, flow.AgentIDs); err != nil {
			return acct, flow, err
		}
	}
	return acct, flow, nil
}

// CancelOAuth drops a pending sign-in (e.g. the user declined).
func CancelOAuth(state string) {
	oauth.mu.Lock()
	delete(oauth.flows, state)
	oauth.mu.Unlock()
}

// fresh makes sure a signed-in account's access token is usable, refreshing (and saving) it
// when it's about to expire, or always when force is set (after the server rejected it).
func (m *Manager) fresh(ctx context.Context, acct Account, force bool) (Account, error) {
	raw := acct.Credentials["oauth"]
	if raw == "" {
		return acct, nil
	}
	var t OAuthTokens
	if err := json.Unmarshal([]byte(raw), &t); err != nil {
		return acct, err
	}
	if acct.Credentials["oauth_error"] != "" {
		return acct, reconnectErr(acct.Name)
	}
	if !force && !t.expiring(time.Now()) {
		return acct, nil
	}

	oauth.mu.Lock()
	l, ok := oauth.refreshMu[acct.ID]
	if !ok {
		l = &sync.Mutex{}
		oauth.refreshMu[acct.ID] = l
	}
	oauth.mu.Unlock()
	l.Lock()
	defer l.Unlock()

	// Someone may have refreshed while we waited.
	latest, _, err := m.account(ctx, acct.ID)
	if err != nil {
		return acct, err
	}
	var current OAuthTokens
	_ = json.Unmarshal([]byte(latest.Credentials["oauth"]), &current)
	if current.AccessToken != t.AccessToken && !current.expiring(time.Now()) {
		return latest, nil
	}

	rctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	next, err := refreshTokens(rctx, current)
	if errors.Is(err, errReconnect) {
		latest.Credentials["oauth_error"] = "expired"
		if serr := m.storeCredentials(ctx, latest); serr != nil {
			slog.Warn("mark sign-in expired", "account", acct.ID, "err", serr)
		}
		return latest, reconnectErr(acct.Name)
	}
	if err != nil {
		return latest, err
	}
	b, _ := json.Marshal(next)
	latest.Credentials["oauth"] = string(b)
	if err := m.storeCredentials(ctx, latest); err != nil {
		return latest, err
	}
	return latest, nil
}

func reconnectErr(name string) error {
	return &UserError{Code: "reconnect",
		Message: fmt.Sprintf("%s needs to be signed in again: ask the user to open Settings → Connectors → %s and click Reconnect", name, name)}
}

// storeCredentials saves an account's (decrypted) credentials, encrypting them.
func (m *Manager) storeCredentials(ctx context.Context, acct Account) error {
	sa, err := m.store.GetAccount(ctx, acct.ID)
	if err != nil {
		return err
	}
	b, _ := json.Marshal(acct.Credentials)
	sealed, err := m.box.Seal(string(b))
	if err != nil {
		return err
	}
	sa.Credentials = sealed
	return m.store.UpdateAccount(ctx, sa)
}

// SignedIn reports whether an account connects through a sign-in, and whether it needs a new one.
func SignedIn(acct Account) (oauth, expired bool) {
	return acct.Credentials["oauth"] != "", acct.Credentials["oauth_error"] != ""
}
