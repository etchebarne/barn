package api

import (
	"context"
	"errors"
	"net/http"
	"net/url"

	"github.com/etchebarne/openbot/internal/api/gen"
	"github.com/etchebarne/openbot/internal/connectors"
	"github.com/etchebarne/openbot/internal/store"
	"github.com/etchebarne/openbot/internal/view"
)

func (s *Server) ListConnectorCatalog(w http.ResponseWriter, r *http.Request) {
	out := make([]gen.CatalogApp, 0, len(connectors.Catalog))
	for _, app := range connectors.Catalog {
		out = append(out, gen.CatalogApp{Id: app.ID, Name: app.Name, Description: app.Description, Url: app.URL})
	}
	writeJSON(w, http.StatusOK, out)
}

// browserOrigin is openbot's address as the user's browser reaches it (the sign-in callback goes
// there, unlike webhooks, which may use OPENBOT_PUBLIC_URL).
func (s *Server) browserOrigin(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil || s.opts.SecureCookies || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

func (s *Server) StartSignIn(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req gen.StartSignInRequest
	if !decode(w, r, &req) {
		return
	}
	in := connectors.OAuthRequest{RedirectURI: s.browserOrigin(r) + "/oauth/callback"}
	switch {
	case req.MessageId != nil:
		msg, err := s.store.GetMessage(ctx, *req.MessageId)
		if err != nil || msg.Prompt == nil || msg.Prompt.Kind != "connect" || msg.Prompt.Connection == nil ||
			msg.Prompt.Connection.Type != "mcp" {
			writeError(w, http.StatusNotFound, "connect card not found")
			return
		}
		if msg.Prompt.Status != "pending" {
			writeError(w, http.StatusConflict, "this card was already answered")
			return
		}
		c := msg.Prompt.Connection
		in.MCPURL, in.Name, in.AgentIDs, in.MessageID = c.Config["url"], c.Name, c.AgentIDs, msg.ID
	case req.ConnectorId != nil:
		acct, _, err := s.Connectors.Account(ctx, *req.ConnectorId)
		if err != nil || acct.Type != "mcp" {
			writeError(w, http.StatusNotFound, "connection not found")
			return
		}
		in.MCPURL, in.Name, in.AccountID = acct.Config["url"], acct.Name, acct.ID
	default:
		if req.Url == nil || *req.Url == "" {
			writeError(w, http.StatusBadRequest, "url is required")
			return
		}
		in.MCPURL = *req.Url
		if req.Name != nil {
			in.Name = *req.Name
		}
		if req.AgentIds != nil {
			in.AgentIDs = *req.AgentIds
		}
		if err := s.validAgents(ctx, in.AgentIDs); err != nil {
			connectorErr(w, err)
			return
		}
	}
	authURL, pasteBack, err := s.Connectors.BeginOAuth(ctx, in)
	if err != nil {
		connectorErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, gen.StartSignInResponse{AuthorizeUrl: authURL, PasteBack: pasteBack})
}

// finishSignIn completes a sign-in and answers its connect card, if any.
func (s *Server) finishSignIn(ctx context.Context, state, code string) (store.ConnectorAccount, *connectors.OAuthFlow, string, error) {
	acct, flow, err := s.Connectors.CompleteOAuth(ctx, state, code)
	if err != nil {
		return acct, flow, "", err
	}
	chatID := ""
	if flow.MessageID != "" {
		msg, err := s.runtime.ConnectedBySignIn(ctx, flow.MessageID, acct.ID, acct.Name)
		if err != nil {
			return acct, flow, "", err
		}
		chatID = msg.ChatID
		s.bus.Publish(view.MessageUpdated(view.Message(msg)))
	}
	return acct, flow, chatID, nil
}

func (s *Server) CompleteSignIn(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req gen.CompleteSignInRequest
	if !decode(w, r, &req) {
		return
	}
	code, state, err := connectors.ParseCallback(req.CallbackUrl)
	if err != nil {
		connectorErr(w, err)
		return
	}
	acct, _, chatID, err := s.finishSignIn(ctx, state, code)
	if errors.Is(err, store.ErrPromptClosed) {
		writeError(w, http.StatusConflict, "this card was already answered")
		return
	}
	if err != nil {
		connectorErr(w, err)
		return
	}
	v, err := s.connectorView(ctx, r, acct)
	if err != nil {
		internalError(w, err)
		return
	}
	out := gen.SignInResult{Connector: v}
	if chatID != "" {
		out.ChatId = &chatID
	}
	writeJSON(w, http.StatusOK, out)
}

// oauthCallback is where the service sends the browser back after Allow. It finishes the
// sign-in and redirects into the app, which shows the outcome.
func (s *Server) oauthCallback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	state := q.Get("state")
	back := func(chatID string, params url.Values) {
		dest := "/settings"
		if chatID != "" {
			dest = "/chats/" + chatID
		}
		http.Redirect(w, r, dest+"?"+params.Encode(), http.StatusSeeOther)
	}
	flow, ok := connectors.PeekOAuth(state)
	chatOf := func() string {
		if ok && flow.MessageID != "" {
			if msg, err := s.store.GetMessage(r.Context(), flow.MessageID); err == nil {
				return msg.ChatID
			}
		}
		return ""
	}
	if e := q.Get("error"); e != "" {
		connectors.CancelOAuth(state)
		msg := "Sign-in was cancelled"
		if e != "access_denied" {
			msg = "Sign-in failed: " + firstNonEmpty(q.Get("error_description"), e)
		}
		back(chatOf(), url.Values{"signin_error": {msg}})
		return
	}
	chatID := chatOf()
	acct, _, cardChat, err := s.finishSignIn(r.Context(), state, q.Get("code"))
	if err != nil {
		var ue *connectors.UserError
		msg := "Sign-in failed"
		if errors.As(err, &ue) {
			msg = sentence(ue.Message)
		} else if errors.Is(err, store.ErrPromptClosed) {
			msg = "This card was already answered"
		}
		back(chatID, url.Values{"signin_error": {msg}})
		return
	}
	if cardChat != "" {
		back(cardChat, url.Values{"connected": {acct.ID}})
		return
	}
	back("", url.Values{"connector": {acct.ID}, "connected": {"1"}})
}

func firstNonEmpty(s ...string) string {
	for _, v := range s {
		if v != "" {
			return v
		}
	}
	return ""
}
