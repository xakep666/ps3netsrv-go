package fs

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/spf13/afero"
)

type fileType int

const (
	genericFile fileType = iota
	virtualISOFile
	virtualPS3ISOFile
)

const (
	virtualISOMask    = string(filepath.Separator) + "***DVD***"
	virtualPS3ISOMask = string(filepath.Separator) + "***PS3***"
)

type (
	privateFs   = afero.Fs // alias to embed Fs but not expose it
	privateFile = afero.File
)

type FS struct {
	afero.Fs
}

func translatePath(path string) (string, fileType) {
	switch {
	case strings.HasPrefix(path, virtualISOMask+string(filepath.Separator)):
		return strings.TrimPrefix(path, virtualISOMask), virtualISOFile
	case strings.HasPrefix(path, virtualPS3ISOMask+string(filepath.Separator)):
		return strings.TrimPrefix(path, virtualPS3ISOMask), virtualPS3ISOFile
	default:
		return path, genericFile
	}
}

func (fsys *FS) Open(path string) (afero.File, error) {
	return fsys.OpenFile(path, os.O_RDONLY, 0)
}

func (fsys *FS) OpenFile(path string, flags int, perm fs.FileMode) (afero.File, error) {
	modificationsEnabled := flags&(os.O_WRONLY|os.O_APPEND|os.O_TRUNC|os.O_CREATE) != 0

	path, typ := translatePath(path)
	if typ == virtualPS3ISOFile || typ == virtualISOFile {
		if modificationsEnabled {
			return nil, syscall.EPERM
		}

		return NewVirtualISO(fsys.Fs, path, typ == virtualPS3ISOFile)
	}

	f, err := fsys.Fs.OpenFile(path, flags, perm)
	if err != nil || modificationsEnabled { // do not try wrappers if modifications enabled
		return f, err
	}

	key, err := tryGetRedumpKey(fsys.Fs, path)
	switch {
	case errors.Is(err, nil):
		ef, err := NewEncryptedISO(f, key, false)
		if err != nil {
			_ = f.Close()
			return nil, err
		}

		return ef, nil
	case errors.Is(err, afero.ErrFileNotFound):
		// pass
	default:
		_ = f.Close()
		return nil, fmt.Errorf("redump key read failed: %w", err)
	}

	key, err = Test3k3yImage(f)
	switch {
	case errors.Is(err, nil) && len(key) != 0:
		ef, err := NewEncryptedISO(f, key, false)
		if err != nil {
			_ = f.Close()
			return nil, err
		}

		_3k3yf, err := NewISO3k3y(ef)
		if err != nil {
			_ = f.Close()
			return nil, err
		}

		return _3k3yf, nil
	case errors.Is(err, nil) && len(key) == 0:
		_3k3yf, err := NewISO3k3y(f)
		if err != nil {
			_ = f.Close()
			return nil, err
		}

		return _3k3yf, nil
	case errors.Is(err, ErrNot3k3y):
		// pass
	default:
		_ = f.Close()
		return nil, fmt.Errorf("3k3y test failed: %w", err)
	}

	return f, nil
}
