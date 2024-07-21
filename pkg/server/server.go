package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"path/filepath"
	"time"

	"github.com/xakep666/ps3netsrv-go/internal/logutil"
	"github.com/xakep666/ps3netsrv-go/pkg/proto"
)

type Server[StateT any] struct {
	Handler     Handler[StateT]
	ReadTimeout time.Duration

	// ConnContext optionally specifies a function that modifies
	// the context used for a new connection c.
	ConnContext func(ctx context.Context, c net.Conn) context.Context

	// Logger is the logger for the server.
	Logger *slog.Logger
}

func (s *Server[StateT]) Serve(ln net.Listener) error {
	defer ln.Close()

	for {
		conn, err := ln.Accept()
		if err != nil {
			return fmt.Errorf("accept failed: %w", err)
		}

		go s.serveConn(conn)
	}
}

func (s *Server[StateT]) setConnReadDeadline(conn net.Conn) error {
	if s.ReadTimeout <= 0 {
		return nil
	}

	return conn.SetReadDeadline(time.Now().Add(s.ReadTimeout))
}

func (s *Server[StateT]) deriveConnContext(conn net.Conn) context.Context {
	if s.ConnContext == nil {
		return context.Background()
	}

	return s.ConnContext(context.Background(), conn)
}

func (s *Server[StateT]) serveConn(conn net.Conn) {
	ctx := &Context[StateT]{
		RemoteAddr: conn.RemoteAddr(),
		rd:         proto.Reader{Reader: conn},
		wr:         proto.Writer{Writer: conn},
	}
	ctx.Context, ctx.cancel = context.WithCancel(s.deriveConnContext(conn))

	log := s.Logger.With(logutil.StringerAttr("remote", conn.RemoteAddr()))

	log.Info("Client connected")
	defer log.Info("Client disconnected")

	defer func() {
		if err := ctx.Close(); err != nil {
			log.WarnContext(ctx, "Context closed with errors", logutil.ErrorAttr(err))
		}
	}()

	defer conn.Close()

	for {
		if err := s.setConnReadDeadline(conn); err != nil {
			log.ErrorContext(ctx, "Failed to set read deadline", logutil.ErrorAttr(err))
			return
		}

		opCode, err := ctx.rd.ReadCommand()
		switch {
		case errors.Is(err, nil):
			// pass
		case errors.Is(err, io.EOF):
			log.InfoContext(ctx, "Connection closed")
			return
		default:
			log.ErrorContext(ctx, "Read command failed", logutil.ErrorAttr(err))
			return
		}

		oclog := log.With(logutil.StringerAttr("opcode", opCode))

		oclog.DebugContext(ctx, "Received opcode")

		if err := s.handleCommand(opCode, ctx); err != nil {
			oclog.ErrorContext(ctx, "Command handler failed")
			return
		}
	}
}

func (s *Server[StateT]) handleCommand(opCode proto.OpCode, ctx *Context[StateT]) error {
	switch opCode {
	case proto.CmdOpenDir:
		return s.handleOpenDir(ctx)
	case proto.CmdReadDir:
		return s.handleReadDir(ctx)
	case proto.CmdStatFile:
		return s.handleStatFile(ctx)
	case proto.CmdOpenFile:
		return s.handleOpenFile(ctx)
	case proto.CmdReadFile:
		return s.handleReadFile(ctx)
	case proto.CmdReadFileCritical:
		return s.handleReadFileCritical(ctx)
	case proto.CmdReadCD2048Critical:
		return s.handleReadCD2048Critical(ctx)
	case proto.CmdReadDirEntry:
		return s.handleReadDirEntry(ctx)
	case proto.CmdReadDirEntryV2:
		return s.handleReadDirEntryV2(ctx)
	case proto.CmdCreateFile:
		return s.handleCreateFile(ctx)
	case proto.CmdWriteFile:
		return s.handleWriteFile(ctx)
	case proto.CmdDeleteFile:
		return s.handleDeleteFile(ctx)
	case proto.CmdMkdir:
		return s.handleMkdir(ctx)
	case proto.CmdRmdir:
		return s.handleRmdir(ctx)
	case proto.CmdGetDirSize:
		return s.handleGetDirSize(ctx)
	default:
		return fmt.Errorf("unknown opCode: %d", opCode)
	}
}

func (s *Server[StateT]) handleOpenDir(ctx *Context[StateT]) error {
	// here we should check that we can read requested dir and set state if it's true
	dirPath, err := ctx.rd.ReadOpenDir()
	if err != nil {
		return fmt.Errorf("read dir failed: %w", err)
	}

	return ctx.wr.SendOpenDirResult(s.Handler.HandleOpenDir(ctx, dirPath))
}

func (s *Server[StateT]) handleReadDirEntry(ctx *Context[StateT]) error {
	return ctx.wr.SendReadDirEntryResult(s.Handler.HandleReadDirEntry(ctx))
}

func (s *Server[StateT]) handleReadDirEntryV2(ctx *Context[StateT]) error {
	return ctx.wr.SendReadDirEntryV2Result(s.Handler.HandleReadDirEntry(ctx))
}

func (s *Server[StateT]) handleReadDir(ctx *Context[StateT]) error {
	return ctx.wr.SendReadDirResult(s.Handler.HandleReadDir(ctx))
}

func (s *Server[StateT]) handleStatFile(ctx *Context[StateT]) error {
	filePath, err := ctx.rd.ReadStatFile()
	if err != nil {
		return fmt.Errorf("read stat path failed: %w", err)
	}

	fi, err := s.Handler.HandleStatFile(ctx, filePath)
	if err != nil {
		return ctx.wr.SendStatFileError()
	}

	return ctx.wr.SendStatFileResult(fi)
}

func (s *Server[StateT]) handleOpenFile(ctx *Context[StateT]) error {
	// Here can be some special paths:
	// * CLOSEFILE (in original code it's just send success with closing already opened file if present)

	filePath, err := ctx.rd.ReadOpenFile()
	if err != nil {
		return fmt.Errorf("read file to open path failed: %w", err)
	}

	filePath = filepath.Clean(filePath)

	if _, name := filepath.Split(filePath); name == "CLOSEFILE" {
		s.Handler.HandleCloseFile(ctx)
		return ctx.wr.SendOpenFileForCLOSEFILE()
	}

	fi, err := s.Handler.HandleOpenFile(ctx, filePath)
	if err != nil {
		return ctx.wr.SendOpenFileError()
	}

	return ctx.wr.SendOpenFileResult(fi)
}

type readFileResponseWriter struct {
	dataLength int32
	upstream   io.Writer
}

func (w *readFileResponseWriter) WriteHeader(length int32) { w.dataLength = length }

func (w *readFileResponseWriter) Write(p []byte) (n int, err error) {
	if w.dataLength <= 0 {
		return 0, fmt.Errorf("WriteHeader wasn't called")
	}

	return w.upstream.Write(p)
}

func (s *Server[StateT]) handleReadFile(ctx *Context[StateT]) error {
	toRead, off, err := ctx.rd.ReadReadFile()
	if err != nil {
		return fmt.Errorf("read read file params failed: %w", err)
	}

	return s.Handler.HandleReadFile(ctx, toRead, off, &readFileResponseWriter{
		dataLength: -1,
		upstream:   ctx.wr.Writer,
	})
}

func (s *Server[StateT]) handleReadFileCritical(ctx *Context[StateT]) error {
	toRead, off, err := ctx.rd.ReadReadFileCritical()
	if err != nil {
		return fmt.Errorf("read read file critical params failed: %w", err)
	}

	return s.Handler.HandleReadFileCritical(ctx, toRead, off, ctx.wr.Writer)
}

func (s *Server[StateT]) handleReadCD2048Critical(ctx *Context[StateT]) error {
	startSector, sectorsToRead, err := ctx.rd.ReadReadCD2048Critical()
	if err != nil {
		return fmt.Errorf("read CD2048 critical params failed: %w", err)
	}

	return s.Handler.HandleReadCD2048Critical(ctx, startSector, sectorsToRead, ctx.wr.Writer)
}

func (s *Server[StateT]) handleCreateFile(ctx *Context[StateT]) error {
	path, err := ctx.rd.ReadCreateFile()
	if err != nil {
		return fmt.Errorf("read file to create path failed: %w", err)
	}

	if err = s.Handler.HandleCreateFile(ctx, path); err != nil {
		return ctx.wr.SendCreateFileError()
	}

	return ctx.wr.SendCreateFileResult()
}

func (s *Server[StateT]) handleWriteFile(ctx *Context[StateT]) error {
	data, err := ctx.rd.ReadWriteFile()
	if err != nil {
		return fmt.Errorf("read file data to write failed: %w", err)
	}

	written, err := s.Handler.HandleWriteFile(ctx, data)
	if err != nil {
		return ctx.wr.SendWriteFileError()
	}

	return ctx.wr.SendWriteFileResult(written)
}

func (s *Server[StateT]) handleDeleteFile(ctx *Context[StateT]) error {
	path, err := ctx.rd.ReadDeleteFile()
	if err != nil {
		return fmt.Errorf("read file to delete path failed: %w", err)
	}

	if err = s.Handler.HandleDeleteFile(ctx, path); err != nil {
		return ctx.wr.SendDeleteFileError()
	}

	return ctx.wr.SendDeleteFileResult()
}

func (s *Server[StateT]) handleMkdir(ctx *Context[StateT]) error {
	path, err := ctx.rd.ReadMkdir()
	if err != nil {
		return fmt.Errorf("read directory to create path failed: %w", err)
	}

	if err = s.Handler.HandleMkdir(ctx, path); err != nil {
		return ctx.wr.SendMkdirError()
	}

	return ctx.wr.SendMkdirResult()
}

func (s *Server[StateT]) handleRmdir(ctx *Context[StateT]) error {
	path, err := ctx.rd.ReadRmdir()
	if err != nil {
		return fmt.Errorf("read directory to remove path failed: %w", err)
	}

	if err = s.Handler.HandleRmdir(ctx, path); err != nil {
		return ctx.wr.SendRmdirError()
	}

	return ctx.wr.SendMkdirResult()
}

func (s *Server[StateT]) handleGetDirSize(ctx *Context[StateT]) error {
	path, err := ctx.rd.ReadGetDirSize()
	if err != nil {
		return fmt.Errorf("read directory to calculate size path failed: %w", err)
	}

	size, err := s.Handler.HandleGetDirSize(ctx, path)
	if err != nil {
		return ctx.wr.SendGetDirectorySizeError()
	}

	return ctx.wr.SendGetDirectorySizeResult(size)
}
