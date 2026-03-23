//go:build aix || ppc64

package chd

import (
	"errors"
	"io/fs"
	"log/slog"

	"github.com/xakep666/ps3netsrv-go/internal/handler"
)

type LibCHDR struct{}

func NewLibCHDR(logger *slog.Logger) (*LibCHDR, error) {
	return nil, errors.ErrUnsupported
}

func (*LibCHDR) NewFile(handler.File) (*File, error) {
	return nil, errors.ErrUnsupported
}

func (*LibCHDR) ReadHeader(handler.File) (*FileHeader, error) {
	return nil, errors.ErrUnsupported
}

func (*File) Read(b []byte) (int, error) {
	return 0, errors.ErrUnsupported
}

func (*File) Seek(offset int64, whence int) (int64, error) {
	return 0, errors.ErrUnsupported
}

func (*File) Stat() (fs.FileInfo, error) {
	return nil, errors.ErrUnsupported
}

func (*File) Close() error {
	return errors.ErrUnsupported
}
