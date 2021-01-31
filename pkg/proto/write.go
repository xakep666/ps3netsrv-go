package proto

import (
	"encoding/binary"
	"fmt"
	"io"
	"net/http/httputil"
	"os"

	"github.com/spf13/afero"
)

type Writer struct {
	io.Writer
	BufferPool httputil.BufferPool
}

func (w *Writer) SendOpenDirResult(success bool) error {
	res := OpenDirResult{}
	if !success {
		res.Result = -1
	}

	return w.sendResult(res)
}

func (w *Writer) SendReadDirResult(entries []os.FileInfo) error {
	err := w.sendResult(ReadDirResult{Size: int64(len(entries))})
	if err != nil {
		return fmt.Errorf("sendResult with size failed: %w", err)
	}

	for _, entry := range entries {
		dirEntry := DirEntry{
			ModTime:     uint64(entry.ModTime().UTC().Unix()),
			IsDirectory: entry.IsDir(),
		}
		if !entry.IsDir() {
			dirEntry.FileSize = entry.Size()
		}
		copy(dirEntry.Name[:], entry.Name())

		err := w.sendResult(dirEntry)
		if err != nil {
			return fmt.Errorf("sendResult for %s failed: %w", entry.Name(), err)
		}
	}

	return nil
}

func (w *Writer) SendStatFileResult(entry os.FileInfo) error {
	modTime := uint64(entry.ModTime().UTC().Unix())
	result := StatFileResult{
		ModTime:     modTime,
		AccessTime:  modTime,
		ChangeTime:  modTime,
		IsDirectory: entry.IsDir(),
	}
	if !entry.IsDir() {
		result.FileSize = entry.Size()
	}

	// TODO: fill AccessTime, ChangeTime using entry.Sys() for different platforms

	if err := w.sendResult(result); err != nil {
		return fmt.Errorf("sendResult failed: %w", err)
	}

	return nil
}

func (w *Writer) SendStatFileError() error {
	if err := w.sendResult(StatFileResult{FileSize: -1}); err != nil {
		return fmt.Errorf("sendResult failed: %w", err)
	}

	return nil
}

func (w *Writer) SendOpenFileResult(f afero.File) error {
	info, err := f.Stat()
	if err != nil {
		return fmt.Errorf("stat failed: %w", err)
	}

	result := OpenFileResult{
		FileSize: info.Size(),
		ModTime:  uint64(info.ModTime().UTC().Unix()),
	}

	if err := w.sendResult(result); err != nil {
		return fmt.Errorf("sendResult failed: %w", err)
	}

	return nil
}

func (w *Writer) SendOpenFileForCLOSEFILE() error {
	if err := w.sendResult(OpenFileResult{}); err != nil {
		return fmt.Errorf("sendResult failed: %w", err)
	}

	return nil
}

func (w *Writer) SendOpenFileError() error {
	if err := w.sendResult(OpenFileResult{FileSize: -1}); err != nil {
		return fmt.Errorf("sendResult failed: %w", err)
	}

	return nil
}

type LenReader interface {
	io.Reader

	Len() int
}

func (w *Writer) SendReadFileResult(data LenReader) error {
	if err := w.sendResult(ReadFileResult{BytesRead: int32(data.Len())}); err != nil {
		return fmt.Errorf("sendResult failed: %w", err)
	}

	_, err := w.copyBuffered(w.Writer, data)
	if err != nil {
		return fmt.Errorf("buffer write failed: %w", err)
	}

	return nil
}

func (w *Writer) SendReadFileCriticalResult(data io.Reader) error {
	_, err := w.copyBuffered(w.Writer, data)
	if err != nil {
		return fmt.Errorf("buffer write failed: %w", err)
	}

	return nil
}

type readerOnly struct{ io.Reader }

type writerOnly struct{ io.Writer }

func (w *Writer) copyBuffered(to io.Writer, from io.Reader) (int64, error) {
	// ensure that we will use plain copy
	to = writerOnly{to}
	from = readerOnly{from}

	if w.BufferPool == nil {
		return io.Copy(to, from)
	}

	buf := w.BufferPool.Get()
	defer w.BufferPool.Put(buf)
	return io.CopyBuffer(to, from, buf)
}

func (w *Writer) sendResult(data interface{}) error {
	err := binary.Write(w.Writer, binary.BigEndian, data)
	if err != nil {
		return fmt.Errorf("binary.Write failed: %w", err)
	}

	return nil
}
