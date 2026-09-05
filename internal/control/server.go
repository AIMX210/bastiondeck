// Package control exposes a local-only control plane over a Unix domain
// socket.
// Access is gated by filesystem permissions on the socket; actions here are
// operator-local (status, doctor, graceful shutdown, key rotation) and never
// reach the network.
package control

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// Hooks are invoked by control endpoints; the daemon supplies concrete funcs.
type Hooks struct {
	Status    func() any
	Doctor    func() any
	Shutdown  func(reason string) error
	RotateKey func(newHex string) error
}

// Server wraps a unix-socket HTTP listener.
type Server struct {
	sock  string
	hooks Hooks
	srv   *http.Server
	ln    net.Listener
}

// New constructs the control server.
func New(sockPath string, hooks Hooks) *Server {
	s := &Server{sock: sockPath, hooks: hooks}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /status", s.handleStatus)
	mux.HandleFunc("GET /doctor", s.handleDoctor)
	mux.HandleFunc("POST /shutdown", s.handleShutdown)
	mux.HandleFunc("POST /rotate-key", s.handleRotate)
	s.srv = &http.Server{Handler: mux}
	return s
}

// Listen removes a stale socket, creates the parent directory with 0o700 and
// binds a socket accessible only to the owner.
func (s *Server) Listen() error {
	if err := os.MkdirAll(filepath.Dir(s.sock), 0o700); err != nil {
		return err
	}
	_ = os.Remove(s.sock)
	ln, err := net.Listen("unix", s.sock)
	if err != nil {
		return err
	}
	if err := os.Chmod(s.sock, 0o600); err != nil {
		_ = ln.Close()
		return err
	}
	s.ln = ln
	return nil
}

// Serve blocks serving the control plane.
func (s *Server) Serve() error {
	if s.ln == nil {
		if err := s.Listen(); err != nil {
			return err
		}
	}
	return s.srv.Serve(s.ln)
}

// Close stops the server and removes the socket.
func (s *Server) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = s.srv.Shutdown(ctx)
	_ = os.Remove(s.sock)
	return nil
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if s.hooks.Status != nil {
		writeJSON(w, s.hooks.Status())
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) handleDoctor(w http.ResponseWriter, r *http.Request) {
	if s.hooks.Doctor != nil {
		writeJSON(w, s.hooks.Doctor())
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) handleShutdown(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Reason string `json:"reason"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	writeJSON(w, map[string]any{"ok": true})
	go func() {
		time.Sleep(200 * time.Millisecond)
		if s.hooks.Shutdown != nil {
			_ = s.hooks.Shutdown(req.Reason)
		}
	}()
}

func (s *Server) handleRotate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		MasterKeyHex string `json:"masterKeyHex"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if s.hooks.RotateKey != nil {
		if err := s.hooks.RotateKey(req.MasterKeyHex); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
	}
	writeJSON(w, map[string]any{"ok": true})
}
