package httpx

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/http"

	"bastiondeck/internal/connector"
)

// connectFS resolves a host and opens its remote filesystem.
func (s *Server) connectFS(w http.ResponseWriter, r *http.Request, hostID string) (connector.Client, connector.FS, bool) {
	cli, err := s.deps.Connector.Connect(r.Context(), hostID)
	if err != nil {
		writeErr(w, 502, "conn_lost", err.Error())
		return nil, nil, false
	}
	fs, err := cli.FS()
	if err != nil {
		_ = cli.Close()
		writeErr(w, 502, "conn_lost", err.Error())
		return nil, nil, false
	}
	return cli, fs, true
}

func (s *Server) fsList(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	cli, fs, ok := s.connectFS(w, r, q.Get("hostId"))
	if !ok {
		return
	}
	defer cli.Close()
	entries, err := fs.List(r.Context(), q.Get("path"))
	if err != nil {
		writeErr(w, 502, "fs_error", err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"path": q.Get("path"), "entries": entries})
}

func (s *Server) fsStat(w http.ResponseWriter, r *http.Request) {
	var req struct {
		HostID string `json:"hostId"`
		Path   string `json:"path"`
	}
	if err := decodeJSON(r, &req, 0); err != nil {
		writeErr(w, 400, "bad_request", err.Error())
		return
	}
	cli, fs, ok := s.connectFS(w, r, req.HostID)
	if !ok {
		return
	}
	defer cli.Close()
	st, err := fs.Stat(r.Context(), req.Path)
	if err != nil {
		writeErr(w, 502, "fs_error", err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"stat": st})
}

func (s *Server) fsRead(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit := int64(atoiDefault(q.Get("limit"), 1_000_000))
	cli, fs, ok := s.connectFS(w, r, q.Get("hostId"))
	if !ok {
		return
	}
	defer cli.Close()
	b, err := fs.Read(r.Context(), q.Get("path"), limit)
	if err != nil {
		writeErr(w, 502, "fs_error", err.Error())
		return
	}
	// sha256 of the returned bytes feeds the optimistic-lock ExpectedSha on
	// write. Files larger than the read limit are treated as read-only by the
	// UI, so within the editable range this equals the whole-file digest.
	sum := sha256.Sum256(b)
	writeJSON(w, 200, map[string]any{
		"path": q.Get("path"), "size": len(b),
		"sha256":        hex.EncodeToString(sum[:]),
		"contentBase64": base64.StdEncoding.EncodeToString(b)})
}

func (s *Server) fsWrite(w http.ResponseWriter, r *http.Request) {
	var req struct {
		HostID      string `json:"hostId"`
		Path        string `json:"path"`
		ContentB64  string `json:"contentBase64"`
		ExpectedSHA string `json:"expectedSha"`
	}
	if err := decodeJSON(r, &req, 16<<20); err != nil {
		writeErr(w, 400, "bad_request", err.Error())
		return
	}
	content, err := base64.StdEncoding.DecodeString(req.ContentB64)
	if err != nil {
		writeErr(w, 400, "bad_request", "invalid base64")
		return
	}
	cli, fs, ok := s.connectFS(w, r, req.HostID)
	if !ok {
		return
	}
	defer cli.Close()
	newSHA, err := fs.Write(r.Context(), req.Path, content, req.ExpectedSHA)
	if err != nil {
		writeErr(w, 409, "modified", err.Error())
		return
	}
	_, _ = s.deps.Audit.Write(r.Context(), s.actorOf(r), "fs.write", "host", req.HostID, "success",
		map[string]any{"path": req.Path, "bytes": len(content)})
	writeJSON(w, 200, map[string]any{"sha256": newSHA})
}

func (s *Server) fsMkdir(w http.ResponseWriter, r *http.Request) {
	var req struct {
		HostID string `json:"hostId"`
		Path   string `json:"path"`
	}
	if err := decodeJSON(r, &req, 0); err != nil {
		writeErr(w, 400, "bad_request", err.Error())
		return
	}
	cli, fs, ok := s.connectFS(w, r, req.HostID)
	if !ok {
		return
	}
	defer cli.Close()
	if err := fs.Mkdir(r.Context(), req.Path); err != nil {
		writeErr(w, 502, "fs_error", err.Error())
		return
	}
	_, _ = s.deps.Audit.Write(r.Context(), s.actorOf(r), "fs.mkdir", "host", req.HostID, "success",
		map[string]any{"path": req.Path})
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) fsRename(w http.ResponseWriter, r *http.Request) {
	var req struct {
		HostID string `json:"hostId"`
		From   string `json:"from"`
		To     string `json:"to"`
	}
	if err := decodeJSON(r, &req, 0); err != nil {
		writeErr(w, 400, "bad_request", err.Error())
		return
	}
	cli, fs, ok := s.connectFS(w, r, req.HostID)
	if !ok {
		return
	}
	defer cli.Close()
	if err := fs.Rename(r.Context(), req.From, req.To); err != nil {
		writeErr(w, 502, "fs_error", err.Error())
		return
	}
	_, _ = s.deps.Audit.Write(r.Context(), s.actorOf(r), "fs.rename", "host", req.HostID, "success",
		map[string]any{"from": req.From, "to": req.To})
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) fsDelete(w http.ResponseWriter, r *http.Request) {
	var req struct {
		HostID string `json:"hostId"`
		Path   string `json:"path"`
	}
	if err := decodeJSON(r, &req, 0); err != nil {
		writeErr(w, 400, "bad_request", err.Error())
		return
	}
	cli, fs, ok := s.connectFS(w, r, req.HostID)
	if !ok {
		return
	}
	defer cli.Close()
	if err := fs.Remove(r.Context(), req.Path); err != nil {
		writeErr(w, 502, "fs_error", err.Error())
		return
	}
	_, _ = s.deps.Audit.Write(r.Context(), s.actorOf(r), "fs.delete", "host", req.HostID, "success",
		map[string]any{"path": req.Path})
	writeJSON(w, 200, map[string]any{"ok": true})
}
