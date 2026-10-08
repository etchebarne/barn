package api

import (
	"errors"
	"net/http"

	"github.com/etchebarne/openbot/internal/api/gen"
	"github.com/etchebarne/openbot/internal/runtime"
	"github.com/etchebarne/openbot/internal/store"
	"github.com/etchebarne/openbot/internal/view"
)

// recentRuns is how many runs the Schedule page lists.
const recentRuns = 30

func (s *Server) GetSchedule(w http.ResponseWriter, r *http.Request, params gen.GetScheduleParams) {
	ctx := r.Context()
	days := 7
	if params.Days != nil {
		days = min(max(*params.Days, 1), 31)
	}
	tasks, err := s.store.AllTasks(ctx)
	if err != nil {
		internalError(w, err)
		return
	}
	names, err := s.accountNames(ctx)
	if err != nil {
		internalError(w, err)
		return
	}
	runs, err := s.store.RecentTaskRuns(ctx, recentRuns, false)
	if err != nil {
		internalError(w, err)
		return
	}
	out := gen.Schedule{Tasks: make([]gen.Task, 0, len(tasks)), Upcoming: s.runtime.Upcoming(ctx, tasks, days),
		Recent: make([]gen.TaskRun, 0, len(runs))}
	for _, t := range tasks {
		out.Tasks = append(out.Tasks, taskView(t, names))
	}
	for _, run := range runs {
		out.Recent = append(out.Recent, view.TaskRun(run))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) ListTaskRuns(w http.ResponseWriter, r *http.Request, taskID string) {
	ctx := r.Context()
	if _, err := s.store.GetTask(ctx, taskID); errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "task not found")
		return
	} else if err != nil {
		internalError(w, err)
		return
	}
	runs, err := s.store.TaskRuns(ctx, taskID, 100)
	if err != nil {
		internalError(w, err)
		return
	}
	out := make([]gen.TaskRun, 0, len(runs))
	for _, run := range runs {
		out = append(out, view.TaskRun(run))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) RunTaskNow(w http.ResponseWriter, r *http.Request, taskID string) {
	err := s.runtime.RunTaskNow(r.Context(), taskID)
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "task not found")
	case errors.Is(err, runtime.ErrSignalTask):
		writeError(w, http.StatusBadRequest, err.Error())
	case err != nil:
		internalError(w, err)
	default:
		w.WriteHeader(http.StatusAccepted)
	}
}
