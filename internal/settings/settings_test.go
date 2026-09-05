package settings

import (
	"context"
	"testing"

	"bastiondeck/internal/testutil"
)

func newSvc(t *testing.T) *Service {
	t.Helper()
	h := testutil.NewHarness(t)
	return New(h.Store.DB)
}

func TestDefaultsAndOverride(t *testing.T) {
	s := newSvc(t)
	// Absent key falls back to documented default.
	if got := s.Get("exec.defaultTimeoutSec"); got != Defaults["exec.defaultTimeoutSec"] {
		t.Fatalf("default = %q, want %q", got, Defaults["exec.defaultTimeoutSec"])
	}
	if got := s.GetInt("exec.maxConcurrency", 9); got != 20 {
		t.Fatalf("default int = %d, want 20", got)
	}
	// Unknown key has no default and GetInt must use the caller fallback.
	if got := s.GetInt("nope.key", 7); got != 7 {
		t.Fatalf("fallback int = %d, want 7", got)
	}
	if err := s.Set(context.Background(), "exec.defaultTimeoutSec", "120"); err != nil {
		t.Fatal(err)
	}
	if got := s.Get("exec.defaultTimeoutSec"); got != "120" {
		t.Fatalf("override = %q, want 120", got)
	}
	if got := s.GetInt("exec.defaultTimeoutSec", 60); got != 120 {
		t.Fatalf("override int = %d, want 120", got)
	}
	// Malformed int falls back.
	if err := s.Set(context.Background(), "exec.defaultTimeoutSec", "abc"); err != nil {
		t.Fatal(err)
	}
	if got := s.GetInt("exec.defaultTimeoutSec", 33); got != 33 {
		t.Fatalf("malformed int = %d, want fallback 33", got)
	}
}

func TestAllMergesDefaults(t *testing.T) {
	s := newSvc(t)
	if err := s.Set(context.Background(), "theme.default", "dark"); err != nil {
		t.Fatal(err)
	}
	all := s.All()
	if all["theme.default"] != "dark" {
		t.Fatalf("override missing: %q", all["theme.default"])
	}
	// Untouched defaults survive the merge.
	if all["metrics.enabled"] != "true" {
		t.Fatalf("default lost in merge: %q", all["metrics.enabled"])
	}
}

func TestSetIdempotentUpsert(t *testing.T) {
	s := newSvc(t)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if err := s.Set(ctx, "k", "v"); err != nil {
			t.Fatal(err)
		}
	}
	if got := s.Get("k"); got != "v" {
		t.Fatalf("upsert = %q", got)
	}
}
