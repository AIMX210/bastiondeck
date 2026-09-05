package httpx

import (
	"net/http"

	"bastiondeck/internal/audit"
	"bastiondeck/internal/auth"
)

func (s *Server) actorOf(r *http.Request) audit.Actor {
	u, _ := CurrentUser(r)
	a := audit.Actor{IP: s.clientIP(r)}
	if u != nil {
		a.ID = u.ID
		a.Name = u.Username
	}
	return a
}

func (s *Server) listUsers(w http.ResponseWriter, r *http.Request) {
	users, err := s.deps.Auth.ListUsers(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"users": users})
}

func (s *Server) createUser(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username    string `json:"username"`
		DisplayName string `json:"displayName"`
		Role        string `json:"role"`
		Password    string `json:"password"`
	}
	if err := decodeJSON(r, &req, 0); err != nil {
		writeErr(w, 400, "bad_request", err.Error())
		return
	}
	actor, _ := CurrentUser(r)
	// Only owner may mint admin/owner.
	if (req.Role == auth.RoleOwner || req.Role == auth.RoleAdmin) && actor.Role != auth.RoleOwner {
		writeErr(w, 403, "forbidden", "only owner can create admin/owner")
		return
	}
	u, err := s.deps.Auth.CreateUser(r.Context(), auth.CreateUserInput{
		Username: req.Username, DisplayName: req.DisplayName, Role: req.Role, Password: req.Password})
	if err != nil {
		fail(w, err)
		return
	}
	_, _ = s.deps.Audit.Write(r.Context(), s.actorOf(r), "user.create", "user", u.ID, "success", nil)
	writeJSON(w, 201, map[string]any{"user": u})
}

func (s *Server) updateUser(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		Role        *string `json:"role"`
		DisplayName *string `json:"displayName"`
		Disabled    *bool   `json:"disabled"`
	}
	if err := decodeJSON(r, &req, 0); err != nil {
		writeErr(w, 400, "bad_request", err.Error())
		return
	}
	actor, _ := CurrentUser(r)
	if req.Role != nil && (*req.Role == auth.RoleOwner || *req.Role == auth.RoleAdmin) && actor.Role != auth.RoleOwner {
		writeErr(w, 403, "forbidden", "only owner can assign admin/owner")
		return
	}
	if err := s.deps.Auth.UpdateUser(r.Context(), id, auth.UpdateUserFields{
		Role: req.Role, DisplayName: req.DisplayName, Disabled: req.Disabled}); err != nil {
		fail(w, err)
		return
	}
	_, _ = s.deps.Audit.Write(r.Context(), s.actorOf(r), "user.update", "user", id, "success", nil)
	u, _ := s.deps.Auth.GetUserByID(r.Context(), id)
	writeJSON(w, 200, map[string]any{"user": u})
}

func (s *Server) deleteUser(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	actor, _ := CurrentUser(r)
	if id == actor.ID {
		writeErr(w, 409, "conflict", "cannot delete the account you are using")
		return
	}
	if err := s.deps.Auth.DeleteUser(r.Context(), id); err != nil {
		fail(w, err)
		return
	}
	_, _ = s.deps.Audit.Write(r.Context(), s.actorOf(r), "user.delete", "user", id, "success", nil)
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) userSessions(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sess, err := s.deps.Auth.ListSessions(r.Context(), id, "")
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"sessions": sess})
}

func (s *Server) mySessions(w http.ResponseWriter, r *http.Request) {
	u, digest := CurrentUser(r)
	sess, err := s.deps.Auth.ListSessions(r.Context(), u.ID, digest)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"sessions": sess})
}

func (s *Server) revokeSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.deps.Auth.Revoke(r.Context(), id, "manual"); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) revokeAllSessions(w http.ResponseWriter, r *http.Request) {
	u, digest := CurrentUser(r)
	n, err := s.deps.Auth.RevokeOthers(r.Context(), u.ID, digest)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"revoked": n})
}
