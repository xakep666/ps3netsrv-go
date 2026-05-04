package chd

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/xakep666/ps3netsrv-go/internal/handler"
	"github.com/xakep666/ps3netsrv-go/internal/logutil"
	pkgfs "github.com/xakep666/ps3netsrv-go/pkg/fs"
)

const (
	chdExt = ".chd"
)

type Opener struct {
	lib    *LibCHDR
	logger *slog.Logger
}

func NewOpener(logger *slog.Logger) *Opener {
	lib, err := NewLibCHDR(logger)
	if err != nil {
		logger.Warn("libchdr load failed, chd support is disabled", logutil.ErrorAttr(err))
		return nil
	}

	logger.Info("libchdr loaded, enabling chd support")

	return &Opener{
		lib:    lib,
		logger: logger,
	}
}

func (o *Opener) canProceed(path string) bool {
	if o == nil {
		return false
	}

	return strings.EqualFold(filepath.Ext(path), chdExt)
}

func (o *Opener) Open(ctx context.Context, fsys *pkgfs.FS, path string) (handler.File, error) {
	if !o.canProceed(path) {
		return nil, pkgfs.ErrTryNext
	}

	o.logger.DebugContext(ctx, "Trying to open CHD file", slog.String("path", path))
	f, err := fsys.SystemRoot().Open(path) // prevent recursion
	if err != nil {
		return nil, err
	}

	return o.openFromFile(ctx, path, f)
}

func (o *Opener) openFromFile(ctx context.Context, path string, f *os.File) (handler.File, error) {
	cf, err := o.lib.NewFile(f)
	switch {
	case errors.Is(err, nil):
		// pass
	case errors.Is(err, errors.ErrUnsupported):
		_ = f.Close()
		return nil, pkgfs.ErrTryNext
	default:
		_ = f.Close()
		return nil, err
	}

	if cf.Header.IsCDCodesOnly() {
		cdFile, err := cf.AsCD()
		if err != nil {
			_ = f.Close()
			return nil, err
		}
		o.logger.DebugContext(ctx, "Detected CD-encoded CHD, wrapping",
			slog.String("path", path),
			slog.Int("sector_data_size", cdFile.SectorDataSize),
			slog.Int64("sectors_count", cdFile.SectorsCount),
		)
		return cdFile, nil
	}

	return cf, nil
}

func (*Opener) Name() string {
	return "chd"
}

func (o *Opener) Stat(ctx context.Context, fsys *pkgfs.FS, path string) (fs.FileInfo, error) {
	if !o.canProceed(path) {
		return nil, pkgfs.ErrTryNext
	}

	o.logger.DebugContext(ctx, "Trying to open CHD file for stat", slog.String("path", path))
	f, err := fsys.SystemRoot().Open(path) // prevent recursion
	if err != nil {
		// report as try next file if open fails with unhandled error to not block directory listing
		o.logger.ErrorContext(ctx, "CHD file open for stat failed, report as try next", logutil.ErrorAttr(err))
		return nil, pkgfs.ErrTryNext
	}

	// if file is not cd-encoded we don't need to fully open it in libchdr
	hdr, err := o.lib.ReadHeader(f)
	if err != nil {
		o.logger.DebugContext(ctx, "CHD read header failed, fallback to NewFile", logutil.ErrorAttr(err))
	}
	if err == nil && !hdr.IsCDCodesOnly() {
		defer f.Close()

		fi, err := f.Stat()
		if err != nil {
			return nil, err
		}

		return &fileStat{
			FileInfo: fi,
			header:   hdr,
		}, nil
	}

	cf, err := o.openFromFile(ctx, path, f)
	switch {
	case errors.Is(err, nil):
		// pass
	case errors.Is(err, errors.ErrUnsupported),
		errors.Is(err, pkgfs.ErrTryNext):
		return nil, pkgfs.ErrTryNext
	default:
		// report as try next file if open fails with unhandled error to not block directory listing
		o.logger.ErrorContext(ctx, "CHD file open (NewFile) for stat failed, report as try next", logutil.ErrorAttr(err))
		return nil, pkgfs.ErrTryNext
	}

	defer cf.Close()

	return cf.Stat()
}
