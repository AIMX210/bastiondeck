package sftplite_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"strings"
	"testing"
	"time"

	"bastiondeck/internal/sftplite"
	"bastiondeck/internal/sshlite"
	"bastiondeck/internal/testutil"
)

func fsFor(t *testing.T) (*sftplite.FS, func()) {
	t.Helper()
	// A non-empty sftp root switches the in-process SFTP subsystem on.
	srv := testutil.NewFakeSSH(t, "pw", t.TempDir(), nil)
	h := testutil.NewHarness(t)
	addr, port := srv.Addr()
	cred := h.MustCredential("c", "pw")
	host := h.MustHost("h", addr, port, "tester", cred)
	pool := sshlite.NewPool(&sshlite.Dialer{Hosts: h.Hosts, Creds: h.Creds, DialTimeout: 5 * time.Second}, time.Minute)
	cli, err := pool.Connect(context.Background(), host.ID)
	if err != nil {
		t.Fatal(err)
	}
	fs, err := sftplite.NewFS(cli.(*sshlite.Client).SSH())
	if err != nil {
		t.Fatal(err)
	}
	return fs, func() { _ = fs.Close(); pool.CloseAll(); srv.Close() }
}

func sha(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

func TestWriteReadRoundTrip(t *testing.T) {
	fs, closeFn := fsFor(t)
	defer closeFn()
	ctx := context.Background()
	content := []byte("hello sftp world\n")
	gotSHA, err := fs.Write(ctx, "/a.txt", content, "")
	if err != nil {
		t.Fatal(err)
	}
	if gotSHA != sha(content) {
		t.Fatal("write sha mismatch")
	}
	got, err := fs.Read(ctx, "/a.txt", 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, content) {
		t.Fatalf("read = %q", got)
	}
	st, err := fs.Stat(ctx, "/a.txt")
	if err != nil {
		t.Fatal(err)
	}
	if st.Size != int64(len(content)) {
		t.Fatalf("size = %d", st.Size)
	}
}

func TestWriteRejectsStaleExpectedSHA(t *testing.T) {
	fs, closeFn := fsFor(t)
	defer closeFn()
	ctx := context.Background()
	if _, err := fs.Write(ctx, "/b.txt", []byte("new"), sha([]byte("different"))); err == nil {
		t.Fatal("expected ModifiedError for mismatched expected sha")
	} else if !strings.Contains(err.Error(), "changed on server") {
		t.Fatalf("want changed-on-server error, got %v", err)
	}
}

func TestWriteAcceptsMatchingExpectedSHA(t *testing.T) {
	fs, closeFn := fsFor(t)
	defer closeFn()
	ctx := context.Background()
	first := []byte("first")
	if _, err := fs.Write(ctx, "/c.txt", first, ""); err != nil {
		t.Fatal(err)
	}
	// Overwrite is allowed only when expected matches the bytes on the server.
	next := []byte("exact")
	if _, err := fs.Write(ctx, "/c.txt", next, sha(first)); err != nil {
		t.Fatalf("matching current sha should be accepted: %v", err)
	}
	got, err := fs.Read(ctx, "/c.txt", 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, next) {
		t.Fatalf("content = %q", got)
	}
}

func TestReadEnforcesLimit(t *testing.T) {
	fs, closeFn := fsFor(t)
	defer closeFn()
	ctx := context.Background()
	if _, err := fs.Write(ctx, "/d.bin", make([]byte, 1024), ""); err != nil {
		t.Fatal(err)
	}
	if _, err := fs.Read(ctx, "/d.bin", 128); err == nil {
		t.Fatal("oversized read must return TooLargeError")
	}
}

func TestUploadDownload(t *testing.T) {
	fs, closeFn := fsFor(t)
	defer closeFn()
	ctx := context.Background()
	payload := bytes.Repeat([]byte("abc123"), 100)
	var uploaded int64
	if err := fs.Upload(ctx, bytes.NewReader(payload), "/u.bin", func(n int64) { uploaded = n }); err != nil {
		t.Fatal(err)
	}
	if uploaded != int64(len(payload)) {
		t.Fatalf("progress = %d want %d", uploaded, len(payload))
	}
	var buf bytes.Buffer
	var downloaded int64
	if err := fs.Download(ctx, "/u.bin", &buf, func(n int64) { downloaded = n }); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(buf.Bytes(), payload) {
		t.Fatal("download payload mismatch")
	}
	if downloaded != int64(len(payload)) {
		t.Fatalf("download progress = %d", downloaded)
	}
}

func TestListAndMkdirRemove(t *testing.T) {
	fs, closeFn := fsFor(t)
	defer closeFn()
	ctx := context.Background()
	if err := fs.Mkdir(ctx, "/dir-x"); err != nil {
		t.Fatal(err)
	}
	if _, err := fs.Write(ctx, "/dir-x/f.txt", []byte("x"), ""); err != nil {
		t.Fatal(err)
	}
	entries, err := fs.List(ctx, "/dir-x")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range entries {
		if e.Name == "f.txt" {
			found = true
		}
	}
	if !found {
		t.Fatalf("f.txt not listed: %+v", entries)
	}
	if err := fs.Rename(ctx, "/dir-x/f.txt", "/dir-x/g.txt"); err != nil {
		t.Fatal(err)
	}
	if err := fs.Remove(ctx, "/dir-x/g.txt"); err != nil {
		t.Fatal(err)
	}
	entries, _ = fs.List(ctx, "/dir-x")
	for _, e := range entries {
		if e.Name == "g.txt" {
			t.Fatal("file should have been removed")
		}
	}
}

var _ io.Reader = (*bytes.Reader)(nil)
