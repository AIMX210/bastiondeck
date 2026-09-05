package httpx

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"bastiondeck/internal/audit"
	"bastiondeck/internal/auth"
	"bastiondeck/internal/bootstrap"
	"bastiondeck/internal/store"
	"bastiondeck/internal/validate"
)

func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	required, _ := s.deps.Bootstrap.SetupRequired(r.Context())
	writeJSON(w, 200, map[string]any{
		"version":       s.deps.Version,
		"setupRequired": required,
	})
}

func (s *Server) healthz(w http.ResponseWriter, r *http.Request) {
	if err := s.deps.Store.DB.PingContext(r.Context()); err != nil {
		writeErr(w, 503, "db_unhealthy", err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "time": store.Now()})
}

type setupReq struct {
	Username    string `json:"username"`
	Password    string `json:"password"`
	DisplayName string `json:"displayName"`
}

func (s *Server) setup(w http.ResponseWriter, r *http.Request) {
	var req setupReq
	if err := decodeJSON(r, &req, 0); err != nil {
		writeErr(w, 400, "bad_request", err.Error())
		return
	}
	u, err := s.deps.Bootstrap.InitialSetup(r.Context(), bootstrap.SetupInput{
		Username: req.Username, Password: req.Password, DisplayName: req.DisplayName}, s.clientIP(r))
	if err != nil {
		fail(w, err)
		return
	}
	token, err := s.deps.Auth.CreateSessionForUser(r.Context(), u.ID, s.clientIP(r), r.UserAgent())
	if err != nil {
		fail(w, err)
		return
	}
	http.SetCookie(w, s.sessionCookie(token))
	writeJSON(w, 200, map[string]any{"user": u})
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
		TOTP     string `json:"totp"`
	}
	if err := decodeJSON(r, &req, 0); err != nil {
		writeErr(w, 400, "bad_request", err.Error())
		return
	}
	out, err := s.deps.Auth.Login(r.Context(), req.Username, req.Password, req.TOTP,
		s.clientIP(r), r.UserAgent())
	if err != nil {
		switch {
		case err == auth.ErrLocked:
			writeErr(w, 429, "rate_limited", err.Error())
		default:
			writeErr(w, 401, "unauthenticated", "invalid credentials")
		}
		return
	}
	_, _ = s.deps.Audit.Write(r.Context(), audit.Actor{ID: out.User.ID, Name: out.User.Username, IP: s.clientIP(r)},
		"auth.login", "user", out.User.ID, "success", nil)
	http.SetCookie(w, s.sessionCookie(out.SessionToken))
	writeJSON(w, 200, map[string]any{"user": out.User, "token": out.SessionToken})
}

func (s *Server) sessionCookie(token string) *http.Cookie {
	return &http.Cookie{
		Name: "bdk_session", Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode,
		MaxAge: int(s.deps.Cfg.SessionTTL.Seconds()),
	}
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if token := sessionToken(r); token != "" {
		_ = s.deps.Auth.Logout(r.Context(), token)
	}
	http.SetCookie(w, &http.Cookie{Name: "bdk_session", Value: "", Path: "/", MaxAge: -1})
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	u, _ := CurrentUser(r)
	writeJSON(w, 200, map[string]any{"user": u, "permissions": permsFor(u.Role)})
}

func permsFor(role string) []string {
	all := []auth.Permission{auth.PermRead, auth.PermExec, auth.PermManageInventory,
		auth.PermAudit, auth.PermManageUsers, auth.PermOwner}
	var out []string
	for _, p := range all {
		if auth.Can(role, p) {
			out = append(out, string(p))
		}
	}
	return out
}

func (s *Server) totpSetup(w http.ResponseWriter, r *http.Request) {
	u, _ := CurrentUser(r)
	secret, uri, err := s.deps.Auth.BeginTOTPEnroll(r.Context(), u.ID)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"secret": secret, "uri": uri})
}

func (s *Server) totpEnable(w http.ResponseWriter, r *http.Request) {
	u, _ := CurrentUser(r)
	var req struct{ Code string }
	if err := decodeJSON(r, &req, 0); err != nil {
		writeErr(w, 400, "bad_request", err.Error())
		return
	}
	if err := s.deps.Auth.EnableTOTP(r.Context(), u.ID, req.Code); err != nil {
		writeErr(w, 400, "totp_invalid", err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) changePassword(w http.ResponseWriter, r *http.Request) {
	u, _ := CurrentUser(r)
	var req struct {
		Old string `json:"oldPassword"`
		New string `json:"newPassword"`
	}
	if err := decodeJSON(r, &req, 0); err != nil {
		writeErr(w, 400, "bad_request", err.Error())
		return
	}
	// Re-authenticate by verifying old password through a throwaway login flow.
	out, err := s.deps.Auth.Login(r.Context(), u.Username, req.Old, "", s.clientIP(r), r.UserAgent())
	if err != nil {
		writeErr(w, 401, "unauthenticated", "current password incorrect")
		return
	}
	_ = out
	if err := s.deps.Auth.SetPassword(r.Context(), u.ID, req.New); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) doctor(w http.ResponseWriter, r *http.Request) {
	rep := validate.Run(r.Context(), validate.Input{
		Store: s.deps.Store, Audit: s.deps.Audit, Cfg: s.deps.Cfg,
		Hub: s.deps.Hub, Version: s.deps.Version,
	})
	writeJSON(w, 200, rep)
}

// sseEvents streams the realtime hub as Server-Sent Events.
func (s *Server) sseEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, 500, "no_sse", "streaming unsupported")
		return
	}
	_, ch, unsub := s.deps.Hub.Subscribe(128)
	defer unsub()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Connection", "keep-alive")
	fmt.Fprintf(w, "retry: 5000\n\n")
	flusher.Flush()
	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-ch:
			if !ok {
				return
			}
			b, _ := json.Marshal(ev)
			_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Type, b)
			flusher.Flush()
		case <-time.After(25 * time.Second):
			_, _ = fmt.Fprint(w, ": keepalive\n\n")
			flusher.Flush()
		}
	}
}
