//go:build !aix && !ppc64

package chd

import (
	"io"
	"io/fs"
	"syscall"
	"time"
)

func (f *CDFile) offsetToPosition(offset int64) (hunkNum, posInHunk, end int) {
	unitNum := offset / int64(f.SectorDataSize)
	unitsPerHunk := int64(f.Header.HunkBytes / f.Header.UnitBytes)

	hunkNum = int(unitNum / unitsPerHunk)
	unitInHunk := int(unitNum % unitsPerHunk)

	posInUnit := int(offset % int64(f.SectorDataSize))
	unitStart := unitInHunk * int(f.Header.UnitBytes)

	posInHunk = unitStart + posInUnit
	end = unitStart + f.SectorDataSize

	return hunkNum, posInHunk, end
}

func (f *CDFile) Read(b []byte) (int, error) {
	if err := f.init(); err != nil {
		return 0, err
	}

	if f.offset >= f.Size {
		// at EOF
		return 0, io.EOF
	}

	read := 0
	newOffset := f.offset
	// either buffer is filled or file is ended
	for len(b) > 0 && newOffset < f.Size {
		desiredHunkNum, offsetInHunk, end := f.offsetToPosition(newOffset)
		if desiredHunkNum < 0 || desiredHunkNum >= int(f.Header.TotalHunks) {
			break
		}

		if err := f.loadHunk(desiredHunkNum); err != nil {
			return read, err
		}

		copied := copy(b, f.currentHunkData[offsetInHunk:end])
		newOffset += int64(copied)
		read += copied
		b = b[copied:]
	}

	f.offset = newOffset
	return read, nil
}

func (f *CDFile) Seek(offset int64, whence int) (int64, error) {
	if err := f.init(); err != nil {
		return 0, err
	}

	switch whence {
	case io.SeekStart:
	case io.SeekCurrent:
		offset += f.offset
	case io.SeekEnd:
		offset = f.offset - offset - 1
	default:
		return 0, syscall.EINVAL
	}

	if offset < 0 || offset > f.Size {
		return 0, syscall.EINVAL
	}

	f.offset = offset
	return offset, nil
}

type cdFileStat struct {
	*CDFile
}

func (s *cdFileStat) Size() int64 {
	return s.CDFile.Size
}

func (s *cdFileStat) Mode() fs.FileMode {
	return s.originalFileInfo.Mode() | fs.ModeIrregular
}

func (s *cdFileStat) Sys() any {
	return s.originalFileInfo.Sys()
}

func (s *cdFileStat) IsDir() bool {
	return false
}

func (s *cdFileStat) ModTime() time.Time {
	return s.originalFileInfo.ModTime()
}

func (f *CDFile) Stat() (fs.FileInfo, error) {
	if err := f.init(); err != nil {
		return nil, err
	}
	return &cdFileStat{f}, nil
}
