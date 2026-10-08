package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/etchebarne/openbot/internal/api/gen"
	"github.com/etchebarne/openbot/internal/runtime"
)

func (s *Server) ExportAgents(w http.ResponseWriter, r *http.Request) {
	b, err := s.runtime.ExportAgents(r.Context())
	if err != nil {
		internalError(w, err)
		return
	}
	w.Header().Set("Content-Disposition", `attachment; filename="openbot-agents-`+b.ExportedAt.Format("2006-01-02")+`.json"`)
	writeJSON(w, http.StatusOK, b)
}

// maxBackup bounds a backup upload (agents with many memories and tasks fit easily).
const maxBackup = 16 << 20

func (s *Server) RestoreAgents(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBackup)
	var b gen.AgentsBackup
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		writeError(w, http.StatusBadRequest, "that file isn't a valid backup")
		return
	}
	res, err := s.runtime.RestoreAgents(r.Context(), b)
	var me *runtime.ModelError
	switch {
	case errors.As(err, &me):
		writeError(w, http.StatusBadRequest, me.Message)
	case err != nil:
		internalError(w, err)
	default:
		writeJSON(w, http.StatusOK, res)
	}
}
