package sshlite_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"bastiondeck/internal/connector"
	"bastiondeck/internal/inventory"
	"bastiondeck/internal/sshlite"
	"bastiondeck/internal/testutil"
)

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func dialerFor(h *testutil.Harness) *sshlite.Dialer {
	return &sshlite.Dialer{Hosts: h.Hosts, Creds: h.Creds, DialTimeout: 5 * time.Second}
}

func TestExecSuccessAndFailure(t *testing.T) {
	srv := testutil.NewFakeSSH(t, "pw", "", func(cmd string) ([]byte, []byte, int) {
		if strings.Contains(cmd, "fail") {
			return nil, []byte("boom\n"), 3
		}
		return []byte("hello\n"), nil, 0
	})
	defer srv.Close()
	h := testutil.NewHarness(t)
	addr, port := srv.Addr()
	cred := h.MustCredential("c", "pw")
	host := h.MustHost("h1", addr, port, "tester", cred)
	pool := sshlite.NewPool(dialerFor(h), time.Minute)
	defer pool.CloseAll()

	cli, err := pool.Connect(context.Background(), host.ID)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	res, err := cli.Exec(context.Background(), connector.ExecRequest{Command: "echo hello", Timeout: 3 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != "success" || string(res.Stdout) != "hello\n" || res.ExitCode != 0 {
		t.Fatalf("unexpected success result: %+v", res)
	}
	res, err = cli.Exec(context.Background(), connector.ExecRequest{Command: "fail-please", Timeout: 3 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != "failed" || res.ExitCode != 3 || string(res.Stderr) != "boom\n" {
		t.Fatalf("unexpected failure result: %+v stderr=%q", res, res.Stderr)
	}
}

func TestExecTimeout(t *testing.T) {
	srv := testutil.NewFakeSSH(t, "pw", "", func(cmd string) ([]byte, []byte, int) {
		time.Sleep(400 * time.Millisecond)
		return []byte("late"), nil, 0
	})
	defer srv.Close()
	h := testutil.NewHarness(t)
	addr, port := srv.Addr()
	cred := h.MustCredential("c", "pw")
	host := h.MustHost("slow", addr, port, "tester", cred)
	d := dialerFor(h)
	cli, err := d.Connect(context.Background(), host.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer cli.Close()
	start := time.Now()
	res, _ := cli.Exec(context.Background(), connector.ExecRequest{Command: "sleep", Timeout: 100 * time.Millisecond})
	if res.Status != connector.StatusTimeout {
		t.Fatalf("want timeout got %s", res.Status)
	}
	if time.Since(start) > time.Second {
		t.Fatal("timeout did not return promptly")
	}
}

func TestTOFUKeyChangeRejected(t *testing.T) {
	srv1 := testutil.NewFakeSSH(t, "pw", "", func(string) ([]byte, []byte, int) {
		return []byte("one"), nil, 0
	})
	h := testutil.NewHarness(t)
	a1, p1 := srv1.Addr()
	cred := h.MustCredential("c", "pw")
	host := h.MustHost("hk", a1, p1, "tester", cred)
	d := dialerFor(h)
	c1, err := d.Connect(context.Background(), host.ID)
	if err != nil {
		t.Fatal(err)
	}
	_ = c1.Close()
	srv1.Close()

	// New server = new host key; must be rejected without explicit reset.
	srv2 := testutil.NewFakeSSH(t, "pw", "", func(string) ([]byte, []byte, int) {
		return []byte("two"), nil, 0
	})
	defer srv2.Close()
	a2, p2 := srv2.Addr()
	updated, _ := h.Hosts.Get(context.Background(), host.ID)
	in := inventory.HostInput{
		Name: updated.Name, Address: a2, Port: p2, Username: updated.Username,
		CredentialID: deref(updated.CredentialID), Tags: updated.Tags,
	}
	_, err = h.Hosts.Update(context.Background(), host.ID, in)
	if err != nil {
		t.Fatal(err)
	}
	_, err = d.Connect(context.Background(), host.ID)
	if err == nil {
		t.Fatal("expected host key change rejection")
	}
	var changed *sshlite.ErrHostKeyChanged
	if !asErr(err, &changed) {
		t.Fatalf("want ErrHostKeyChanged got %T %v", err, err)
	}
	// After explicit reset, connect succeeds and records new fingerprint.
	if err := h.Hosts.ResetHostKey(context.Background(), host.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Connect(context.Background(), host.ID); err != nil {
		t.Fatalf("connect after reset: %v", err)
	}
}

func asErr(err error, target **sshlite.ErrHostKeyChanged) bool {
	for err != nil {
		if e, ok := err.(*sshlite.ErrHostKeyChanged); ok {
			*target = e
			return true
		}
		type unwrapper interface{ Unwrap() error }
		u, ok := err.(unwrapper)
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}

func TestSFTPWriteReadAndOptimisticLock(t *testing.T) {
	root := t.TempDir()
	srv := testutil.NewFakeSSH(t, "pw", root, nil)
	defer srv.Close()
	h := testutil.NewHarness(t)
	addr, port := srv.Addr()
	cred := h.MustCredential("c", "pw")
	host := h.MustHost("fs", addr, port, "tester", cred)
	d := dialerFor(h)
	cli, err := d.Connect(context.Background(), host.ID)
	if err != nil {
		t.Fatal(err)
	}
	fs, err := cli.FS()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	newSHA, err := fs.Write(ctx, "/notes.txt", []byte("first"), "")
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte("first"))
	if newSHA != hex.EncodeToString(sum[:]) {
		t.Fatal("returned sha mismatch")
	}
	b, err := fs.Read(ctx, "/notes.txt", 1<<20)
	if err != nil || string(b) != "first" {
		t.Fatalf("read back: %q %v", b, err)
	}
	// Stale expected sha must conflict.
	if _, err := fs.Write(ctx, "/notes.txt", []byte("second"), "deadbeef"); err == nil {
		t.Fatal("expected optimistic lock conflict")
	}
	// Correct sha updates cleanly.
	if _, err := fs.Write(ctx, "/notes.txt", []byte("second"), newSHA); err != nil {
		t.Fatalf("valid update: %v", err)
	}
	entries, err := fs.List(ctx, "/")
	if err != nil || len(entries) != 1 || entries[0].Name != "notes.txt" {
		t.Fatalf("list: %+v err=%v", entries, err)
	}
}
