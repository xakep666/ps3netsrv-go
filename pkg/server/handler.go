package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"

	"github.com/xakep666/ps3netsrv-go/pkg/proto"

	"github.com/spf13/afero"
)

type LenReader = proto.LenReader

type State struct {
	Cwd       *string
	CwdHandle afero.File
	ROFile    afero.File
}

type Context struct {
	context.Context

	RemoteAddr net.Addr

	State

	rd     proto.Reader
	wr     proto.Writer
	cancel context.CancelFunc
}

func (s *Context) Close() error {
	var errs []error

	if s.cancel != nil {
		s.cancel()
	}

	if s.ROFile != nil {
		if err := s.ROFile.Close(); err != nil {
			errs = append(errs, fmt.Errorf("ROFile close failed: %w", err))
		}

		s.ROFile = nil
	}

	if s.CwdHandle != nil {
		if err := s.CwdHandle.Close(); err != nil {
			errs = append(errs, fmt.Errorf("CwdHandle close failed: %w", err))
		}

		s.CwdHandle = nil
	}

	return errors.Join(errs...)
}

type Handler interface {
	HandleOpenDir(ctx *Context, path string) bool
	HandleReadDir(ctx *Context) []os.FileInfo
	HandleReadDirEntry(ctx *Context) os.FileInfo
	HandleStatFile(ctx *Context, path string) (os.FileInfo, error)
	HandleOpenFile(ctx *Context, path string) error
	HandleCloseFile(ctx *Context)
	HandleReadFile(ctx *Context, limit uint32, offset uint64) LenReader
	HandleReadFileCritical(ctx *Context, limit uint32, offset uint64) (io.Reader, error)
}
