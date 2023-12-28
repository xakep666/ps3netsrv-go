package proto

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"

	"github.com/spf13/afero"

	"github.com/xakep666/ps3netsrv-go/internal/copier"
)

type Writer struct {
	io.Writer

	Copier *copier.Copier
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

func (w *Writer) SendReadDirEntryResult(entry os.FileInfo) error {
	dirEntryResult := ReadDirEntryResult{}

	if entry == nil {
		dirEntryResult.FileSize = -1
	} else {
		dirEntryResult.FilenameLen = uint16(len(entry.Name()))
		if entry.IsDir() {
			dirEntryResult.FileSize = 0
			dirEntryResult.IsDirectory = true
		} else {
			dirEntryResult.FileSize = entry.Size()
			dirEntryResult.IsDirectory = false
		}
	}

	err := w.sendResult(dirEntryResult)
	if err != nil {
		return fmt.Errorf("sendResult for %s failed: %w", entry.Name(), err)
	}

	if dirEntryResult.FilenameLen > 0 {
		err := w.sendResult([]byte(entry.Name()))
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

func (w *Writer) SendCreateFileResult() error {
	if err := w.sendResult(CreateFileResult{}); err != nil {
		return fmt.Errorf("sendResult failed: %w", err)
	}

	return nil
}

func (w *Writer) SendCreateFileError() error {
	if err := w.sendResult(CreateFileResult{Result: -1}); err != nil {
		return fmt.Errorf("sendResult failed: %w", err)
	}

	return nil
}

func (w *Writer) SendWriteFileResult(written int32) error {
	if err := w.sendResult(WriteFileResult{BytesWritten: written}); err != nil {
		return fmt.Errorf("sendResult failed: %w", err)
	}

	return nil
}

func (w *Writer) SendWriteFileError() error {
	if err := w.sendResult(WriteFileResult{BytesWritten: -1}); err != nil {
		return fmt.Errorf("sendResult failed: %w", err)
	}

	return nil
}

func (w *Writer) SendDeleteFileResult() error {
	if err := w.sendResult(DeleteFileResult{}); err != nil {
		return fmt.Errorf("sendResult failed: %w", err)
	}

	return nil
}

func (w *Writer) SendDeleteFileError() error {
	if err := w.sendResult(DeleteFileResult{Result: -1}); err != nil {
		return fmt.Errorf("sendResult failed: %w", err)
	}

	return nil
}

func (w *Writer) SendMkdirResult() error {
	if err := w.sendResult(MkdirResult{}); err != nil {
		return fmt.Errorf("sendResult failed: %w", err)
	}

	return nil
}

func (w *Writer) SendMkdirError() error {
	if err := w.sendResult(MkdirResult{Result: -1}); err != nil {
		return fmt.Errorf("sendResult failed: %w", err)
	}

	return nil
}

func (w *Writer) SendRmdirResult() error {
	if err := w.sendResult(RmdirResult{}); err != nil {
		return fmt.Errorf("sendResult failed: %w", err)
	}

	return nil
}

func (w *Writer) SendRmdirError() error {
	if err := w.sendResult(RmdirResult{Result: -1}); err != nil {
		return fmt.Errorf("sendResult failed: %w", err)
	}

	return nil
}

func (w *Writer) SendGetDirectorySizeResult(size int64) error {
	if err := w.sendResult(GetDirSizeResult{Size: size}); err != nil {
		return fmt.Errorf("sendResult failed: %w", err)
	}

	return nil
}

func (w *Writer) SendGetDirectorySizeError() error {
	if err := w.sendResult(GetDirSizeResult{Size: -1}); err != nil {
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

	_, err := w.Copier.Copy(w.Writer, data)
	if err != nil {
		return fmt.Errorf("buffer write failed: %w", err)
	}

	return nil
}

func (w *Writer) SendReadFileCriticalResult(data io.Reader) error {
	_, err := w.Copier.Copy(w.Writer, data)
	if err != nil {
		return fmt.Errorf("buffer write failed: %w", err)
	}

	return nil
}

func (w *Writer) sendResult(data interface{}) error {
	err := binary.Write(w.Writer, binary.BigEndian, data)
	if err != nil {
		return fmt.Errorf("binary.Write failed: %w", err)
	}

	return nil
}
