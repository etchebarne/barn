package connectors

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/etchebarne/barn/internal/secrets"
	"github.com/etchebarne/barn/internal/store"
)

// Registry is every connector type barn supports.
var Registry = []Type{Webhook{}, GitHub{}, Linear{}, Slack{}, Render{}, MCP{}}

// TypeByName finds a connector type in the Registry.
func TypeByName(name string) (Type, bool) { return typeByName(name) }

func typeByName(name string) (Type, bool) {
	i := slices.IndexFunc(Registry, func(t Type) bool { return t.Name() == name })
	if i < 0 {
		return nil, false
	}
	return Registry[i], true
}

// AgentTool is a connector tool as an agent sees it: namespaced by account.
type AgentTool struct {
	Name      string // "<account slug>__<tool>"
	AccountID string
	Tool      Tool
}

// Manager owns connector accounts: credentials, tools, webhooks, and listeners.
type Manager struct {
	store *store.Store
	box   *secrets.Box
	// OnSignal is called for every incoming signal (set before Start).
	OnSignal func(ctx context.Context, sig store.Signal, fields map[string]string)

	mu        sync.Mutex
	ctx       context.Context
	listeners map[string]context.CancelFunc // account id -> stop
}

func NewManager(st *store.Store, box *secrets.Box) *Manager {
	return &Manager{store: st, box: box, listeners: map[string]context.CancelFunc{}}
}

// Start begins listening on every account whose type listens (e.g. Slack Socket Mode).
func (m *Manager) Start(ctx context.Context) error {
	m.mu.Lock()
	m.ctx = ctx
	m.mu.Unlock()
	accounts, err := m.store.Accounts(ctx)
	if err != nil {
		return err
	}
	for _, a := range accounts {
		m.restartListener(a.ID)
	}
	return nil
}

// Account loads and decrypts an account.
func (m *Manager) Account(ctx context.Context, id string) (Account, Type, error) {
	return m.account(ctx, id)
}

func (m *Manager) account(ctx context.Context, id string) (Account, Type, error) {
	sa, err := m.store.GetAccount(ctx, id)
	if err != nil {
		return Account{}, nil, err
	}
	return m.decode(sa)
}

func (m *Manager) decode(sa store.ConnectorAccount) (Account, Type, error) {
	t, ok := typeByName(sa.Type)
	if !ok {
		return Account{}, nil, fmt.Errorf("unknown connector type %q", sa.Type)
	}
	acct := Account{ID: sa.ID, Type: sa.Type, Name: sa.Name, Credentials: map[string]string{}, Config: map[string]string{}}
	if sa.Credentials != "" {
		plain, err := m.box.Open(sa.Credentials)
		if err != nil {
			return acct, t, fmt.Errorf("decrypt credentials: %w", err)
		}
		if err := json.Unmarshal([]byte(plain), &acct.Credentials); err != nil {
			return acct, t, err
		}
	}
	_ = json.Unmarshal(sa.Config, &acct.Config)
	return acct, t, nil
}

// Save creates (id "") or updates an account after verifying its credentials. Empty secret
// values on update keep the stored ones.
func (m *Manager) Save(ctx context.Context, id, typeName, name string, creds, config map[string]string) (store.ConnectorAccount, error) {
	t, ok := typeByName(typeName)
	if !ok {
		return store.ConnectorAccount{}, userErr("unknown connector type %q", typeName)
	}
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > 64 {
		return store.ConnectorAccount{}, userErr("name must be 1–64 characters")
	}
	if id != "" {
		old, _, err := m.account(ctx, id)
		if err != nil {
			return store.ConnectorAccount{}, err
		}
		for k, v := range old.Credentials {
			if creds[k] == "" {
				creds[k] = v
			}
		}
	}
	if typeName == "webhook" && creds["token"] == "" {
		creds["token"] = randomToken()
	}
	for _, f := range t.CredentialFields() {
		if !f.Optional && strings.TrimSpace(creds[f.Key]) == "" {
			return store.ConnectorAccount{}, userErr("%s is required", f.Label)
		}
	}
	for _, f := range t.ConfigFields() {
		if !f.Optional && strings.TrimSpace(config[f.Key]) == "" {
			return store.ConnectorAccount{}, userErr("%s is required", f.Label)
		}
	}
	vctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if err := t.Verify(vctx, Account{ID: id, Type: typeName, Name: name, Credentials: creds, Config: config}); err != nil {
		return store.ConnectorAccount{}, err
	}
	credJSON, _ := json.Marshal(creds)
	sealed, err := m.box.Seal(string(credJSON))
	if err != nil {
		return store.ConnectorAccount{}, err
	}
	cfgJSON, _ := json.Marshal(config)
	sa := store.ConnectorAccount{ID: id, Type: typeName, Name: name, Credentials: sealed, Config: cfgJSON}
	if id == "" {
		sa, err = m.store.CreateAccount(ctx, sa)
	} else {
		err = m.store.UpdateAccount(ctx, sa)
	}
	if err != nil {
		return sa, err
	}
	m.restartListener(sa.ID)
	return sa, nil
}

// Delete removes an account and stops its listener.
func (m *Manager) Delete(ctx context.Context, id string) error {
	m.stopListener(id)
	return m.store.DeleteAccount(ctx, id)
}

// Secret returns a credential (for showing the webhook token in Settings).
func (m *Manager) Secret(ctx context.Context, accountID, key string) (string, error) {
	acct, _, err := m.account(ctx, accountID)
	if err != nil {
		return "", err
	}
	return acct.Credentials[key], nil
}

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

// Slug turns an account name into a tool-name prefix ("Slack — work" → "slack_work").
func Slug(name string) string {
	s := strings.Trim(slugRe.ReplaceAllString(strings.ToLower(name), "_"), "_")
	if s == "" {
		s = "account"
	}
	if len(s) > 24 {
		s = s[:24]
	}
	return s
}

// AgentAccounts returns the accounts an agent may use, with their types.
func (m *Manager) AgentAccounts(ctx context.Context, agentID string) ([]Account, []Type, error) {
	sas, err := m.store.AgentAccounts(ctx, agentID)
	if err != nil {
		return nil, nil, err
	}
	var accts []Account
	var types []Type
	for _, sa := range sas {
		a, t, err := m.decode(sa)
		if err != nil {
			slog.Warn("connector account unusable", "account", sa.ID, "err", err)
			continue
		}
		accts = append(accts, a)
		types = append(types, t)
	}
	return accts, types, nil
}

// ToolsFor lists the connector tools an agent may call.
func (m *Manager) ToolsFor(ctx context.Context, agentID string) ([]AgentTool, error) {
	accts, types, err := m.AgentAccounts(ctx, agentID)
	if err != nil {
		return nil, err
	}
	var out []AgentTool
	seen := map[string]int{}
	for i, a := range accts {
		tools, err := types[i].Tools(ctx, a)
		if err != nil {
			slog.Warn("connector tools", "account", a.ID, "err", err)
			continue
		}
		base := Slug(a.Name)
		prefix := base
		if n := seen[base]; n > 0 {
			prefix = fmt.Sprintf("%s%d", base, n+1)
		}
		seen[base]++
		for _, t := range tools {
			out = append(out, AgentTool{Name: prefix + "__" + t.Name, AccountID: a.ID, Tool: t})
		}
	}
	return out, nil
}

// Call runs a connector tool for an agent (which must have been granted the account).
func (m *Manager) Call(ctx context.Context, agentID, name string, args json.RawMessage) (any, error) {
	tools, err := m.ToolsFor(ctx, agentID)
	if err != nil {
		return nil, err
	}
	i := slices.IndexFunc(tools, func(t AgentTool) bool { return t.Name == name })
	if i < 0 {
		return nil, userErr("no connector tool %q (or no access to it)", name)
	}
	acct, t, err := m.account(ctx, tools[i].AccountID)
	if err != nil {
		return nil, err
	}
	cctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	return t.Call(cctx, acct, tools[i].Tool.Name, args)
}

// ServeWebhook handles POST /hooks/{accountID}.
func (m *Manager) ServeWebhook(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("accountID")
	acct, t, err := m.account(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	recv, ok := t.(WebhookReceiver)
	if !ok {
		http.NotFound(w, r)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 2<<20))
	if err != nil {
		http.Error(w, "body too large", http.StatusRequestEntityTooLarge)
		return
	}
	signals, err := recv.HandleWebhook(acct, r, body)
	if errors.Is(err, ErrUnauthorized) {
		http.Error(w, "invalid signature", http.StatusUnauthorized)
		return
	}
	var ue *UserError
	if errors.As(err, &ue) {
		http.Error(w, ue.Message, http.StatusBadRequest)
		return
	}
	if err != nil {
		slog.Warn("webhook", "account", id, "err", err)
		http.Error(w, "error", http.StatusInternalServerError)
		return
	}
	for _, sig := range signals {
		m.emit(r.Context(), acct.ID, sig)
	}
	w.WriteHeader(http.StatusOK)
}

func (m *Manager) emit(ctx context.Context, accountID string, sig Signal) {
	payload, _ := json.Marshal(sig)
	stored, err := m.store.InsertSignal(ctx, store.Signal{AccountID: accountID, Type: sig.Type, Payload: payload})
	if err != nil {
		slog.Error("store signal", "err", err)
		return
	}
	slog.Info("signal", "account", accountID, "type", sig.Type)
	if m.OnSignal != nil {
		m.OnSignal(ctx, stored, sig.Fields)
	}
}

func (m *Manager) restartListener(accountID string) {
	m.stopListener(accountID)
	m.mu.Lock()
	parent := m.ctx
	m.mu.Unlock()
	if parent == nil {
		return
	}
	acct, t, err := m.account(parent, accountID)
	if err != nil {
		return
	}
	l, ok := t.(Listener)
	if !ok {
		return
	}
	ctx, cancel := context.WithCancel(parent)
	m.mu.Lock()
	m.listeners[accountID] = cancel
	m.mu.Unlock()
	go func() {
		for ctx.Err() == nil {
			err := l.Listen(ctx, acct, func(sig Signal) { m.emit(context.Background(), acct.ID, sig) })
			if ctx.Err() != nil {
				return
			}
			slog.Warn("connector listener stopped; retrying", "account", acct.ID, "err", err)
			select {
			case <-ctx.Done():
			case <-time.After(15 * time.Second):
			}
		}
	}()
}

func (m *Manager) stopListener(accountID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if cancel, ok := m.listeners[accountID]; ok {
		cancel()
		delete(m.listeners, accountID)
	}
}

// Match reports whether signal fields satisfy a task's filter: every filter value must appear
// (case-insensitively) in the field with the same name.
func Match(fields, filter map[string]string) bool {
	for k, want := range filter {
		if !strings.Contains(strings.ToLower(fields[k]), strings.ToLower(want)) {
			return false
		}
	}
	return true
}
