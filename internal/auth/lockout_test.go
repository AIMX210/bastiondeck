package auth_test

import (
	"context"
	"testing"

	"bastiondeck/internal/auth"
	"bastiondeck/internal/testutil"
)

// After 10 failures in the 10-minute window, further attempts (even with the
// correct password) must be refused with ErrLocked until the window passes.
func TestLoginLockout(t *testing.T) {
	h := testutil.NewHarness(t)
	const pw = "correcthorse123"
	h.MustOwner("owner", pw)
	ctx := context.Background()

	for i := 0; i < 10; i++ {
		if _, err := h.Auth.Login(ctx, "owner", "wrong", "", "127.0.0.1", "test"); err != auth.ErrBadCredentials {
			t.Fatalf("attempt %d: want ErrBadCredentials, got %v", i, err)
		}
	}
	// 11th attempt, even with the right password, is locked.
	if _, err := h.Auth.Login(ctx, "owner", pw, "", "127.0.0.1", "test"); err != auth.ErrLocked {
		t.Fatalf("want ErrLocked, got %v", err)
	}
}

func TestLockoutScopedToPrincipalOrIP(t *testing.T) {
	h := testutil.NewHarness(t)
	const pw = "correcthorse123"
	h.MustOwner("owner", pw)
	// A second, unrelated principal from a distinct IP.
	other, err := h.Auth.CreateUser(context.Background(), auth.CreateUserInput{
		Username: "other", Password: pw, Role: auth.RoleViewer,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for i := 0; i < 10; i++ {
		_, _ = h.Auth.Login(ctx, "owner", "bad", "", "10.0.0.1", "t")
	}
	// owner is locked from any IP (account dimension), and 10.0.0.1 is locked
	// for any username (IP dimension); an unrelated principal from a clean IP
	// must still be able to log in.
	if _, err := h.Auth.Login(ctx, "owner", pw, "", "10.0.0.9", "t"); err != auth.ErrLocked {
		t.Fatalf("account lock must follow username across IPs, got %v", err)
	}
	out, err := h.Auth.Login(ctx, other.Username, pw, "", "10.0.0.2", "t")
	if err != nil {
		t.Fatalf("unrelated principal+IP should succeed: %v", err)
	}
	if out.SessionToken == "" {
		t.Fatal("session token missing")
	}
}

func TestSuccessfulLoginResetsFailures(t *testing.T) {
	h := testutil.NewHarness(t)
	const pw = "correcthorse123"
	h.MustOwner("owner", pw)
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		_, _ = h.Auth.Login(ctx, "owner", "bad", "", "ip", "t")
	}
	if _, err := h.Auth.Login(ctx, "owner", pw, "", "ip", "t"); err != nil {
		t.Fatalf("successful login: %v", err)
	}
	// Five more failures after the reset must not trip the limit.
	for i := 0; i < 5; i++ {
		_, _ = h.Auth.Login(ctx, "owner", "bad", "", "ip", "t")
	}
	if _, err := h.Auth.Login(ctx, "owner", pw, "", "ip", "t"); err != nil {
		t.Fatalf("counter should have reset: %v", err)
	}
}

func TestUnknownUserStillReturnsBadCredentials(t *testing.T) {
	h := testutil.NewHarness(t)
	_, err := h.Auth.Login(context.Background(), "ghost", "x", "", "ip", "t")
	if err != auth.ErrBadCredentials {
		t.Fatalf("want ErrBadCredentials, got %v", err)
	}
}
