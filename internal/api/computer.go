package api

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/coder/websocket"

	"github.com/etchebarne/openbot/internal/api/gen"
	"github.com/etchebarne/openbot/internal/attachments"
	"github.com/etchebarne/openbot/internal/runtime"
	"github.com/etchebarne/openbot/internal/sandbox"
	"github.com/etchebarne/openbot/internal/store"
)

// An agent's computer, opened by the user: files and a terminal in its sandbox.

// maxComputerFile is the largest file the user can upload or download.
const maxComputerFile = 200 << 20

// computer finds the agent's sandbox, writing an error response if it can't.
func (s *Server) computer(w http.ResponseWriter, r *http.Request, agentID string) (runtime.Computer, string, bool) {
	c, id, err := s.runtime.Computer(r.Context(), agentID)
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "agent not found")
	case errors.Is(err, sandbox.ErrUnavailable):
		writeError(w, http.StatusServiceUnavailable, err.Error())
	case err != nil:
		internalError(w, err)
	default:
		return c, id, true
	}
	return nil, "", false
}

// fileError writes the response for a failed file operation.
func fileError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, sandbox.ErrNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, sandbox.ErrExists):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, sandbox.ErrUnavailable):
		writeError(w, http.StatusServiceUnavailable, err.Error())
	default:
		writeError(w, http.StatusBadRequest, err.Error())
	}
}

func (s *Server) ListSandboxFiles(w http.ResponseWriter, r *http.Request, agentID string, params gen.ListSandboxFilesParams) {
	c, box, ok := s.computer(w, r, agentID)
	if !ok {
		return
	}
	entries, cut, err := c.List(r.Context(), box, params.Path)
	if err != nil {
		fileError(w, err)
		return
	}
	out := make([]gen.FileEntry, 0, len(entries))
	for _, e := range entries {
		out = append(out, gen.FileEntry{Name: e.Name, Kind: gen.FileEntryKind(e.Kind), Size: e.Size, ModifiedAt: e.Modified.UTC()})
	}
	folder := func(k gen.FileEntryKind) bool { return k == gen.Dir || k == gen.Dirlink }
	slices.SortFunc(out, func(a, b gen.FileEntry) int {
		if fa, fb := folder(a.Kind), folder(b.Kind); fa != fb {
			if fa {
				return -1
			}
			return 1
		}
		return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})
	writeJSON(w, http.StatusOK, gen.FolderListing{Path: path.Clean(params.Path), Entries: out, Truncated: cut})
}

func (s *Server) WriteSandboxFile(w http.ResponseWriter, r *http.Request, agentID string, params gen.WriteSandboxFileParams) {
	c, box, ok := s.computer(w, r, agentID)
	if !ok {
		return
	}
	if r.ContentLength > maxComputerFile {
		writeError(w, http.StatusRequestEntityTooLarge, "files can be up to 200 MB")
		return
	}
	body := http.MaxBytesReader(w, r.Body, maxComputerFile)
	if err := c.CopyIn(r.Context(), box, params.Path, body); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			writeError(w, http.StatusRequestEntityTooLarge, "files can be up to 200 MB")
			return
		}
		fileError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) DeleteSandboxFile(w http.ResponseWriter, r *http.Request, agentID string, params gen.DeleteSandboxFileParams) {
	c, box, ok := s.computer(w, r, agentID)
	if !ok {
		return
	}
	if err := c.Delete(r.Context(), box, params.Path); err != nil {
		fileError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) CreateSandboxFolder(w http.ResponseWriter, r *http.Request, agentID string) {
	var req gen.PathRequest
	if !decode(w, r, &req) {
		return
	}
	c, box, ok := s.computer(w, r, agentID)
	if !ok {
		return
	}
	if err := c.Mkdir(r.Context(), box, req.Path); err != nil {
		fileError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) MoveSandboxFile(w http.ResponseWriter, r *http.Request, agentID string) {
	var req gen.MoveRequest
	if !decode(w, r, &req) {
		return
	}
	c, box, ok := s.computer(w, r, agentID)
	if !ok {
		return
	}
	if err := c.Move(r.Context(), box, req.From, req.To); err != nil {
		fileError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) DownloadSandboxFile(w http.ResponseWriter, r *http.Request, agentID string, params gen.DownloadSandboxFileParams) {
	c, box, ok := s.computer(w, r, agentID)
	if !ok {
		return
	}
	rc, size, err := c.CopyOut(r.Context(), box, params.Path, maxComputerFile)
	if err != nil {
		if size > maxComputerFile {
			writeError(w, http.StatusRequestEntityTooLarge, "files over 200 MB can't be downloaded here; use the terminal")
			return
		}
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	defer rc.Close()
	br := bufio.NewReaderSize(rc, 512)
	head, _ := br.Peek(512)
	name := path.Base(params.Path)
	mimeType := attachments.DetectType(name, head)
	disposition, contentType := "attachment", "application/octet-stream" // never rendered as a page
	if attachments.Inline(mimeType) {
		contentType = mimeType
		if params.Inline != nil && *params.Inline {
			disposition = "inline"
		}
	}
	h := w.Header()
	h.Set("Content-Type", contentType)
	h.Set("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": name}))
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Content-Security-Policy", "default-src 'none'; img-src 'self'; style-src 'unsafe-inline'; sandbox")
	h.Set("Cache-Control", "no-store")
	h.Set("Content-Length", fmt.Sprint(size))
	if _, err := io.Copy(w, br); err != nil {
		slog.Debug("download from a computer", "path", params.Path, "err", err)
	}
}

// SandboxTerminal connects a WebSocket to a shell in the agent's computer.
func (s *Server) SandboxTerminal(w http.ResponseWriter, r *http.Request, agentID string, params gen.SandboxTerminalParams) {
	c, box, ok := s.computer(w, r, agentID)
	if !ok {
		return
	}
	size := func(v *int, def uint16) uint16 {
		if v == nil || *v < 1 || *v > 1000 {
			return def
		}
		return uint16(*v)
	}
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: originHosts(s.opts.AllowedOrigins)})
	if err != nil {
		slog.Debug("terminal accept", "err", err)
		return
	}
	defer conn.CloseNow()
	conn.SetReadLimit(1 << 20)
	ctx := r.Context()

	term, err := c.Shell(ctx, box, size(params.Cols, 80), size(params.Rows, 24))
	if err != nil {
		conn.Close(websocket.StatusInternalError, truncateReason("couldn't start a shell: "+err.Error()))
		return
	}
	defer term.Close()

	// Shell → browser, until the shell exits and its last output is sent.
	output := make(chan struct{})
	go func() {
		defer close(output)
		buf := make([]byte, 32<<10)
		for {
			n, err := term.Read(buf)
			if n > 0 {
				wctx, cancel := context.WithTimeout(ctx, 30*time.Second)
				werr := conn.Write(wctx, websocket.MessageBinary, buf[:n])
				cancel()
				if werr != nil {
					return
				}
			}
			if err != nil {
				return // the shell exited
			}
		}
	}()
	// Browser → shell, until the browser goes away.
	browser := make(chan struct{})
	go func() {
		defer close(browser)
		for {
			typ, data, err := conn.Read(ctx)
			if err != nil {
				return
			}
			if typ == websocket.MessageBinary {
				if _, err := term.Write(data); err != nil {
					return
				}
				continue
			}
			var msg struct {
				Type       string `json:"type"`
				Cols, Rows int
			}
			if json.Unmarshal(data, &msg) == nil && msg.Type == "resize" {
				c, rw := msg.Cols, msg.Rows
				_ = term.Resize(size(&c, 80), size(&rw, 24))
			}
		}
	}()

	select {
	case <-output:
		conn.Close(websocket.StatusNormalClosure, "the shell exited")
	case <-browser:
		// The deferred Close ends the shell.
	}
}

// truncateReason keeps a WebSocket close reason within the protocol's 123 bytes.
func truncateReason(s string) string {
	if len(s) <= 120 {
		return s
	}
	return s[:120]
}
