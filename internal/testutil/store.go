package testutil

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"bastiondeck/internal/audit"
	"bastiondeck/internal/auth"
	"bastiondeck/internal/credentials"
	"bastiondeck/internal/inventory"
	"bastiondeck/internal/store"
	"bastiondeck/internal/vault"
)

// Harness bundles wired services backed by a temp data dir.
type Harness struct {
	T       testing.TB
	DataDir string
	Store   *store.Store
	Vault   *vault.Vault
	Audit   *audit.Service
	Auth    *auth.Service
	Creds   *credentials.Service
	Hosts   *inventory.Repo
}

// NewHarness builds a fully wired in-memory-ish service graph on disk temp.
func NewHarness(t testing.TB) *Harness {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	v, err := vault.Load("", dir)
	if err != nil {
		t.Fatalf("vault: %v", err)
	}
	au := audit.New(st.DB)
	as := auth.NewService(st.DB, v, time.Hour)
	cs := credentials.New(st.DB, v)
	hr := inventory.NewRepo(st.DB)
	h := &Harness{T: t, DataDir: dir, Store: st, Vault: v, Audit: au, Auth: as, Creds: cs, Hosts: hr}
	t.Cleanup(func() { _ = st.Close() })
	return h
}

// MustOwner creates an owner and returns it.
func (h *Harness) MustOwner(username, pw string) *auth.User {
	h.T.Helper()
	u, err := h.Auth.CreateUser(context.Background(), auth.CreateUserInput{
		Username: username, Password: pw, Role: auth.RoleOwner, DisplayName: "Owner",
	})
	if err != nil {
		h.T.Fatalf("create owner: %v", err)
	}
	return u
}

// MustCredential creates a password credential and returns its id.
func (h *Harness) MustCredential(name, password string) string {
	h.T.Helper()
	c, err := h.Creds.Create(context.Background(), name, credentials.KindPassword, password, "", "test")
	if err != nil {
		h.T.Fatalf("create credential: %v", err)
	}
	return c.ID
}

// MustHost creates a host record.
func (h *Harness) MustHost(name, addr string, port int, user, credID string) *inventory.Host {
	h.T.Helper()
	host, err := h.Hosts.Create(context.Background(), inventory.HostInput{
		Name: name, Address: addr, Port: port, Username: user, CredentialID: credID,
	})
	if err != nil {
		h.T.Fatalf("create host: %v", err)
	}
	return host
}

// Abs returns a path inside the temp data dir.
func (h *Harness) Abs(elem ...string) string {
	return filepath.Join(append([]string{h.DataDir}, elem...)...)
}
