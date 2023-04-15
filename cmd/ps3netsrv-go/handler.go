package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/afero"
	"go.uber.org/zap"

	"github.com/xakep666/ps3netsrv-go/pkg/server"
)

type Handler struct {
	Fs  afero.Fs
	Log *zap.Logger
}

func (h *Handler) HandleOpenDir(ctx *server.Context, path string) bool {
	log := h.Log.With(zap.Stringer("remote", ctx.RemoteAddr), zap.String("path", path))
	log.Info("Open dir")

	handle, err := h.Fs.Open(path)
	if err != nil {
		log.Warn("Open failed", zap.Error(err))
		return false
	}

	info, err := handle.Stat()
	if err != nil {
		log.Warn("Stat failed", zap.Error(err))
		return false
	}

	if ctx.CwdHandle != nil {
		if err := ctx.CwdHandle.Close(); err != nil {
			log.Warn("Close ctx.CwdHandle failed", zap.Error(err))
		}
		ctx.CwdHandle = nil
	}

	ctx.CwdHandle = handle
	ctx.Cwd = &path

	// it's crucial to send "true" for directory and "false" for file
	return info.IsDir()
}

func (h *Handler) HandleReadDirEntry(ctx *server.Context) os.FileInfo {
	log := h.Log.With(zap.Stringer("remote", ctx.RemoteAddr))
	log.Info("Read Dir Entry")

	if ctx.Cwd == nil || ctx.CwdHandle == nil {
		log.Warn("No open dir")
		return nil
	}

	for {
		names, err := ctx.CwdHandle.Readdirnames(1)
		if err != nil {
			if !errors.Is(err, io.EOF) {
				log.Warn("Readdirnames failed", zap.Error(err))
			}
			if err := ctx.CwdHandle.Close(); err != nil {
				log.Warn("Close ctx.CwdHandle failed", zap.Error(err))
			}
			ctx.CwdHandle = nil
			return nil
		}

		if names[0] == "." || names[0] == ".." {
			continue
		}

		// Stat to resolve symlink
		fileInfo, err := h.Fs.Stat(filepath.Join(*ctx.Cwd, names[0]))
		if err != nil {
			log.Warn("Stat failed", zap.Error(err))
			// If it doesn't exist (deleted or broken symlink?) or we get a permission error (symlink
			// to file in dir we don't have x on?), or any other error, we just skip it, and try
			// the next entry returned by Readdirnames().
			continue
		}

		return fileInfo
	}
}

func (h *Handler) HandleReadDir(ctx *server.Context) []os.FileInfo {
	log := h.Log.With(zap.Stringer("remote", ctx.RemoteAddr))

	if ctx.Cwd == nil {
		log.Warn("Reading non-opened dir")
		return []os.FileInfo{}
	}

	log = log.With(zap.Stringp("path", ctx.Cwd))
	log.Info("Read dir")

	entries, err := afero.ReadDir(h.Fs, *ctx.Cwd)
	if err != nil {
		log.Warn("Read dir failed", zap.Error(err))
		return []os.FileInfo{}
	}

	var files []os.FileInfo
	for _, entry := range entries {
		if entry.Mode() & os.ModeSymlink != 0 {
			// Stat to resolve symlink
			entry, err = h.Fs.Stat(filepath.Join(*ctx.Cwd, entry.Name()))
			if err != nil {
				log.Warn("Stat failed", zap.Error(err))
				// Ignore broken symbolic links
				continue
			}
		}
		files = append(files, entry)
	}

	return files
}

func (h *Handler) HandleStatFile(ctx *server.Context, path string) (os.FileInfo, error) {
	log := h.Log.With(zap.Stringer("remote", ctx.RemoteAddr), zap.String("path", path))
	log.Info("Stat file")

	info, err := h.Fs.Stat(path)
	switch {
	case errors.Is(err, nil):
		return info, nil
	case os.IsNotExist(err):
		return nil, err
	default:
		log.Warn("Stat file failed", zap.Error(err))
		return nil, err
	}
}

func (h *Handler) HandleOpenFile(ctx *server.Context, path string) error {
	log := h.Log.With(zap.Stringer("remote", ctx.RemoteAddr), zap.String("path", path))
	log.Info("Open R/O file")

	if ctx.ROFile != nil {
		if err := ctx.ROFile.Close(); err != nil {
			log.Warn("Close already opened R/O file failed", zap.Error(err))
		}

		ctx.ROFile = nil
	}

	f, err := h.Fs.Open(path)
	if err != nil {
		log.Warn("Open r/o file failed", zap.Error(err))
		return err
	}

	ctx.ROFile = f

	return nil
}

func (h *Handler) HandleCloseFile(ctx *server.Context) {
	if ctx.ROFile == nil {
		return
	}

	log := h.Log.With(zap.Stringer("remote", ctx.RemoteAddr))
	log.Debug("Close R/O file")

	if err := ctx.ROFile.Close(); err != nil {
		log.Warn("Close R/O file failed", zap.Error(err))
	}
}

func (h *Handler) HandleReadFile(ctx *server.Context, limit uint32, offset uint64) server.LenReader {
	log := h.Log.With(zap.Stringer("remote", ctx.RemoteAddr), zap.Uint32("limit", limit), zap.Uint64("offset", offset))
	log.Debug("Read file")

	if ctx.ROFile == nil {
		log.Warn("No file opened")
		return &bytes.Buffer{}
	}

	if _, err := ctx.ROFile.Seek(int64(offset), io.SeekStart); err != nil {
		log.Warn("Seek failed", zap.Error(err))
		return &bytes.Buffer{}
	}

	var buf bytes.Buffer

	n, err := buf.ReadFrom(io.LimitReader(ctx.ROFile, int64(limit)))
	if err != nil {
		log.Warn("Read failed", zap.Error(err))
		return &bytes.Buffer{}
	}

	log.Debug("Read file", zap.Int64("read", n))

	return &buf
}

func (h *Handler) HandleReadFileCritical(ctx *server.Context, limit uint32, offset uint64) (io.Reader, error) {
	log := h.Log.With(zap.Stringer("remote", ctx.RemoteAddr), zap.Uint32("limit", limit), zap.Uint64("offset", offset))
	log.Debug("Read file critical")

	if ctx.ROFile == nil {
		return nil, fmt.Errorf("no file opened")
	}

	if _, err := ctx.ROFile.Seek(int64(offset), io.SeekStart); err != nil {
		return nil, fmt.Errorf("seek failed: %w", err)
	}

	return io.LimitReader(ctx.ROFile, int64(limit)), nil
}
