package httpx

import (
	"context"
	"net"
	"net/http"
	"strings"
	"time"

	"bastiondeck/internal/auth"
)

type ctxKey int

const userKey ctxKey = 1

// withUser stores the authenticated user + session digest on the request.
func withUser(r *http.Request, u *auth.User, sessionDigest string) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), userKey, [2]any{u, sessionDigest}))
}

// CurrentUser returns the authenticated user (nil when anonymous).
func CurrentUser(r *http.Request) (*auth.User, string) {
	v := r.Context().Value(userKey)
	if v == nil {
		return nil, ""
	}
	pair := v.([2]any)
	return pair[0].(*auth.User), pair[1].(string)
}

// securityHeaders enforces a locked-down, offline-capable CSP.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Content-Security-Policy",
			"default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		next.ServeHTTP(w, r)
	})
}

// clientIP extracts the peer IP, honouring TrustProxy only when configured.
func (s *Server) clientIP(r *http.Request) string {
	if s.deps.Cfg.TrustProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			return strings.TrimSpace(strings.Split(xff, ",")[0])
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// authenticate resolves the session cookie or bearer token.
func (s *Server) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := sessionToken(r)
		if token == "" {
			next.ServeHTTP(w, r)
			return
		}
		u, sess, err := s.deps.Auth.Resolve(r.Context(), token)
		if err != nil {
			// Expired/invalid token: clear cookie and continue anonymous.
			http.SetCookie(w, &http.Cookie{Name: "bdk_session", Value: "", Path: "/", MaxAge: -1})
			next.ServeHTTP(w, r)
			return
		}
		next.ServeHTTP(w, withUser(r, u, sess.ID))
	})
}

func sessionToken(r *http.Request) string {
	if c, err := r.Cookie("bdk_session"); err == nil && c.Value != "" {
		return c.Value
	}
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return strings.TrimPrefix(h, "Bearer ")
	}
	return ""
}

// requireRole rejects requests below the required role.
func (s *Server) requireRole(required string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, _ := CurrentUser(r)
		if u == nil {
			writeErr(w, 401, "unauthenticated", "authentication required")
			return
		}
		if !auth.AtLeast(u.Role, required) {
			writeErr(w, 403, "forbidden", "role "+required+" required")
			return
		}
		next(w, r)
	}
}

// requirePerm guards by permission category.
func (s *Server) requirePerm(p auth.Permission, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, _ := CurrentUser(r)
		if u == nil {
			writeErr(w, 401, "unauthenticated", "authentication required")
			return
		}
		if !auth.Can(u.Role, p) {
			writeErr(w, 403, "forbidden", "missing permission "+string(p))
			return
		}
		next(w, r)
	}
}

// csrfGuard enforces the custom-header rule for cookie-authenticated browser
// writes. Bearer clients (CLI/TUI/SDK) are exempt because browsers cannot set
// custom headers cross-site but native clients can.
func csrfGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)
			return
		}
		_, isBearer := bearer(r)
		if !isBearer && r.Header.Get("X-BDK-CSRF") == "" {
			writeErr(w, 403, "csrf_required", "missing X-BDK-CSRF header")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func bearer(r *http.Request) (string, bool) {
	h := r.Header.Get("Authorization")
	if strings.HasPrefix(h, "Bearer ") {
		return strings.TrimPrefix(h, "Bearer "), true
	}
	return "", false
}

// recoverMiddleware turns panics into 500s without leaking internals.
func recoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				writeErr(w, 500, "internal", "internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// noCache marks dynamic API responses.
func noCache(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

var _ = time.Second
