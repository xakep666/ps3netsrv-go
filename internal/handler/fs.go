package handler

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"path/filepath"
)

type File interface {
	fs.ReadDirFile
	io.Seeker
	Name() string
}

type WritableFile interface {
	File
	io.Writer
}

type FS interface {
	Open(ctx context.Context, name string) (File, error)
	Create(ctx context.Context, name string) (WritableFile, error)
	Stat(ctx context.Context, name string) (fs.FileInfo, error)
	Remove(ctx context.Context, name string) error
	Mkdir(ctx context.Context, name string, mode fs.FileMode) error
}

func WalkDir(ctx context.Context, fsys FS, root string, fn fs.WalkDirFunc) error {
	info, err := fsys.Stat(ctx, root)
	if err != nil {
		err = fn(root, nil, err)
	} else {
		err = walkDir(ctx, fsys, root, fs.FileInfoToDirEntry(info), fn)
	}
	if errors.Is(err, fs.SkipDir) || errors.Is(err, fs.SkipAll) {
		return nil
	}
	return err
}

func reportDirError(name string, err error, d fs.DirEntry, walkDirFn fs.WalkDirFunc) error {
	err = walkDirFn(name, d, err)
	if err != nil {
		if errors.Is(err, fs.SkipDir) && d.IsDir() {
			// Successfully skipped directory.
			err = nil
		}
	}
	return err
}

func walkDir(ctx context.Context, fsys FS, name string, d fs.DirEntry, walkDirFn fs.WalkDirFunc) error {
	if err := walkDirFn(name, d, nil); err != nil || !d.IsDir() {
		if errors.Is(err, fs.SkipDir) && d.IsDir() {
			// Successfully skipped directory.
			err = nil
		}
		return err
	}

	dirFile, err := fsys.Open(ctx, name)
	if err != nil {
		return reportDirError(name, err, d, walkDirFn)
	}
	defer dirFile.Close()

	for {
		dirs, err := dirFile.ReadDir(100)
		switch {
		case errors.Is(err, nil):
			// pass
		case errors.Is(err, io.EOF):
			return nil
		default:
			return reportDirError(name, err, d, walkDirFn)
		}

		for _, d1 := range dirs {
			name1 := filepath.Join(name, d1.Name())
			if err := walkDir(ctx, fsys, name1, d1, walkDirFn); err != nil {
				if errors.Is(err, fs.SkipDir) {
					break
				}
				return err
			}
		}
	}
}

// FileAsType works like [errors.AsType] but for [File].
func FileAsType[T File](f File) (T, bool) {
	for {
		if e, ok := f.(T); ok {
			return e, true
		}
		uw, isuw := f.(interface{ Unwrap() File })
		if !isuw {
			return *new(T), false
		}
		f = uw.Unwrap()
	}
}
