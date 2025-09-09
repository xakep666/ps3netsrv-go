package proto

import (
	"encoding/binary"
	"fmt"
	"io"
	"io/fs"
	"time"
)

type Writer struct {
	io.Writer
}

// AccessTimeFileInfo may be implemented by fs.FileInfo to provide access time information
type AccessTimeFileInfo interface {
	fs.FileInfo

	AccessTime() time.Time
}

// AccessChangeTimeFileInfo may be implemented by fs.FileInfo to provide access and change time information.
// This interface hierarchy used because AccessTime supported on more platforms than ChangeTime and all platforms
// having ChangeTime also have AccessTime.
type AccessChangeTimeFileInfo interface {
	AccessTimeFileInfo

	ChangeTime() time.Time
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

		dirEntryResult.ModTime, dirEntryResult.ChangeTime, dirEntryResult.AccessTime = fileInfoTimes(entry)

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
	result := StatFileResult{
		IsDirectory: entry.IsDir(),
	}
	result.ModTime, result.ChangeTime, result.AccessTime = fileInfoTimes(entry)
	if !entry.IsDir() {
		result.FileSize = entry.Size()
	}

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

func (w *Writer) sendResult(data any) error {
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

func fileInfoTimes(info fs.FileInfo) (mtime, ctime, atime uint64) {
	mtime = uint64(info.ModTime().UTC().Unix())
	atime = mtime
	ctime = mtime

	if accessTime, ok := info.(AccessTimeFileInfo); ok && !accessTime.AccessTime().IsZero() {
		atime = uint64(accessTime.AccessTime().UTC().Unix())
	}

	if changeTime, ok := info.(AccessChangeTimeFileInfo); ok && !changeTime.ChangeTime().IsZero() {
		ctime = uint64(changeTime.ChangeTime().UTC().Unix())
	}

	return
}
