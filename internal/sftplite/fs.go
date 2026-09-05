// Package sftplite adapts pkg/sftp to the connector.FS contract: bounded
// reads, atomic temp-file+rename writes with sha256 optimistic locking, and
// cancellable uploads/downloads with progress callbacks.
package sftplite

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"

	"bastiondeck/internal/connector"
)

// MaxReadBytes bounds text-file reads (editor path).
const MaxReadBytes = 1 << 20

// FS implements connector.FS.
type FS struct {
	c *sftp.Client
}

// NewFS opens an SFTP channel over an ssh client.
func NewFS(raw *ssh.Client) (*FS, error) {
	c, err := sftp.NewClient(raw, sftp.UseConcurrentReads(true), sftp.UseFstat(true))
	if err != nil {
		return nil, err
	}
	return &FS{c: c}, nil
}

// Close ends the SFTP session.
func (f *FS) Close() error { return f.c.Close() }

// List lists a directory.
func (f *FS) List(ctx context.Context, p string) ([]connector.DirEntry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	infos, err := f.c.ReadDir(p)
	if err != nil {
		return nil, err
	}
	out := make([]connector.DirEntry, 0, len(infos))
	for _, fi := range infos {
		out = append(out, connector.DirEntry{
			Name:  fi.Name(),
			Size:  fi.Size(),
			Mode:  uint32(fi.Mode().Perm()),
			IsDir: fi.IsDir(),
			MTime: fi.ModTime().Unix(),
		})
	}
	return out, nil
}

// Stat returns metadata for a path.
func (f *FS) Stat(ctx context.Context, p string) (*connector.FileStat, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	fi, err := f.c.Stat(p)
	if err != nil {
		return nil, err
	}
	return &connector.FileStat{Name: fi.Name(), Size: fi.Size(), Mode: uint32(fi.Mode().Perm()),
		IsDir: fi.IsDir(), MTime: fi.ModTime().Unix()}, nil
}

// Read reads up to MaxReadBytes; larger files are rejected for the editor.
func (f *FS) Read(ctx context.Context, p string, limit int64) ([]byte, error) {
	if limit <= 0 {
		limit = MaxReadBytes
	}
	r, err := f.c.Open(p)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	buf := make([]byte, limit+1)
	n, err := io.ReadFull(r, buf)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return nil, err
	}
	if int64(n) > limit {
		return nil, &TooLargeError{Path: p, Limit: limit}
	}
	return buf[:n], nil
}

// TooLargeError signals an editor read above the cap.
type TooLargeError struct {
	Path  string
	Limit int64
}

func (e *TooLargeError) Error() string {
	return fmt.Sprintf("%s exceeds read limit of %d bytes", e.Path, e.Limit)
}

// currentSHA256 returns the hex sha256 of an existing remote file or "".
func (f *FS) currentSHA256(p string) (string, error) {
	r, err := f.c.Open(p)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	defer r.Close()
	h := sha256.New()
	if _, err := io.Copy(h, r); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// Write writes to a temp file then renames atomically. When expectedSHA is
// non-empty the current file must match, otherwise ModifiedError is returned
// and nothing is written.
func (f *FS) Write(ctx context.Context, p string, content []byte, expectedSHA string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if expectedSHA != "" {
		cur, err := f.currentSHA256(p)
		if err != nil {
			return "", err
		}
		if cur != expectedSHA {
			return "", &ModifiedError{Path: p, Expected: expectedSHA, Actual: cur}
		}
	}
	dir := path.Dir(p)
	tmp := fmt.Sprintf("%s/.bdk-%d.tmp", dir, time.Now().UnixNano())
	w, err := f.c.Create(tmp)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(w, bytes.NewReader(content)); err != nil {
		_ = w.Close()
		_ = f.c.Remove(tmp)
		return "", err
	}
	if err := w.Close(); err != nil {
		_ = f.c.Remove(tmp)
		return "", err
	}
	// POSIX rename is atomic within a directory.
	if err := f.c.Rename(tmp, p); err != nil {
		_ = f.c.Remove(tmp)
		return "", err
	}
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:]), nil
}

// ModifiedError is the optimistic-lock conflict.
type ModifiedError struct {
	Path, Expected, Actual string
}

func (e *ModifiedError) Error() string {
	return fmt.Sprintf("%s changed on server: expected %s actual %s", e.Path, e.Expected, e.Actual)
}

// Mkdir creates a directory (parents required to exist).
func (f *FS) Mkdir(_ context.Context, p string) error { return f.c.Mkdir(p) }

// Rename moves a path.
func (f *FS) Rename(_ context.Context, from, to string) error { return f.c.Rename(from, to) }

// Remove deletes a file or empty directory.
func (f *FS) Remove(_ context.Context, p string) error { return f.c.Remove(p) }

// Upload streams a reader to a remote temp file then renames it.
func (f *FS) Upload(ctx context.Context, r io.Reader, remote string, progress func(int64)) error {
	dir := path.Dir(remote)
	tmp := fmt.Sprintf("%s/.bdk-up-%d.tmp", dir, time.Now().UnixNano())
	w, err := f.c.Create(tmp)
	if err != nil {
		return err
	}
	pr := &progReader{r: r, cb: progress}
	_, copyErr := io.Copy(w, pr)
	closeErr := w.Close()
	if copyErr != nil {
		_ = f.c.Remove(tmp)
		return copyErr
	}
	if closeErr != nil {
		_ = f.c.Remove(tmp)
		return closeErr
	}
	if err := f.c.Rename(tmp, remote); err != nil {
		_ = f.c.Remove(tmp)
		return err
	}
	return ctx.Err()
}

// Download streams a remote file to w with progress.
func (f *FS) Download(ctx context.Context, remote string, w io.Writer, progress func(int64)) error {
	r, err := f.c.Open(remote)
	if err != nil {
		return err
	}
	defer r.Close()
	pr := &progReader{r: r, cb: progress}
	_, err = io.Copy(w, pr)
	if err != nil {
		return err
	}
	return ctx.Err()
}

type progReader struct {
	r  io.Reader
	n  int64
	cb func(int64)
}

func (p *progReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	p.n += int64(n)
	if p.cb != nil && n > 0 {
		p.cb(p.n)
	}
	return n, err
}
