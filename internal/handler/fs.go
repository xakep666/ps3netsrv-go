package handler

import (
	"errors"
	"io"
	"io/fs"
	"path"
)

type File interface {
	fs.ReadDirFile
	io.ReaderAt
	io.Seeker
	Name() string
}

type WritableFile interface {
	File
	io.Writer
}

type FS interface {
	Open(name string) (File, error)
	Create(name string) (WritableFile, error)
	Stat(name string) (fs.FileInfo, error)
	Remove(name string) error
	Mkdir(name string, mode fs.FileMode) error
}

func WalkDir(fsys FS, root string, fn fs.WalkDirFunc) error {
	info, err := fsys.Stat(root)
	if err != nil {
		err = fn(root, nil, err)
	} else {
		err = walkDir(fsys, root, fs.FileInfoToDirEntry(info), fn)
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

func walkDir(fsys FS, name string, d fs.DirEntry, walkDirFn fs.WalkDirFunc) error {
	if err := walkDirFn(name, d, nil); err != nil || !d.IsDir() {
		if errors.Is(err, fs.SkipDir) && d.IsDir() {
			// Successfully skipped directory.
			err = nil
		}
		return err
	}

	dirFile, err := fsys.Open(name)
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
			name1 := path.Join(name, d1.Name())
			if err := walkDir(fsys, name1, d1, walkDirFn); err != nil {
				if errors.Is(err, fs.SkipDir) {
					break
				}
				return err
			}
		}
	}
}
