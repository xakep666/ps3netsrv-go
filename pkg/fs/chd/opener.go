package chd

import (
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

	ext1 := filepath.Ext(path)
	if strings.EqualFold(ext1, chdExt) {
		return true
	}

	ext2 := filepath.Ext(strings.TrimSuffix(path, ext1))

	return strings.EqualFold(ext1, isoExt) && strings.EqualFold(ext2, chdExt)
}

func (o *Opener) Open(fsys pkgfs.SystemRoot, path string) (handler.File, error) {
	// .chd file will be reported and requested as .chd.iso
	if !o.canProceed(path) {
		return nil, fs.ErrNotExist
	}

	if ext := filepath.Ext(path); strings.EqualFold(ext, isoExt) {
		path = strings.TrimSuffix(path, ext)
	}

	o.logger.Debug("Trying to open CHD file", slog.String("path", path))
	f, err := fsys.Open(path)
	if err != nil {
		return nil, err
	}

	cf, err := o.lib.NewFile(f)
	switch {
	case errors.Is(err, nil):
		// pass
	case errors.Is(err, fs.ErrNotExist),
		errors.Is(err, errors.ErrUnsupported):
		return nil, fs.ErrNotExist
	default:
		return nil, err
	}

	if cf.Header.IsCDCodesOnly() {
		cdFile, err := cf.AsCD()
		if err != nil {
			return nil, err
		}
		o.logger.Debug("Detected CD-encoded CHD, wrapping",
			slog.String("path", path),
			slog.Int("sector_data_size", cdFile.SectorDataSize),
			slog.Int64("sectors_count", cdFile.SectorsCount),
		)
		return &fileView{File: cdFile, openPath: path}, nil
	}

	return &fileView{File: cf, openPath: path}, nil
}

func (*Opener) Name() string {
	return "chd"
}

type fileView struct {
	handler.File
	openPath string
}

func (c *fileView) Name() string {
	// add .iso to make ps3 recognise it as disk image
	return c.openPath + isoExt
}

func (c *fileView) Stat() (fs.FileInfo, error) {
	fi, err := c.File.Stat()
	if err != nil {
		return nil, err
	}
	return &fakeNameFileStat{fi}, nil
}

func (o *Opener) Stat(fsys pkgfs.SystemRoot, path string) (fs.FileInfo, error) {
	cf, err := o.Open(fsys, path)
	switch {
	case errors.Is(err, nil):
		// pass
	case errors.Is(err, fs.ErrNotExist):
		return nil, err
	default:
		// report as non-existing file if open fails with unhandled error to not block directory listing
		o.logger.Error("CHD file open for stat failed, report as non-existing", logutil.ErrorAttr(err))
		return nil, fs.ErrNotExist
	}

	defer cf.Close()

	return cf.Stat()
}

type fakeNameFileStat struct {
	fs.FileInfo
}

func (c *fakeNameFileStat) Name() string {
	return c.FileInfo.Name() + isoExt
}
