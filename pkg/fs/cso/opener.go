package cso

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/xakep666/ps3netsrv-go/internal/handler"
	"github.com/xakep666/ps3netsrv-go/internal/logutil"
	pkgfs "github.com/xakep666/ps3netsrv-go/pkg/fs"
)

const (
	isoExt = ".iso"
	csoExt = ".cso"
	zsoExt = ".zso"
)

type Opener struct{}

func (Opener) canProceed(path string) bool {
	ext1 := filepath.Ext(path)
	if strings.EqualFold(ext1, csoExt) || strings.EqualFold(ext1, zsoExt) {
		return true
	}

	ext2 := filepath.Ext(strings.TrimSuffix(path, ext1))
	return strings.EqualFold(ext1, isoExt) && (strings.EqualFold(ext2, csoExt) || strings.EqualFold(ext2, zsoExt))
}

func (o Opener) openSystem(ctx context.Context, fsys *pkgfs.FS, path string) (handler.File, error) {
	// .cso/zso file will be reported and requested as .cso.iso
	if !o.canProceed(path) {
		return nil, pkgfs.ErrTryNext
	}

	if ext := filepath.Ext(path); strings.EqualFold(ext, isoExt) {
		path = strings.TrimSuffix(path, ext)
	}

	slog.DebugContext(ctx, "Trying to open CSO file", slog.String("path", path))
	return fsys.SystemRoot().Open(path) // prevent recursion
}

func (o Opener) Open(ctx context.Context, fsys *pkgfs.FS, path string) (handler.File, error) {
	f, err := o.openSystem(ctx, fsys, path)
	if err != nil {
		return nil, err
	}
	cf, err := NewFile(f)
	if err != nil {
		return nil, err
	}

	return &fileView{File: cf, openPath: path}, nil
}

func (o Opener) Stat(ctx context.Context, fsys *pkgfs.FS, path string) (fs.FileInfo, error) {
	f, err := o.openSystem(ctx, fsys, path)
	switch {
	case errors.Is(err, nil):
		// pass
	case errors.Is(err, pkgfs.ErrTryNext):
		return nil, err
	default:
		// report as try next file if open fails with unhandled error to not block directory listing
		slog.ErrorContext(ctx, "CSO file open for stat failed, report as try next", logutil.ErrorAttr(err))
		return nil, pkgfs.ErrTryNext
	}

	defer f.Close()

	hdr, err := ReadHeader(f)
	if err != nil {
		slog.ErrorContext(ctx, "CSO file header read failed, report as try next", logutil.ErrorAttr(err))
		return nil, pkgfs.ErrTryNext
	}

	fi, err := f.Stat()
	if err != nil {
		slog.ErrorContext(ctx, "CSO file stat failed, report as try next", logutil.ErrorAttr(err))
		return nil, pkgfs.ErrTryNext
	}

	return &fakeNameFileStat{
		FileInfo: &csoStat{
			FileInfo: fi,
			hdr:      hdr,
		},
	}, nil
}

func (Opener) Name() string {
	return "cso"
}

type fileView struct {
	handler.File
	openPath string
}

func (c *fileView) Name() string {
	// add .iso to make ps3 recognise it as disk image
	return c.openPath + isoExt
}

func (c *fileView) Unwrap() handler.File {
	return c.File
}

func (c *fileView) Stat() (fs.FileInfo, error) {
	fi, err := c.File.Stat()
	if err != nil {
		return nil, err
	}
	return &fakeNameFileStat{fi}, nil
}

type fakeNameFileStat struct {
	fs.FileInfo
}

func (c *fakeNameFileStat) Name() string {
	return c.FileInfo.Name() + isoExt
}

func (c *fakeNameFileStat) Unwrap() fs.FileInfo {
	return c.FileInfo
}
