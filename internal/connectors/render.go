package connectors

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Render: services and deploys through the REST API, and deploy/service events through
// webhooks (Standard Webhooks signatures).
type Render struct{}

func (Render) Name() string        { return "render" }
func (Render) DisplayName() string { return "Render" }
func (Render) Description() string {
	return "Check services and deploys, trigger deploys; get notified when deploys finish or services fail."
}
func (Render) CredentialFields() []Field {
	return []Field{
		{Key: "api_key", Label: "API key", Secret: true, Help: "Render → Account settings → API keys."},
		{Key: "webhook_secret", Label: "Webhook signing secret", Secret: true, Optional: true,
			Help: "Create a webhook in Render → Integrations → Webhooks pointing at the URL below, then paste its secret (whsec_…)."},
	}
}
func (Render) ConfigFields() []Field { return nil }
func (Render) SignalTypes() []SignalType {
	return []SignalType{{Type: "render.event", Description: "A deploy started or ended, a service failed, a cron job ran…",
		Fields: []string{"event", "service_id", "service", "status", "deploy_id"}}}
}

func (Render) Tools(context.Context, Account) ([]Tool, error) {
	return []Tool{
		{Name: "list_services", Description: "List services with their type and URL.", Parameters: params(map[string]string{})},
		{Name: "list_deploys", Description: "Recent deploys of a service.",
			Parameters: params(map[string]string{"service_id": "service id (srv-…)"})},
		{Name: "trigger_deploy", Description: "Deploy the latest commit of a service.", External: true,
			Parameters: params(map[string]string{"service_id": "service id (srv-…)"})},
		{Name: "restart_service", Description: "Restart a service.", External: true,
			Parameters: params(map[string]string{"service_id": "service id (srv-…)"})},
	}, nil
}

func (Render) api(ctx context.Context, acct Account, method, path string, body, out any) error {
	h := http.Header{}
	h.Set("Authorization", "Bearer "+acct.Credentials["api_key"])
	return apiRequest(ctx, method, baseURL(acct, "https://api.render.com/v1")+path, h, body, out)
}

func (r Render) Call(ctx context.Context, acct Account, tool string, args json.RawMessage) (any, error) {
	var a struct {
		ServiceID string `json:"service_id"`
	}
	if err := decodeArgs(args, &a); err != nil {
		return nil, err
	}
	if tool != "list_services" && (!strings.HasPrefix(a.ServiceID, "srv-") && !strings.HasPrefix(a.ServiceID, "crn-") || strings.ContainsAny(a.ServiceID, "/?#")) {
		return nil, userErr("service_id must be a Render service id like srv-…")
	}
	switch tool {
	case "list_services":
		var items []struct {
			Service map[string]any `json:"service"`
		}
		if err := r.api(ctx, acct, "GET", "/services?limit=50", nil, &items); err != nil {
			return nil, err
		}
		var out []map[string]string
		for _, it := range items {
			out = append(out, map[string]string{"id": str(it.Service, "id"), "name": str(it.Service, "name"),
				"type": str(it.Service, "type"), "url": str(it.Service, "serviceDetails.url"), "suspended": str(it.Service, "suspended")})
		}
		return out, nil
	case "list_deploys":
		var items []struct {
			Deploy map[string]any `json:"deploy"`
		}
		if err := r.api(ctx, acct, "GET", "/services/"+a.ServiceID+"/deploys?limit=10", nil, &items); err != nil {
			return nil, err
		}
		var out []map[string]string
		for _, it := range items {
			out = append(out, map[string]string{"id": str(it.Deploy, "id"), "status": str(it.Deploy, "status"),
				"commit": strings.SplitN(str(it.Deploy, "commit.message"), "\n", 2)[0], "created_at": str(it.Deploy, "createdAt"),
				"finished_at": str(it.Deploy, "finishedAt")})
		}
		return out, nil
	case "trigger_deploy":
		var d map[string]any
		if err := r.api(ctx, acct, "POST", "/services/"+a.ServiceID+"/deploys", map[string]string{"clearCache": "do_not_clear"}, &d); err != nil {
			return nil, err
		}
		return map[string]string{"deploy_id": str(d, "id"), "status": str(d, "status")}, nil
	case "restart_service":
		if err := r.api(ctx, acct, "POST", "/services/"+a.ServiceID+"/restart", nil, nil); err != nil {
			return nil, err
		}
		return map[string]string{"restarted": a.ServiceID}, nil
	}
	return nil, userErr("unknown tool %q", tool)
}

func (r Render) Verify(ctx context.Context, acct Account) error {
	var owners []any
	return r.api(ctx, acct, "GET", "/owners?limit=1", nil, &owners)
}

// HandleWebhook verifies a Standard Webhooks signature: base64(HMAC-SHA256(secret, id.ts.body)),
// sent as "v1,<sig>" (possibly several, space-separated).
func (Render) HandleWebhook(acct Account, r *http.Request, body []byte) ([]Signal, error) {
	secret := strings.TrimPrefix(acct.Credentials["webhook_secret"], "whsec_")
	key, err := base64.StdEncoding.DecodeString(secret)
	if err != nil || len(key) == 0 {
		return nil, ErrUnauthorized
	}
	id, ts := r.Header.Get("webhook-id"), r.Header.Get("webhook-timestamp")
	sec, err := strconv.ParseInt(ts, 10, 64)
	if err != nil || time.Since(time.Unix(sec, 0)).Abs() > 5*time.Minute {
		return nil, ErrUnauthorized
	}
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(id + "." + ts + "."))
	mac.Write(body)
	want := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	ok := false
	for _, s := range strings.Fields(r.Header.Get("webhook-signature")) {
		if v, sig, _ := strings.Cut(s, ","); v == "v1" && hmac.Equal([]byte(sig), []byte(want)) {
			ok = true
		}
	}
	if !ok {
		return nil, ErrUnauthorized
	}
	var p map[string]any
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, userErr("invalid JSON")
	}
	return []Signal{{Type: "render.event", Fields: map[string]string{
		"event": str(p, "type"), "service_id": str(p, "data.serviceId"), "service": str(p, "data.serviceName"),
		"status": str(p, "data.status"), "deploy_id": str(p, "data.id"),
	}}}, nil
}
