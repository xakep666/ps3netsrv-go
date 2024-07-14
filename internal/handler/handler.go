package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/spf13/afero"

	"github.com/xakep666/ps3netsrv-go/internal/copier"
	"github.com/xakep666/ps3netsrv-go/internal/logutil"
	"github.com/xakep666/ps3netsrv-go/pkg/server"
)

var ErrWriteForbidden = fmt.Errorf("write operation forbidden")

type HandlerContext = server.Context[State]

type Handler struct {
	Fs afero.Fs

	Copier     *copier.Copier
	AllowWrite bool
}

func (h *Handler) HandleOpenDir(ctx *HandlerContext, path string) bool {
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

	if ctx.State.CwdHandle != nil {
		if err := ctx.State.CwdHandle.Close(); err != nil {
			log.WarnContext(ctx, "Close ctx.State.CwdHandle failed", logutil.ErrorAttr(err))
		}
		ctx.State.CwdHandle = nil
	}

	ctx.State.CwdHandle = handle

	// it's crucial to send "true" for directory and "false" for file
	return info.IsDir()
}

func (h *Handler) HandleReadDirEntry(ctx *HandlerContext) fs.FileInfo {
	log := slog.Default()

	log.InfoContext(ctx, "Read Dir Entry")

	if ctx.State.CwdHandle == nil {
		log.WarnContext(ctx, "No open dir")
		return nil
	}

	for {
		names, err := ctx.State.CwdHandle.Readdirnames(1)
		if err != nil {
			if !errors.Is(err, io.EOF) {
				log.WarnContext(ctx, "Readdirnames failed", logutil.ErrorAttr(err))
			}
			if err := ctx.State.CwdHandle.Close(); err != nil {
				log.WarnContext(ctx, "Close ctx.State.CwdHandle failed", logutil.ErrorAttr(err))
			}
			ctx.State.CwdHandle = nil
			return nil
		}

		if names[0] == "." || names[0] == ".." {
			continue
		}

		// Stat to resolve symlink
		fileInfo, err := h.Fs.Stat(filepath.Join(ctx.State.CwdHandle.Name(), names[0]))
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

func (h *Handler) HandleReadDir(ctx *HandlerContext) []fs.FileInfo {
	log := slog.Default()

	if ctx.State.CwdHandle == nil {
		log.WarnContext(ctx, "Reading non-opened dir")
		return []fs.FileInfo{}
	}

	log = log.With(slog.String("path", ctx.State.CwdHandle.Name()))
	log.InfoContext(ctx, "Read dir")

	entries, err := ctx.State.CwdHandle.Readdir(-1)
	if err != nil {
		log.WarnContext(ctx, "Read dir failed", logutil.ErrorAttr(err))
		return []fs.FileInfo{}
	}

	var files []fs.FileInfo
	for _, entry := range entries {
		if entry.Mode()&fs.ModeSymlink != 0 {
			// Stat to resolve symlink
			entry, err = h.Fs.Stat(filepath.Join(ctx.State.CwdHandle.Name(), entry.Name()))
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

func (h *Handler) HandleStatFile(ctx *HandlerContext, path string) (fs.FileInfo, error) {
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

func (h *Handler) HandleOpenFile(ctx *HandlerContext, path string) (fs.FileInfo, error) {
	log := slog.With(slog.String("path", path))
	log.InfoContext(ctx, "Open R/O file")

	if ctx.State.ROFile != nil {
		if err := ctx.State.ROFile.Close(); err != nil {
			log.WarnContext(ctx, "Close already opened R/O file failed", logutil.ErrorAttr(err))
		}

		ctx.State.ROFile = nil
	}

	f, err := h.Fs.Open(path)
	if err != nil {
		log.WarnContext(ctx, "Open r/o file failed", logutil.ErrorAttr(err))
		return nil, err
	}

	ctx.State.ROFile = f

	return f.Stat()
}

func (h *Handler) HandleCloseFile(ctx *HandlerContext) {
	if ctx.State.ROFile == nil {
		return
	}

	log := slog.Default()
	log.DebugContext(ctx, "Close R/O file")

	if err := ctx.State.ROFile.Close(); err != nil {
		log.WarnContext(ctx, "Close R/O file failed", logutil.ErrorAttr(err))
	}
}

func (h *Handler) HandleReadFile(ctx *HandlerContext, limit uint32, offset uint64) server.LenReader {
	log := slog.With(slog.Uint64("limit", uint64(limit)), slog.Uint64("offset", offset))
	log.DebugContext(ctx, "Read file")

	if ctx.State.ROFile == nil {
		log.WarnContext(ctx, "No file opened")
		return &bytes.Buffer{}
	}

	if _, err := ctx.State.ROFile.Seek(int64(offset), io.SeekStart); err != nil {
		log.WarnContext(ctx, "Seek failed", logutil.ErrorAttr(err))
		return &bytes.Buffer{}
	}

	var buf bytes.Buffer

	n, err := buf.ReadFrom(io.LimitReader(ctx.State.ROFile, int64(limit)))
	if err != nil {
		log.ErrorContext(ctx, "Read failed", logutil.ErrorAttr(err))
		return &bytes.Buffer{}
	}

	log.DebugContext(ctx, "Read file", slog.Int64("read", n))

	return &buf
}

func (h *Handler) HandleReadFileCritical(ctx *HandlerContext, limit uint32, offset uint64) (io.Reader, error) {
	slog.DebugContext(ctx, "Read file critical", slog.Uint64("limit", uint64(limit)), slog.Uint64("offset", offset))

	if ctx.State.ROFile == nil {
		return nil, fmt.Errorf("no file opened")
	}

	if _, err := ctx.State.ROFile.Seek(int64(offset), io.SeekStart); err != nil {
		return nil, fmt.Errorf("seek failed: %w", err)
	}

	return io.LimitReader(ctx.State.ROFile, int64(limit)), nil
}

func (h *Handler) HandleCreateFile(ctx *HandlerContext, path string) error {
	log := slog.With(slog.String("path", path))
	log.DebugContext(ctx, "Create file")

	if !h.AllowWrite {
		log.WarnContext(ctx, "Modifying operation forbidden", slog.String("op", "create"))
		return ErrWriteForbidden
	}

	if ctx.State.WOFile != nil {
		if err := ctx.State.WOFile.Close(); err != nil {
			log.WarnContext(ctx, "Close already opened W/O file failed", logutil.ErrorAttr(err))
		}

		ctx.State.WOFile = nil
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

	ctx.State.WOFile = f
	return nil
}

func (h *Handler) HandleWriteFile(ctx *HandlerContext, data io.Reader) (int32, error) {
	slog.DebugContext(ctx, "Write file")

	if !h.AllowWrite {
		slog.WarnContext(ctx, "Modifying operation forbidden", slog.String("op", "write"))
		return 0, ErrWriteForbidden
	}

	if ctx.State.WOFile == nil {
		slog.WarnContext(ctx, "File for writing was not opened")
		return 0, fmt.Errorf("file for writing was not opened")
	}

	written, err := h.Copier.Copy(ctx.State.WOFile, data)
	if err != nil {
		slog.WarnContext(ctx, "Write data failed", logutil.ErrorAttr(err))
		return 0, err
	}

	slog.DebugContext(ctx, "Written data", slog.Int64("bytes", written))

	return int32(written), nil
}

func (h *Handler) HandleDeleteFile(ctx *HandlerContext, path string) error {
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

func (h *Handler) HandleMkdir(ctx *HandlerContext, path string) error {
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

func (h *Handler) HandleRmdir(ctx *HandlerContext, path string) error {
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

func (h *Handler) HandleGetDirSize(ctx *HandlerContext, path string) (int64, error) {
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
