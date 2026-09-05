package inventory_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"bastiondeck/internal/inventory"
	"bastiondeck/internal/testutil"
)

func ctxBg() context.Context { return context.Background() }

func TestHostCRUDAndSearch(t *testing.T) {
	h := testutil.NewHarness(t)
	host, err := h.Hosts.Create(ctxBg(), inventory.HostInput{
		Name: "web-1", Address: "10.0.0.1", Port: 22, Username: "root",
		Tags: []string{"prod", "web"},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := h.Hosts.Get(ctxBg(), host.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "web-1" || len(got.Tags) != 2 {
		t.Fatalf("read back wrong: %+v", got)
	}
	list, err := h.Hosts.List(ctxBg(), inventory.HostFilter{Query: "WEB"})
	if err != nil || len(list) != 1 {
		t.Fatalf("case-insensitive search failed: %d %v", len(list), err)
	}
	if err := h.Hosts.Delete(ctxBg(), host.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Hosts.Get(ctxBg(), host.ID); !errors.Is(err, inventory.ErrNotFound) {
		t.Fatalf("want not found got %v", err)
	}
}

func makeHost(t *testing.T, h *testutil.Harness, name string) *inventory.Host {
	t.Helper()
	hh, err := h.Hosts.Create(ctxBg(), inventory.HostInput{Name: name, Address: name, Port: 22, Username: "u"})
	if err != nil {
		t.Fatal(err)
	}
	return hh
}

func TestJumpCycleRejected(t *testing.T) {
	h := testutil.NewHarness(t)
	a := makeHost(t, h, "a")
	b := makeHost(t, h, "b")
	c := makeHost(t, h, "c")
	// a -> b -> c
	mkJump := func(id, jump string) {
		hh, _ := h.Hosts.Get(ctxBg(), id)
		in := inventory.HostInput{Name: hh.Name, Address: hh.Address, Port: 22, Username: "u", JumpHostID: jump}
		if _, err := h.Hosts.Update(ctxBg(), id, in); err != nil {
			t.Fatal(err)
		}
	}
	mkJump(a.ID, b.ID)
	mkJump(b.ID, c.ID)
	// closing the loop c -> a must fail
	cc, _ := h.Hosts.Get(ctxBg(), c.ID)
	_, err := h.Hosts.Update(ctxBg(), c.ID, inventory.HostInput{
		Name: cc.Name, Address: cc.Address, Port: 22, Username: "u", JumpHostID: a.ID})
	if !errors.Is(err, inventory.ErrJumpCycle) {
		t.Fatalf("want cycle error, got %v", err)
	}
}

func TestJumpDepthLimit(t *testing.T) {
	h := testutil.NewHarness(t)
	var prev string
	for i := 0; i < inventory.MaxJumpDepth+2; i++ {
		hh := makeHost(t, h, string(rune('a'+i)))
		if prev != "" {
			cur, _ := h.Hosts.Get(ctxBg(), hh.ID)
			_, err := h.Hosts.Update(ctxBg(), hh.ID, inventory.HostInput{
				Name: cur.Name, Address: cur.Address, Port: 22, Username: "u", JumpHostID: prev})
			if i > inventory.MaxJumpDepth && !errors.Is(err, inventory.ErrJumpTooDeep) {
				t.Fatalf("depth %d: want ErrJumpTooDeep got %v", i, err)
			}
		}
		prev = hh.ID
	}
}

func TestDeleteJumpHostBlocked(t *testing.T) {
	h := testutil.NewHarness(t)
	a := makeHost(t, h, "jump")
	b, _ := h.Hosts.Create(ctxBg(), inventory.HostInput{
		Name: "target", Address: "x", Port: 22, Username: "u", JumpHostID: a.ID})
	_ = b
	if err := h.Hosts.Delete(ctxBg(), a.ID); !errors.Is(err, inventory.ErrIsJumpHost) {
		t.Fatalf("want is-jump-host got %v", err)
	}
}

func TestParseSSHConfig(t *testing.T) {
	src := `
# a comment
Host *
  User wildcard
Host web1
  HostName 192.168.1.10
  User deploy
  Port 2222
Host db1
  HostName 10.0.0.2
Host pat*ern
  HostName ignored
`
	cands, err := inventory.ParseSSHConfig(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(cands) != 2 {
		t.Fatalf("want 2 concrete hosts (wildcards dropped), got %d: %+v", len(cands), cands)
	}
	if cands[0].Host != "web1" || cands[0].Port != 2222 || cands[0].User != "deploy" {
		t.Fatalf("web1 parse wrong: %+v", cands[0])
	}
	if cands[1].Port != 22 {
		t.Fatal("default port should be 22")
	}
}
