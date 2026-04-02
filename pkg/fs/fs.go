package fs

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
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
	Open(ctx context.Context, fsys *FS, path string) (handler.File, error)
	Stat(ctx context.Context, fsys *FS, path string) (fs.FileInfo, error)
	Name() string
}

// FileWrapper applies on already open file. It returns unmodified input and no error if it's not applied.
type FileWrapper interface {
	WrapFile(ctx context.Context, fsys *FS, file handler.File) (handler.File, error)
	Name() string
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

func (fsys *FS) Open(ctx context.Context, path string) (handler.File, error) {
	path = strings.TrimPrefix(path, string(filepath.Separator))
	log := slog.With(slog.String("path_request", path), slog.String("fs_op", "open"))

	var file handler.File
	var err error
openerLoop:
	for _, opener := range fsys.openers {
		log.DebugContext(ctx, "Trying opener", slog.String("opener", opener.Name()))
		file, err = opener.Open(ctx, fsys, path)
		switch {
		case errors.Is(err, nil):
			log.DebugContext(ctx, "Opener succeeded", slog.String("opener", opener.Name()))
			break openerLoop
		case errors.Is(err, fs.ErrNotExist):
			continue
		default:
			return nil, fmt.Errorf("opener %s: %w", opener.Name(), err)
		}
	}

	// if we're here try to open raw requested path
	if file == nil {
		log.DebugContext(ctx, "Openers didn't succeed, trying native")
		file, err = fsys.root.Open(path)
		if err != nil {
			return nil, err
		}
	}

	// special wrapper for directories to process ReadDir with opener's Stat
	stat, err := file.Stat()
	if err != nil {
		return nil, err
	}

	if stat.IsDir() {
		file = &dirWrapper{
			File: file,

			ctx:      context.WithoutCancel(ctx),
			fsys:     fsys,
			openPath: path,
			openers:  fsys.openers,
		}
	}

	for _, wrapper := range fsys.wrappers {
		log.Debug("Applying wrapper", slog.String("wrapper", wrapper.Name()))
		file, err = wrapper.WrapFile(ctx, fsys, file)
		if err != nil {
			return nil, fmt.Errorf("wrapper %s: %w", wrapper.Name(), err)
		}
	}

	return file, nil
}

func (fsys *FS) Create(ctx context.Context, name string) (handler.WritableFile, error) {
	return fsys.root.Create(strings.TrimPrefix(name, string(filepath.Separator)))
}

func (fsys *FS) Stat(ctx context.Context, name string) (fs.FileInfo, error) {
	name = strings.TrimPrefix(name, string(filepath.Separator))
	log := slog.With(slog.String("path_request", name), slog.String("fs_op", "stat"))
	for i, opener := range fsys.openers {
		log.DebugContext(ctx, "Trying opener", slog.String("opener", opener.Name()))
		st, err := opener.Stat(ctx, fsys, name)
		switch {
		case errors.Is(err, nil):
			log.DebugContext(ctx, "Opener succeeded", slog.String("opener", opener.Name()))
			return st, err
		case errors.Is(err, fs.ErrNotExist):
			continue
		default:
			return nil, fmt.Errorf("opener %d: %w", i, err)
		}
	}

	log.DebugContext(ctx, "Openers didn't succeed, trying native")
	return fsys.root.Stat(name)
}

func (fsys *FS) Remove(ctx context.Context, name string) error {
	return fsys.root.Remove(strings.TrimPrefix(name, string(filepath.Separator)))
}

func (fsys *FS) Mkdir(ctx context.Context, name string, mode os.FileMode) error {
	return fsys.root.Mkdir(strings.TrimPrefix(name, string(filepath.Separator)), mode)
}

func (fsys *FS) SystemRoot() SystemRoot {
	return fsys.root
}
