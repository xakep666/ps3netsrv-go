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

// SystemRoot needed to abstract *os.Root and it's relaxed implementation that allows outside symlinks.
type SystemRoot interface {
	Open(path string) (*os.File, error)
	Create(path string) (*os.File, error)
	Stat(path string) (fs.FileInfo, error)
	Remove(path string) error
	Mkdir(path string, mode os.FileMode) error
}

// FileOpener is a wrapper that incapsulates a path detection/translation logic.
// It's methods return [fs.ErrNotExist] in case it didn't perform.
type FileOpener interface {
	Open(fsys SystemRoot, path string) (handler.File, error)
	Stat(fsys SystemRoot, path string) (fs.FileInfo, error)
}

// FileWrapper applies on already open file. It returns unmodified input and no error if it's not applied.
type FileWrapper interface {
	WrapFile(fsys SystemRoot, file handler.File) (handler.File, error)
}

type FS struct {
	root     SystemRoot
	openers  []FileOpener  // iterated until success
	wrappers []FileWrapper // wraps file in chain, used in case openers didn't success
}

func NewFS(root SystemRoot, openers []FileOpener, wrappers []FileWrapper) *FS {
	return &FS{
		root:     root,
		openers:  openers,
		wrappers: wrappers,
	}
}

func (fsys *FS) Open(path string) (handler.File, error) {
	path = strings.TrimPrefix(path, string(filepath.Separator))
	for i, opener := range fsys.openers {
		file, err := opener.Open(fsys.root, path)
		switch {
		case errors.Is(err, nil):
			return file, err
		case errors.Is(err, fs.ErrNotExist):
			continue
		default:
			return nil, fmt.Errorf("opener %d: %w", i, err)
		}
	}

	// if we're here try to open raw requested path
	f, err := fsys.root.Open(path)
	if err != nil {
		return f, err
	}

	// special wrapper for directories to process ReadDir with opener's Stat
	stat, err := f.Stat()
	if err != nil {
		return f, err
	}

	ret := handler.File(f)
	if stat.IsDir() {
		ret = &dirWrapper{
			File:    f,
			fsys:    fsys.root,
			openers: fsys.openers,
		}
	}

	for i, wrapper := range fsys.wrappers {
		ret, err = wrapper.WrapFile(fsys.root, ret)
		if err != nil {
			return nil, fmt.Errorf("wrapper %d: %w", i, err)
		}
	}

	return ret, nil
}

func (fsys *FS) Create(name string) (handler.WritableFile, error) {
	return fsys.root.Create(strings.TrimPrefix(name, string(filepath.Separator)))
}

func (fsys *FS) Stat(name string) (fs.FileInfo, error) {
	name = strings.TrimPrefix(name, string(filepath.Separator))
	for i, opener := range fsys.openers {
		st, err := opener.Stat(fsys.root, name)
		switch {
		case errors.Is(err, nil):
			return st, err
		case errors.Is(err, fs.ErrNotExist):
			continue
		default:
			return nil, fmt.Errorf("opener %d: %w", i, err)
		}
	}

	return fsys.root.Stat(name)
}

func (fsys *FS) Remove(name string) error {
	return fsys.root.Remove(strings.TrimPrefix(name, string(filepath.Separator)))
}

func (fsys *FS) Mkdir(name string, mode os.FileMode) error {
	return fsys.root.Mkdir(strings.TrimPrefix(name, string(filepath.Separator)), mode)
}
