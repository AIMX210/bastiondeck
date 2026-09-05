package httpx_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"bastiondeck/internal/agentconn"
	"bastiondeck/internal/audit"
	"bastiondeck/internal/auth"
	"bastiondeck/internal/backup"
	"bastiondeck/internal/bootstrap"
	"bastiondeck/internal/config"
	"bastiondeck/internal/connector"
	"bastiondeck/internal/credentials"
	"bastiondeck/internal/httpx"
	"bastiondeck/internal/inventory"
	"bastiondeck/internal/jobs"
	"bastiondeck/internal/metricsx"
	"bastiondeck/internal/realtime"
	"bastiondeck/internal/settings"
	"bastiondeck/internal/snippets"
	"bastiondeck/internal/sshlite"
	"bastiondeck/internal/testutil"
	"bastiondeck/internal/tunnel"
)

type testServer struct {
	*httptest.Server
	h *testutil.Harness
}

func newTestServer(t *testing.T, fakeAddr string, fakePort int, exec func(string) ([]byte, []byte, int)) *testServer {
	h := testutil.NewHarness(t)
	cfg := config.Defaults()
	cfg.DataDir = h.DataDir
	logs := audit.New(h.Store.DB)
	v := h.Vault
	authSvc := auth.NewService(h.Store.DB, v, time.Hour)
	boot := bootstrap.New(authSvc, logs)
	creds := credentials.New(h.Store.DB, v)
	hosts := inventory.NewRepo(h.Store.DB)
	snips := snippets.New(h.Store.DB)
	hub := realtime.NewHub()
	agents := agentconn.New(h.Store.DB)
	dialer := &sshlite.Dialer{Hosts: hosts, Creds: creds, DialTimeout: 3 * time.Second}
	pool := sshlite.NewPool(dialer, time.Minute)
	t.Cleanup(pool.CloseAll)
	mgr := &connector.Manager{Hosts: hosts, SSH: pool}
	jr := jobs.NewRepo(h.Store.DB)
	eng := jobs.NewEngine(jr, h.Store.DB, mgr, hub, logs, h.DataDir, 1<<20)
	sched := jobs.NewScheduler(eng, jr)
	srv := httpx.New(httpx.Deps{
		Cfg: cfg, Store: h.Store, Vault: v, Auth: authSvc, Audit: logs, Bootstrap: boot,
		Creds: creds, Hosts: hosts, Snippets: snips, Jobs: eng, JobRepo: jr,
		Tunnels: tunnel.New(h.Store.DB, pool), Metrics: metricsx.New(h.Store.DB, mgr), Hub: hub,
		Connector: mgr, Scheduler: sched, Agents: agents,
		Backup: backup.New(h.Store.DB, h.Store.Path), Settings: settings.New(h.Store.DB), Version: "test",
	})
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return &testServer{Server: ts, h: h}
}

func (ts *testServer) do(t *testing.T, method, path string, cookie *http.Cookie, body any) (int, map[string]any) {
	t.Helper()
	var rdr *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	} else {
		rdr = bytes.NewReader(nil)
	}
	req, _ := http.NewRequest(method, ts.URL+path, rdr)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-BDK-CSRF", "1")
	if cookie != nil {
		req.AddCookie(cookie)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func setupOwner(t *testing.T, ts *testServer) *http.Cookie {
	t.Helper()
	rec := httptest.NewRecorder()
	body, _ := json.Marshal(map[string]string{"username": "root", "password": "Sup3rSecret!"})
	req, _ := http.NewRequest("POST", "/api/setup", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-BDK-CSRF", "1")
	ts.Server.Config.Handler.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("setup status %d body %s", rec.Code, rec.Body.String())
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == "bdk_session" {
			return c
		}
	}
	t.Fatal("no session cookie after setup")
	return nil
}

func TestFullFlowSetupToExec(t *testing.T) {
	srv := testutil.NewFakeSSH(t, "pw", "", func(string) ([]byte, []byte, int) {
		return []byte("hello\n"), nil, 0
	})
	defer srv.Close()
	addr, port := srv.Addr()
	ts := newTestServer(t, addr, port, nil)

	cookie := setupOwner(t, ts)

	// status reports setup complete.
	st, body := ts.do(t, "GET", "/api/status", cookie, nil)
	if st != 200 || body["data"].(map[string]any)["setupRequired"].(bool) {
		t.Fatalf("status wrong: %v", body)
	}

	// create credential.
	st, body = ts.do(t, "POST", "/api/credentials", cookie,
		map[string]string{"name": "c1", "kind": "password", "secret": "pw"})
	if st != 201 {
		t.Fatalf("credential create %d %v", st, body)
	}
	credID := body["data"].(map[string]any)["credential"].(map[string]any)["id"].(string)

	// create host.
	st, body = ts.do(t, "POST", "/api/hosts", cookie, map[string]any{
		"name": "h1", "address": addr, "port": port, "username": "tester",
		"credentialId": credID, "authKind": "credential"})
	if st != 201 {
		t.Fatalf("host create %d %v", st, body)
	}
	hostID := body["data"].(map[string]any)["host"].(map[string]any)["id"].(string)

	// test host.
	st, body = ts.do(t, "POST", "/api/hosts/"+hostID+"/test", cookie, map[string]any{})
	if st != 200 || body["data"].(map[string]any)["ok"] != true {
		t.Fatalf("host test %d %v", st, body)
	}

	// exec and poll to success.
	st, body = ts.do(t, "POST", "/api/exec", cookie, map[string]any{
		"command": "echo hello", "targetIds": []string{hostID}, "timeoutSec": 5})
	if st != 202 {
		t.Fatalf("exec %d %v", st, body)
	}
	runID := body["data"].(map[string]any)["runId"].(string)

	deadline := time.Now().Add(6 * time.Second)
	var final map[string]any
	for time.Now().Before(deadline) {
		st, body = ts.do(t, "GET", "/api/runs/"+runID, cookie, nil)
		if st != 200 {
		t.Fatalf("get run %d", st)
		}
		run := body["data"].(map[string]any)["run"].(map[string]any)
		status := run["status"].(string)
		if status == "success" {
			final = run
			break
		}
		if status == "failed" || status == "lost" {
			t.Fatalf("run ended %s: %v", status, run)
		}
		time.Sleep(40 * time.Millisecond)
	}
	if final == nil {
		t.Fatal("run never succeeded")
	}
}

// Ad-hoc (non-snippet) commands must still have ${var} placeholders rendered;
// an unfilled variable is rejected with 422 rather than sent literally.
func TestAdhocCommandRendersVars(t *testing.T) {
	seen := make(chan string, 4)
	srv := testutil.NewFakeSSH(t, "pw", "", func(cmd string) ([]byte, []byte, int) {
		seen <- cmd
		return []byte("ok\n"), nil, 0
	})
	defer srv.Close()
	addr, port := srv.Addr()
	ts := newTestServer(t, addr, port, nil)
	cookie := setupOwner(t, ts)

	// Missing variable -> 422 before any target is contacted.
	st, body := ts.do(t, "POST", "/api/exec", cookie, map[string]any{
		"command": "echo ${who}", "targetIds": []string{}, "timeoutSec": 5})
	if st != 422 || body["error"].(map[string]any)["code"] != "missing_vars" {
		t.Fatalf("missing-var exec = %d %v", st, body)
	}

	// Set up a host for the filled-variable path.
	st, body = ts.do(t, "POST", "/api/credentials", cookie,
		map[string]string{"name": "c", "kind": "password", "secret": "pw"})
	credID := body["data"].(map[string]any)["credential"].(map[string]any)["id"].(string)
	st, body = ts.do(t, "POST", "/api/hosts", cookie, map[string]any{
		"name": "h", "address": addr, "port": port, "username": "tester",
		"credentialId": credID, "authKind": "credential"})
	hostID := body["data"].(map[string]any)["host"].(map[string]any)["id"].(string)

	// Filled variable (with tolerated inner whitespace) reaches SSH rendered.
	st, body = ts.do(t, "POST", "/api/exec", cookie, map[string]any{
		"command": "echo ${ who }", "vars": map[string]string{"who": "world"},
		"targetIds": []string{hostID}, "timeoutSec": 5})
	if st != 202 {
		t.Fatalf("exec filled = %d %v", st, body)
	}
	select {
	case cmd := <-seen:
		if cmd != "echo world" {
			t.Fatalf("remote command = %q, want %q", cmd, "echo world")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("rendered command never reached the SSH server")
	}
}

func TestCSRFRejectsCookieWriteWithoutHeader(t *testing.T) {
	ts := newTestServer(t, "127.0.0.1", 1, nil)
	cookie := setupOwner(t, ts)
	body, _ := json.Marshal(map[string]string{"name": "x"})
	req, _ := http.NewRequest("POST", ts.URL+"/api/groups", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie) // no X-BDK-CSRF header
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Fatalf("expected csrf 403, got %d", resp.StatusCode)
	}
}

func TestAnonymousRejected(t *testing.T) {
	ts := newTestServer(t, "127.0.0.1", 1, nil)
	st, _ := ts.do(t, "GET", "/api/hosts", nil, nil)
	if st != 401 {
		t.Fatalf("want 401 got %d", st)
	}
}

func TestSetupIdempotentGuard(t *testing.T) {
	ts := newTestServer(t, "127.0.0.1", 1, nil)
	_ = setupOwner(t, ts)
	// second setup must be refused.
	st, _ := ts.do(t, "POST", "/api/setup", nil,
		map[string]string{"username": "attacker", "password": "Sup3rSecret!!"})
	if st != 409 && st != 400 {
		t.Fatalf("second setup should be refused, got %d", st)
	}
}

var _ = context.Background
