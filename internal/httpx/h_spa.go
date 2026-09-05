package httpx

import (
	"io/fs"
	"net/http"
	"path"
	"strings"

	"bastiondeck/internal/webui"
)

// spa serves the embedded frontend, falling back to index.html for client
// routes. An on-disk StaticDir (dev mode) takes precedence when configured.
func (s *Server) spa(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/ws/") {
		writeErr(w, 404, "not_found", "no such endpoint")
		return
	}
	if dir := s.deps.Cfg.StaticDir; dir != "" {
		s.serveDisk(w, r, dir)
		return
	}
	s.serveEmbedded(w, r)
}

func (s *Server) serveEmbedded(w http.ResponseWriter, r *http.Request) {
	root := webui.FS()
	p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if p == "" {
		p = "index.html"
	}
	if f, err := root.Open(p); err == nil {
		_ = f.Close()
		http.FileServer(http.FS(root)).ServeHTTP(w, r)
		return
	}
	// Client-side route fallback.
	data, err := fs.ReadFile(root, "index.html")
	if err != nil {
		writeErr(w, 404, "not_found", "frontend bundle missing")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(data)
}

func (s *Server) serveDisk(w http.ResponseWriter, r *http.Request, dir string) {
	rel := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if rel == "" {
		rel = "index.html"
	}
	http.ServeFile(w, r, path.Join(dir, rel))
}
