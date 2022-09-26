package boltedsftp

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path"
	"strings"
	"time"

	"github.com/draganm/bolted"
	"github.com/draganm/bolted/dbpath"
	"github.com/pkg/sftp"
)

type dbHandler struct {
	bolted.Database
}

func (db dbHandler) Fileread(req *sftp.Request) (io.ReaderAt, error) {
	var d []byte
	parts := strings.Split(path.Clean(req.Filepath), "/")
	if parts[0] == "" {
		parts = parts[1:]
	}
	dbpath := dbpath.ToPath(parts...)
	err := bolted.SugaredRead(db.Database, func(tx bolted.SugaredReadTx) error {
		d = tx.Get(dbpath)
		return nil
	})
	if errors.Is(err, bolted.ErrNotFound) {
		return nil, os.ErrNotExist
	}
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(d), nil
}

func (db dbHandler) Filewrite(req *sftp.Request) (io.WriterAt, error) {
	return nil, errors.New("not yet implemented")
}

func (db dbHandler) Filecmd(req *sftp.Request) error {
	return errors.New("not yet implemented")
}

func (db dbHandler) Filelist(req *sftp.Request) (sftp.ListerAt, error) {
	parts := []string{}
	pth := path.Clean(req.Filepath)

	if pth != "" && pth != "/" {
		parts = strings.Split(req.Filepath, "/")
		if len(parts[0]) == 0 {
			parts = parts[1:]
		}
	}

	return &lister{db: db.Database, path: dbpath.Path(parts)}, nil
}

type lister struct {
	db   bolted.Database
	path dbpath.Path
}

type minimalFileInfo struct {
	name  string
	isDir bool
	size  int64
}

func (mfi minimalFileInfo) Name() string {
	return mfi.name
}

func (mfi minimalFileInfo) Size() int64 {
	return mfi.size
}

func (mfi minimalFileInfo) Mode() os.FileMode {
	if mfi.isDir {
		return os.ModeDir | 0700
	}
	return 0700
}

func (mfi minimalFileInfo) ModTime() time.Time {
	return time.Now()
}

func (mfi minimalFileInfo) IsDir() bool {
	return mfi.isDir
}

func (mfi minimalFileInfo) Sys() any {
	return nil
}

func (l *lister) ListAt(infos []os.FileInfo, from int64) (cnt int, err error) {
	if from == 0 && len(infos) == 1 {
		infos[0] = minimalFileInfo{name: ".", isDir: true}
		infos = infos[1:]
		return 1, nil
	}
	// TODO remember the last entry and seek to it.
	err = bolted.SugaredRead(l.db, func(tx bolted.SugaredReadTx) error {
		if !tx.Exists(l.path) {
			return os.ErrNotExist
		}
		for it := tx.Iterator(l.path); !it.IsDone(); it.Next() {
			if from > 0 {
				from--
				continue
			}
			infos[cnt] = minimalFileInfo{name: it.GetKey(), isDir: it.GetValue() == nil, size: int64(len(it.GetValue()))}
			cnt++
			if cnt == len(infos) {
				return nil
			}
		}
		if cnt == 0 {
			return io.EOF
		}

		return nil

	})

	if err != nil {
		return cnt, err
	}
	return cnt, nil
}
