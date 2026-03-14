//go:build !aix && !ppc64

package chd

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"runtime"
	"syscall"
	"unsafe"

	"github.com/ebitengine/purego"

	"github.com/xakep666/ps3netsrv-go/internal/handler"
	"github.com/xakep666/ps3netsrv-go/internal/logutil"
	"github.com/xakep666/ps3netsrv-go/internal/osutil"
)

type fileHandle uintptr

type fileMode int

const (
	fileModeRead = 1
	fileModeRW   = 2
)

type LibCHDR struct {
	log       *slog.Logger
	callbacks *fileCallbacks

	openFileCallbacks   func(callbacks *fileCallbacks, userdata *osutil.Handle, mode fileMode, parent fileHandle, chdFile *fileHandle) errorCode
	precache            func(chdFile fileHandle) errorCode
	close               func(chdFile fileHandle)
	getHeader           func(chdFile fileHandle) *FileHeader
	readHeaderCallbacks func(callbacks *fileCallbacks, userdata *osutil.Handle, header *FileHeader) errorCode
	read                func(chdFile fileHandle, hunknum uint32, buffer *byte) errorCode
	errorString         func(code errorCode) string
}

func NewLibCHDR(logger *slog.Logger) (*LibCHDR, error) {
	handle, err := osutil.LoadLibrary("libchdr")
	if err != nil {
		return nil, err
	}

	ret := &LibCHDR{
		log:       logger,
		callbacks: newFileCallbacks(logger),
	}
	purego.RegisterLibFunc(&ret.openFileCallbacks, handle, "chd_open_core_file_callbacks")
	purego.RegisterLibFunc(&ret.precache, handle, "chd_precache")
	purego.RegisterLibFunc(&ret.close, handle, "chd_close")
	purego.RegisterLibFunc(&ret.getHeader, handle, "chd_get_header")
	purego.RegisterLibFunc(&ret.readHeaderCallbacks, handle, "chd_read_header_core_file_callbacks")
	purego.RegisterLibFunc(&ret.read, handle, "chd_read")
	purego.RegisterLibFunc(&ret.errorString, handle, "chd_error_string")

	runtime.AddCleanup(ret, func(h uintptr) {
		if err := osutil.UnloadLibrary(h); err != nil {
			logger.Warn("chd: unload libchdr", logutil.ErrorAttr(err))
		}
	}, handle)

	return ret, nil
}

func (l *LibCHDR) NewFile(f handler.File) (handler.File, error) {
	fi, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat: %w", err)
	}

	var chdFileHandle fileHandle
	chdErrCode := l.openFileCallbacks(l.callbacks, new(osutil.NewHandle(f)), fileModeRead, 0, &chdFileHandle)
	if err := l.makeError(chdErrCode); err != nil {
		return nil, fmt.Errorf("chd: open: %w", err)
	}

	if err := l.makeError(l.precache(chdFileHandle)); err != nil {
		return nil, fmt.Errorf("chd: precache: %w", err)
	}

	ret := &chdFile{
		lib:              l,
		handle:           chdFileHandle,
		originalName:     f.Name(),
		originalFileInfo: fi,
	}
	ret.cleanup = runtime.AddCleanup(ret, l.close, chdFileHandle)

	return ret, nil
}

func (l *LibCHDR) ReadHeader(f handler.File) (*FileHeader, error) {
	cgoFileHandle := osutil.NewHandle(f)
	defer cgoFileHandle.Delete()

	var header FileHeader
	if err := l.makeError(l.readHeaderCallbacks(l.callbacks, &cgoFileHandle, &header)); err == nil {
		return &header, nil
	}

	// workaround until https://github.com/rtissera/libchdr/pull/146 is merged or that issue is fixed somehow
	var chdFileHandle fileHandle
	// special wrapper to avoid file closing by libchdr
	chdErrCode := l.openFileCallbacks(l.callbacks, new(osutil.NewHandle(&nopCloserFile{f})), fileModeRead, 0, &chdFileHandle)
	if err := l.makeError(chdErrCode); err != nil {
		return nil, fmt.Errorf("chd: open: %w", err)
	}
	defer l.close(chdFileHandle)

	return new(*l.getHeader(chdFileHandle)), nil
}

type nopCloserFile struct {
	handler.File
}

func (*nopCloserFile) Close() error {
	return nil
}

func (l *LibCHDR) makeError(code errorCode) error {
	if code == 0 {
		return nil
	}

	return &Error{code: code, message: l.errorString(code)}
}

type chdFile struct {
	lib              *LibCHDR
	handle           fileHandle
	originalName     string
	originalFileInfo fs.FileInfo
	header           *FileHeader
	cleanup          runtime.Cleanup

	offset          int64
	currentHunkNum  int64
	currentHunkData []byte
}

func (f *chdFile) init() error {
	if f.handle == 0 {
		return fs.ErrClosed
	}

	if f.header != nil {
		return nil
	}

	f.header = new(*f.lib.getHeader(f.handle)) // clone to not refer to C memory
	return nil
}

func (f *chdFile) Read(b []byte) (int, error) {
	if err := f.init(); err != nil {
		return 0, err
	}

	if f.offset >= int64(f.header.LogicalBytes) {
		// at EOF
		return 0, io.EOF
	}

	if len(f.currentHunkData) == 0 {
		f.currentHunkData = make([]byte, f.header.HunkBytes)
	}

	read := 0
	newOffset := f.offset
	// either buffer is filled or filed is ended
	for len(b) > 0 && newOffset < int64(f.header.LogicalBytes) {
		// decompress hunk if needed
		desiredHunkNum := newOffset / int64(f.header.HunkBytes)
		if desiredHunkNum < 0 || desiredHunkNum > int64(f.header.TotalHunks) {
			break
		}
		if desiredHunkNum != f.currentHunkNum {
			readRes := f.lib.read(f.handle, uint32(desiredHunkNum), unsafe.SliceData(f.currentHunkData))
			if err := f.lib.makeError(readRes); err != nil {
				return read, fmt.Errorf("chd: read hunk %d: %w", desiredHunkNum, err)
			}
			f.currentHunkNum = desiredHunkNum
		}

		// now just copy unpacked data to our target buffer
		offsetInHunk := newOffset % int64(f.header.HunkBytes)
		copied := copy(b, f.currentHunkData[offsetInHunk:])
		read += copied
		newOffset += int64(copied)
		b = b[copied:]
	}

	f.offset = newOffset
	return read, nil
}

func (f *chdFile) Seek(offset int64, whence int) (int64, error) {
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

	if offset < 0 || offset > int64(f.header.LogicalBytes) {
		return 0, syscall.EINVAL
	}

	f.offset = offset
	return offset, nil
}

func (f *chdFile) Stat() (fs.FileInfo, error) {
	if err := f.init(); err != nil {
		return nil, err
	}
	return &chdFileStat{
		FileInfo: f.originalFileInfo,
		header:   f.header,
	}, nil
}

func (f *chdFile) ReadDir(int) ([]fs.DirEntry, error) {
	return nil, errors.ErrUnsupported
}

func (f *chdFile) Name() string {
	return f.originalName
}

func (f *chdFile) Close() error {
	if f.handle == 0 {
		return fs.ErrClosed
	}

	f.cleanup.Stop()
	f.lib.close(f.handle)
	return nil
}
