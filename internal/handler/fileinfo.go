package handler

import (
	"io/fs"
	"time"

	"github.com/djherbis/times"
)

type fileInfoWithAccessTime struct {
	fs.FileInfo
	spec times.Timespec
}

func (f fileInfoWithAccessTime) AccessTime() time.Time {
	return f.spec.AccessTime()
}

type fileInfoWithAccessChangeTime struct {
	fileInfoWithAccessTime
}

func (f fileInfoWithAccessChangeTime) ChangeTime() time.Time {
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

	spec := times.Get(fi)
	if spec.HasChangeTime() {
		return fileInfoWithAccessChangeTime{fileInfoWithAccessTime{fi, spec}}
	}

	return fileInfoWithAccessTime{fi, spec}
}
