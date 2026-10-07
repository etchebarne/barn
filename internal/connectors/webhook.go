package connectors

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"net/http"
)

// Webhook receives JSON events from any service that can POST to a URL. The URL includes a
// secret token. Top-level fields of the JSON become signal fields.
type Webhook struct{}

func (Webhook) Name() string        { return "webhook" }
func (Webhook) DisplayName() string { return "Webhook" }
func (Webhook) Description() string {
	return "Receive events from any service that can send webhooks (JSON POST)."
}
func (Webhook) CredentialFields() []Field { return nil }
func (Webhook) ConfigFields() []Field     { return nil }
func (Webhook) SignalTypes() []SignalType {
	return []SignalType{{Type: "webhook.received", Description: "A JSON payload was posted to the webhook URL",
		Fields: []string{"(each top-level JSON field)", "body"}}}
}
func (Webhook) Tools(context.Context, Account) ([]Tool, error) { return nil, nil }
func (Webhook) Call(context.Context, Account, string, json.RawMessage) (any, error) {
	return nil, userErr("webhook accounts have no tools")
}
func (Webhook) Verify(context.Context, Account) error { return nil }

func (Webhook) HandleWebhook(acct Account, r *http.Request, body []byte) ([]Signal, error) {
	token := r.URL.Query().Get("token")
	if token == "" {
		token = r.Header.Get("X-Barn-Token")
	}
	want := acct.Credentials["token"]
	if want == "" || subtle.ConstantTimeCompare([]byte(token), []byte(want)) != 1 {
		return nil, ErrUnauthorized
	}
	fields := map[string]string{"body": truncateStr(string(body), 4000)}
	var obj map[string]any
	if json.Unmarshal(body, &obj) == nil {
		for k := range obj {
			fields[k] = truncateStr(str(obj, k), 1000)
		}
	}
	raw := json.RawMessage(nil)
	if json.Valid(body) {
		raw = body
	}
	return []Signal{{Type: "webhook.received", Fields: fields, Raw: raw}}, nil
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
