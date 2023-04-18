package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/rs/zerolog"
	"github.com/spf13/afero"

	"github.com/xakep666/ps3netsrv-go/pkg/server"
)

type Handler struct {
	Fs afero.Fs
}

func (h *Handler) HandleOpenDir(ctx *server.Context, path string) bool {
	log := zerolog.Ctx(ctx).With().Str("path", path).Logger()

	log.Info().Msg("Open dir")

	handle, err := h.Fs.Open(path)
	if err != nil {
		log.Warn().Err(err).Msg("Open failed")
		return false
	}

	info, err := handle.Stat()
	if err != nil {
		log.Warn().Err(err).Msg("Stat failed")
		return false
	}

	if ctx.CwdHandle != nil {
		if err := ctx.CwdHandle.Close(); err != nil {
			log.Warn().Err(err).Msg("Close ctx.CwdHandle failed")
		}
		ctx.CwdHandle = nil
	}

	ctx.CwdHandle = handle
	ctx.Cwd = &path

	// it's crucial to send "true" for directory and "false" for file
	return info.IsDir()
}

func (h *Handler) HandleReadDirEntry(ctx *server.Context) os.FileInfo {
	log := zerolog.Ctx(ctx)
	log.Info().Msg("Read Dir Entry")

	if ctx.Cwd == nil || ctx.CwdHandle == nil {
		log.Warn().Msg("No open dir")
		return nil
	}

	for {
		names, err := ctx.CwdHandle.Readdirnames(1)
		if err != nil {
			if !errors.Is(err, io.EOF) {
				log.Warn().Err(err).Msg("Readdirnames failed")
			}
			if err := ctx.CwdHandle.Close(); err != nil {
				log.Warn().Err(err).Msg("Close ctx.CwdHandle failed")
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
			log.Warn().Err(err).Msg("Stat failed")
			// If it doesn't exist (deleted or broken symlink?) or we get a permission error (symlink
			// to file in dir we don't have x on?), or any other error, we just skip it, and try
			// the next entry returned by Readdirnames().
			continue
		}

		return fileInfo
	}
}

func (h *Handler) HandleReadDir(ctx *server.Context) []os.FileInfo {
	logc := zerolog.Ctx(ctx)

	if ctx.Cwd == nil {
		logc.Warn().Msg("Reading non-opened dir")
		return []os.FileInfo{}
	}

	log := logc.With().Str("path", *ctx.Cwd).Logger()
	log.Info().Msg("Read dir")

	entries, err := afero.ReadDir(h.Fs, *ctx.Cwd)
	if err != nil {
		log.Info().Msg("Read dir failed")
		return []os.FileInfo{}
	}

	var files []os.FileInfo
	for _, entry := range entries {
		if entry.Mode()&os.ModeSymlink != 0 {
			// Stat to resolve symlink
			entry, err = h.Fs.Stat(filepath.Join(*ctx.Cwd, entry.Name()))
			if err != nil {
				log.Warn().Err(err).Msg("Stat failed")
				// Ignore broken symbolic links
				continue
			}
		}
		files = append(files, entry)
	}

	return files
}

func (h *Handler) HandleStatFile(ctx *server.Context, path string) (os.FileInfo, error) {
	log := zerolog.Ctx(ctx).With().Str("path", path).Logger()
	log.Info().Msg("Stat file")

	info, err := h.Fs.Stat(path)
	switch {
	case errors.Is(err, nil):
		return info, nil
	case os.IsNotExist(err):
		return nil, err
	default:
		log.Warn().Err(err).Msg("Stat file failed")
		return nil, err
	}
}

func (h *Handler) HandleOpenFile(ctx *server.Context, path string) error {
	log := zerolog.Ctx(ctx).With().Str("path", path).Logger()
	log.Info().Msg("Open R/O file")

	if ctx.ROFile != nil {
		if err := ctx.ROFile.Close(); err != nil {
			log.Warn().Err(err).Msg("Close already opened R/O file failed")
		}

		ctx.ROFile = nil
	}

	f, err := h.Fs.Open(path)
	if err != nil {
		log.Warn().Err(err).Msg("Open r/o file failed")
		return err
	}

	ctx.ROFile = f

	return nil
}

func (h *Handler) HandleCloseFile(ctx *server.Context) {
	if ctx.ROFile == nil {
		return
	}

	log := zerolog.Ctx(ctx)
	log.Debug().Msg("Close R/O file")

	if err := ctx.ROFile.Close(); err != nil {
		log.Warn().Err(err).Msg("Close R/O file failed")
	}
}

func (h *Handler) HandleReadFile(ctx *server.Context, limit uint32, offset uint64) server.LenReader {
	log := zerolog.Ctx(ctx).With().Uint32("limit", limit).Uint64("offset", offset).Logger()
	log.Debug().Msg("Read file")

	if ctx.ROFile == nil {
		log.Warn().Msg("No file opened")
		return &bytes.Buffer{}
	}

	if _, err := ctx.ROFile.Seek(int64(offset), io.SeekStart); err != nil {
		log.Warn().Err(err).Msg("Seek failed")
		return &bytes.Buffer{}
	}

	var buf bytes.Buffer

	n, err := buf.ReadFrom(io.LimitReader(ctx.ROFile, int64(limit)))
	if err != nil {
		log.Warn().Err(err).Msg("Read failed")
		return &bytes.Buffer{}
	}

	log.Debug().Int64("read", n).Msg("Read file")

	return &buf
}

func (h *Handler) HandleReadFileCritical(ctx *server.Context, limit uint32, offset uint64) (io.Reader, error) {
	zerolog.Ctx(ctx).Debug().
		Uint32("limit", limit).
		Uint64("offset", offset).
		Msg("Read file critical")

	if ctx.ROFile == nil {
		return nil, fmt.Errorf("no file opened")
	}

	if _, err := ctx.ROFile.Seek(int64(offset), io.SeekStart); err != nil {
		return nil, fmt.Errorf("seek failed: %w", err)
	}

	return io.LimitReader(ctx.ROFile, int64(limit)), nil
}
