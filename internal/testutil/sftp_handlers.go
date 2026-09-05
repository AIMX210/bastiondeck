package testutil

import (
	"io"
	"os"
	"path/filepath"

	"github.com/pkg/sftp"
)

type diskFileGet struct{ root string }

func (d *diskFileGet) Fileread(r *sftp.Request) (io.ReaderAt, error) {
	return os.Open(safeJoin(d.root, r.Filepath))
}

type diskFilePut struct{ root string }

func (d *diskFilePut) Filewrite(r *sftp.Request) (io.WriterAt, error) {
	p := safeJoin(d.root, r.Filepath)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return nil, err
	}
	return os.Create(p)
}

type diskCmd struct{ root string }

// Filecmd implements sftp.FileCmder.
func (d *diskCmd) Filecmd(r *sftp.Request) error {
	p := safeJoin(d.root, r.Filepath)
	switch r.Method {
	case "Rename":
		target := safeJoin(d.root, r.Target)
		return os.Rename(p, target)
	case "Rmdir", "Remove":
		return os.RemoveAll(p)
	case "Mkdir":
		return os.Mkdir(p, 0o755)
	case "Setstat":
		return nil
	}
	return nil
}

type diskList struct{ root string }

func (d *diskList) Filelist(r *sftp.Request) (sftp.ListerAt, error) {
	p := safeJoin(d.root, r.Filepath)
	switch r.Method {
	case "List":
		entries, err := os.ReadDir(p)
		if err != nil {
			return nil, err
		}
		infos := make([]os.FileInfo, 0, len(entries))
		for _, e := range entries {
			if info, err := e.Info(); err == nil {
				infos = append(infos, info)
			}
		}
		return &sliceLister{infos: infos}, nil
	case "Stat":
		fi, err := os.Stat(p)
		if err != nil {
			return nil, err
		}
		return &sliceLister{infos: []os.FileInfo{fi}}, nil
	}
	return &sliceLister{}, nil
}

type sliceLister struct {
	infos []os.FileInfo
}

func (s *sliceLister) ListAt(dst []os.FileInfo, off int64) (int, error) {
	if int(off) >= len(s.infos) {
		return 0, io.EOF
	}
	n := copy(dst, s.infos[off:])
	if int(off)+n < len(s.infos) {
		return n, nil
	}
	return n, io.EOF
}
