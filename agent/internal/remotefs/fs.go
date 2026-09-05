// Package remotefs serves the bounded filesystem operations requested by the
// server. Writes use temp-file+rename and support an optimistic SHA256 check.
package remotefs

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"bd-agent/internal/proto"
)

// DirEntry mirrors the server DTO.
type DirEntry struct {
	Name  string `json:"name"`
	Size  int64  `json:"size"`
	Mode  uint32 `json:"mode"`
	IsDir bool   `json:"isDir"`
	MTime int64  `json:"mtime"`
}

// FileStat mirrors the server DTO.
type FileStat struct {
	Name  string `json:"name"`
	Size  int64  `json:"size"`
	Mode  uint32 `json:"mode"`
	IsDir bool   `json:"isDir"`
	MTime int64  `json:"mtime"`
}

const maxReadBytes = 16 << 20

// Handle processes one fs_req frame and returns the fs_res frame.
func Handle(_ context.Context, f proto.Frame) proto.Frame {
	res := proto.Frame{T: "fs_res", ID: f.ID}
	var err error
	switch f.Op {
	case "list":
		err = opList(&res, f.Path)
	case "stat":
		err = opStat(&res, f.Path)
	case "read":
		err = opRead(&res, f.Path)
	case "write":
		err = opWrite(&res, f.Path, f.ContentB64, f.ExpectedSHA)
	case "mkdir":
		err = os.MkdirAll(f.Path, 0o755)
	case "rename":
		err = os.Rename(f.Path, f.Dest)
	case "remove":
		err = os.RemoveAll(f.Path)
	default:
		err = fmt.Errorf("unknown op %q", f.Op)
	}
	if err != nil {
		res.ErrorText = err.Error()
	}
	return res
}

func opList(res *proto.Frame, p string) error {
	entries, err := os.ReadDir(p)
	if err != nil {
		return err
	}
	out := make([]DirEntry, 0, len(entries))
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, DirEntry{
			Name: e.Name(), Size: info.Size(), Mode: uint32(info.Mode()),
			IsDir: e.IsDir(), MTime: info.ModTime().Unix(),
		})
	}
	b, _ := json.Marshal(out)
	res.Payload = b
	return nil
}

func opStat(res *proto.Frame, p string) error {
	info, err := os.Stat(p)
	if err != nil {
		return err
	}
	b, _ := json.Marshal(FileStat{
		Name: filepath.Base(p), Size: info.Size(), Mode: uint32(info.Mode()),
		IsDir: info.IsDir(), MTime: info.ModTime().Unix()})
	res.Payload = b
	return nil
}

func opRead(res *proto.Frame, p string) error {
	f, err := os.Open(p)
	if err != nil {
		return err
	}
	defer f.Close()
	lr := io.LimitReader(f, maxReadBytes)
	b, err := io.ReadAll(lr)
	if err != nil {
		return err
	}
	res.ContentB64 = base64.StdEncoding.EncodeToString(b)
	return nil
}

func opWrite(res *proto.Frame, p, contentB64, expectedSHA string) error {
	content, err := base64.StdEncoding.DecodeString(contentB64)
	if err != nil {
		return err
	}
	if expectedSHA != "" {
		cur, err := hashFile(p)
		if err == nil && cur != expectedSHA {
			return errors.New("modified: remote file changed since last read")
		}
	}
	dir := filepath.Dir(p)
	tmp, err := os.CreateTemp(dir, ".bdk-write-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, p); err != nil {
		return err
	}
	sum, err := hashFile(p)
	if err != nil {
		return err
	}
	b, _ := json.Marshal(map[string]string{"sha256": sum})
	res.Payload = b
	return nil
}

func hashFile(p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
