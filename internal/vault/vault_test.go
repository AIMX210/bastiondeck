package vault

import (
	"bytes"
	"testing"
)

func TestSealOpenRoundTrip(t *testing.T) {
	dir := t.TempDir()
	v, err := Load("", dir)
	if err != nil {
		t.Fatal(err)
	}
	plain := []byte("super-secret-password")
	blob, err := v.Seal(plain, "crd_123")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(blob, plain) {
		t.Fatal("ciphertext must not equal plaintext")
	}
	got, err := v.Open(blob, "crd_123")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, plain) {
		t.Fatalf("got %q want %q", got, plain)
	}
}

func TestAADBindingRejectsMovedBlob(t *testing.T) {
	dir := t.TempDir()
	v, _ := Load("", dir)
	blob, err := v.Seal([]byte("x"), "crd_a")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := v.Open(blob, "crd_b"); err == nil {
		t.Fatal("expected authentication failure when AAD differs")
	}
}

func TestTamperDetected(t *testing.T) {
	dir := t.TempDir()
	v, _ := Load("", dir)
	blob, _ := v.Seal([]byte("hello"), "id")
	blob[len(blob)-1] ^= 0xff
	if _, err := v.Open(blob, "id"); err == nil {
		t.Fatal("bit flip must be detected by GCM")
	}
}

func TestGeneratedKeyPersisted(t *testing.T) {
	dir := t.TempDir()
	v1, err := Load("", dir)
	if err != nil {
		t.Fatal(err)
	}
	blob, _ := v1.SealString("persist-me", "x")
	v2, err := Load("", dir)
	if err != nil {
		t.Fatal(err)
	}
	got, err := v2.OpenString(blob, "x")
	if err != nil || got != "persist-me" {
		t.Fatalf("reload via key file failed: %v %q", err, got)
	}
}

func TestResealAcrossVaults(t *testing.T) {
	dir := t.TempDir()
	v1, _ := Load("", dir)
	blob, _ := v1.SealString("rotate", "id")
	v2, _ := Load("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", dir)
	re, err := v1.Reseal(blob, "id", v2)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := v1.Open(re, "id"); err == nil {
		t.Fatal("old vault must not open resealed blob")
	}
	got, err := v2.Open(re, "id")
	if err != nil || string(got) != "rotate" {
		t.Fatalf("new vault should open resealed blob: %v", err)
	}
}
