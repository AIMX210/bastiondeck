package httpx

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"time"

	"bastiondeck/internal/audit"
	"bastiondeck/internal/backup"
)

// ---------- Metrics ----------

func (s *Server) hostMetrics(w http.ResponseWriter, r *http.Request) {
	hostID := r.PathValue("id")
	q := r.URL.Query()
	kind := q.Get("kind")
	if kind == "" {
		// No range requested: collect a fresh point then return all kinds latest.
		if err := s.deps.Metrics.CollectHost(r.Context(), hostID); err != nil {
			writeErr(w, 502, "conn_lost", err.Error())
			return
		}
		now := time.Now().UTC()
		from := now.Add(-15 * time.Minute).Format(time.RFC3339)
		out := map[string]any{}
		for _, k := range []string{"cpu", "mem", "disk"} {
			pts, err := s.deps.Metrics.Query(r.Context(), hostID, k, from, now.Format(time.RFC3339), 120)
			if err == nil {
				out[k] = pts
			}
		}
		writeJSON(w, 200, map[string]any{"series": out})
		return
	}
	from := q.Get("from")
	to := q.Get("to")
	if from == "" {
		from = time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	}
	if to == "" {
		to = time.Now().UTC().Format(time.RFC3339)
	}
	pts, err := s.deps.Metrics.Query(r.Context(), hostID, kind, from, to, atoiDefault(q.Get("buckets"), 120))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"kind": kind, "points": pts})
}

// ---------- Audit ----------

func (s *Server) listAudit(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit := clamp(atoiDefault(q.Get("limit"), 50), 1, 200)
	entries, next, err := s.deps.Audit.ListPage(r.Context(), limit, cursorOf(q.Get("cursor")), audit.Filter{
		Actor: q.Get("actor"), Action: q.Get("action"), Result: q.Get("result"),
		From: q.Get("from"), To: q.Get("to")})
	if err != nil {
		fail(w, err)
		return
	}
	resp := map[string]any{"entries": entries}
	if next > 0 {
		resp["nextCursor"] = next
	}
	writeJSON(w, 200, resp)
}

func (s *Server) exportAudit(w http.ResponseWriter, r *http.Request) {
	entries, _, err := s.deps.Audit.ListPage(r.Context(), 200, 0, audit.Filter{})
	if err != nil {
		fail(w, err)
		return
	}
	b, _ := json.MarshalIndent(entries, "", "  ")
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", "attachment; filename=audit-export.json")
	_, _ = w.Write(b)
}

func (s *Server) verifyAudit(w http.ResponseWriter, r *http.Request) {
	rep, err := s.deps.Audit.Verify(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	_, _ = s.deps.Audit.Write(r.Context(), s.actorOf(r), "audit.verify", "audit_log", "",
		boolResult(rep.OK), map[string]any{"checked": rep.Checked})
	writeJSON(w, 200, map[string]any{"chain": rep})
}

func boolResult(ok bool) string {
	if ok {
		return "success"
	}
	return "failure"
}

// ---------- Settings ----------

func (s *Server) getSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"settings": s.deps.Settings.All()})
}

func (s *Server) putSettings(w http.ResponseWriter, r *http.Request) {
	var req map[string]string
	if err := decodeJSON(r, &req, 0); err != nil {
		writeErr(w, 400, "bad_request", err.Error())
		return
	}
	for k, v := range req {
		if err := s.deps.Settings.Set(r.Context(), k, v); err != nil {
			fail(w, err)
			return
		}
	}
	_, _ = s.deps.Audit.Write(r.Context(), s.actorOf(r), "settings.update", "settings", "", "success",
		map[string]any{"keys": len(req)})
	writeJSON(w, 200, map[string]any{"settings": s.deps.Settings.All()})
}

// ---------- Backup ----------

func (s *Server) exportBackup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Passphrase string `json:"passphrase"`
	}
	if err := decodeJSON(r, &req, 0); err != nil {
		writeErr(w, 400, "bad_request", err.Error())
		return
	}
	blob, err := s.deps.Backup.Export(r.Context(), req.Passphrase)
	if err != nil {
		fail(w, err)
		return
	}
	_, _ = s.deps.Audit.Write(r.Context(), s.actorOf(r), "backup.export", "backup", "", "success",
		map[string]any{"bytes": len(blob)})
	writeJSON(w, 200, map[string]any{
		"backupBase64": base64.StdEncoding.EncodeToString(blob), "bytes": len(blob)})
}

func (s *Server) inspectBackup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		BackupB64  string `json:"backupBase64"`
		Passphrase string `json:"passphrase"`
	}
	if err := decodeJSON(r, &req, 64<<20); err != nil {
		writeErr(w, 400, "bad_request", err.Error())
		return
	}
	blob, err := base64.StdEncoding.DecodeString(req.BackupB64)
	if err != nil {
		writeErr(w, 400, "bad_request", "invalid base64")
		return
	}
	_, rep, err := backup.Inspect(blob, req.Passphrase)
	if err != nil {
		writeErr(w, 422, "backup_invalid", err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"report": rep})
}

func (s *Server) restoreBackup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		BackupB64  string `json:"backupBase64"`
		Passphrase string `json:"passphrase"`
		Confirm    bool   `json:"confirm"`
	}
	if err := decodeJSON(r, &req, 64<<20); err != nil {
		writeErr(w, 400, "bad_request", err.Error())
		return
	}
	if !req.Confirm {
		writeErr(w, 422, "confirm_required", "restore requires confirm=true")
		return
	}
	blob, err := base64.StdEncoding.DecodeString(req.BackupB64)
	if err != nil {
		writeErr(w, 400, "bad_request", "invalid base64")
		return
	}
	b, _, err := backup.Inspect(blob, req.Passphrase)
	if err != nil {
		writeErr(w, 422, "backup_invalid", err.Error())
		return
	}
	safety, err := s.deps.Backup.Restore(r.Context(), b, s.deps.Cfg.ArtifactDir("backups"))
	if err != nil {
		fail(w, err)
		return
	}
	_, _ = s.deps.Audit.Write(r.Context(), s.actorOf(r), "backup.restore", "backup", "", "success",
		map[string]any{"safetyCopy": safety})
	writeJSON(w, 200, map[string]any{"ok": true, "safetyCopy": safety})
}

// ---------- Agents ----------

func (s *Server) listAgents(w http.ResponseWriter, r *http.Request) {
	agents, err := s.deps.Agents.List(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"agents": agents})
}

func (s *Server) enrollAgent(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := decodeJSON(r, &req, 0); err != nil {
		writeErr(w, 400, "bad_request", err.Error())
		return
	}
	id, secret, err := s.deps.Agents.Enroll(r.Context(), req.Name)
	if err != nil {
		fail(w, err)
		return
	}
	_, _ = s.deps.Audit.Write(r.Context(), s.actorOf(r), "agent.enroll", "agent", id, "success", nil)
	// The secret is returned exactly once.
	writeJSON(w, 201, map[string]any{"agentId": id, "enrollSecret": secret})
}

func (s *Server) approveAgent(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.deps.Agents.SetStatus(r.Context(), id, "approved"); err != nil {
		fail(w, err)
		return
	}
	_, _ = s.deps.Audit.Write(r.Context(), s.actorOf(r), "agent.approve", "agent", id, "success", nil)
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) blockAgent(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.deps.Agents.SetStatus(r.Context(), id, "blocked"); err != nil {
		fail(w, err)
		return
	}
	_, _ = s.deps.Audit.Write(r.Context(), s.actorOf(r), "agent.block", "agent", id, "success", nil)
	writeJSON(w, 200, map[string]any{"ok": true})
}
