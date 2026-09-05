package remotefs

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"

	"bd-agent/internal/proto"
)

func TestWriteReadListStat(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.txt")
	// write
	w := Handle(context.Background(), proto.Frame{ID: "1", Op: "write", Path: p,
		ContentB64: base64.StdEncoding.EncodeToString([]byte("abc"))})
	if w.ErrorText != "" {
		t.Fatalf("write: %s", w.ErrorText)
	}
	if w.Payload == nil {
		t.Fatal("write should return sha payload")
	}
	// read
	r := Handle(context.Background(), proto.Frame{ID: "2", Op: "read", Path: p})
	b, _ := base64.StdEncoding.DecodeString(r.ContentB64)
	if string(b) != "abc" {
		t.Fatalf("read %q", b)
	}
	// stat
	st := Handle(context.Background(), proto.Frame{ID: "3", Op: "stat", Path: p})
	if st.ErrorText != "" || st.Payload == nil {
		t.Fatalf("stat: %s", st.ErrorText)
	}
	// list
	l := Handle(context.Background(), proto.Frame{ID: "4", Op: "list", Path: dir})
	if l.ErrorText != "" || l.Payload == nil {
		t.Fatalf("list: %s", l.ErrorText)
	}
}

func TestOptimisticWriteConflict(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "x.txt")
	_ = os.WriteFile(p, []byte("current"), 0o600)
	// expected sha of a *different* content must be rejected.
	res := Handle(context.Background(), proto.Frame{Op: "write", Path: p,
		ContentB64:  base64.StdEncoding.EncodeToString([]byte("new")),
		ExpectedSHA: "deadbeef"})
	if res.ErrorText == "" {
		t.Fatal("expected modified conflict")
	}
	got, _ := os.ReadFile(p)
	if string(got) != "current" {
		t.Fatal("conflicting write must not change the file")
	}
}

func TestMkdirRenameRemove(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if res := Handle(context.Background(), proto.Frame{Op: "mkdir", Path: sub}); res.ErrorText != "" {
		t.Fatal(res.ErrorText)
	}
	dst := filepath.Join(dir, "renamed")
	if res := Handle(context.Background(), proto.Frame{Op: "rename", Path: sub, Dest: dst}); res.ErrorText != "" {
		t.Fatal(res.ErrorText)
	}
	if res := Handle(context.Background(), proto.Frame{Op: "remove", Path: dst}); res.ErrorText != "" {
		t.Fatal(res.ErrorText)
	}
	if _, err := os.Stat(dst); !os.IsNotExist(err) {
		t.Fatal("remove failed")
	}
}
