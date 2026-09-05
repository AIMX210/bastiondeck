package httpx

import (
	"net/http"

	"bastiondeck/internal/tunnel"
)

func (s *Server) listTunnels(w http.ResponseWriter, r *http.Request) {
	views, err := s.deps.Tunnels.List(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"tunnels": views})
}

func (s *Server) createTunnel(w http.ResponseWriter, r *http.Request) {
	var req tunnel.Spec
	if err := decodeJSON(r, &req, 0); err != nil {
		writeErr(w, 400, "bad_request", err.Error())
		return
	}
	u, _ := CurrentUser(r)
	req.StartedBy = u.ID
	v, err := s.deps.Tunnels.Create(r.Context(), req)
	if err != nil {
		fail(w, err)
		return
	}
	_, _ = s.deps.Audit.Write(r.Context(), s.actorOf(r), "tunnel.create", "tunnel", v.ID, "success",
		map[string]any{"kind": v.Kind, "localPort": v.LocalPort, "remotePort": v.RemotePort})
	writeJSON(w, 201, map[string]any{"tunnel": v})
}

func (s *Server) stopTunnel(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.deps.Tunnels.Stop(r.Context(), id); err != nil {
		fail(w, err)
		return
	}
	_, _ = s.deps.Audit.Write(r.Context(), s.actorOf(r), "tunnel.stop", "tunnel", id, "success", nil)
	writeJSON(w, 200, map[string]any{"ok": true})
}
