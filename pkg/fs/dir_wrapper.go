package fs

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"unsafe"
)

type dirWrapper struct {
	*os.File

	fsys    SystemRoot
	openers []FileOpener
}

func (dw *dirWrapper) ReadDir(n int) ([]fs.DirEntry, error) {
	items, err := dw.File.ReadDir(n)
	if err != nil {
		return items, err
	}

	log := slog.With(slog.String("request_path", dw.Name()), slog.String("op", "readdir"))

	// to reduce allocations during full path generation
	sb := append([]byte(dw.File.Name()), filepath.Separator)
	fileNameStart := len(sb)

	for i, item := range items {
		for j, opener := range dw.openers {
			sb = append(sb[:fileNameStart], item.Name()...)

			log.Debug("Trying opener", slog.String("opener", opener.Name()), slog.String("path_suffix", item.Name()))
			st, err := opener.Stat(dw.fsys, unsafe.String(unsafe.SliceData(sb), len(sb)))
			switch {
			case errors.Is(err, nil):
				log.Debug("Opener succeded", slog.String("opener", opener.Name()), slog.String("path_suffix", item.Name()))
				items[i] = fs.FileInfoToDirEntry(st)
			case errors.Is(err, fs.ErrNotExist):
				continue
			default:
				return nil, fmt.Errorf("stat via opener %d: %w", j, err)
			}
		}
	}

	return items, nil
}

func (dw *dirWrapper) Readdir(n int) ([]os.DirEntry, error) {
	// this is legacy method but wrap it anyway
	items, err := dw.File.ReadDir(n)
	if err != nil {
		return items, err
	}

	log := slog.With(slog.String("request_path", dw.Name()), slog.String("op", "readdir"))

	// to reduce allocations during full path generation
	sb := append([]byte(dw.File.Name()), filepath.Separator)
	fileNameStart := len(sb)

	for i, item := range items {
		for j, opener := range dw.openers {
			sb = append(sb[:fileNameStart], item.Name()...)

			log.Debug("Trying opener", slog.String("opener", opener.Name()), slog.String("path_suffix", item.Name()))
			st, err := opener.Stat(dw.fsys, unsafe.String(unsafe.SliceData(sb), len(sb)))
			switch {
			case errors.Is(err, nil):
				log.Debug("Opener succeded", slog.String("opener", opener.Name()), slog.String("path_suffix", item.Name()))
				items[i] = fs.FileInfoToDirEntry(st)
			case errors.Is(err, fs.ErrNotExist):
				continue
			default:
				return nil, fmt.Errorf("stat via opener %d: %w", j, err)
			}
		}
	}

	return items, nil
}
