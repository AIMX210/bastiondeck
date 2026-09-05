package auth_test

import (
	"context"
	"testing"
	"time"

	"bastiondeck/internal/auth"
	"bastiondeck/internal/testutil"
)

func TestPasswordHashVerify(t *testing.T) {
	h, err := auth.HashPassword("secret123")
	if err != nil {
		t.Fatal(err)
	}
	ok, err := auth.VerifyPassword("secret123", h)
	if err != nil || !ok {
		t.Fatalf("verify failed: %v", err)
	}
	ok, _ = auth.VerifyPassword("wrong1234", h)
	if ok {
		t.Fatal("wrong password must not verify")
	}
	if len(auth.PasswordStrength("abc")) == 0 {
		t.Fatal("weak password should be rejected")
	}
}

// RFC 6238 published SHA-1 test vector: secret "12345678901234567890"
// base32 = GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ, at T=59 -> 287082.
func TestTOTPRFCVector(t *testing.T) {
	secret := "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
	at := time.Unix(59, 0)
	code, err := auth.TOTPCodeAt(secret, at)
	if err != nil {
		t.Fatal(err)
	}
	if code != "287082" {
		t.Fatalf("got %s want 287082", code)
	}
	if !auth.ValidateTOTP(secret, "287082", at) {
		t.Fatal("validate should accept")
	}
	if auth.ValidateTOTP(secret, "000000", at) {
		t.Fatal("wrong code accepted")
	}
}

func TestLoginAndSession(t *testing.T) {
	h := testutil.NewHarness(t)
	ctx := context.Background()
	if _, err := h.Auth.CreateUser(ctx, auth.CreateUserInput{
		Username: "alice", Password: "passw0rd1", Role: auth.RoleOperator}); err != nil {
		t.Fatal(err)
	}
	out, err := h.Auth.Login(ctx, "alice", "passw0rd1", "", "127.0.0.1", "test")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	u, sess, err := h.Auth.Resolve(ctx, out.SessionToken)
	if err != nil || u.Username != "alice" || sess == nil {
		t.Fatalf("resolve: %v %v", err, u)
	}
	if err := h.Auth.Logout(ctx, out.SessionToken); err != nil {
		t.Fatal(err)
	}
	if _, _, err := h.Auth.Resolve(ctx, out.SessionToken); err == nil {
		t.Fatal("revoked session must not resolve")
	}
}

func TestLoginRateLimit(t *testing.T) {
	h := testutil.NewHarness(t)
	ctx := context.Background()
	_, _ = h.Auth.CreateUser(ctx, auth.CreateUserInput{Username: "bob", Password: "passw0rd1", Role: auth.RoleViewer})
	var lastErr error
	for i := 0; i < 11; i++ {
		_, lastErr = h.Auth.Login(ctx, "bob", "WRONGpass1", "", "127.0.0.9", "t")
	}
	if lastErr != auth.ErrLocked {
		t.Fatalf("expected lock, got %v", lastErr)
	}
}

func TestRBACMatrix(t *testing.T) {
	cases := []struct {
		role string
		perm auth.Permission
		want bool
	}{
		{"viewer", auth.PermRead, true},
		{"viewer", auth.PermExec, false},
		{"operator", auth.PermExec, true},
		{"operator", auth.PermManageInventory, false},
		{"admin", auth.PermManageInventory, true},
		{"admin", auth.PermOwner, false},
		{"owner", auth.PermOwner, true},
	}
	for _, c := range cases {
		if got := auth.Can(c.role, c.perm); got != c.want {
			t.Errorf("%s/%s = %v want %v", c.role, c.perm, got, c.want)
		}
	}
	if auth.CanAssignRole("admin", "owner") {
		t.Fatal("admin must not assign owner")
	}
	if !auth.CanAssignRole("owner", "admin") {
		t.Fatal("owner should assign admin")
	}
}
