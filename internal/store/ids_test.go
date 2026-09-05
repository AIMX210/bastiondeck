package store_test

import (
	"strings"
	"testing"

	"bastiondeck/internal/store"
)

func TestNewIDPrefixAndShape(t *testing.T) {
	id := store.NewID(store.PrefixHost)
	if !strings.HasPrefix(id, "hst_") {
		t.Fatalf("id = %q", id)
	}
	// Prefix + 24-character random body, unique across many draws.
	seen := map[string]bool{}
	for i := 0; i < 1000; i++ {
		x := store.NewID(store.PrefixRun)
		if seen[x] {
			t.Fatal("duplicate id")
		}
		seen[x] = true
		if !strings.HasPrefix(x, "run_") {
			t.Fatalf("bad prefix: %s", x)
		}
	}
}

func TestAllPrefixes(t *testing.T) {
	pairs := map[string]string{
		store.PrefixUser:    "usr_",
		store.PrefixCred:    "crd_",
		store.PrefixHost:    "hst_",
		store.PrefixSnippet: "snp_",
		store.PrefixJob:     "job_",
		store.PrefixRun:     "run_",
		store.PrefixTarget:  "tgt_",
		store.PrefixTunnel:  "tun_",
		store.PrefixAgent:   "agt_",
		store.PrefixAudit:   "aud_",
		store.PrefixSession: "ses_",
	}
	for in, want := range pairs {
		if got := store.NewID(in); !strings.HasPrefix(got, want) {
			t.Fatalf("%s → %s want prefix %s", in, got, want)
		}
	}
}

func TestRandomToken(t *testing.T) {
	a := store.RandomToken(32)
	b := store.RandomToken(32)
	if a == b {
		t.Fatal("tokens must differ")
	}
	if len(a) != 64 { // 32 bytes → 64 hex chars
		t.Fatalf("len = %d", len(a))
	}
	if store.RandomToken(0) != "" {
		t.Fatal("zero bytes → empty token")
	}
}

func TestHashTokenDeterministic(t *testing.T) {
	h1 := store.HashToken("abc")
	h2 := store.HashToken("abc")
	if h1 != h2 {
		t.Fatal("same token must hash identically")
	}
	if store.HashToken("abd") == h1 {
		t.Fatal("different tokens must not collide")
	}
	if len(h1) != 64 {
		t.Fatal("expect sha256 hex")
	}
	if h1 == "abc" {
		t.Fatal("must not store the raw token")
	}
}

func TestNowIsUTC(t *testing.T) {
	now := store.Now()
	if !strings.HasSuffix(now, "Z") {
		t.Fatalf("Now() must be UTC Z-suffixed, got %s", now)
	}
}
