package handler

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

const psxPrefixSize = 24

var ErrWriteForbidden = fmt.Errorf("write operation forbidden")

type Context = server.Context[State]

type Handler struct {
	Fs afero.Fs

	Copier     *copier.Copier
	AllowWrite bool
}

func (h *Handler) HandleOpenDir(ctx *Context, path string) bool {
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

func (h *Handler) HandleReadDirEntry(ctx *Context) fs.FileInfo {
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

		return wrapFileInfoForExtendedTimes(fileInfo)
	}
}

func (h *Handler) HandleReadDir(ctx *Context) []fs.FileInfo {
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

func (h *Handler) HandleStatFile(ctx *Context, path string) (fs.FileInfo, error) {
	log := slog.With(slog.String("path", path))
	log.InfoContext(ctx, "Stat file")

	info, err := h.Fs.Stat(path)
	switch {
	case errors.Is(err, nil):
		return wrapFileInfoForExtendedTimes(info), nil
	case errors.Is(err, afero.ErrFileNotFound):
		return nil, err
	default:
		log.WarnContext(ctx, "Stat file failed", logutil.ErrorAttr(err))
		return nil, err
	}
}

func (h *Handler) HandleOpenFile(ctx *Context, path string) (fs.FileInfo, error) {
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
	ctx.State.CDSectorSize = 2352 // default sector size

	fi, err := f.Stat()
	if err != nil {
		log.WarnContext(ctx, "Stat failed", logutil.ErrorAttr(err))
		return nil, err
	}

	// if file size between 2Mb and 848Mb we should try to detect sector size
	if fi.Size() >= 0x200000 && fi.Size() <= 0x35000000 {
		sectorSize, err := determineSectorSize(f)
		if err != nil {
			log.WarnContext(ctx, "Determine sector size failed", logutil.ErrorAttr(err))
		}
		if sectorSize > 0 && sectorSize != ctx.State.CDSectorSize {
			log.InfoContext(ctx, "Sector size determined", slog.Int("size", sectorSize))
			ctx.State.CDSectorSize = sectorSize
		}
	}

	return fi, nil
}

func (h *Handler) HandleCloseFile(ctx *Context) {
	if ctx.State.ROFile == nil {
		return
	}

	log := slog.Default()
	log.DebugContext(ctx, "Close R/O file")

	if err := ctx.State.ROFile.Close(); err != nil {
		log.WarnContext(ctx, "Close R/O file failed", logutil.ErrorAttr(err))
	}

	ctx.State.ROFile = nil
	ctx.State.CDSectorSize = 0
}

func (h *Handler) HandleReadFile(ctx *Context, limit uint32, offset uint64, wr server.ReadFileResponseWriter) error {
	log := slog.With(slog.Uint64("limit", uint64(limit)), slog.Uint64("offset", offset))
	log.DebugContext(ctx, "Read file")

	if ctx.State.ROFile == nil {
		return fmt.Errorf("no file opened")
	}

	if _, err := ctx.State.ROFile.Seek(int64(offset), io.SeekStart); err != nil {
		return fmt.Errorf("seek failed: %w", err)
	}

	var buf bytes.Buffer

	n, err := buf.ReadFrom(io.LimitReader(ctx.State.ROFile, int64(limit)))
	if err != nil {
		return fmt.Errorf("read failed: %w", err)
	}

	log.DebugContext(ctx, "Read file", slog.Int64("read", n))

	wr.WriteHeader(int32(n))
	_, err = buf.WriteTo(wr)
	return err
}

func (h *Handler) HandleReadFileCritical(ctx *Context, limit uint32, offset uint64, w io.Writer) error {
	slog.DebugContext(ctx, "Read file critical", slog.Uint64("limit", uint64(limit)), slog.Uint64("offset", offset))

	if ctx.State.ROFile == nil {
		return fmt.Errorf("no file opened")
	}

	if _, err := ctx.State.ROFile.Seek(int64(offset), io.SeekStart); err != nil {
		return fmt.Errorf("seek failed: %w", err)
	}

	_, err := h.Copier.CopyN(w, ctx.State.ROFile, int64(limit))
	return err
}

func (h *Handler) HandleReadCD2048Critical(ctx *Context, startSector, sectorsCount uint32, w io.Writer) error {
	const readSize = 2048
	slog.DebugContext(ctx, "Read CD2048 critical",
		slog.Int("sectorSize", ctx.State.CDSectorSize),
		slog.Uint64("startSector", uint64(startSector)),
		slog.Uint64("sectorsCount", uint64(sectorsCount)),
	)

	if ctx.State.ROFile == nil {
		return fmt.Errorf("no file opened")
	}

	if ctx.State.CDSectorSize <= 0 {
		return fmt.Errorf("sector size was not determined")
	}

	// this command treats sectors as 2048-sized, so if sector size is non-standard, we must skip some bytes at the end
	offset := psxPrefixSize + int64(startSector)*int64(ctx.State.CDSectorSize)
	for range sectorsCount {
		_, err := ctx.State.ROFile.Seek(offset, io.SeekStart)
		if err != nil {
			return fmt.Errorf("seek failed: %w", err)
		}

		_, err = h.Copier.CopyN(w, ctx.State.ROFile, readSize)
		if err != nil {
			return fmt.Errorf("copy failed: %w", err)
		}

		offset += int64(ctx.State.CDSectorSize)
	}

	return nil
}

func (h *Handler) HandleCreateFile(ctx *Context, path string) error {
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

func (h *Handler) HandleWriteFile(ctx *Context, data io.Reader) (int32, error) {
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

func (h *Handler) HandleDeleteFile(ctx *Context, path string) error {
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

func (h *Handler) HandleMkdir(ctx *Context, path string) error {
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

func (h *Handler) HandleRmdir(ctx *Context, path string) error {
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

func (h *Handler) HandleGetDirSize(ctx *Context, path string) (int64, error) {
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

func determineSectorSize(f io.ReaderAt) (int, error) {
	// sorted
	sectorSizes := [...]int{2048, 2328, 2336, 2340, 2352, 2368, 2448}
	const (
		magic1            = "\x01CD001"
		magic2            = "PLAYSTATION "
		extraBytes        = 2 // between magic1 and magic2
		systemAreaSectors = 16
	)
	// We can detect sector size for 1 read(at) system call
	// to do this we read amount of data that equals to difference between maximum and minimum sector size
	// plus length of magic1 and magic2 plus two bytes between them.
	// After successful reading we just try to locate magic1 or magic2 by offsets determined by
	// subtraction between probed sector size and minimal sector size.
	minMaxDifference := sectorSizes[len(sectorSizes)-1] - sectorSizes[0]
	buf := make([]byte, minMaxDifference+len(magic1)+extraBytes+len(magic2))

	n, err := f.ReadAt(buf, psxPrefixSize+systemAreaSectors*int64(sectorSizes[0]))
	if err != nil {
		return -1, fmt.Errorf("read failed: %w", err)
	}
	if n != len(buf) {
		return -1, fmt.Errorf("read failed: expected %d bytes, got %d", len(buf), n)
	}

	for _, sectorSize := range sectorSizes {
		idxMagic1 := sectorSize - sectorSizes[0]
		if string(buf[idxMagic1:idxMagic1+len(magic1)]) == magic1 {
			return sectorSize, nil
		}

		idxMagic2 := idxMagic1 + len(magic1) + extraBytes
		if string(buf[idxMagic2:idxMagic2+len(magic2)]) == magic2 {
			return sectorSize, nil
		}
	}

	// for everything except psx images we will be here
	return -1, nil
}
