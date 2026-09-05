package httpx

import (
	"net/http"
)

func (s *Server) listCredentials(w http.ResponseWriter, r *http.Request) {
	cs, err := s.deps.Creds.List(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"credentials": cs})
}

func (s *Server) createCredential(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name       string `json:"name"`
		Kind       string `json:"kind"`
		Secret     string `json:"secret"`
		Passphrase string `json:"passphrase"`
	}
	if err := decodeJSON(r, &req, 0); err != nil {
		writeErr(w, 400, "bad_request", err.Error())
		return
	}
	u, _ := CurrentUser(r)
	c, err := s.deps.Creds.Create(r.Context(), req.Name, req.Kind, req.Secret, req.Passphrase, u.ID)
	if err != nil {
		fail(w, err)
		return
	}
	_, _ = s.deps.Audit.Write(r.Context(), s.actorOf(r), "credential.create", "credential", c.ID, "success", nil)
	writeJSON(w, 201, map[string]any{"credential": c})
}

func (s *Server) updateCredential(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		Name       string  `json:"name"`
		Secret     *string `json:"secret"`
		Passphrase *string `json:"passphrase"`
	}
	if err := decodeJSON(r, &req, 0); err != nil {
		writeErr(w, 400, "bad_request", err.Error())
		return
	}
	c, err := s.deps.Creds.Update(r.Context(), id, req.Name, req.Secret, req.Passphrase)
	if err != nil {
		fail(w, err)
		return
	}
	_, _ = s.deps.Audit.Write(r.Context(), s.actorOf(r), "credential.update", "credential", id, "success", nil)
	writeJSON(w, 200, map[string]any{"credential": c})
}

func (s *Server) deleteCredential(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if n, err := s.deps.Creds.InUse(r.Context(), id); err != nil {
		fail(w, err)
		return
	} else if n > 0 {
		writeErr(w, 409, "in_use", "credential referenced by "+itoa(n)+" host(s)")
		return
	}
	if err := s.deps.Creds.Delete(r.Context(), id); err != nil {
		fail(w, err)
		return
	}
	_, _ = s.deps.Audit.Write(r.Context(), s.actorOf(r), "credential.delete", "credential", id, "success", nil)
	writeJSON(w, 200, map[string]any{"ok": true})
}
