package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http/httputil"
	"os"
	"path/filepath"

	"github.com/spf13/afero"

	"github.com/xakep666/ps3netsrv-go/pkg/logutil"
	"github.com/xakep666/ps3netsrv-go/pkg/server"
)

var ErrWriteForbidden = fmt.Errorf("write operation forbidden")

type Handler struct {
	Fs afero.Fs

	BufferPool httputil.BufferPool
	AllowWrite bool
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

	// it's crucial to send "true" for directory and "false" for file
	return info.IsDir()
}

func (h *Handler) HandleReadDirEntry(ctx *server.Context) fs.FileInfo {
	log := slog.Default()

	log.InfoContext(ctx, "Read Dir Entry")

	if ctx.CwdHandle == nil {
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
		fileInfo, err := h.Fs.Stat(filepath.Join(ctx.CwdHandle.Name(), names[0]))
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

func (h *Handler) HandleReadDir(ctx *server.Context) []fs.FileInfo {
	log := slog.Default()

	if ctx.CwdHandle == nil {
		log.WarnContext(ctx, "Reading non-opened dir")
		return []fs.FileInfo{}
	}

	log = log.With(slog.String("path", ctx.CwdHandle.Name()))
	log.InfoContext(ctx, "Read dir")

	entries, err := ctx.CwdHandle.Readdir(-1)
	if err != nil {
		log.WarnContext(ctx, "Read dir failed", logutil.ErrorAttr(err))
		return []fs.FileInfo{}
	}

	var files []fs.FileInfo
	for _, entry := range entries {
		if entry.Mode()&fs.ModeSymlink != 0 {
			// Stat to resolve symlink
			entry, err = h.Fs.Stat(filepath.Join(ctx.CwdHandle.Name(), entry.Name()))
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

func (h *Handler) HandleStatFile(ctx *server.Context, path string) (fs.FileInfo, error) {
	log := slog.With(slog.String("path", path))
	log.InfoContext(ctx, "Stat file")

	info, err := h.Fs.Stat(path)
	switch {
	case errors.Is(err, nil):
		return info, nil
	case errors.Is(err, afero.ErrFileNotFound):
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

func (h *Handler) HandleCreateFile(ctx *server.Context, path string) error {
	log := slog.With(slog.String("path", path))
	log.DebugContext(ctx, "Create file")

	if !h.AllowWrite {
		log.WarnContext(ctx, "Modifying operation forbidden", slog.String("op", "create"))
		return ErrWriteForbidden
	}

	if ctx.WOFile != nil {
		if err := ctx.WOFile.Close(); err != nil {
			log.WarnContext(ctx, "Close already opened W/O file failed", logutil.ErrorAttr(err))
		}

		ctx.WOFile = nil
	}

	// path is a directory -> closing file, just return
	stat, err := h.Fs.Stat(path)
	if err != nil {
		log.WarnContext(ctx, "Stat failed", logutil.ErrorAttr(err))
		return err
	}
	if stat.IsDir() {
		return nil
	}

	f, err := h.Fs.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, fs.ModePerm)
	if err != nil {
		log.WarnContext(ctx, "Create file failed", logutil.ErrorAttr(err))
		return err
	}

	ctx.WOFile = f
	return nil
}

func (h *Handler) HandleWriteFile(ctx *server.Context, data io.Reader) (int32, error) {
	slog.DebugContext(ctx, "Write file")

	if !h.AllowWrite {
		slog.WarnContext(ctx, "Modifying operation forbidden", slog.String("op", "write"))
		return 0, ErrWriteForbidden
	}

	if ctx.WOFile == nil {
		slog.WarnContext(ctx, "File for writing was not opened")
		return 0, fmt.Errorf("file for writing was not opened")
	}

	written, err := h.copyBuffered(ctx.WOFile, data)
	if err != nil {
		slog.WarnContext(ctx, "Write data failed", logutil.ErrorAttr(err))
		return 0, err
	}

	slog.DebugContext(ctx, "Written data", slog.Int64("bytes", written))

	return int32(written), nil
}

func (h *Handler) HandleDeleteFile(ctx *server.Context, path string) error {
	log := slog.With(slog.String("path", path))
	log.DebugContext(ctx, "Delete file")

	if !h.AllowWrite {
		log.WarnContext(ctx, "Modifying operation forbidden", slog.String("op", "rm"))
		return ErrWriteForbidden
	}

	if err := h.Fs.Remove(path); err != nil {
		log.WarnContext(ctx, "Remove file failed", logutil.ErrorAttr(err))
		return err
	}

	return nil
}

func (h *Handler) HandleMkdir(ctx *server.Context, path string) error {
	log := slog.With(slog.String("path", path))
	log.DebugContext(ctx, "Create directory")

	if !h.AllowWrite {
		log.WarnContext(ctx, "Modifying operation forbidden", slog.String("op", "mkdir"))
		return ErrWriteForbidden
	}

	if err := h.Fs.Mkdir(path, os.ModePerm); err != nil {
		log.WarnContext(ctx, "Create directory failed", logutil.ErrorAttr(err))
		return err
	}

	return nil
}

func (h *Handler) HandleRmdir(ctx *server.Context, path string) error {
	log := slog.With(slog.String("path", path))
	log.DebugContext(ctx, "Remove directory")

	if !h.AllowWrite {
		log.WarnContext(ctx, "Modifying operation forbidden", slog.String("op", "rmdir"))
		return ErrWriteForbidden
	}

	if err := h.Fs.Remove(path); err != nil {
		log.WarnContext(ctx, "Remove directory failed", logutil.ErrorAttr(err))
		return err
	}

	return nil
}

// fsOnly needed to detach all "optional" interfaces like afero.Lstater.
type fsOnly struct{ afero.Fs }

func (h *Handler) HandleGetDirSize(ctx *server.Context, path string) (int64, error) {
	log := slog.With(slog.String("path", path))
	log.DebugContext(ctx, "Get directory size")

	var size int64
	// detach afero.Lstater interface to resolve symlinks in afero.Walk.
	_ = afero.Walk(&fsOnly{h.Fs}, ".", func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			log.WarnContext(ctx, "Skipping path because of error",
				slog.String("path", path), logutil.ErrorAttr(err))
			return nil
		}

		if info.IsDir() {
			return nil
		}

		size += info.Size()
		return nil
	})

	log.DebugContext(ctx, "Directory size calculated", slog.Int64("size", size))

	return size, nil
}

type readerOnly struct{ io.Reader }

type writerOnly struct{ io.Writer }

func (h *Handler) copyBuffered(to io.Writer, from io.Reader) (int64, error) {
	// ensure that we will use plain copy
	to = writerOnly{to}
	from = readerOnly{from}

	if h.BufferPool == nil {
		return io.Copy(to, from)
	}

	buf := h.BufferPool.Get()
	defer h.BufferPool.Put(buf)
	return io.CopyBuffer(to, from, buf)
}
