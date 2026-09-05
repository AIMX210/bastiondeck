package apiclient_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"bastiondeck/internal/apiclient"
)

// envelope writes the standard success envelope.
func envelope(v any) map[string]any { return map[string]any{"data": v} }

func TestStatusAndLogin(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/status", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(envelope(map[string]any{"version": "test", "setupRequired": true}))
	})
	mux.HandleFunc("POST /api/auth/login", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["username"] != "owner" {
			http.Error(w, "bad", 400)
			return
		}
		_ = json.NewEncoder(w).Encode(envelope(map[string]any{
			"token": "tok-123",
			"user":  map[string]any{"id": "usr_1", "username": "owner", "role": "owner"},
		}))
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	c := apiclient.New(ts.URL, "")
	st, err := c.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !st.SetupRequired || st.Version != "test" {
		t.Fatalf("status = %+v", st)
	}
	tok, u, err := c.Login(context.Background(), "owner", "pw", "")
	if err != nil || tok != "tok-123" || u.Username != "owner" {
		t.Fatalf("login tok=%q user=%+v err=%v", tok, u, err)
	}
}

func TestBearerAndCSRFHeaders(t *testing.T) {
	var seen http.Header
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/exec", func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Clone()
		_ = json.NewEncoder(w).Encode(envelope(map[string]any{"runId": "run_1"}))
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	c := apiclient.New(ts.URL, "abc")
	id, err := c.Exec(context.Background(), "id", []string{"hst_1"}, 10)
	if err != nil || id != "run_1" {
		t.Fatalf("exec id=%q err=%v", id, err)
	}
	if seen.Get("Authorization") != "Bearer abc" {
		t.Fatalf("bearer = %q", seen.Get("Authorization"))
	}
	if seen.Get("X-BDK-CSRF") == "" {
		t.Fatal("CSRF header missing on write")
	}
}

func TestErrorEnvelope(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/hosts", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(403)
		_, _ = io.WriteString(w, `{"error":{"code":"csrf_required","message":"missing header"}}`)
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()
	c := apiclient.New(ts.URL, "")
	_, err := c.ListHosts(context.Background(), "")
	if err == nil || !strings.Contains(err.Error(), "csrf_required") {
		t.Fatalf("expected coded error, got %v", err)
	}
}

func TestListHostsEnvelope(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/hosts", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("q") != "web" {
			t.Errorf("query not forwarded: %q", r.URL.RawQuery)
		}
		_ = json.NewEncoder(w).Encode(envelope(map[string]any{"hosts": []map[string]any{
			{"id": "hst_1", "name": "web-1", "address": "10.0.0.1", "port": 22, "username": "root",
				"authKind": "credential", "tags": []string{}, "lastStatus": "pending"},
		}}))
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()
	c := apiclient.New(ts.URL, "")
	hosts, err := c.ListHosts(context.Background(), "web")
	if err != nil || len(hosts) != 1 || hosts[0].Name != "web-1" {
		t.Fatalf("hosts = %+v err=%v", hosts, err)
	}
}
