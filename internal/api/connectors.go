package api

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/etchebarne/barn/internal/api/gen"
	"github.com/etchebarne/barn/internal/connectors"
	"github.com/etchebarne/barn/internal/store"
)

func connectorErr(w http.ResponseWriter, err error) {
	var ue *connectors.UserError
	switch {
	case errors.As(err, &ue):
		writeError(w, http.StatusBadRequest, sentence(ue.Message))
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "Connection not found")
	default:
		writeError(w, http.StatusBadGateway, sentence(err.Error()))
	}
}

// sentence capitalizes a connector's message, which is shown as-is under the form.
func sentence(msg string) string {
	r, n := utf8.DecodeRuneInString(msg)
	return string(unicode.ToUpper(r)) + msg[n:]
}

func fieldsView(fields []connectors.Field) []gen.ConnectorField {
	out := make([]gen.ConnectorField, 0, len(fields))
	for _, f := range fields {
		gf := gen.ConnectorField{Key: f.Key, Label: f.Label, Secret: f.Secret, Optional: f.Optional}
		if f.Help != "" {
			help := f.Help
			gf.Help = &help
		}
		out = append(out, gf)
	}
	return out
}

func (s *Server) ListConnectorTypes(w http.ResponseWriter, r *http.Request) {
	out := make([]gen.ConnectorType, 0, len(connectors.Registry))
	for _, t := range connectors.Registry {
		sigs := make([]gen.ConnectorSignalType, 0)
		for _, st := range t.SignalTypes() {
			sigs = append(sigs, gen.ConnectorSignalType{Type: st.Type, Description: st.Description, Fields: st.Fields})
		}
		_, webhooks := t.(connectors.WebhookReceiver)
		out = append(out, gen.ConnectorType{Type: t.Name(), Name: t.DisplayName(), Description: t.Description(),
			CredentialFields: fieldsView(t.CredentialFields()), ConfigFields: fieldsView(t.ConfigFields()), Signals: sigs, Webhooks: webhooks})
	}
	writeJSON(w, http.StatusOK, out)
}

// publicOrigin is where external services reach barnd: BARN_PUBLIC_URL, or this request's host.
func (s *Server) publicOrigin(r *http.Request) string {
	if s.opts.PublicURL != "" {
		return strings.TrimRight(s.opts.PublicURL, "/")
	}
	scheme := "http"
	if r.TLS != nil || s.opts.SecureCookies {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

func (s *Server) connectorView(ctx context.Context, r *http.Request, a store.ConnectorAccount) (gen.Connector, error) {
	agentIDs, err := s.store.Grants(ctx, a.ID)
	if err != nil {
		return gen.Connector{}, err
	}
	if agentIDs == nil {
		agentIDs = []string{}
	}
	out := gen.Connector{Id: a.ID, Type: a.Type, Name: a.Name, AgentIds: agentIDs, Config: map[string]string{},
		CredentialsSet: []string{}, CreatedAt: store.Time(a.CreatedAt)}
	acct, t, err := s.connectorsAccount(ctx, a.ID)
	if err != nil {
		return out, err
	}
	out.Config = acct.Config
	for k, v := range acct.Credentials {
		if v != "" {
			out.CredentialsSet = append(out.CredentialsSet, k)
		}
	}
	slices.Sort(out.CredentialsSet)
	if _, ok := t.(connectors.WebhookReceiver); ok {
		u := s.publicOrigin(r) + "/hooks/" + a.ID
		if a.Type == "webhook" {
			u += "?token=" + url.QueryEscape(acct.Credentials["token"])
		}
		out.WebhookUrl = &u
	}
	return out, nil
}

func (s *Server) connectorsAccount(ctx context.Context, id string) (connectors.Account, connectors.Type, error) {
	return s.Connectors.Account(ctx, id)
}

func (s *Server) ListConnectors(w http.ResponseWriter, r *http.Request) {
	accounts, err := s.store.Accounts(r.Context())
	if err != nil {
		internalError(w, err)
		return
	}
	out := make([]gen.Connector, 0, len(accounts))
	for _, a := range accounts {
		v, err := s.connectorView(r.Context(), r, a)
		if err != nil {
			internalError(w, err)
			return
		}
		out = append(out, v)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) validAgents(ctx context.Context, ids []string) error {
	agents, err := s.store.ListAgents(ctx)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if !slices.ContainsFunc(agents, func(a store.Agent) bool { return a.ID == id }) {
			return &connectors.UserError{Message: "unknown agent " + id}
		}
	}
	return nil
}

func (s *Server) CreateConnector(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req gen.CreateConnectorRequest
	if !decode(w, r, &req) {
		return
	}
	var agentIDs []string
	if req.AgentIds != nil {
		agentIDs = *req.AgentIds
	}
	if err := s.validAgents(ctx, agentIDs); err != nil {
		connectorErr(w, err)
		return
	}
	config := map[string]string{}
	if req.Config != nil {
		config = *req.Config
	}
	a, err := s.Connectors.Save(ctx, "", req.Type, req.Name, req.Credentials, config)
	if err != nil {
		connectorErr(w, err)
		return
	}
	if err := s.store.SetGrants(ctx, a.ID, agentIDs); err != nil {
		internalError(w, err)
		return
	}
	v, err := s.connectorView(ctx, r, a)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, v)
}

func (s *Server) UpdateConnector(w http.ResponseWriter, r *http.Request, id string) {
	ctx := r.Context()
	var req gen.UpdateConnectorRequest
	if !decode(w, r, &req) {
		return
	}
	existing, err := s.store.GetAccount(ctx, id)
	if err != nil {
		connectorErr(w, err)
		return
	}
	if req.Name != nil || req.Credentials != nil || req.Config != nil {
		acct, _, err := s.connectorsAccount(ctx, id)
		if err != nil {
			internalError(w, err)
			return
		}
		name, config, creds := existing.Name, acct.Config, map[string]string{}
		if req.Name != nil {
			name = *req.Name
		}
		if req.Config != nil {
			config = *req.Config
		}
		if req.Credentials != nil {
			creds = *req.Credentials
		}
		if _, err := s.Connectors.Save(ctx, id, existing.Type, name, creds, config); err != nil {
			connectorErr(w, err)
			return
		}
	}
	if req.AgentIds != nil {
		if err := s.validAgents(ctx, *req.AgentIds); err != nil {
			connectorErr(w, err)
			return
		}
		if err := s.store.SetGrants(ctx, id, *req.AgentIds); err != nil {
			internalError(w, err)
			return
		}
	}
	a, err := s.store.GetAccount(ctx, id)
	if err != nil {
		internalError(w, err)
		return
	}
	v, err := s.connectorView(ctx, r, a)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) DeleteConnector(w http.ResponseWriter, r *http.Request, id string) {
	if err := s.Connectors.Delete(r.Context(), id); err != nil {
		connectorErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
