package api

import (
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/etchebarne/barn/internal/api/gen"
	"github.com/etchebarne/barn/internal/model"
	"github.com/etchebarne/barn/internal/runtime"
	"github.com/etchebarne/barn/internal/store"
	"github.com/etchebarne/barn/internal/view"
)

func (s *Server) ListChats(w http.ResponseWriter, r *http.Request) {
	chats, err := s.store.ListChats(r.Context())
	if err != nil {
		internalError(w, err)
		return
	}
	out := make([]gen.Chat, 0, len(chats))
	for _, c := range chats {
		out = append(out, view.Chat(c))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) ListMessages(w http.ResponseWriter, r *http.Request, chatID gen.ChatId, params gen.ListMessagesParams) {
	if !s.chatExists(w, r, chatID) {
		return
	}
	limit := 50
	if params.Limit != nil {
		limit = min(max(*params.Limit, 1), 200)
	}
	before := ""
	if params.Before != nil {
		before = *params.Before
	}
	msgs, hasMore, err := s.store.ListMessages(r.Context(), chatID, before, limit)
	if err != nil {
		internalError(w, err)
		return
	}
	page := gen.MessagePage{Messages: make([]gen.Message, 0, len(msgs)), HasMore: hasMore}
	for _, m := range msgs {
		page.Messages = append(page.Messages, view.Message(m))
	}
	writeJSON(w, http.StatusOK, page)
}

func (s *Server) SendMessage(w http.ResponseWriter, r *http.Request, chatID gen.ChatId) {
	ctx := r.Context()
	var req gen.SendMessageRequest
	if !decode(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Body) == "" || utf8.RuneCountInString(req.Body) > 32000 {
		writeError(w, http.StatusBadRequest, "message must be 1–32000 characters")
		return
	}
	if req.ClientId != nil && len(*req.ClientId) > 64 {
		writeError(w, http.StatusBadRequest, "clientId must be at most 64 characters")
		return
	}
	if !s.chatExists(w, r, chatID) {
		return
	}
	msg, err := s.store.InsertMessage(ctx, chatID, "user", nil, req.Body)
	if err != nil {
		internalError(w, err)
		return
	}
	// Sending implies having read everything up to this message.
	if err := s.store.MarkRead(ctx, chatID, msg.ID); err != nil {
		internalError(w, err)
		return
	}
	out := view.Message(msg)
	out.ClientId = req.ClientId
	s.bus.Publish(view.MessageCreated(out))
	if err := s.runtime.DeliverUserMessage(ctx, msg); err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (s *Server) MarkChatRead(w http.ResponseWriter, r *http.Request, chatID gen.ChatId) {
	var req gen.MarkReadRequest
	if !decode(w, r, &req) {
		return
	}
	if req.LastMessageId == "" {
		writeError(w, http.StatusBadRequest, "lastMessageId is required")
		return
	}
	if !s.chatExists(w, r, chatID) {
		return
	}
	if err := s.store.MarkRead(r.Context(), chatID, req.LastMessageId); err != nil {
		internalError(w, err)
		return
	}
	s.bus.Publish(gen.WsChatRead{Type: "chat.read", ChatId: chatID, LastMessageId: req.LastMessageId})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) chatExists(w http.ResponseWriter, r *http.Request, chatID string) bool {
	_, err := s.store.GetChat(r.Context(), chatID)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "chat not found")
		return false
	}
	if err != nil {
		internalError(w, err)
		return false
	}
	return true
}

func (s *Server) ListAgents(w http.ResponseWriter, r *http.Request) {
	agents, err := s.store.ListAgents(r.Context())
	if err != nil {
		internalError(w, err)
		return
	}
	out := make([]gen.Agent, 0, len(agents))
	for _, a := range agents {
		out = append(out, view.Agent(a, s.runtime.Activity(a.ID)))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) ListModels(w http.ResponseWriter, r *http.Request) {
	models, err := s.llm.Models(r.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, "couldn't load models: "+err.Error())
		return
	}
	out := make([]gen.Model, 0, len(models))
	for _, id := range models {
		out = append(out, gen.Model{Id: id})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) UpdateAgent(w http.ResponseWriter, r *http.Request, agentID string) {
	ctx := r.Context()
	var req gen.UpdateAgentRequest
	if !decode(w, r, &req) {
		return
	}
	if _, err := s.store.GetAgent(ctx, agentID); errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	} else if err != nil {
		internalError(w, err)
		return
	}
	if req.Model != nil {
		if !s.checkModel(w, r, *req.Model) {
			return
		}
		err := s.store.UpdateAgentModel(ctx, agentID, *req.Model)
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "agent not found")
			return
		}
		if err != nil {
			internalError(w, err)
			return
		}
	}
	agent, err := s.store.GetAgent(ctx, agentID)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	}
	if err != nil {
		internalError(w, err)
		return
	}
	out := view.Agent(agent, s.runtime.Activity(agent.ID))
	s.bus.Publish(gen.WsAgentUpdated{Type: "agent.updated", Agent: out})

	// A new model is the usual fix for a failed turn, so pick up where the agent left off.
	if req.Model != nil {
		if failed, err := s.runtime.LastTurnFailed(ctx, agentID); err != nil {
			slog.Warn("check last turn", "agent", agentID, "err", err)
		} else if failed {
			if err := s.runtime.Retry(ctx, agentID); err != nil && !errors.Is(err, runtime.ErrBusy) {
				slog.Warn("retry after model change", "agent", agentID, "err", err)
			}
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) RetryAgent(w http.ResponseWriter, r *http.Request, agentID string) {
	ctx := r.Context()
	if _, err := s.store.GetAgent(ctx, agentID); errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	} else if err != nil {
		internalError(w, err)
		return
	}
	err := s.runtime.Retry(ctx, agentID)
	if errors.Is(err, runtime.ErrBusy) {
		writeError(w, http.StatusConflict, "the agent is already working")
		return
	}
	if err != nil {
		internalError(w, err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

// checkModel verifies a model exists and is usable with the saved key (a one-token request),
// writing a user-facing error and returning false if not.
func (s *Server) checkModel(w http.ResponseWriter, r *http.Request, id string) bool {
	ctx := r.Context()
	models, err := s.llm.Models(ctx)
	if err != nil {
		writeError(w, http.StatusBadGateway, "couldn't load models: "+err.Error())
		return false
	}
	if !slices.Contains(models, id) {
		writeError(w, http.StatusBadRequest, "unknown model "+id)
		return false
	}
	switch err := s.llm.ProbeModel(ctx, id); {
	case err == nil:
		return true
	case errors.Is(err, model.ErrTrainsOnData):
		writeError(w, http.StatusBadRequest, id+" is blocked by your OpenCode workspace's Privacy settings "+
			"(its provider trains on request data). Pick another model, or allow these models in OpenCode.")
	case errors.Is(err, model.ErrInvalidKey), errors.Is(err, model.ErrNoKey):
		writeError(w, http.StatusBadRequest, "OpenCode Go rejected your API key. Replace it in Settings.")
	default:
		writeError(w, http.StatusBadGateway, "couldn't reach "+id+": "+err.Error())
	}
	return false
}
