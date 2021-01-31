package server

import (
	"fmt"
	"io"
	"net"
	"os"

	"github.com/xakep666/ps3netsrv-go/pkg/proto"

	"github.com/spf13/afero"
	"go.uber.org/multierr"
)

type LenReader = proto.LenReader

type State struct {
	Cwd    *string
	ROFile afero.File
}

type Context struct {
	RemoteAddr net.Addr

	State

	rd proto.Reader
	wr proto.Writer
}

func (s *Context) Close() error {
	var errs []error

	if s.ROFile != nil {
		if err := s.ROFile.Close(); err != nil {
			errs = append(errs, fmt.Errorf("ROFile close failed: %w", err))
		}

		s.ROFile = nil
	}

	return multierr.Combine(errs...)
}

type Handler interface {
	HandleOpenDir(ctx *Context, path string) bool
	HandleReadDir(ctx *Context) []os.FileInfo
	HandleStatFile(ctx *Context, path string) (os.FileInfo, error)
	HandleOpenFile(ctx *Context, path string) error
	HandleCloseFile(ctx *Context)
	HandleReadFile(ctx *Context, limit uint32, offset uint64) LenReader
	HandleReadFileCritical(ctx *Context, limit uint32, offset uint64) (io.Reader, error)
}
