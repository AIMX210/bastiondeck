package httpx

import (
	"net/http"
	"strings"

	"bastiondeck/internal/inventory"
)

func (s *Server) listHosts(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	hosts, err := s.deps.Hosts.List(r.Context(), inventory.HostFilter{
		Query: q.Get("q"), Tag: q.Get("tag"), GroupID: q.Get("groupId"),
		Status: q.Get("status"), FavoritesOnly: q.Get("favorite") == "true"})
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"hosts": hosts})
}

type hostReq struct {
	Name         string            `json:"name"`
	Address      string            `json:"address"`
	Port         int               `json:"port"`
	Username     string            `json:"username"`
	CredentialID string            `json:"credentialId"`
	AuthKind     string            `json:"authKind"`
	AgentID      string            `json:"agentId"`
	JumpHostID   string            `json:"jumpHostId"`
	GroupID      string            `json:"groupId"`
	Tags         []string          `json:"tags"`
	Notes        string            `json:"notes"`
	Favorite     bool              `json:"favorite"`
	Options      map[string]string `json:"options"`
}

func (h hostReq) input() inventory.HostInput {
	return inventory.HostInput{
		Name: h.Name, Address: h.Address, Port: h.Port, Username: h.Username,
		CredentialID: h.CredentialID, AuthKind: h.AuthKind, AgentID: h.AgentID,
		JumpHostID: h.JumpHostID, GroupID: h.GroupID, Tags: h.Tags, Notes: h.Notes,
		Favorite: h.Favorite, Options: h.Options,
	}
}

func (s *Server) createHost(w http.ResponseWriter, r *http.Request) {
	var req hostReq
	if err := decodeJSON(r, &req, 0); err != nil {
		writeErr(w, 400, "bad_request", err.Error())
		return
	}
	h, err := s.deps.Hosts.Create(r.Context(), req.input())
	if err != nil {
		fail(w, err)
		return
	}
	_, _ = s.deps.Audit.Write(r.Context(), s.actorOf(r), "host.create", "host", h.ID, "success",
		map[string]any{"address": h.Address})
	writeJSON(w, 201, map[string]any{"host": h})
}

func (s *Server) getHost(w http.ResponseWriter, r *http.Request) {
	h, err := s.deps.Hosts.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		writeErr(w, 404, "not_found", err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"host": h})
}

func (s *Server) updateHost(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req hostReq
	if err := decodeJSON(r, &req, 0); err != nil {
		writeErr(w, 400, "bad_request", err.Error())
		return
	}
	h, err := s.deps.Hosts.Update(r.Context(), id, req.input())
	if err != nil {
		fail(w, err)
		return
	}
	_, _ = s.deps.Audit.Write(r.Context(), s.actorOf(r), "host.update", "host", id, "success", nil)
	writeJSON(w, 200, map[string]any{"host": h})
}

func (s *Server) deleteHost(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.deps.Hosts.Delete(r.Context(), id); err != nil {
		switch err {
		case inventory.ErrIsJumpHost:
			writeErr(w, 409, "is_jump_host", err.Error())
		default:
			fail(w, err)
		}
		return
	}
	_, _ = s.deps.Audit.Write(r.Context(), s.actorOf(r), "host.delete", "host", id, "success", nil)
	writeJSON(w, 200, map[string]any{"ok": true})
}

// testHost performs a real connect+exec probe and reports the outcome.
func (s *Server) testHost(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	cli, err := s.deps.Connector.Connect(r.Context(), id)
	if err != nil {
		writeErr(w, 502, "conn_lost", err.Error())
		return
	}
	defer cli.Close()
	res, err := cli.Exec(r.Context(), execReq("true", 8))
	if err != nil {
		writeErr(w, 502, "conn_lost", err.Error())
		return
	}
	kt, fp, _ := cli.Fingerprint()
	writeJSON(w, 200, map[string]any{
		"ok": res.Status == "success", "status": res.Status, "exitCode": res.ExitCode,
		"keyType": kt, "fingerprint": fp, "durationMs": res.DurationMs,
	})
}

func (s *Server) resetHostKey(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.deps.Hosts.ResetHostKey(r.Context(), id); err != nil {
		fail(w, err)
		return
	}
	_, _ = s.deps.Audit.Write(r.Context(), s.actorOf(r), "host.reset_key", "host", id, "success", nil)
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) hostFacts(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	cli, err := s.deps.Connector.Connect(r.Context(), id)
	if err != nil {
		writeErr(w, 502, "conn_lost", err.Error())
		return
	}
	defer cli.Close()
	facts, err := cli.Facts(r.Context())
	if err != nil {
		writeErr(w, 502, "conn_lost", err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"facts": facts})
}

func (s *Server) importSSHConfig(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Text         string `json:"text"`
		CredentialID string `json:"credentialId"`
		DefaultUser  string `json:"defaultUser"`
	}
	if err := decodeJSON(r, &req, 0); err != nil {
		writeErr(w, 400, "bad_request", err.Error())
		return
	}
	cands, err := inventory.ParseSSHConfig(strings.NewReader(req.Text))
	if err != nil {
		writeErr(w, 400, "bad_request", err.Error())
		return
	}
	type created struct {
		Candidate inventory.SSHConfigCandidate `json:"candidate"`
		HostID    string                       `json:"hostId,omitempty"`
		Error     string                       `json:"error,omitempty"`
	}
	var out []created
	for _, c := range cands {
		user := c.User
		if user == "" {
			user = req.DefaultUser
		}
		h, err := s.deps.Hosts.Create(r.Context(), inventory.HostInput{
			Name: c.Host, Address: c.HostName, Port: c.Port, Username: user,
			CredentialID: req.CredentialID, AuthKind: "credential"})
		row := created{Candidate: c}
		if err != nil {
			row.Error = err.Error()
		} else {
			row.HostID = h.ID
		}
		out = append(out, row)
	}
	writeJSON(w, 200, map[string]any{"imported": out, "count": len(out)})
}

func (s *Server) listGroups(w http.ResponseWriter, r *http.Request) {
	g, err := s.deps.Hosts.ListGroups(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"groups": g})
}

func (s *Server) createGroup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name     string `json:"name"`
		ParentID string `json:"parentId"`
	}
	if err := decodeJSON(r, &req, 0); err != nil {
		writeErr(w, 400, "bad_request", err.Error())
		return
	}
	g, err := s.deps.Hosts.CreateGroup(r.Context(), req.Name, req.ParentID)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 201, map[string]any{"group": g})
}

func (s *Server) updateGroup(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		Name string `json:"name"`
	}
	if err := decodeJSON(r, &req, 0); err != nil {
		writeErr(w, 400, "bad_request", err.Error())
		return
	}
	if err := s.deps.Hosts.RenameGroup(r.Context(), id, req.Name); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) deleteGroup(w http.ResponseWriter, r *http.Request) {
	if err := s.deps.Hosts.DeleteGroup(r.Context(), r.PathValue("id")); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}
