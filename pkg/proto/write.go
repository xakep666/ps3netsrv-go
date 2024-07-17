package proto

import (
	"encoding/binary"
	"fmt"
	"io"
	"io/fs"
)

type Writer struct {
	io.Writer
}

func (w *Writer) SendOpenDirResult(success bool) error {
	res := OpenDirResult{}
	if !success {
		res.Result = -1
	}

	return w.sendResult(res)
}

func (w *Writer) SendReadDirResult(entries []fs.FileInfo) error {
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

func (w *Writer) SendReadDirEntryResult(entry fs.FileInfo) error {
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
		return fmt.Errorf("sendResult for %s failed: %w", entry, err)
	}

	if dirEntryResult.FilenameLen > 0 && entry != nil {
		err := w.sendResult(entry.Name())
		if err != nil {
			return fmt.Errorf("sendResult for %s failed: %w", entry.Name(), err)
		}
	}

	return nil
}

func (w *Writer) SendReadDirEntryV2Result(entry fs.FileInfo) error {
	dirEntryResult := ReadDirEntryV2Result{}

	if entry == nil {
		dirEntryResult.FileSize = -1
	} else {
		dirEntryResult.FilenameLen = uint16(len(entry.Name()))

		// TODO: fill AccessTime, ChangeTime using entry.Sys() for different platforms
		dirEntryResult.ModTime = uint64(entry.ModTime().UTC().Unix())
		dirEntryResult.ChangeTime = uint64(entry.ModTime().UTC().Unix())
		dirEntryResult.AccessTime = uint64(entry.ModTime().UTC().Unix())

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
		return fmt.Errorf("sendResult for %s failed: %w", entry, err)
	}

	if dirEntryResult.FilenameLen > 0 && entry != nil {
		err := w.sendResult(entry.Name())
		if err != nil {
			return fmt.Errorf("sendResult for %s failed: %w", entry.Name(), err)
		}
	}

	return nil
}

func (w *Writer) SendStatFileResult(entry fs.FileInfo) error {
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

func (w *Writer) SendOpenFileResult(info fs.FileInfo) error {
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

// SendReadFileResultLen sends only size of data that follows after this message
func (w *Writer) SendReadFileResultLen(dataLen int32) error {
	if err := w.sendResult(ReadFileResult{BytesRead: dataLen}); err != nil {
		return fmt.Errorf("sendResult failed: %w", err)
	}

	return nil
}

func (w *Writer) sendResult(data interface{}) error {
	// we can send string directly
	if str, ok := data.(string); ok {
		_, err := io.WriteString(w.Writer, str)
		if err != nil {
			return fmt.Errorf("io.WriteString failed: %w", err)
		}
	}

	err := binary.Write(w.Writer, binary.BigEndian, data)
	if err != nil {
		return fmt.Errorf("binary.Write failed: %w", err)
	}

	return nil
}
