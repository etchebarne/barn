package api

import (
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// spaHandler serves the built web app from files, falling back to index.html for client routes.
func spaHandler(files fs.FS) http.Handler {
	fileServer := http.FileServerFS(files)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		info, err := fs.Stat(files, p)
		if p == "" || err != nil || info.IsDir() {
			w.Header().Set("Cache-Control", "no-cache")
			http.ServeFileFS(w, r, files, "index.html")
			return
		}
		if strings.HasPrefix(p, "assets/") {
			// Vite emits content-hashed filenames under /assets.
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		fileServer.ServeHTTP(w, r)
	})
}
