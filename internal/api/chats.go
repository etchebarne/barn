package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/etchebarne/openbot/internal/api/gen"
	"github.com/etchebarne/openbot/internal/runtime"
	"github.com/etchebarne/openbot/internal/sandbox"
	"github.com/etchebarne/openbot/internal/store"
	"github.com/etchebarne/openbot/internal/view"
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
	var attachmentIDs []string
	if req.AttachmentIds != nil {
		attachmentIDs = *req.AttachmentIds
	}
	if len(attachmentIDs) > 10 {
		writeError(w, http.StatusBadRequest, "at most 10 attachments per message")
		return
	}
	if (strings.TrimSpace(req.Body) == "" && len(attachmentIDs) == 0) || utf8.RuneCountInString(req.Body) > 32000 {
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
	replyTo := ""
	if req.ReplyToId != nil {
		replyTo = *req.ReplyToId
	}
	msg, err := s.store.InsertFull(ctx, store.NewMessage{ChatID: chatID, AuthorKind: "user", Body: req.Body,
		Mentions: s.runtime.MentionsIn(ctx, chatID, req.Body), AttachmentIDs: attachmentIDs, ReplyTo: replyTo})
	if errors.Is(err, store.ErrBadAttachment) {
		writeError(w, http.StatusBadRequest, "an attachment isn't an upload in this chat, or was already sent")
		return
	}
	if errors.Is(err, store.ErrBadReply) {
		writeError(w, http.StatusBadRequest, "replyToId isn't a message in this chat")
		return
	}
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

func (s *Server) ClearChatHistory(w http.ResponseWriter, r *http.Request, chatID gen.ChatId) {
	err := s.runtime.ClearDM(r.Context(), chatID)
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "chat not found")
	case errors.Is(err, runtime.ErrNotDM):
		writeError(w, http.StatusBadRequest, "only DMs can be cleared")
	case err != nil:
		internalError(w, err)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
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
	u := store.AgentUpdate{Name: req.Name, Instructions: req.Instructions, Personality: req.Personality, Model: req.Model,
		Language: req.Language, Notifications: req.Notifications}
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

func (s *Server) StopAgent(w http.ResponseWriter, r *http.Request, agentID string) {
	if _, err := s.store.GetAgent(r.Context(), agentID); errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	} else if err != nil {
		internalError(w, err)
		return
	}
	if err := s.runtime.Stop(agentID); errors.Is(err, runtime.ErrNotWorking) {
		writeError(w, http.StatusConflict, "the agent isn't working")
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
	case "secret":
		// Providing goes through POST /messages/{id}/secret; an answer here is "Not now".
		if len(a.Selected) != 1 || a.Selected[0] != 1 || hasText {
			return bad("provide the secret with its own endpoint, or decline")
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

func (s *Server) CreateMemory(w http.ResponseWriter, r *http.Request, agentID string) {
	var req gen.MemoryRequest
	if !decode(w, r, &req) {
		return
	}
	text, ok := memoryText(w, req.Text)
	if !ok || !s.agentExists(w, r, agentID) {
		return
	}
	m, err := s.store.AddMemory(r.Context(), agentID, text, nil)
	if err != nil {
		internalError(w, err)
		return
	}
	s.memoriesChanged(r, agentID)
	writeJSON(w, http.StatusCreated, gen.Memory{Id: m.ID, Text: m.Text, CreatedAt: store.Time(m.CreatedAt)})
}

func (s *Server) UpdateMemory(w http.ResponseWriter, r *http.Request, agentID, memoryID string) {
	var req gen.MemoryRequest
	if !decode(w, r, &req) {
		return
	}
	text, ok := memoryText(w, req.Text)
	if !ok {
		return
	}
	m, err := s.store.UpdateMemory(r.Context(), agentID, memoryID, text)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "memory not found")
		return
	}
	if err != nil {
		internalError(w, err)
		return
	}
	s.memoriesChanged(r, agentID)
	writeJSON(w, http.StatusOK, gen.Memory{Id: m.ID, Text: m.Text, CreatedAt: store.Time(m.CreatedAt)})
}

// memoryText validates a memory's text, writing a 400 if it's not usable.
func memoryText(w http.ResponseWriter, text string) (string, bool) {
	text = strings.TrimSpace(text)
	if text == "" || utf8.RuneCountInString(text) > runtime.MaxMemoryLength {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("a memory must be 1–%d characters", runtime.MaxMemoryLength))
		return "", false
	}
	return text, true
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
	s.memoriesChanged(r, agentID)
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
	if t.Check != "" {
		check := t.Check
		out.Check = &check
	}
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

func (s *Server) CreateTask(w http.ResponseWriter, r *http.Request, agentID string) {
	var req gen.CreateTaskRequest
	if !decode(w, r, &req) {
		return
	}
	if !s.agentExists(w, r, agentID) {
		return
	}
	t, err := s.runtime.SaveTask(r.Context(), agentID, "", runtime.TaskChange{
		Name: &req.Name, Purpose: &req.Purpose, Cron: req.Cron, At: req.At, Check: req.Check,
	})
	s.writeTask(w, r, t, err, http.StatusCreated)
}

func (s *Server) UpdateTask(w http.ResponseWriter, r *http.Request, taskID string) {
	var req gen.UpdateTaskRequest
	if !decode(w, r, &req) {
		return
	}
	t, err := s.store.GetTask(r.Context(), taskID)
	if err == nil {
		t, err = s.runtime.SaveTask(r.Context(), t.AgentID, taskID, runtime.TaskChange{
			Name: req.Name, Purpose: req.Purpose, Cron: req.Cron, At: req.At, Enabled: req.Enabled, Check: req.Check,
		})
	}
	s.writeTask(w, r, t, err, http.StatusOK)
}

func (s *Server) writeTask(w http.ResponseWriter, r *http.Request, t store.Task, err error, status int) {
	var me *runtime.ModelError
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "task not found")
		return
	case errors.As(err, &me):
		writeError(w, http.StatusBadRequest, me.Message)
		return
	case err != nil:
		internalError(w, err)
		return
	}
	names, err := s.accountNames(r.Context())
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, status, taskView(t, names))
}

// agentExists writes a 404 when the agent doesn't exist.
func (s *Server) agentExists(w http.ResponseWriter, r *http.Request, agentID string) bool {
	if _, err := s.store.GetAgent(r.Context(), agentID); errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "agent not found")
		return false
	} else if err != nil {
		internalError(w, err)
		return false
	}
	return true
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

func (s *Server) DeleteAgent(w http.ResponseWriter, r *http.Request, agentID string) {
	err := s.runtime.DeleteAgent(r.Context(), agentID)
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "agent not found")
	case errors.Is(err, store.ErrLastAdmin):
		writeError(w, http.StatusConflict, "openbot needs at least one admin agent, so this one can't be deleted")
	case err != nil:
		internalError(w, err)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Server) ListStandingApprovals(w http.ResponseWriter, r *http.Request, agentID string) {
	if _, err := s.store.GetAgent(r.Context(), agentID); err != nil {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	}
	rules, err := s.store.ApprovalRules(r.Context(), agentID)
	if err != nil {
		internalError(w, err)
		return
	}
	out := make([]gen.StandingApproval, 0, len(rules))
	for _, rule := range rules {
		out = append(out, gen.StandingApproval{Id: rule.ID, Label: rule.Label, CreatedAt: store.Time(rule.CreatedAt)})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) RevokeStandingApproval(w http.ResponseWriter, r *http.Request, agentID, approvalID string) {
	switch err := s.store.RevokeApproval(r.Context(), agentID, approvalID); {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "not found")
	case err != nil:
		internalError(w, err)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Server) ProvideSecret(w http.ResponseWriter, r *http.Request, messageID string) {
	var req gen.SecretValue
	if !decode(w, r, &req) {
		return
	}
	msg, err := s.runtime.ProvideSecret(r.Context(), messageID, req.Value)
	var bad *runtime.ErrBadSecret
	switch {
	case errors.As(err, &bad):
		writeError(w, http.StatusBadRequest, bad.Message)
		return
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "no secret request here")
		return
	case errors.Is(err, store.ErrPromptClosed):
		writeError(w, http.StatusConflict, "this request was already answered")
		return
	case err != nil:
		internalError(w, err)
		return
	}
	if err := s.store.MarkRead(r.Context(), msg.ChatID, msg.ID); err != nil {
		internalError(w, err)
		return
	}
	out := view.Message(msg)
	s.bus.Publish(view.MessageUpdated(out))
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) ListAgentSecrets(w http.ResponseWriter, r *http.Request, agentID string) {
	if !s.agentExists(w, r, agentID) {
		return
	}
	secrets, err := s.store.AgentSecrets(r.Context(), agentID)
	if err != nil {
		internalError(w, err)
		return
	}
	out := make([]gen.AgentSecret, 0, len(secrets))
	for _, a := range secrets {
		out = append(out, gen.AgentSecret{Name: a.Name, Description: a.Description,
			CreatedAt: store.Time(a.CreatedAt), UpdatedAt: store.Time(a.UpdatedAt)})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) SetAgentSecret(w http.ResponseWriter, r *http.Request, agentID, name string) {
	var req gen.SetSecretRequest
	if !decode(w, r, &req) {
		return
	}
	if !s.agentExists(w, r, agentID) {
		return
	}
	description := ""
	if req.Description != nil {
		description = *req.Description
	}
	err := s.runtime.SetSecret(r.Context(), agentID, name, description, req.Value)
	var bad *runtime.ErrBadSecret
	switch {
	case errors.As(err, &bad):
		writeError(w, http.StatusBadRequest, bad.Message)
	case err != nil:
		internalError(w, err)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Server) DeleteAgentSecret(w http.ResponseWriter, r *http.Request, agentID, name string) {
	err := s.store.DeleteAgentSecret(r.Context(), agentID, name)
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "secret not found")
	case err != nil:
		internalError(w, err)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

// memoriesChanged has the agent's prompt rebuilt, so it sees memories the user edited.
func (s *Server) memoriesChanged(r *http.Request, agentID string) {
	if err := s.store.InvalidatePrompt(r.Context(), agentID); err != nil {
		slog.Warn("invalidate prompt", "agent", agentID, "err", err)
	}
}
