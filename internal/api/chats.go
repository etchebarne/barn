package api

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/etchebarne/barn/internal/api/gen"
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
	err := s.runtime.CheckModel(r.Context(), id)
	if err == nil {
		return true
	}
	var me *runtime.ModelError
	if errors.As(err, &me) {
		writeError(w, http.StatusBadRequest, me.Message)
	} else {
		writeError(w, http.StatusBadGateway, err.Error())
	}
	return false
}

func (s *Server) AnswerPrompt(w http.ResponseWriter, r *http.Request, messageID string) {
	ctx := r.Context()
	var req gen.PromptAnswer
	if !decode(w, r, &req) {
		return
	}
	answer := store.PromptAnswer{}
	if req.Selected != nil {
		answer.Selected = *req.Selected
	}
	if req.Text != nil {
		answer.Text = strings.TrimSpace(*req.Text)
	}
	msg, err := s.store.UpdatePrompt(ctx, messageID, func(p store.Prompt) (store.Prompt, error) {
		if err := validateAnswer(p, answer); err != nil {
			return p, err
		}
		p.Status, p.Answer = "answered", &answer
		return p, nil
	})
	var invalid *invalidAnswerError
	switch {
	case errors.As(err, &invalid):
		writeError(w, http.StatusBadRequest, invalid.msg)
		return
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "prompt not found")
		return
	case errors.Is(err, store.ErrPromptClosed):
		writeError(w, http.StatusConflict, "this question was already answered")
		return
	case err != nil:
		internalError(w, err)
		return
	}
	// Answering implies having read the chat up to the question.
	if err := s.store.MarkRead(ctx, msg.ChatID, msg.ID); err != nil {
		internalError(w, err)
		return
	}
	out := view.Message(msg)
	s.bus.Publish(view.MessageUpdated(out))
	if err := s.runtime.DeliverAnswer(ctx, msg); err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) DismissPrompt(w http.ResponseWriter, r *http.Request, messageID string) {
	msg, err := s.store.UpdatePrompt(r.Context(), messageID, func(p store.Prompt) (store.Prompt, error) {
		p.Status = "dismissed"
		return p, nil
	})
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "prompt not found")
		return
	case errors.Is(err, store.ErrPromptClosed):
		writeError(w, http.StatusConflict, "this question was already answered")
		return
	case err != nil:
		internalError(w, err)
		return
	}
	out := view.Message(msg)
	s.bus.Publish(view.MessageUpdated(out))
	writeJSON(w, http.StatusOK, out)
}

type invalidAnswerError struct{ msg string }

func (e *invalidAnswerError) Error() string { return e.msg }

func validateAnswer(p store.Prompt, a store.PromptAnswer) error {
	bad := func(msg string) error { return &invalidAnswerError{msg} }
	if utf8.RuneCountInString(a.Text) > 4000 {
		return bad("answer is too long")
	}
	seen := map[int]bool{}
	for _, i := range a.Selected {
		if i < 0 || i >= len(p.Options) {
			return bad("selected option doesn't exist")
		}
		if seen[i] {
			return bad("an option was selected twice")
		}
		seen[i] = true
	}
	hasText := a.Text != ""
	switch p.Kind {
	case "text":
		if !hasText || len(a.Selected) > 0 {
			return bad("type an answer")
		}
	case "single":
		if len(a.Selected)+btoi(hasText) != 1 {
			return bad("choose one option")
		}
	case "multi":
		if len(a.Selected) == 0 && !hasText {
			return bad("choose at least one option")
		}
	}
	if hasText && p.Kind != "text" && !p.AllowOther {
		return bad("this question doesn't take a typed answer")
	}
	return nil
}

func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}
