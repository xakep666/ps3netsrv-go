package chd

import (
	"io/fs"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/xakep666/ps3netsrv-go/internal/handler"
	"github.com/xakep666/ps3netsrv-go/internal/logutil"
	pkgfs "github.com/xakep666/ps3netsrv-go/pkg/fs"
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
	ext2 := filepath.Ext(strings.TrimSuffix(path, ext1))

	// file name must be "<name>.chd.iso"
	return strings.ToLower(ext1) == ".iso" && strings.ToLower(ext2) == ".chd"
}

func (o *Opener) Open(fsys pkgfs.SystemRoot, path string) (handler.File, error) {
	if !o.canProceed(path) {
		return nil, fs.ErrNotExist
	}

	f, err := fsys.Open(path)
	if err != nil {
		return nil, err
	}

	return o.lib.NewFile(f)
}

func (o *Opener) Stat(fsys pkgfs.SystemRoot, path string) (fs.FileInfo, error) {
	if !o.canProceed(path) {
		return nil, fs.ErrNotExist
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

	return &chdFileStat{
		FileInfo: fi,
		header:   hdr,
	}, nil
}
