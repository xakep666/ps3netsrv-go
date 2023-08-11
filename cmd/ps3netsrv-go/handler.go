package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/spf13/afero"

	"github.com/xakep666/ps3netsrv-go/pkg/logutil"
	"github.com/xakep666/ps3netsrv-go/pkg/server"
)

type Handler struct {
	Fs afero.Fs
}

func (h *Handler) HandleOpenDir(ctx *server.Context, path string) bool {
	log := slog.With(slog.String("path", path))

	log.InfoContext(ctx, "Open dir")

	handle, err := h.Fs.Open(path)
	if err != nil {
		log.WarnContext(ctx, "Open failed", logutil.ErrorAttr(err))
		return false
	}

	info, err := handle.Stat()
	if err != nil {
		log.WarnContext(ctx, "Stat failed", logutil.ErrorAttr(err))
		return false
	}

	if ctx.CwdHandle != nil {
		if err := ctx.CwdHandle.Close(); err != nil {
			log.WarnContext(ctx, "Close ctx.CwdHandle failed", logutil.ErrorAttr(err))
		}
		ctx.CwdHandle = nil
	}

	ctx.CwdHandle = handle
	ctx.Cwd = &path

	// it's crucial to send "true" for directory and "false" for file
	return info.IsDir()
}

func (h *Handler) HandleReadDirEntry(ctx *server.Context) os.FileInfo {
	log := slog.Default()

	log.InfoContext(ctx, "Read Dir Entry")

	if ctx.Cwd == nil || ctx.CwdHandle == nil {
		log.WarnContext(ctx, "No open dir")
		return nil
	}

	for {
		names, err := ctx.CwdHandle.Readdirnames(1)
		if err != nil {
			if !errors.Is(err, io.EOF) {
				log.WarnContext(ctx, "Readdirnames failed", logutil.ErrorAttr(err))
			}
			if err := ctx.CwdHandle.Close(); err != nil {
				log.WarnContext(ctx, "Close ctx.CwdHandle failed", logutil.ErrorAttr(err))
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
			log.WarnContext(ctx, "Stat failed", logutil.ErrorAttr(err))
			// If it doesn't exist (deleted or broken symlink?) or we get a permission error (symlink
			// to file in dir we don't have x on?), or any other error, we just skip it, and try
			// the next entry returned by Readdirnames().
			continue
		}

		return fileInfo
	}
}

func (h *Handler) HandleReadDir(ctx *server.Context) []os.FileInfo {
	log := slog.Default()

	if ctx.Cwd == nil {
		log.WarnContext(ctx, "Reading non-opened dir")
		return []os.FileInfo{}
	}

	log = log.With(slog.String("path", *ctx.Cwd))
	log.InfoContext(ctx, "Read dir")

	entries, err := afero.ReadDir(h.Fs, *ctx.Cwd)
	if err != nil {
		log.WarnContext(ctx, "Read dir failed", logutil.ErrorAttr(err))
		return []os.FileInfo{}
	}

	var files []os.FileInfo
	for _, entry := range entries {
		if entry.Mode()&os.ModeSymlink != 0 {
			// Stat to resolve symlink
			entry, err = h.Fs.Stat(filepath.Join(*ctx.Cwd, entry.Name()))
			if err != nil {
				log.WarnContext(ctx, "Stat failed", logutil.ErrorAttr(err))
				// Ignore broken symbolic links
				continue
			}
		}
		files = append(files, entry)
	}

	return files
}

func (h *Handler) HandleStatFile(ctx *server.Context, path string) (os.FileInfo, error) {
	log := slog.With(slog.String("path", path))
	log.InfoContext(ctx, "Stat file")

	info, err := h.Fs.Stat(path)
	switch {
	case errors.Is(err, nil):
		return info, nil
	case os.IsNotExist(err):
		return nil, err
	default:
		log.WarnContext(ctx, "Stat file failed", logutil.ErrorAttr(err))
		return nil, err
	}
}

func (h *Handler) HandleOpenFile(ctx *server.Context, path string) error {
	log := slog.With(slog.String("path", path))
	log.InfoContext(ctx, "Open R/O file")

	if ctx.ROFile != nil {
		if err := ctx.ROFile.Close(); err != nil {
			log.WarnContext(ctx, "Close already opened R/O file failed", logutil.ErrorAttr(err))
		}

		ctx.ROFile = nil
	}

	f, err := h.Fs.Open(path)
	if err != nil {
		log.WarnContext(ctx, "Open r/o file failed", logutil.ErrorAttr(err))
		return err
	}

	ctx.ROFile = f

	return nil
}

func (h *Handler) HandleCloseFile(ctx *server.Context) {
	if ctx.ROFile == nil {
		return
	}

	log := slog.Default()
	log.DebugContext(ctx, "Close R/O file")

	if err := ctx.ROFile.Close(); err != nil {
		log.WarnContext(ctx, "Close R/O file failed", logutil.ErrorAttr(err))
	}
}

func (h *Handler) HandleReadFile(ctx *server.Context, limit uint32, offset uint64) server.LenReader {
	log := slog.With(slog.Uint64("limit", uint64(limit)), slog.Uint64("offset", offset))
	log.DebugContext(ctx, "Read file")

	if ctx.ROFile == nil {
		log.WarnContext(ctx, "No file opened")
		return &bytes.Buffer{}
	}

	if _, err := ctx.ROFile.Seek(int64(offset), io.SeekStart); err != nil {
		log.WarnContext(ctx, "Seek failed", logutil.ErrorAttr(err))
		return &bytes.Buffer{}
	}

	var buf bytes.Buffer

	n, err := buf.ReadFrom(io.LimitReader(ctx.ROFile, int64(limit)))
	if err != nil {
		log.ErrorContext(ctx, "Read failed", logutil.ErrorAttr(err))
		return &bytes.Buffer{}
	}

	log.DebugContext(ctx, "Read file", slog.Int64("read", n))

	return &buf
}

func (h *Handler) HandleReadFileCritical(ctx *server.Context, limit uint32, offset uint64) (io.Reader, error) {
	slog.DebugContext(ctx, "Read file critical", slog.Uint64("limit", uint64(limit)), slog.Uint64("offset", offset))

	if ctx.ROFile == nil {
		return nil, fmt.Errorf("no file opened")
	}

	if _, err := ctx.ROFile.Seek(int64(offset), io.SeekStart); err != nil {
		return nil, fmt.Errorf("seek failed: %w", err)
	}

	return io.LimitReader(ctx.ROFile, int64(limit)), nil
}
