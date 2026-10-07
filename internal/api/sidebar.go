package api

import (
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/etchebarne/barn/internal/api/gen"
	"github.com/etchebarne/barn/internal/store"
)

func categoryView(c store.SidebarCategory) gen.SidebarCategory {
	return gen.SidebarCategory{Id: c.ID, Name: c.Name, Collapsed: c.Collapsed}
}

func (s *Server) sidebarUpdated() {
	s.bus.Publish(gen.WsSidebarUpdated{Type: "sidebar.updated"})
}

func validCategoryName(w http.ResponseWriter, name string) (string, bool) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > 40 {
		writeError(w, http.StatusBadRequest, "category names are 1–40 characters")
		return "", false
	}
	return name, true
}

func (s *Server) ListSidebarCategories(w http.ResponseWriter, r *http.Request) {
	cats, err := s.store.SidebarCategories(r.Context())
	if err != nil {
		internalError(w, err)
		return
	}
	out := make([]gen.SidebarCategory, 0, len(cats))
	for _, c := range cats {
		out = append(out, categoryView(c))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) CreateSidebarCategory(w http.ResponseWriter, r *http.Request) {
	var req gen.CreateSidebarCategoryRequest
	if !decode(w, r, &req) {
		return
	}
	name, ok := validCategoryName(w, req.Name)
	if !ok {
		return
	}
	c, err := s.store.CreateSidebarCategory(r.Context(), name)
	if err != nil {
		internalError(w, err)
		return
	}
	s.sidebarUpdated()
	writeJSON(w, http.StatusCreated, categoryView(c))
}

func (s *Server) UpdateSidebarCategory(w http.ResponseWriter, r *http.Request, id string) {
	var req gen.UpdateSidebarCategoryRequest
	if !decode(w, r, &req) {
		return
	}
	if req.Name != nil {
		name, ok := validCategoryName(w, *req.Name)
		if !ok {
			return
		}
		req.Name = &name
	}
	c, err := s.store.UpdateSidebarCategory(r.Context(), id, req.Name, req.Collapsed)
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "category not found")
	case err != nil:
		internalError(w, err)
	default:
		s.sidebarUpdated()
		writeJSON(w, http.StatusOK, categoryView(c))
	}
}

func (s *Server) DeleteSidebarCategory(w http.ResponseWriter, r *http.Request, id string) {
	switch err := s.store.DeleteSidebarCategory(r.Context(), id); {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "category not found")
	case err != nil:
		internalError(w, err)
	default:
		s.sidebarUpdated()
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Server) SetSidebarLayout(w http.ResponseWriter, r *http.Request) {
	var req gen.SidebarLayout
	if !decode(w, r, &req) {
		return
	}
	sections := make([]store.SidebarSection, 0, len(req.Sections))
	for _, sec := range req.Sections {
		sections = append(sections, store.SidebarSection{CategoryID: sec.CategoryId, ChatIDs: sec.ChatIds})
	}
	switch err := s.store.SetSidebarLayout(r.Context(), req.CategoryOrder, sections); {
	case errors.Is(err, store.ErrBadLayout):
		writeError(w, http.StatusBadRequest, "the layout names an unknown chat or category, or leaves a category out")
	case err != nil:
		internalError(w, err)
	default:
		s.sidebarUpdated()
		w.WriteHeader(http.StatusNoContent)
	}
}
