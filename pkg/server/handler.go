package server

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"net"

	"github.com/xakep666/ps3netsrv-go/pkg/proto"
)

type Context[StateT any] struct {
	context.Context

	RemoteAddr net.Addr

	State StateT

	rd     proto.Reader
	wr     proto.Writer
	cancel context.CancelFunc
}

func (s *Context[StateT]) Close() error {
	if s.cancel != nil {
		s.cancel()
	}

	if closer, ok := any(&s.State).(io.Closer); ok {
		if err := closer.Close(); err != nil {
			return fmt.Errorf("state close failed: %w", err)
		}
	}

	return nil
}

type ReadFileResponseWriter interface {
	WriteHeader(length int32)
	io.Writer
}

type Handler[StateT any] interface {
	HandleOpenDir(ctx *Context[StateT], path string) bool
	HandleReadDir(ctx *Context[StateT]) []fs.FileInfo
	HandleReadDirEntry(ctx *Context[StateT]) fs.FileInfo
	HandleStatFile(ctx *Context[StateT], path string) (fs.FileInfo, error)
	HandleOpenFile(ctx *Context[StateT], path string) (fs.FileInfo, error)
	HandleCloseFile(ctx *Context[StateT])
	HandleReadFile(ctx *Context[StateT], limit uint32, offset uint64, w ReadFileResponseWriter) error
	HandleReadFileCritical(ctx *Context[StateT], limit uint32, offset uint64, w io.Writer) error
	HandleReadCD2048Critical(ctx *Context[StateT], startSector, sectorsToRead uint32, w io.Writer) error
	HandleCreateFile(ctx *Context[StateT], path string) error
	HandleWriteFile(ctx *Context[StateT], data io.Reader) (int32, error)
	HandleDeleteFile(ctx *Context[StateT], path string) error
	HandleMkdir(ctx *Context[StateT], path string) error
	HandleRmdir(ctx *Context[StateT], path string) error
	HandleGetDirSize(ctx *Context[StateT], path string) (int64, error)
}
