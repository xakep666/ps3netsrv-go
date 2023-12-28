package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http/httputil"
	"path/filepath"
	"time"

	"github.com/xakep666/ps3netsrv-go/internal/logutil"

	"github.com/xakep666/ps3netsrv-go/pkg/proto"
)

type Server struct {
	Handler     Handler
	BufferPool  httputil.BufferPool
	ReadTimeout time.Duration

	// ConnContext optionally specifies a function that modifies
	// the context used for a new connection c.
	ConnContext func(ctx context.Context, c net.Conn) context.Context
}

func (s *Server) Serve(ln net.Listener) error {
	defer ln.Close()

	for {
		conn, err := ln.Accept()
		if err != nil {
			return fmt.Errorf("accept failed: %w", err)
		}

		go s.serveConn(conn)
	}
}

func (s *Server) setConnReadDeadline(conn net.Conn) error {
	if s.ReadTimeout <= 0 {
		return nil
	}

	return conn.SetReadDeadline(time.Now().Add(s.ReadTimeout))
}

func (s *Server) deriveConnContext(conn net.Conn) context.Context {
	if s.ConnContext == nil {
		return context.Background()
	}

	return s.ConnContext(context.Background(), conn)
}

func (s *Server) serveConn(conn net.Conn) {
	ctx := &Context{
		RemoteAddr: conn.RemoteAddr(),
		rd:         proto.Reader{Reader: conn},
		wr:         proto.Writer{Writer: conn, BufferPool: s.BufferPool},
	}
	ctx.Context, ctx.cancel = context.WithCancel(s.deriveConnContext(conn))

	log := slog.Default().With(logutil.StringerAttr("remote", conn.RemoteAddr()))

	log.Info("Client connected")
	defer log.Info("Client disconnected")

	defer func() {
		if err := ctx.Close(); err != nil {
			log.WarnContext(ctx, "State closed with errors", logutil.ErrorAttr(err))
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

func (s *Server) handleCommand(opCode proto.OpCode, ctx *Context) error {
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
	case proto.CmdReadDirEntry:
		return s.handleReadDirEntry(ctx)
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

func (s *Server) handleOpenDir(ctx *Context) error {
	// here we should check that we can read requested dir and set state if it's true
	dirPath, err := ctx.rd.ReadOpenDir()
	if err != nil {
		return fmt.Errorf("read dir failed: %w", err)
	}

	return ctx.wr.SendOpenDirResult(s.Handler.HandleOpenDir(ctx, dirPath))
}

func (s *Server) handleReadDirEntry(ctx *Context) error {
	return ctx.wr.SendReadDirEntryResult(s.Handler.HandleReadDirEntry(ctx))
}

func (s *Server) handleReadDir(ctx *Context) error {
	return ctx.wr.SendReadDirResult(s.Handler.HandleReadDir(ctx))
}

func (s *Server) handleStatFile(ctx *Context) error {
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

func (s *Server) handleOpenFile(ctx *Context) error {
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

	if err := s.Handler.HandleOpenFile(ctx, filePath); err != nil {
		return ctx.wr.SendOpenFileError()
	}

	return ctx.wr.SendOpenFileResult(ctx.State.ROFile)
}

func (s *Server) handleReadFile(ctx *Context) error {
	toRead, off, err := ctx.rd.ReadReadFile()
	if err != nil {
		return fmt.Errorf("read read file params failed: %w", err)
	}

	return ctx.wr.SendReadFileResult(s.Handler.HandleReadFile(ctx, toRead, off))
}

func (s *Server) handleReadFileCritical(ctx *Context) error {
	toRead, off, err := ctx.rd.ReadReadFileCritical()
	if err != nil {
		return fmt.Errorf("read read file critical params failed: %w", err)
	}

	rd, err := s.Handler.HandleReadFileCritical(ctx, toRead, off)
	if err != nil {
		return fmt.Errorf("read file critical failed: %w", err)
	}

	return ctx.wr.SendReadFileCriticalResult(rd)
}

func (s *Server) handleCreateFile(ctx *Context) error {
	path, err := ctx.rd.ReadCreateFile()
	if err != nil {
		return fmt.Errorf("read file to create path failed: %w", err)
	}

	if err = s.Handler.HandleCreateFile(ctx, path); err != nil {
		return ctx.wr.SendCreateFileError()
	}

	return ctx.wr.SendCreateFileResult()
}

func (s *Server) handleWriteFile(ctx *Context) error {
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

func (s *Server) handleDeleteFile(ctx *Context) error {
	path, err := ctx.rd.ReadDeleteFile()
	if err != nil {
		return fmt.Errorf("read file to delete path failed: %w", err)
	}

	if err = s.Handler.HandleDeleteFile(ctx, path); err != nil {
		return ctx.wr.SendDeleteFileError()
	}

	return ctx.wr.SendDeleteFileResult()
}

func (s *Server) handleMkdir(ctx *Context) error {
	path, err := ctx.rd.ReadMkdir()
	if err != nil {
		return fmt.Errorf("read directory to create path failed: %w", err)
	}

	if err = s.Handler.HandleMkdir(ctx, path); err != nil {
		return ctx.wr.SendMkdirError()
	}

	return ctx.wr.SendMkdirResult()
}

func (s *Server) handleRmdir(ctx *Context) error {
	path, err := ctx.rd.ReadRmdir()
	if err != nil {
		return fmt.Errorf("read directory to remove path failed: %w", err)
	}

	if err = s.Handler.HandleRmdir(ctx, path); err != nil {
		return ctx.wr.SendRmdirError()
	}

	return ctx.wr.SendMkdirResult()
}

func (s *Server) handleGetDirSize(ctx *Context) error {
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
