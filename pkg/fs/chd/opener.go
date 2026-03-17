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
	lib *LibCHDR
}

func NewOpener(logger *slog.Logger) *Opener {
	lib, err := NewLibCHDR(logger)
	if err != nil {
		logger.Warn("libchdr load failed, chd support is disabled", logutil.ErrorAttr(err))
		return nil
	}

	logger.Info("libchdr loaded, enabling chd support")

	return &Opener{
		lib: lib,
	}
}

func (o *Opener) canProceed(path string) bool {
	if o == nil {
		return false
	}

	ext1 := filepath.Ext(path)
	if strings.ToLower(ext1) == chdExt {
		return true
	}

	ext2 := filepath.Ext(strings.TrimSuffix(path, ext1))

	return strings.ToLower(ext1) == isoExt && strings.ToLower(ext2) == chdExt
}

func (o *Opener) Open(fsys pkgfs.SystemRoot, path string) (handler.File, error) {
	// .chd file will be reported and requested as .chd.iso
	if !o.canProceed(path) {
		return nil, fs.ErrNotExist
	}

	if ext := filepath.Ext(path); strings.ToLower(ext) == isoExt {
		path = strings.TrimSuffix(path, ext)
	}

	f, err := fsys.Open(strings.TrimSuffix(path, filepath.Ext(path)))
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

	return &fileView{cf}, nil
}

type fileView struct {
	handler.File
}

func (c *fileView) Name() string {
	// add .iso to make ps3 recognise it as disk image
	return c.File.Name() + isoExt
}

func (c *fileView) Stat() (fs.FileInfo, error) {
	fi, err := c.File.Stat()
	if err != nil {
		return nil, err
	}
	return &fakeNameFileStat{
		fileStat: fi.(*fileStat),
	}, nil
}

func (o *Opener) Stat(fsys pkgfs.SystemRoot, path string) (fs.FileInfo, error) {
	if !o.canProceed(path) {
		return nil, fs.ErrNotExist
	}

	if ext := filepath.Ext(path); strings.ToLower(ext) == isoExt {
		path = strings.TrimSuffix(path, ext)
	}

	f, err := fsys.Open(path)
	if err != nil {
		return nil, err
	}

	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}

	hdr, err := o.lib.ReadHeader(f)
	if err != nil {
		return nil, err
	}

	return &fakeNameFileStat{
		&fileStat{
			FileInfo: fi,
			header:   hdr,
		},
	}, nil
}

type fakeNameFileStat struct {
	*fileStat
}

func (c *fakeNameFileStat) Name() string {
	return c.fileStat.FileInfo.Name() + isoExt
}
