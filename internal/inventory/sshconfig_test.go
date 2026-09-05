package inventory

import (
	"strings"
	"testing"
)

func TestParseSSHConfigBasic(t *testing.T) {
	in := `
# top comment
Host web-1
  HostName 10.0.0.11
  User deploy
  Port 2222

Host db-1
  HostName 10.0.0.21
  User root
  IdentityFile ~/.ssh/id_ed25519
`
	got, err := ParseSSHConfig(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 candidates, got %d: %+v", len(got), got)
	}
	if got[0].Host != "web-1" || got[0].HostName != "10.0.0.11" || got[0].User != "deploy" || got[0].Port != 2222 {
		t.Fatalf("first candidate wrong: %+v", got[0])
	}
	if got[1].Identity == "" {
		t.Fatal("identity file should be captured")
	}
}

func TestParseSSHConfigSkipsWildcards(t *testing.T) {
	in := `
Host *
  User ubuntu
Host *.internal
  User ops
Host concrete
  HostName 1.2.3.4
`
	got, err := ParseSSHConfig(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Host != "concrete" {
		t.Fatalf("wildcards must be skipped, got %+v", got)
	}
}

func TestParseSSHConfigNegatedAndMultiplePatterns(t *testing.T) {
	in := `
Host !bastion *
  User nobody
Host edge edge-backup
  HostName 172.16.0.5
Host one
  HostName 172.16.0.6
`
	got, err := ParseSSHConfig(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	// Negated/wildcard blocks and multi-alias blocks are both non-concrete;
	// only the single concrete host survives.
	if len(got) != 1 || got[0].Host != "one" || got[0].HostName != "172.16.0.6" {
		t.Fatalf("got %+v", got)
	}
}

func TestParseSSHConfigDefaults(t *testing.T) {
	in := `Host only
  HostName 5.6.7.8`
	got, err := ParseSSHConfig(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %+v", got)
	}
	// Unset port falls back to the SSH default 22.
	if got[0].Port != 22 {
		t.Fatalf("unset port must default to 22, got %d", got[0].Port)
	}
}

func TestParseSSHConfigCaseInsensitiveKeys(t *testing.T) {
	in := `
Host mixed
  hostname 9.9.9.9
  USER admin
  pOrT 2022
`
	got, err := ParseSSHConfig(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].HostName != "9.9.9.9" || got[0].User != "admin" || got[0].Port != 2022 {
		t.Fatalf("case-insensitive keys failed: %+v", got)
	}
}

func TestDedupeCandidates(t *testing.T) {
	in := `
Host dup
  HostName 1.1.1.1
Host dup
  HostName 1.1.1.1
Host uniq
  HostName 2.2.2.2
`
	got, err := ParseSSHConfig(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("duplicate block must collapse, got %+v", got)
	}
}
