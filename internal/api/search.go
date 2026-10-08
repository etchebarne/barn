package api

import (
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/etchebarne/openbot/internal/api/gen"
	"github.com/etchebarne/openbot/internal/store"
)

func (s *Server) Search(w http.ResponseWriter, r *http.Request, params gen.SearchParams) {
	ctx := r.Context()
	q := strings.TrimSpace(params.Q)
	if q == "" || utf8.RuneCountInString(q) > 200 {
		writeError(w, http.StatusBadRequest, "q must be 1–200 characters")
		return
	}
	limit := 20
	if params.Limit != nil {
		limit = min(max(*params.Limit, 1), 50)
	}
	msgs, err := s.store.SearchMessages(ctx, q, limit)
	if err != nil {
		internalError(w, err)
		return
	}
	mems, err := s.store.SearchMemories(ctx, q, limit)
	if err != nil {
		internalError(w, err)
		return
	}
	tasks, err := s.store.SearchTasks(ctx, q, limit)
	if err != nil {
		internalError(w, err)
		return
	}
	names, err := s.accountNames(ctx)
	if err != nil {
		internalError(w, err)
		return
	}
	out := gen.SearchResults{
		Messages: make([]gen.MessageHit, 0, len(msgs)),
		Memories: make([]gen.MemoryHit, 0, len(mems)),
		Tasks:    make([]gen.Task, 0, len(tasks)),
	}
	for _, m := range msgs {
		out.Messages = append(out.Messages, gen.MessageHit{Id: m.ID, ChatId: m.ChatID, Snippet: m.Snippet,
			Author:    gen.MessageAuthor{Kind: gen.MessageAuthorKind(m.AuthorKind), AgentId: m.AuthorAgentID},
			CreatedAt: store.Time(m.CreatedAt)})
	}
	for _, m := range mems {
		out.Memories = append(out.Memories, gen.MemoryHit{Id: m.ID, AgentId: m.AgentID, Text: m.Text})
	}
	for _, t := range tasks {
		out.Tasks = append(out.Tasks, taskView(t, names))
	}
	writeJSON(w, http.StatusOK, out)
}
