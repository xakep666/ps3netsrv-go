package fs

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"path/filepath"
	"unsafe"

	"github.com/xakep666/ps3netsrv-go/internal/handler"
)

type dirWrapper struct {
	handler.File

	fsys     SystemRoot
	openPath string // preserve path which used in Open
	openers  []FileOpener
}

func (dw *dirWrapper) ReadDir(n int) ([]fs.DirEntry, error) {
	items, err := dw.File.ReadDir(n)
	if err != nil {
		return items, err
	}

	if err = dw.modifyEntries(items); err != nil {
		return nil, err
	}

	return items, nil
}

func (dw *dirWrapper) modifyEntries(items []fs.DirEntry) error {
	log := slog.With(slog.String("request_path", dw.openPath), slog.String("op", "readdir"))

	// to reduce allocations during full path generation
	sb := append([]byte(dw.openPath), filepath.Separator)
	fileNameStart := len(sb)

itemsLoop:
	for i, item := range items {
		for j, opener := range dw.openers {
			sb = append(sb[:fileNameStart], item.Name()...)

			log.Debug("Trying opener", slog.String("opener", opener.Name()), slog.String("path_suffix", item.Name()))
			st, err := opener.Stat(dw.fsys, unsafe.String(unsafe.SliceData(sb), len(sb)))
			switch {
			case errors.Is(err, nil):
				log.Debug("Opener succeded", slog.String("opener", opener.Name()), slog.String("path_suffix", item.Name()))
				items[i] = fs.FileInfoToDirEntry(st)
				continue itemsLoop
			case errors.Is(err, fs.ErrNotExist):
				continue
			default:
				return fmt.Errorf("stat via opener %d: %w", j, err)
			}
		}
	}

	return nil
}

func (dw *dirWrapper) Unwrap() handler.File {
	return dw.File
}
