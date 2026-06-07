package seekablezstd

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"path/filepath"
	"strings"

	seekable "github.com/SaveTheRbtz/zstd-seekable-format-go/pkg"
	"github.com/klauspost/compress/zstd"
	"github.com/xakep666/ps3netsrv-go/internal/handler"
	"github.com/xakep666/ps3netsrv-go/internal/logutil"
	pkgfs "github.com/xakep666/ps3netsrv-go/pkg/fs"
)

const (
	isoExt = ".iso"
	zstExt = ".zst"
)

type Opener struct{}

func (Opener) canProceed(path string) bool {
	ext1 := filepath.Ext(path)
	if strings.EqualFold(ext1, zstExt) {
		return true
	}

	ext2 := filepath.Ext(strings.TrimSuffix(path, ext1))

	return strings.EqualFold(ext1, isoExt) && strings.EqualFold(ext2, zstExt)
}

func (o Opener) Open(ctx context.Context, fsys *pkgfs.FS, path string) (handler.File, error) {
	if !o.canProceed(path) {
		return nil, pkgfs.ErrTryNext
	}

	if ext := filepath.Ext(path); strings.EqualFold(ext, isoExt) {
		path = strings.TrimSuffix(path, ext)
	}

	slog.DebugContext(ctx, "Trying to open Seekable ZSTD file", slog.String("path", path))
	f, err := fsys.SystemRoot().Open(path) // prevent recursion
	if err != nil {
		return nil, err
	}

	zr, err := zstd.NewReader(nil)
	if err != nil {
		return nil, err
	}

	szr, err := seekable.NewReader(f, zr, seekable.WithReaderLogger(slog.Default()))
	if err != nil {
		return nil, err
	}

	return &file{
		originalFile: f,
		openPath:     path,
		reader:       szr,
	}, nil
}

func (o Opener) Stat(ctx context.Context, fsys *pkgfs.FS, path string) (fs.FileInfo, error) {
	cf, err := o.Open(ctx, fsys, path)
	switch {
	case errors.Is(err, nil):
		// pass
	case errors.Is(err, pkgfs.ErrTryNext):
		return nil, err
	default:
		// report as try next file if open fails with unhandled error to not block directory listing
		slog.ErrorContext(ctx, "Seekable ZSTD file open for stat failed, report as try next", logutil.ErrorAttr(err))
		return nil, pkgfs.ErrTryNext
	}

	defer cf.Close()

	return cf.Stat()
}

func (Opener) Name() string {
	return "seekable-zstd"
}

type file struct {
	originalFile handler.File
	openPath     string

	reader *seekable.Reader
}

func (f *file) Read(b []byte) (int, error) {
	return f.reader.Read(b)
}

func (f *file) Close() error {
	return f.reader.Close()
}

func (f *file) ReadDir(n int) ([]fs.DirEntry, error) {
	return nil, errors.ErrUnsupported
}

func (f *file) Seek(offset int64, whence int) (int64, error) {
	return f.reader.Seek(offset, whence)
}

func (f *file) Name() string {
	// add .iso to make ps3 recognise it as disk image
	return f.openPath + isoExt
}

type fileInfo struct {
	fs.FileInfo
	uncompressedSize int64
}

func (fi *fileInfo) Size() int64 {
	return fi.uncompressedSize
}

func (fi *fileInfo) Mode() fs.FileMode {
	return fi.FileInfo.Mode() | fs.ModeIrregular
}

func (fi *fileInfo) Name() string {
	return fi.FileInfo.Name() + isoExt
}

func (fi *fileInfo) Unwrap() fs.FileInfo {
	return fi.FileInfo
}

func (f *file) Stat() (fs.FileInfo, error) {
	fi, err := f.originalFile.Stat()
	if err != nil {
		return nil, err
	}

	currOffset, err := f.reader.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, err
	}

	size, err := f.reader.Seek(0, io.SeekEnd)
	if err != nil {
		return nil, err
	}

	_, err = f.reader.Seek(currOffset, io.SeekStart)
	if err != nil {
		return nil, err
	}

	return &fileInfo{
		FileInfo:         fi,
		uncompressedSize: size,
	}, nil
}

func (c *file) Unwrap() handler.File {
	return c.originalFile
}
