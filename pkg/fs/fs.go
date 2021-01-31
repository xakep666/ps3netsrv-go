package fs

import (
	"os"
	"strings"
	"syscall"

	"github.com/spf13/afero"
)

type FileType int

const (
	GenericFile FileType = iota
	ISOFile
	PS3ISOFile
)

const (
	virtualISOMask    = string(os.PathSeparator) + "***DVD***"
	virtualPS3ISOMask = string(os.PathSeparator) + "***PS3***"
)

type FS struct {
	afero.Fs
}

func translatePath(path string) (string, FileType) {
	switch {
	case strings.HasPrefix(path, virtualISOMask):
		return strings.TrimPrefix(path, virtualISOMask), ISOFile
	case strings.HasPrefix(path, virtualPS3ISOMask):
		return strings.TrimPrefix(path, virtualPS3ISOMask), PS3ISOFile
	default:
		return path, GenericFile
	}
}

func (fs *FS) Open(path string) (afero.File, error) {
	path, typ := translatePath(path)
	if typ == GenericFile {
		return fs.Fs.Open(path)
	}

	return NewVirtualISO(fs.Fs, path, typ == PS3ISOFile)
}

func (fs *FS) OpenFile(path string, flags int, perm os.FileMode) (afero.File, error) {
	path, typ := translatePath(path)
	if typ == GenericFile {
		return fs.Fs.OpenFile(path, flags, perm)
	}

	if flags&(os.O_RDWR|os.O_WRONLY|os.O_APPEND) != 0 {
		return nil, syscall.EPERM
	}

	return NewVirtualISO(fs.Fs, path, typ == PS3ISOFile)
}
