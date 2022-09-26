package boltedsftp

import (
	"errors"
	"io"
	"os"
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
	return nil, errors.New("not yet implemented")
}

func (db dbHandler) Filewrite(req *sftp.Request) (io.WriterAt, error) {
	return nil, errors.New("not yet implemented")
}

func (db dbHandler) Filecmd(req *sftp.Request) error {
	return errors.New("not yet implemented")
}

func (db dbHandler) Filelist(req *sftp.Request) (sftp.ListerAt, error) {

	parts := []string{}
	if req.Filepath != "" && req.Filepath != "/" {
		parts = strings.Split(req.Filepath, "/")
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
