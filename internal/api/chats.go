package api

import (
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/etchebarne/barn/internal/api/gen"
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
