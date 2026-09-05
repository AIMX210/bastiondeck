package apiclient

import (
	"context"
	"net/url"
)

// ---------- Generic DTOs ----------

// Host mirrors inventory.Host over the wire.
type Host struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Address      string   `json:"address"`
	Port         int      `json:"port"`
	Username     string   `json:"username"`
	CredentialID *string  `json:"credentialId,omitempty"`
	AuthKind     string   `json:"authKind"`
	AgentID      *string  `json:"agentId,omitempty"`
	JumpHostID   *string  `json:"jumpHostId,omitempty"`
	GroupID      *string  `json:"groupId,omitempty"`
	Tags         []string `json:"tags"`
	Notes        string   `json:"notes"`
	Favorite     bool     `json:"favorite"`
	Status       string   `json:"status"`
}

// Credential is the safe projection.
type Credential struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	Fingerprint string `json:"fingerprint,omitempty"`
}

// Run is a job run view.
type Run struct {
	ID      string `json:"id"`
	JobID   string `json:"jobId"`
	Status  string `json:"status"`
	Summary struct {
		Total, Success, Failed, Timeout, Cancelled, Lost int
	} `json:"summary"`
	Targets []RunTarget `json:"targets"`
}

// RunTarget is a per-host result.
type RunTarget struct {
	ID            string `json:"id"`
	HostID        string `json:"hostId"`
	Status        string `json:"status"`
	ExitCode      *int   `json:"exitCode"`
	ErrorText     string `json:"errorText"`
	StdoutPreview string `json:"stdoutPreview"`
	StderrPreview string `json:"stderrPreview"`
}

// Snippet is a reusable command.
type Snippet struct {
	ID    string   `json:"id"`
	Title string   `json:"title"`
	Body  string   `json:"body"`
	Tags  []string `json:"tags"`
}

// AuditEntry is one audit row.
type AuditEntry struct {
	EventID   string `json:"eventId"`
	At        string `json:"at"`
	ActorName string `json:"actorName"`
	Action    string `json:"action"`
	ObjectID  string `json:"objectId"`
	Result    string `json:"result"`
}

// ---------- Auth / status ----------

// StatusResult is the public instance status.
type StatusResult struct {
	Version       string `json:"version"`
	SetupRequired bool   `json:"setupRequired"`
}

// Status returns public instance status.
func (c *Client) Status(ctx context.Context) (*StatusResult, error) {
	var out StatusResult
	if err := c.Get(ctx, "/api/status", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Login exchanges credentials for a bearer token (also usable as the value
// stored in the bdk_session cookie by browsers).
func (c *Client) Login(ctx context.Context, username, password, totp string) (string, *User, error) {
	var out struct {
		User  User   `json:"user"`
		Token string `json:"token"`
	}
	err := c.Post(ctx, "/api/auth/login", map[string]string{
		"username": username, "password": password, "totp": totp}, &out)
	if err != nil {
		return "", nil, err
	}
	return out.Token, &out.User, err
}

// User mirrors auth.User.
type User struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Role     string `json:"role"`
}

// Me returns the current principal.
func (c *Client) Me(ctx context.Context) (map[string]any, error) {
	var out map[string]any
	return out, c.Get(ctx, "/api/auth/me", nil, &out)
}

// ---------- Hosts ----------

// ListHosts returns hosts filtered by query.
func (c *Client) ListHosts(ctx context.Context, q string) ([]Host, error) {
	v := url.Values{}
	if q != "" {
		v.Set("q", q)
	}
	var out struct {
		Hosts []Host `json:"hosts"`
	}
	if err := c.Get(ctx, "/api/hosts", v, &out); err != nil {
		return nil, err
	}
	return out.Hosts, nil
}

// CreateHost adds a host.
func (c *Client) CreateHost(ctx context.Context, body map[string]any) (*Host, error) {
	var out struct {
		Host Host `json:"host"`
	}
	if err := c.Post(ctx, "/api/hosts", body, &out); err != nil {
		return nil, err
	}
	return &out.Host, nil
}

// DeleteHost removes a host.
func (c *Client) DeleteHost(ctx context.Context, id string) error {
	return c.Delete(ctx, "/api/hosts/"+id, nil)
}

// TestHost probes connectivity.
func (c *Client) TestHost(ctx context.Context, id string) (map[string]any, error) {
	var out map[string]any
	return out, c.Post(ctx, "/api/hosts/"+id+"/test", map[string]any{}, &out)
}

// ---------- Credentials ----------

// ListCredentials lists credentials.
func (c *Client) ListCredentials(ctx context.Context) ([]Credential, error) {
	var out struct {
		Credentials []Credential `json:"credentials"`
	}
	if err := c.Get(ctx, "/api/credentials", nil, &out); err != nil {
		return nil, err
	}
	return out.Credentials, nil
}

// CreateCredential creates a credential.
func (c *Client) CreateCredential(ctx context.Context, name, kind, secret string) (*Credential, error) {
	var out struct {
		Credential Credential `json:"credential"`
	}
	if err := c.Post(ctx, "/api/credentials",
		map[string]string{"name": name, "kind": kind, "secret": secret}, &out); err != nil {
		return nil, err
	}
	return &out.Credential, nil
}

// ---------- Execution ----------

// Exec starts an adhoc run and returns the run id.
func (c *Client) Exec(ctx context.Context, command string, targetIDs []string, timeoutSec int) (string, error) {
	var out struct {
		RunID string `json:"runId"`
	}
	body := map[string]any{"command": command, "targetIds": targetIDs, "timeoutSec": timeoutSec}
	if err := c.Post(ctx, "/api/exec", body, &out); err != nil {
		return "", err
	}
	return out.RunID, nil
}

// GetRun fetches a run with targets.
func (c *Client) GetRun(ctx context.Context, id string) (*Run, bool, error) {
	var out struct {
		Run  Run  `json:"run"`
		Live bool `json:"live"`
	}
	if err := c.Get(ctx, "/api/runs/"+id, nil, &out); err != nil {
		return nil, false, err
	}
	return &out.Run, out.Live, nil
}

// CancelRun cancels a run.
func (c *Client) CancelRun(ctx context.Context, id string) error {
	return c.Post(ctx, "/api/runs/"+id+"/cancel", map[string]any{}, nil)
}

// ListRuns pages recent runs.
func (c *Client) ListRuns(ctx context.Context, limit int) ([]Run, error) {
	v := url.Values{}
	if limit > 0 {
		v.Set("limit", itoa(limit))
	}
	var out struct {
		Runs []Run `json:"runs"`
	}
	if err := c.Get(ctx, "/api/runs", v, &out); err != nil {
		return nil, err
	}
	return out.Runs, nil
}

// TargetOutput reads captured output from an offset.
func (c *Client) TargetOutput(ctx context.Context, runID, targetID, stream string, offset int64) (string, int64, error) {
	v := url.Values{"stream": {stream}, "offset": {itoa64(offset)}}
	var out struct {
		Chunk  string `json:"chunk"`
		Offset int64  `json:"offset"`
	}
	if err := c.Get(ctx, "/api/runs/"+runID+"/targets/"+targetID+"/output", v, &out); err != nil {
		return "", offset, err
	}
	return out.Chunk, out.Offset, nil
}

// ---------- Snippets ----------

// ListSnippets returns snippets.
func (c *Client) ListSnippets(ctx context.Context) ([]Snippet, error) {
	var out struct {
		Snippets []Snippet `json:"snippets"`
	}
	if err := c.Get(ctx, "/api/snippets", nil, &out); err != nil {
		return nil, err
	}
	return out.Snippets, nil
}

// ---------- Audit ----------

// ListAudit pages the audit trail.
func (c *Client) ListAudit(ctx context.Context, limit int) ([]AuditEntry, error) {
	v := url.Values{}
	if limit > 0 {
		v.Set("limit", itoa(limit))
	}
	var out struct {
		Entries []AuditEntry `json:"entries"`
	}
	if err := c.Get(ctx, "/api/audit", v, &out); err != nil {
		return nil, err
	}
	return out.Entries, nil
}

// VerifyAudit verifies the hash chain.
func (c *Client) VerifyAudit(ctx context.Context) (map[string]any, error) {
	var out map[string]any
	return out, c.Post(ctx, "/api/audit/verify", map[string]any{}, &out)
}

// ---------- Doctor ----------

// Doctor runs server self-checks.
func (c *Client) Doctor(ctx context.Context) (map[string]any, error) {
	var out map[string]any
	return out, c.Get(ctx, "/api/doctor", nil, &out)
}
