package fs

import (
	"io/fs"
	"os"
	"path/filepath"
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
	virtualISOMask    = string(filepath.Separator) + "***DVD***"
	virtualPS3ISOMask = string(filepath.Separator) + "***PS3***"
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

func (fsys *FS) Open(path string) (afero.File, error) {
	path, typ := translatePath(path)
	if typ == GenericFile {
		return fsys.Fs.Open(path)
	}

	return NewVirtualISO(fsys.Fs, path, typ == PS3ISOFile)
}

func (fsys *FS) OpenFile(path string, flags int, perm fs.FileMode) (afero.File, error) {
	path, typ := translatePath(path)
	if typ == GenericFile {
		return fsys.Fs.OpenFile(path, flags, perm)
	}

	if flags&(os.O_RDWR|os.O_WRONLY|os.O_APPEND) != 0 {
		return nil, syscall.EPERM
	}

	return NewVirtualISO(fsys.Fs, path, typ == PS3ISOFile)
}
