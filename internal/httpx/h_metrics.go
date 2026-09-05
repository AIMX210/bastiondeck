package httpx

import (
	"net/http"
	"time"

	"bastiondeck/internal/connector"
	"bastiondeck/internal/factscat"
)

// hostMetrics collects ServerCat-style /proc metrics over the host connector
// (SSH or agent) and returns a diffed snapshot. POST (runs commands) with
// PermExec, matching the /facts route's posture. Brought over from the
// Android port (2026-09-06).
func (s *Server) hostFactscat(w http.ResponseWriter, r *http.Request) {
	hostID := r.PathValue("id")
	if hostID == "" {
		writeErr(w, 400, "bad_request", "hostId required")
		return
	}
	if s.deps.Factscat == nil {
		writeErr(w, 501, "not_enabled", "metrics collector not wired")
		return
	}
	cli, err := s.deps.Connector.Connect(r.Context(), hostID)
	if err != nil {
		writeErr(w, 502, "conn_lost", err.Error())
		return
	}
	defer cli.Close()
	res, err := cli.Exec(r.Context(), connector.ExecRequest{
		Command: factscat.CollectCmd, Timeout: 20 * time.Second, MaxBufferBytes: 1 << 20,
	})
	if err != nil {
		writeErr(w, 502, "exec_lost", err.Error())
		return
	}
	if res.Status != connector.StatusSuccess {
		writeErr(w, 502, "exec_failed", res.ErrorText)
		return
	}
	raw := factscat.Parse(string(res.Stdout))
	if raw == nil {
		writeErr(w, 502, "parse_failed", "not a Linux /proc target?")
		return
	}
	snap := s.deps.Factscat.Put(hostID, raw)
	writeJSON(w, 200, map[string]any{"snapshot": snap})
}
