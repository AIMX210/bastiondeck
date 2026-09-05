package backup

import (
	"context"
	"path/filepath"
	"testing"

	"bastiondeck/internal/auth"
	"bastiondeck/internal/testutil"
)

// seed creates an owner so the logical export contains at least one row.
func seed(t *testing.T, h *testutil.Harness) {
	t.Helper()
	if _, err := h.Auth.CreateUser(context.Background(), auth.CreateUserInput{
		Username: "owner", Password: "correcthorse123", Role: auth.RoleOwner,
	}); err != nil {
		t.Fatalf("seed user: %v", err)
	}
}

func TestExportInspectRoundtrip(t *testing.T) {
	h := testutil.NewHarness(t)
	seed(t, h)
	svc := New(h.Store.DB, h.Store.Path)
	ctx := context.Background()

	blob, err := svc.Export(ctx, "backup-passphrase")
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if len(blob) == 0 {
		t.Fatal("empty blob")
	}

	bundle, rep, err := Inspect(blob, "backup-passphrase")
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if bundle.Version != bundleVersion {
		t.Fatalf("version = %d", bundle.Version)
	}
	if rep.Counts["users"] < 1 {
		t.Fatalf("users count = %d, want >=1", rep.Counts["users"])
	}
}

func TestWrongPassphraseFails(t *testing.T) {
	h := testutil.NewHarness(t)
	seed(t, h)
	svc := New(h.Store.DB, h.Store.Path)
	blob, err := svc.Export(context.Background(), "right-pass")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := Inspect(blob, "wrong-pass"); err == nil {
		t.Fatal("wrong passphrase must fail AES-GCM open")
	}
}

func TestTamperedBlobFails(t *testing.T) {
	h := testutil.NewHarness(t)
	seed(t, h)
	svc := New(h.Store.DB, h.Store.Path)
	blob, err := svc.Export(context.Background(), "passphrase-1")
	if err != nil {
		t.Fatal(err)
	}
	// Flip a ciphertext byte well past salt+nonce header.
	idx := len(blob) - 4
	blob[idx] ^= 0xFF
	if _, _, err := Inspect(blob, "passphrase-1"); err == nil {
		t.Fatal("tampered ciphertext must fail authentication")
	}
}

func TestRestoreWritesSafetyCopy(t *testing.T) {
	h := testutil.NewHarness(t)
	seed(t, h)
	svc := New(h.Store.DB, h.Store.Path)
	ctx := context.Background()
	blob, err := svc.Export(ctx, "passphrase-1")
	if err != nil {
		t.Fatal(err)
	}
	bundle, _, err := Inspect(blob, "passphrase-1")
	if err != nil {
		t.Fatal(err)
	}
	safety := filepath.Join(h.DataDir, "safety")
	copyPath, err := svc.Restore(ctx, bundle, safety)
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if copyPath == "" {
		t.Fatal("restore must return the safety copy path")
	}
	// Users table must still be queryable with the seeded row.
	var n int
	if err := h.Store.DB.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n < 1 {
		t.Fatalf("users after restore = %d", n)
	}
}
