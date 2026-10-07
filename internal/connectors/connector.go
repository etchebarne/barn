// Package connectors links agents to external services (GitHub, Linear, Slack, …). A connector
// type provides tools agents can call and signals (incoming events) that can wake agents up.
// Each configured account has its own credentials; agents use accounts they've been granted.
package connectors

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
)

// Field describes one credential or config value the user enters in Settings.
type Field struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Help     string `json:"help,omitempty"`
	Secret   bool   `json:"secret"`
	Optional bool   `json:"optional"`
}

// Tool is an action agents can take with an account.
type Tool struct {
	Name        string
	Description string
	Parameters  json.RawMessage
	// External tools act on the user's behalf (post, create, deploy) and need approval unless
	// the agent is trusted.
	External bool
	// How the approval card shows a call. All optional: Title defaults to the humanized name,
	// Verb (the approve button) to "Approve", and arguments are labelled by their names.
	Title  string
	Verb   string
	Body   string            // the argument that is the main content, e.g. a message's text
	Labels map[string]string // argument labels, e.g. {"thread_ts": "Thread"}
}

// SignalType is a kind of event a connector emits, with the fields signal tasks can match on.
type SignalType struct {
	Type        string   `json:"type"`
	Description string   `json:"description"`
	Fields      []string `json:"fields"`
}

// Signal is one incoming event, normalized to flat string fields (for matching and for
// agents to read), plus the original payload.
type Signal struct {
	Type   string            `json:"type"`
	Fields map[string]string `json:"fields"`
	Raw    json.RawMessage   `json:"raw,omitempty"`
}

// Account is a configured account with decrypted credentials.
type Account struct {
	ID          string
	Type        string
	Name        string
	Credentials map[string]string
	Config      map[string]string
}

// Type is a kind of connector (one per service).
type Type interface {
	Name() string        // machine name, e.g. "github"
	DisplayName() string // e.g. "GitHub"
	Description() string
	CredentialFields() []Field
	ConfigFields() []Field
	SignalTypes() []SignalType
	// Tools lists the actions for an account (most types return a fixed list).
	Tools(ctx context.Context, acct Account) ([]Tool, error)
	Call(ctx context.Context, acct Account, tool string, args json.RawMessage) (any, error)
	// Verify checks the credentials work (e.g. by fetching the authenticated user).
	Verify(ctx context.Context, acct Account) error
}

// WebhookReceiver is implemented by types that receive signals over HTTP. HandleWebhook
// verifies the request (signatures) and turns it into signals.
type WebhookReceiver interface {
	HandleWebhook(acct Account, r *http.Request, body []byte) ([]Signal, error)
}

// Listener is implemented by types that receive signals over a persistent connection (e.g.
// Slack Socket Mode). Listen runs until ctx ends, reconnecting as needed.
type Listener interface {
	Listen(ctx context.Context, acct Account, emit func(Signal)) error
}

// ErrUnauthorized means a webhook request failed verification.
var ErrUnauthorized = errors.New("webhook verification failed")

// UserError is a problem the agent or user can act on (bad arguments, not found, …).
type UserError struct{ Message string }

func (e *UserError) Error() string { return e.Message }

func userErr(format string, args ...any) error {
	return &UserError{Message: sprintf(format, args...)}
}
