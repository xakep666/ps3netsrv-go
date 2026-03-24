package osutil

import (
	"io/fs"
	"time"

	"github.com/djherbis/times"

	"github.com/xakep666/ps3netsrv-go/internal/handler"
	pkgfs "github.com/xakep666/ps3netsrv-go/pkg/fs"
)

type FileTimesWrapper struct{}

func (FileTimesWrapper) WrapFile(fsys pkgfs.SystemRoot, f handler.File) (handler.File, error) {
	return &fileWithTimesStat{f}, nil
}

func (FileTimesWrapper) Name() string {
	return "file_times"
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

func (f *fileWithTimesStat) Unwrap() handler.File {
	return f.File
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

func (f fileInfoWithTimes) Unwrap() fs.FileInfo {
	return f.FileInfo
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
