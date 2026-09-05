package httpx

import (
	"net/http"
	"time"

	"bastiondeck/internal/agentconn"
	"bastiondeck/internal/audit"
	"bastiondeck/internal/auth"
	"bastiondeck/internal/backup"
	"bastiondeck/internal/bootstrap"
	"bastiondeck/internal/config"
	"bastiondeck/internal/connector"
	"bastiondeck/internal/credentials"
	"bastiondeck/internal/inventory"
	"bastiondeck/internal/jobs"
	"bastiondeck/internal/metricsx"
	"bastiondeck/internal/realtime"
	"bastiondeck/internal/settings"
	"bastiondeck/internal/snippets"
	"bastiondeck/internal/store"
	"bastiondeck/internal/tunnel"
)

// Deps wires every service consumed by HTTP handlers.
type Deps struct {
	Cfg   *config.Config
	Store *store.Store
	Vault interface {
		SealString(string, string) ([]byte, error)
	}
	Auth      *auth.Service
	Audit     *audit.Service
	Bootstrap *bootstrap.Service
	Creds     *credentials.Service
	Hosts     *inventory.Repo
	Snippets  *snippets.Service
	Jobs      *jobs.Engine
	JobRepo   *jobs.Repo
	Tunnels   *tunnel.Manager
	Metrics   *metricsx.Collector
	Hub       *realtime.Hub
	Connector *connector.Manager
	Scheduler *jobs.Scheduler
	Agents    *agentconn.Registry
	Backup    *backup.Service
	Settings  *settings.Service
	Version   string
}

// Server is the assembled HTTP application.
type Server struct {
	deps Deps
	mux  *http.ServeMux
}

// New builds the router and registers every route.
func New(d Deps) *Server {
	s := &Server{deps: d, mux: http.NewServeMux()}
	s.routes()
	return s
}

// Handler returns the fully wrapped root handler.
func (s *Server) Handler() http.Handler {
	chain := func(h http.Handler) http.Handler {
		return recoverMiddleware(securityHeaders(noCache(s.authenticate(csrfGuard(h)))))
	}
	return chain(s.mux)
}

// ServerConfig returns an http.Server with safe timeouts.
func (s *Server) ServerConfig(addr string) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           s.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       0, // terminals are long-lived
		WriteTimeout:      0,
		IdleTimeout:       2 * time.Minute,
	}
}
