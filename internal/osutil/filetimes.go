package osutil

import (
	"io/fs"
	"time"

	"github.com/djherbis/times"

	"github.com/xakep666/ps3netsrv-go/internal/handler"
	pkgfs "github.com/xakep666/ps3netsrv-go/pkg/fs"
)

type FileTimesOpener struct{}

func (FileTimesOpener) Open(fsys pkgfs.SystemRoot, name string) (handler.File, error) {
	// should not open file here, this is for regular files
	return nil, fs.ErrNotExist
}

func (FileTimesOpener) Stat(fsys pkgfs.SystemRoot, name string) (ret fs.FileInfo, err error) {
	// perform a normal stat first
	fi, err := fsys.Stat(name)
	if err != nil {
		return fi, err
	}

	return wrapFileInfoForExtendedTimes(fi), nil
}

type FileTimesWrapper struct{}

func (FileTimesWrapper) WrapFile(fsys pkgfs.SystemRoot, f handler.File) (handler.File, error) {
	return &fileWithTimesStat{f}, nil
}

type fileWithTimesStat struct {
	handler.File
}

func (f *fileWithTimesStat) Stat() (fs.FileInfo, error) {
	fi, err := f.File.Stat()
	if err != nil {
		return fi, err
	}

	return wrapFileInfoForExtendedTimes(fi), nil
}

type fileInfoWithTimes struct {
	fs.FileInfo
	spec times.Timespec
}

func (f fileInfoWithTimes) AccessTime() time.Time {
	return f.spec.AccessTime()
}

func (f fileInfoWithTimes) ChangeTime() time.Time {
	return f.spec.ChangeTime()
}

func wrapFileInfoForExtendedTimes(fi fs.FileInfo) (ret fs.FileInfo) {
	sys := fi.Sys()
	if sys == nil {
		return fi
	}

	defer func() {
		// this is small hack to avoid problems with times.Get, because it can panic if fi.Sys() type will be unexpected
		if r := recover(); r != nil {
			ret = fi
		}
	}()

	return &fileInfoWithTimes{FileInfo: fi, spec: times.Get(fi)}
}
