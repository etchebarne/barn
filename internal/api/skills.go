package api

import (
	"errors"
	"net/http"

	"github.com/etchebarne/openbot/internal/api/gen"
	"github.com/etchebarne/openbot/internal/skills"
)

func skillView(s skills.Skill, full bool) gen.Skill {
	out := gen.Skill{Name: s.Name, Description: s.Description, Path: skills.Path(s.Name), UpdatedAt: s.UpdatedAt.UTC()}
	if s.Problem != "" {
		out.Problem = &s.Problem
	}
	if full {
		out.Instructions = &s.Instructions
		files := s.Files
		if files == nil {
			files = []string{}
		}
		out.Files = &files
	}
	return out
}

// library is the skill library, or nil (with a 503 written) when there's none.
func (s *Server) library(w http.ResponseWriter) *skills.Library {
	if s.runtime.Skills == nil {
		writeError(w, http.StatusServiceUnavailable, "skills aren't set up on this server")
	}
	return s.runtime.Skills
}

func (s *Server) ListSkills(w http.ResponseWriter, r *http.Request) {
	lib := s.library(w)
	if lib == nil {
		return
	}
	list, err := lib.List()
	if err != nil {
		internalError(w, err)
		return
	}
	out := make([]gen.Skill, 0, len(list))
	for _, sk := range list {
		out = append(out, skillView(sk, false))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) GetSkill(w http.ResponseWriter, r *http.Request, name string) {
	lib := s.library(w)
	if lib == nil {
		return
	}
	sk, err := lib.Get(name)
	if errors.Is(err, skills.ErrNotFound) {
		writeError(w, http.StatusNotFound, "skill not found")
		return
	}
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, skillView(sk, true))
}

func (s *Server) SaveSkill(w http.ResponseWriter, r *http.Request, name string) {
	var req gen.SaveSkillRequest
	if !decode(w, r, &req) {
		return
	}
	lib := s.library(w)
	if lib == nil {
		return
	}
	if err := skills.Validate(name, req.Description, req.Instructions); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	sk, err := lib.Save(name, req.Description, req.Instructions)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, skillView(sk, true))
}

func (s *Server) DeleteSkill(w http.ResponseWriter, r *http.Request, name string) {
	lib := s.library(w)
	if lib == nil {
		return
	}
	err := lib.Delete(name)
	if errors.Is(err, skills.ErrNotFound) {
		writeError(w, http.StatusNotFound, "skill not found")
		return
	}
	if err != nil {
		internalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
