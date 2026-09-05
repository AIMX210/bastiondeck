package httpx

import "net/http"

func (s *Server) listSnippets(w http.ResponseWriter, r *http.Request) {
	sn, err := s.deps.Snippets.List(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"snippets": sn})
}

func (s *Server) createSnippet(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Title string   `json:"title"`
		Body  string   `json:"body"`
		Tags  []string `json:"tags"`
	}
	if err := decodeJSON(r, &req, 0); err != nil {
		writeErr(w, 400, "bad_request", err.Error())
		return
	}
	u, _ := CurrentUser(r)
	sn, err := s.deps.Snippets.Create(r.Context(), req.Title, req.Body, req.Tags, u.ID)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 201, map[string]any{"snippet": sn})
}

func (s *Server) updateSnippet(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		Title string   `json:"title"`
		Body  string   `json:"body"`
		Tags  []string `json:"tags"`
	}
	if err := decodeJSON(r, &req, 0); err != nil {
		writeErr(w, 400, "bad_request", err.Error())
		return
	}
	sn, err := s.deps.Snippets.Update(r.Context(), id, req.Title, req.Body, req.Tags)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"snippet": sn})
}

func (s *Server) deleteSnippet(w http.ResponseWriter, r *http.Request) {
	if err := s.deps.Snippets.Delete(r.Context(), r.PathValue("id")); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) renderSnippet(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Body string            `json:"body"`
		Vars map[string]string `json:"vars"`
	}
	if err := decodeJSON(r, &req, 0); err != nil {
		writeErr(w, 400, "bad_request", err.Error())
		return
	}
	rendered, missing := renderBody(req.Body, req.Vars)
	writeJSON(w, 200, map[string]any{"rendered": rendered, "missing": missing})
}
