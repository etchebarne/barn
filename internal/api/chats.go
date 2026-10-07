package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/etchebarne/barn/internal/api/gen"
	"github.com/etchebarne/barn/internal/runtime"
	"github.com/etchebarne/barn/internal/sandbox"
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
	msg, err := s.store.InsertMessage(ctx, chatID, "user", nil, req.Body, s.runtime.MentionsIn(ctx, chatID, req.Body)...)
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
	u := store.AgentUpdate{Name: req.Name, Instructions: req.Instructions, Model: req.Model, Language: req.Language, Notifications: req.Notifications}
	if req.TrustMode != nil {
		mode := string(*req.TrustMode)
		u.TrustMode = &mode
	}
	agent, err := s.runtime.UpdateAgent(ctx, agentID, u)
	var me *runtime.ModelError
	if errors.As(err, &me) {
		writeError(w, http.StatusBadRequest, me.Message)
		return
	}
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

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
	writeJSON(w, http.StatusOK, view.Agent(agent, s.runtime.Activity(agent.ID)))
}

func (s *Server) ArchiveAgent(w http.ResponseWriter, r *http.Request, agentID string) {
	err := s.runtime.ArchiveAgent(r.Context(), agentID)
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "agent not found")
	case errors.Is(err, store.ErrLastAdmin):
		writeError(w, http.StatusConflict, err.Error())
	case err != nil:
		internalError(w, err)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Server) ToggleReaction(w http.ResponseWriter, r *http.Request, messageID string) {
	ctx := r.Context()
	var req gen.ToggleReactionRequest
	if !decode(w, r, &req) {
		return
	}
	emoji := strings.TrimSpace(req.Emoji)
	if !runtime.IsEmoji(emoji) {
		writeError(w, http.StatusBadRequest, "emoji must be a single emoji")
		return
	}
	msg, err := s.store.GetMessage(ctx, messageID)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "message not found")
		return
	}
	if err != nil {
		internalError(w, err)
		return
	}
	added, err := s.store.ToggleReaction(ctx, msg.ID, "user", emoji)
	if err != nil {
		internalError(w, err)
		return
	}
	if msg, err = s.store.GetMessage(ctx, msg.ID); err != nil {
		internalError(w, err)
		return
	}
	out := view.Message(msg)
	s.bus.Publish(view.MessageUpdated(out))
	if added {
		if err := s.runtime.DeliverReaction(ctx, msg, emoji); err != nil {
			slog.Warn("deliver reaction", "err", err)
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
	deliver := s.runtime.DeliverAnswer
	if msg.Prompt.Kind == "approval" || msg.Prompt.Kind == "connect" {
		deliver = s.runtime.ResolveApproval
	}
	if err := deliver(ctx, msg); err != nil {
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
	case "approval":
		if len(a.Selected) != 1 || hasText {
			return bad("approve or decline")
		}
	case "connect":
		if len(a.Selected) != 1 || a.Selected[0] != 1 || hasText {
			return bad("connect with POST /messages/{messageId}/connect; answer with option 1 to decline")
		}
	case "multi":
		if len(a.Selected) == 0 && !hasText {
			return bad("choose at least one option")
		}
	}
	if hasText && p.Kind != "text" && (!p.AllowOther || p.Kind == "approval" || p.Kind == "connect") {
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

func (s *Server) ListMemories(w http.ResponseWriter, r *http.Request, agentID string) {
	ctx := r.Context()
	if _, err := s.store.GetAgent(ctx, agentID); errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	} else if err != nil {
		internalError(w, err)
		return
	}
	memories, err := s.store.Memories(ctx, agentID)
	if err != nil {
		internalError(w, err)
		return
	}
	out := make([]gen.Memory, 0, len(memories))
	for _, m := range memories {
		out = append(out, gen.Memory{Id: m.ID, Text: m.Text, CreatedAt: store.Time(m.CreatedAt)})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) DeleteMemory(w http.ResponseWriter, r *http.Request, agentID, memoryID string) {
	err := s.store.DeleteMemory(r.Context(), agentID, memoryID)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "memory not found")
		return
	}
	if err != nil {
		internalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) GetSandbox(w http.ResponseWriter, r *http.Request, agentID string) {
	s.writeSandbox(w, r, agentID)
}

func (s *Server) RestartSandbox(w http.ResponseWriter, r *http.Request, agentID string) {
	err := s.runtime.RestartSandbox(r.Context(), agentID)
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "agent not found")
		return
	case errors.Is(err, sandbox.ErrUnavailable):
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	case err != nil:
		writeError(w, http.StatusBadGateway, "couldn't restart the sandbox: "+err.Error())
		return
	}
	s.writeSandbox(w, r, agentID)
}

func (s *Server) writeSandbox(w http.ResponseWriter, r *http.Request, agentID string) {
	status, shared, err := s.runtime.SandboxState(r.Context(), agentID)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	}
	if err != nil {
		internalError(w, err)
		return
	}
	if shared == nil {
		shared = []string{}
	}
	writeJSON(w, http.StatusOK, gen.Sandbox{Status: gen.SandboxStatus(status), SharedWith: shared})
}

// taskView describes a task; accounts names connector accounts for signal tasks.
func taskView(t store.Task, accounts map[string]string) gen.Task {
	out := gen.Task{Id: t.ID, AgentId: t.AgentID, Name: t.Name, Purpose: t.Purpose, Kind: gen.TaskKind(t.Kind), Enabled: t.Enabled}
	switch t.Kind {
	case "cron":
		c := t.Cron
		out.Cron = &c
	case "once":
		at := store.Time(t.At)
		out.At = &at
	case "signal":
		// "pull_request.opened" reads as "pull request opened".
		desc := strings.NewReplacer(".", " ", "_", " ").Replace(t.SignalType)
		if name := accounts[t.SignalAccountID]; name != "" {
			desc += " in " + name
		}
		keys := make([]string, 0, len(t.SignalMatch))
		for k := range t.SignalMatch {
			keys = append(keys, k)
		}
		slices.Sort(keys)
		for i, k := range keys {
			sep := " where "
			if i > 0 {
				sep = " and "
			}
			desc += sep + k + " contains \"" + t.SignalMatch[k] + "\""
		}
		out.Signal = &desc
	}
	if t.NextFireAt != nil && t.Enabled {
		next := store.Time(*t.NextFireAt)
		out.NextFireAt = &next
	}
	if t.LastFiredAt != nil {
		last := store.Time(*t.LastFiredAt)
		out.LastFiredAt = &last
	}
	return out
}

func (s *Server) ListTasks(w http.ResponseWriter, r *http.Request, agentID string) {
	ctx := r.Context()
	if _, err := s.store.GetAgent(ctx, agentID); errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	} else if err != nil {
		internalError(w, err)
		return
	}
	tasks, err := s.store.Tasks(ctx, agentID)
	if err != nil {
		internalError(w, err)
		return
	}
	names, err := s.accountNames(ctx)
	if err != nil {
		internalError(w, err)
		return
	}
	out := make([]gen.Task, 0, len(tasks))
	for _, t := range tasks {
		out = append(out, taskView(t, names))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) UpdateTask(w http.ResponseWriter, r *http.Request, taskID string) {
	var req gen.UpdateTaskRequest
	if !decode(w, r, &req) {
		return
	}
	t, err := s.runtime.SetTaskEnabled(r.Context(), taskID, req.Enabled)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}
	if err != nil {
		internalError(w, err)
		return
	}
	names, err := s.accountNames(r.Context())
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, taskView(t, names))
}

func (s *Server) DeleteTask(w http.ResponseWriter, r *http.Request, taskID string) {
	err := s.store.DeleteTask(r.Context(), taskID)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}
	if err != nil {
		internalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) SetTimezone(w http.ResponseWriter, r *http.Request) {
	var req gen.SetTimezoneRequest
	if !decode(w, r, &req) {
		return
	}
	if err := s.settings.SetTimezone(r.Context(), req.Timezone); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) GetPushConfig(w http.ResponseWriter, r *http.Request) {
	if s.push == nil {
		writeError(w, http.StatusServiceUnavailable, "notifications aren't available")
		return
	}
	writeJSON(w, http.StatusOK, gen.PushConfig{PublicKey: s.push.PublicKey()})
}

func (s *Server) SubscribePush(w http.ResponseWriter, r *http.Request) {
	var req gen.PushSubscription
	if !decode(w, r, &req) {
		return
	}
	if !strings.HasPrefix(req.Endpoint, "https://") || req.Keys.P256dh == "" || req.Keys.Auth == "" {
		writeError(w, http.StatusBadRequest, "invalid push subscription")
		return
	}
	if err := s.store.SavePushSubscription(r.Context(), store.PushSubscription{
		Endpoint: req.Endpoint, P256dh: req.Keys.P256dh, Auth: req.Keys.Auth,
	}); err != nil {
		internalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) UnsubscribePush(w http.ResponseWriter, r *http.Request) {
	var req gen.PushUnsubscribe
	if !decode(w, r, &req) {
		return
	}
	if err := s.store.DeletePushSubscription(r.Context(), req.Endpoint); err != nil {
		internalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) accountNames(ctx context.Context) (map[string]string, error) {
	accounts, err := s.store.Accounts(ctx)
	if err != nil {
		return nil, err
	}
	names := make(map[string]string, len(accounts))
	for _, a := range accounts {
		names[a.ID] = a.Name
	}
	return names, nil
}
