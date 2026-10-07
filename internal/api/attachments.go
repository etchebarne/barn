package api

import (
	"errors"
	"fmt"
	"mime"
	"net/http"
	"os"

	"github.com/etchebarne/barn/internal/api/gen"
	"github.com/etchebarne/barn/internal/attachments"
	"github.com/etchebarne/barn/internal/store"
	"github.com/etchebarne/barn/internal/view"
)

func (s *Server) UploadAttachment(w http.ResponseWriter, r *http.Request, chatID gen.ChatId) {
	if s.Files == nil {
		writeError(w, http.StatusNotFound, "attachments aren't available")
		return
	}
	if !s.chatExists(w, r, chatID) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, attachments.MaxSize+1<<20)
	file, header, err := r.FormFile("file")
	var tooBig *http.MaxBytesError
	switch {
	case errors.As(err, &tooBig):
		writeError(w, http.StatusRequestEntityTooLarge, "The file is over 25 MB")
		return
	case err != nil:
		writeError(w, http.StatusBadRequest, `send the file as multipart/form-data in the field "file"`)
		return
	}
	defer file.Close()
	a, err := s.Files.Save(r.Context(), chatID, header.Filename, file)
	switch {
	case errors.Is(err, attachments.ErrTooLarge), errors.As(err, &tooBig):
		writeError(w, http.StatusRequestEntityTooLarge, "The file is over 25 MB")
	case err != nil:
		internalError(w, err)
	default:
		writeJSON(w, http.StatusCreated, view.Attachment(a))
	}
}

func (s *Server) GetAttachment(w http.ResponseWriter, r *http.Request, id string, params gen.GetAttachmentParams) {
	if s.Files == nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	a, err := s.store.GetAttachment(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		internalError(w, err)
		return
	}
	f, err := os.Open(s.Files.Path(a))
	if err != nil {
		writeError(w, http.StatusNotFound, "the file is missing")
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		internalError(w, err)
		return
	}
	disposition := "attachment"
	if attachments.Inline(a.Mime) && (params.Download == nil || !*params.Download) {
		disposition = "inline"
	}
	contentType := a.Mime
	if !attachments.Inline(a.Mime) {
		contentType = "application/octet-stream" // never let the browser render it as a page
	}
	h := w.Header()
	h.Set("Content-Type", contentType)
	h.Set("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": a.Name}))
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Content-Security-Policy", "default-src 'none'; img-src 'self'; style-src 'unsafe-inline'; sandbox")
	h.Set("Cache-Control", "private, max-age=31536000, immutable")
	h.Set("Content-Length", fmt.Sprint(info.Size()))
	http.ServeContent(w, r, "", info.ModTime(), f)
}
