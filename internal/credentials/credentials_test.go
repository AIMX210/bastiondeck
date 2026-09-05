package credentials_test

import (
	"context"
	"strings"
	"testing"

	"bastiondeck/internal/testutil"
)

func TestPasswordCredentialRoundtrip(t *testing.T) {
	h := testutil.NewHarness(t)
	ctx := context.Background()
	c, err := h.Creds.Create(ctx, "db password", "password", "s3cret", "", "usr_1")
	if err != nil {
		t.Fatal(err)
	}
	if c.Name != "db password" || c.Kind != "password" {
		t.Fatalf("cred = %+v", c)
	}
	// List must never carry plaintext.
	list, err := h.Creds.List(ctx)
	if err != nil || len(list) != 1 {
		t.Fatalf("list = %d err %v", len(list), err)
	}
	got, err := h.Creds.Reveal(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Password != "s3cret" {
		t.Fatalf("revealed password = %q", got.Password)
	}
}

func TestPrivateKeyFingerprint(t *testing.T) {
	h := testutil.NewHarness(t)
	// A malformed private key must be rejected at write time.
	if _, err := h.Creds.Create(context.Background(), "bad key", "private_key", "not-a-key", "", "u"); err == nil {
		t.Fatal("malformed private key must be rejected")
	}
}

func TestCredentialUpdateRotatesSecret(t *testing.T) {
	h := testutil.NewHarness(t)
	ctx := context.Background()
	c, err := h.Creds.Create(ctx, "c", "password", "old", "", "u")
	if err != nil {
		t.Fatal(err)
	}
	newSecret := "new-secret"
	upd, err := h.Creds.Update(ctx, c.ID, "c renamed", &newSecret, nil)
	if err != nil {
		t.Fatal(err)
	}
	if upd.Name != "c renamed" {
		t.Fatalf("name not updated: %+v", upd)
	}
	revealed, err := h.Creds.Reveal(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if revealed.Password != "new-secret" {
		t.Fatalf("secret not rotated: %q", revealed.Password)
	}
}

func TestCredentialInUseAndDelete(t *testing.T) {
	h := testutil.NewHarness(t)
	ctx := context.Background()
	c, err := h.Creds.Create(ctx, "c", "password", "x", "", "u")
	if err != nil {
		t.Fatal(err)
	}
	n, err := h.Creds.InUse(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("fresh credential cannot be in use, got %d", n)
	}
	if err := h.Creds.Delete(ctx, c.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Creds.Get(ctx, c.ID); err == nil {
		t.Fatal("deleted credential must not be fetchable")
	}
}

func TestRejectUnknownKind(t *testing.T) {
	h := testutil.NewHarness(t)
	_, err := h.Creds.Create(context.Background(), "x", "totp", "123456", "", "u")
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "kind") {
		t.Fatalf("expected kind validation error, got %v", err)
	}
}
