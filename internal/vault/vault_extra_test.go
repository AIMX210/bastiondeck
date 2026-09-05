package vault_test

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"strings"
	"testing"

	"bastiondeck/internal/vault"
)

func newKey(t *testing.T) string {
	t.Helper()
	k := make([]byte, 32)
	if _, err := rand.Read(k); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(k)
}

func TestSealOpenRoundTrip(t *testing.T) {
	v, err := vault.FromHex(newKey(t))
	if err != nil {
		t.Fatal(err)
	}
	plain := []byte("super-secret-password")
	blob, err := v.Seal(plain, "crd_1")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(blob, plain) {
		t.Fatal("ciphertext must not equal plaintext")
	}
	got, err := v.Open(blob, "crd_1")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, plain) {
		t.Fatalf("round trip = %q", got)
	}
}

func TestAADBindingRejectsWrongContext(t *testing.T) {
	v, _ := vault.FromHex(newKey(t))
	blob, err := v.Seal([]byte("x"), "crd_a")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := v.Open(blob, "crd_b"); err == nil {
		t.Fatal("opening with a different AAD must fail")
	}
}

func TestCiphertextNonDeterministic(t *testing.T) {
	v, _ := vault.FromHex(newKey(t))
	b1, _ := v.Seal([]byte("same"), "ctx")
	b2, _ := v.Seal([]byte("same"), "ctx")
	if bytes.Equal(b1, b2) {
		t.Fatal("GCM nonce must make ciphertexts differ")
	}
}

func TestTamperDetected(t *testing.T) {
	v, _ := vault.FromHex(newKey(t))
	blob, _ := v.Seal([]byte("payload"), "ctx")
	blob[len(blob)-1] ^= 0xFF
	if _, err := v.Open(blob, "ctx"); err == nil {
		t.Fatal("flipped ciphertext must fail authentication")
	}
}

func TestWrongKeyFails(t *testing.T) {
	v1, _ := vault.FromHex(newKey(t))
	v2, _ := vault.FromHex(newKey(t))
	blob, _ := v1.Seal([]byte("payload"), "ctx")
	if _, err := v2.Open(blob, "ctx"); err == nil {
		t.Fatal("decrypting under another key must fail")
	}
}

func TestBadKeyRejected(t *testing.T) {
	for _, k := range []string{"", "short", strings.Repeat("z", 63), "xyz123"} {
		if _, err := vault.FromHex(k); err == nil {
			t.Fatalf("key %q must be rejected", k)
		}
	}
}

func TestResealMigratesBetweenKeys(t *testing.T) {
	v1, _ := vault.FromHex(newKey(t))
	v2, _ := vault.FromHex(newKey(t))
	blob, _ := v1.Seal([]byte("move-me"), "crd_9")
	moved, err := v1.Reseal(blob, "crd_9", v2)
	if err != nil {
		t.Fatal(err)
	}
	// Old key can no longer open it.
	if _, err := v1.Open(moved, "crd_9"); err == nil {
		t.Fatal("old key must not open resealed blob")
	}
	got, err := v2.Open(moved, "crd_9")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "move-me" {
		t.Fatalf("resealed plaintext = %q", got)
	}
}

func TestStringHelpers(t *testing.T) {
	v, _ := vault.FromHex(newKey(t))
	blob, err := v.SealString("text-secret", "ctx")
	if err != nil {
		t.Fatal(err)
	}
	s, err := v.OpenString(blob, "ctx")
	if err != nil || s != "text-secret" {
		t.Fatalf("got %q err=%v", s, err)
	}
}
