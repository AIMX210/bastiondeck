package httpx

import (
	"bastiondeck/internal/auth"
)

// routes registers every endpoint. Go 1.22 method-pattern routing keeps the
// table declarative.
func (s *Server) routes() {
	m := s.mux

	// Public / bootstrap.
	m.HandleFunc("GET /api/status", s.status)
	m.HandleFunc("GET /api/healthz", s.healthz)
	m.HandleFunc("POST /api/setup", s.setup)
	m.HandleFunc("POST /api/auth/login", s.login)
	m.HandleFunc("POST /api/auth/logout", s.logout)
	m.HandleFunc("GET /api/auth/me", s.requirePerm(auth.PermRead, s.me))
	m.HandleFunc("POST /api/auth/totp/setup", s.requirePerm(auth.PermRead, s.totpSetup))
	m.HandleFunc("POST /api/auth/totp/enable", s.requirePerm(auth.PermRead, s.totpEnable))
	m.HandleFunc("POST /api/auth/password", s.requirePerm(auth.PermRead, s.changePassword))

	// Users & sessions (admin/owner).
	m.HandleFunc("GET /api/users", s.requirePerm(auth.PermManageUsers, s.listUsers))
	m.HandleFunc("POST /api/users", s.requirePerm(auth.PermManageUsers, s.createUser))
	m.HandleFunc("PATCH /api/users/{id}", s.requirePerm(auth.PermManageUsers, s.updateUser))
	m.HandleFunc("DELETE /api/users/{id}", s.requirePerm(auth.PermManageUsers, s.deleteUser))
	m.HandleFunc("GET /api/users/{id}/sessions", s.requirePerm(auth.PermManageUsers, s.userSessions))
	m.HandleFunc("GET /api/sessions", s.requirePerm(auth.PermRead, s.mySessions))
	m.HandleFunc("POST /api/sessions/revoke-all", s.requirePerm(auth.PermRead, s.revokeAllSessions))
	m.HandleFunc("DELETE /api/sessions/{id}", s.requirePerm(auth.PermRead, s.revokeSession))

	// Credentials.
	m.HandleFunc("GET /api/credentials", s.requirePerm(auth.PermExec, s.listCredentials))
	m.HandleFunc("POST /api/credentials", s.requirePerm(auth.PermManageInventory, s.createCredential))
	m.HandleFunc("PATCH /api/credentials/{id}", s.requirePerm(auth.PermManageInventory, s.updateCredential))
	m.HandleFunc("DELETE /api/credentials/{id}", s.requirePerm(auth.PermManageInventory, s.deleteCredential))

	// Hosts & groups.
	m.HandleFunc("GET /api/hosts", s.requirePerm(auth.PermRead, s.listHosts))
	m.HandleFunc("POST /api/hosts", s.requirePerm(auth.PermManageInventory, s.createHost))
	m.HandleFunc("GET /api/hosts/{id}", s.requirePerm(auth.PermRead, s.getHost))
	m.HandleFunc("PATCH /api/hosts/{id}", s.requirePerm(auth.PermManageInventory, s.updateHost))
	m.HandleFunc("DELETE /api/hosts/{id}", s.requirePerm(auth.PermManageInventory, s.deleteHost))
	m.HandleFunc("POST /api/hosts/{id}/test", s.requirePerm(auth.PermExec, s.testHost))
	m.HandleFunc("POST /api/hosts/{id}/reset-host-key", s.requirePerm(auth.PermManageInventory, s.resetHostKey))
	m.HandleFunc("POST /api/hosts/{id}/facts", s.requirePerm(auth.PermExec, s.hostFacts))
	m.HandleFunc("POST /api/hosts/import-sshconfig", s.requirePerm(auth.PermManageInventory, s.importSSHConfig))
	m.HandleFunc("GET /api/groups", s.requirePerm(auth.PermRead, s.listGroups))
	m.HandleFunc("POST /api/groups", s.requirePerm(auth.PermManageInventory, s.createGroup))
	m.HandleFunc("PATCH /api/groups/{id}", s.requirePerm(auth.PermManageInventory, s.updateGroup))
	m.HandleFunc("DELETE /api/groups/{id}", s.requirePerm(auth.PermManageInventory, s.deleteGroup))

	// Execution & jobs.
	m.HandleFunc("POST /api/exec", s.requirePerm(auth.PermExec, s.execOnce))
	m.HandleFunc("POST /api/jobs/run", s.requirePerm(auth.PermExec, s.runJob))
	m.HandleFunc("GET /api/jobs", s.requirePerm(auth.PermRead, s.listJobs))
	m.HandleFunc("POST /api/jobs", s.requirePerm(auth.PermExec, s.createJob))
	m.HandleFunc("PATCH /api/jobs/{id}", s.requirePerm(auth.PermExec, s.updateJob))
	m.HandleFunc("DELETE /api/jobs/{id}", s.requirePerm(auth.PermExec, s.deleteJob))
	m.HandleFunc("GET /api/runs", s.requirePerm(auth.PermRead, s.listRuns))
	m.HandleFunc("GET /api/runs/{id}", s.requirePerm(auth.PermRead, s.getRun))
	m.HandleFunc("GET /api/runs/{id}/targets/{tid}/output", s.requirePerm(auth.PermRead, s.runOutput))
	m.HandleFunc("POST /api/runs/{id}/cancel", s.requirePerm(auth.PermExec, s.cancelRun))
	m.HandleFunc("POST /api/runs/{id}/retry-failed", s.requirePerm(auth.PermExec, s.retryRun))

	// Terminal / FS / tunnels.
	m.HandleFunc("GET /ws/term", s.requirePerm(auth.PermExec, s.terminalWS))
	m.HandleFunc("GET /api/tunnels", s.requirePerm(auth.PermRead, s.listTunnels))
	m.HandleFunc("POST /api/tunnels", s.requirePerm(auth.PermExec, s.createTunnel))
	m.HandleFunc("POST /api/tunnels/{id}/stop", s.requirePerm(auth.PermExec, s.stopTunnel))
	m.HandleFunc("GET /api/fs/list", s.requirePerm(auth.PermExec, s.fsList))
	m.HandleFunc("GET /api/fs/read", s.requirePerm(auth.PermExec, s.fsRead))
	m.HandleFunc("POST /api/fs/write", s.requirePerm(auth.PermExec, s.fsWrite))
	m.HandleFunc("POST /api/fs/stat", s.requirePerm(auth.PermExec, s.fsStat))
	m.HandleFunc("POST /api/fs/mkdir", s.requirePerm(auth.PermExec, s.fsMkdir))
	m.HandleFunc("POST /api/fs/rename", s.requirePerm(auth.PermExec, s.fsRename))
	m.HandleFunc("POST /api/fs/delete", s.requirePerm(auth.PermExec, s.fsDelete))

	// Snippets.
	m.HandleFunc("GET /api/snippets", s.requirePerm(auth.PermRead, s.listSnippets))
	m.HandleFunc("POST /api/snippets", s.requirePerm(auth.PermManageInventory, s.createSnippet))
	m.HandleFunc("PATCH /api/snippets/{id}", s.requirePerm(auth.PermManageInventory, s.updateSnippet))
	m.HandleFunc("DELETE /api/snippets/{id}", s.requirePerm(auth.PermManageInventory, s.deleteSnippet))
	m.HandleFunc("POST /api/snippets/render", s.requirePerm(auth.PermRead, s.renderSnippet))

	// Metrics, audit, settings, backup, agents, doctor, events.
	m.HandleFunc("GET /api/metrics/hosts/{id}", s.requirePerm(auth.PermRead, s.hostMetrics))
	m.HandleFunc("GET /api/audit", s.requirePerm(auth.PermAudit, s.listAudit))
	m.HandleFunc("GET /api/audit/export", s.requirePerm(auth.PermAudit, s.exportAudit))
	m.HandleFunc("POST /api/audit/verify", s.requirePerm(auth.PermAudit, s.verifyAudit))
	m.HandleFunc("GET /api/settings", s.requirePerm(auth.PermRead, s.getSettings))
	m.HandleFunc("PUT /api/settings", s.requirePerm(auth.PermManageInventory, s.putSettings))
	m.HandleFunc("POST /api/backup/export", s.requirePerm(auth.PermOwner, s.exportBackup))
	m.HandleFunc("POST /api/backup/inspect", s.requirePerm(auth.PermOwner, s.inspectBackup))
	m.HandleFunc("POST /api/backup/restore", s.requirePerm(auth.PermOwner, s.restoreBackup))
	m.HandleFunc("GET /api/agents", s.requirePerm(auth.PermManageInventory, s.listAgents))
	m.HandleFunc("POST /api/agents/enroll", s.requirePerm(auth.PermManageInventory, s.enrollAgent))
	m.HandleFunc("POST /api/agents/{id}/approve", s.requirePerm(auth.PermManageInventory, s.approveAgent))
	m.HandleFunc("POST /api/agents/{id}/block", s.requirePerm(auth.PermManageInventory, s.blockAgent))
	m.HandleFunc("GET /api/doctor", s.requirePerm(auth.PermExec, s.doctor))
	m.HandleFunc("GET /api/events", s.requirePerm(auth.PermRead, s.sseEvents))

	// Agent reverse-connection endpoint: authenticated by enrollment secret,
	// not by the web session, so it is intentionally outside requirePerm.
	if s.deps.Agents != nil {
		m.HandleFunc("GET /agent/connect", s.deps.Agents.ServeWS)
	}

	// Static SPA (registered last; API 404 handled by mux default).
	m.HandleFunc("/", s.spa)
}
