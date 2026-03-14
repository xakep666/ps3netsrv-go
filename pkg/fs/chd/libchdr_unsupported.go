//go:build aix || ppc64

package chd

import (
	"errors"
	"log/slog"

	"github.com/xakep666/ps3netsrv-go/internal/handler"
)

type LibCHDR struct{}

func NewLibCHDR(logger *slog.Logger) (*LibCHDR, error) {
	return nil, errors.ErrUnsupported
}

func (*LibCHDR) NewFile(handler.File) (handler.File, error) {
	return nil, errors.ErrUnsupported
}

func (*LibCHDR) ReadHeader(handler.File) (*FileHeader, error) {
	return nil, errors.ErrUnsupported
}
