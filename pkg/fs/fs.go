package fs

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/xakep666/ps3netsrv-go/internal/handler"
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
	privateFile = handler.File
)

type FS struct {
	root *os.Root
}

func NewFS(root string) (*FS, error) {
	rootFS, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}

	return &FS{root: rootFS}, nil
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

func (fsys *FS) Open(path string) (handler.File, error) {
	path, typ := translatePath(path)
	if typ == virtualPS3ISOFile || typ == virtualISOFile {
		return NewVirtualISO(fsys, path, typ == virtualPS3ISOFile)
	}

	f, err := fsys.root.Open(path)
	if err != nil {
		return f, err
	}

	// do not try wrappers if it is a directory
	stat, err := f.Stat()
	if err != nil {
		return f, err
	}

	if stat.IsDir() {
		return f, nil
	}

	key, err := tryGetRedumpKey(fsys, path)
	switch {
	case errors.Is(err, nil):
		ef, err := NewEncryptedISO(f, key, false)
		if err != nil {
			_ = f.Close()
			return nil, err
		}

		return ef, nil
	case errors.Is(err, fs.ErrNotExist):
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

func (fsys *FS) Create(name string) (handler.WritableFile, error) {
	return fsys.root.Create(name)
}

func (fsys *FS) Stat(name string) (fs.FileInfo, error) {
	return fsys.root.Stat(name)
}

func (fsys *FS) Remove(name string) error {
	return fsys.root.Remove(name)
}

func (fsys *FS) Mkdir(name string, mode os.FileMode) error {
	return fsys.root.Mkdir(name, mode)
}

func (fsys *FS) WriteFile(name string, data []byte, mode os.FileMode) error {
	return fsys.root.WriteFile(name, data, mode)
}

func (fsys *FS) Close() error {
	return fsys.root.Close()
}
