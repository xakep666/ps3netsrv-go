//go:build !aix && !ppc64

package chd

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"runtime"
	"strconv"
	"syscall"

	"github.com/ebitengine/purego"

	"github.com/xakep666/ps3netsrv-go/internal/handler"
	"github.com/xakep666/ps3netsrv-go/internal/logutil"
	"github.com/xakep666/ps3netsrv-go/internal/osutil"
)

type fileMode int

const (
	fileModeRead = 1
	fileModeRW   = 2
)

const (
	cdMetadataOldTag = ('C' << 24) | ('H' << 16) | ('C' << 8) | 'D'
	cdMetadataTag    = ('C' << 24) | ('H' << 16) | ('T' << 8) | 'R'
	cdMetadataTag2   = ('C' << 24) | ('H' << 16) | ('T' << 8) | '2'
)

type LibCHDR struct {
	log       *slog.Logger
	callbacks *fileCallbacks

	openFileCallbacks   func(callbacks *fileCallbacks, userdata osutil.Handle, mode fileMode, parent fileHandle, chdFile *fileHandle) errorCode
	close               func(chdFile fileHandle)
	getHeader           func(chdFile fileHandle) *FileHeader
	getMetadata         func(chdFile fileHandle, searchTag, searchIndex uint32, output *byte, outputLen uint32, resultLen, resultTag *uint32, resultFlags *byte) errorCode
	readHeaderCallbacks func(callbacks *fileCallbacks, userdata osutil.Handle, header *FileHeader) errorCode
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
	purego.RegisterLibFunc(&ret.close, handle, "chd_close")
	purego.RegisterLibFunc(&ret.getHeader, handle, "chd_get_header")
	purego.RegisterLibFunc(&ret.getMetadata, handle, "chd_get_metadata")
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

func (l *LibCHDR) NewFile(f handler.File) (*File, error) {
	fi, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat: %w", err)
	}

	var chdFileHandle fileHandle
	chdErrCode := l.openFileCallbacks(l.callbacks, osutil.NewHandle(f), fileModeRead, 0, &chdFileHandle)
	if err := l.makeError(chdErrCode); err != nil {
		return nil, fmt.Errorf("chd: open: %w", err)
	}

	// clone to not refer to C memory
	chdFileHeader := new(*l.getHeader(chdFileHandle))

	ret := File{
		Header:           chdFileHeader,
		lib:              l,
		handle:           chdFileHandle,
		originalName:     f.Name(),
		originalFileInfo: fi,
	}

	// if first codec is CD, check that other ones are CD too and unit size is cdModeSectorSize
	if chdFileHeader.Compression[0].IsCD() {
		if !chdFileHeader.IsCDCodesOnly() {
			return nil, fmt.Errorf("unsupported codec combination: %v", chdFileHeader.Compression)
		}

		// sanity check: amount of units per hunk x unit size must equal to logical size
		if uint64(chdFileHeader.UnitBytes)*chdFileHeader.UnitCount != chdFileHeader.LogicalBytes {
			return nil, fmt.Errorf("inconsistent data: unitbytes(%d)*unitcount(%d)!=logicalsize(%d)",
				chdFileHeader.UnitBytes, chdFileHeader.UnitCount, chdFileHeader.LogicalBytes)
		}

		metadata, err := l.readMeatadata(chdFileHandle)
		if err != nil {
			return nil, fmt.Errorf("read metadata: %w", err)
		}

		// metadata sanity check: frames sum must equal to unit count
		var totalFrames int
		for _, md := range metadata {
			totalFrames += md.Frames
		}
		if totalFrames > int(chdFileHeader.UnitCount) {
			return nil, fmt.Errorf("inconsistent data: 'frames' sum in metadata(%d) > unitcount(%d)", totalFrames, chdFileHeader.UnitCount)
		}

		ret.CDMetadata = metadata
	}

	pRet := &ret
	pRet.cleanup = runtime.AddCleanup(pRet, l.close, chdFileHandle)
	return pRet, nil
}

func (l *LibCHDR) readMeatadata(handle fileHandle) ([]CDMetadata, error) {
	const metadataNotFound errorCode = 19

	var ret []CDMetadata
	rawTag := make([]byte, 512)
	var rawTagLen uint32
	for idx := uint32(0); ; idx++ {
		errCode := l.getMetadata(handle, cdMetadataOldTag, idx, &rawTag[0], uint32(len(rawTag)), &rawTagLen, nil, nil)
		if errCode == metadataNotFound {
			errCode = l.getMetadata(handle, cdMetadataTag, idx, &rawTag[0], uint32(len(rawTag)), &rawTagLen, nil, nil)
		}
		if errCode == metadataNotFound {
			errCode = l.getMetadata(handle, cdMetadataTag2, idx, &rawTag[0], uint32(len(rawTag)), &rawTagLen, nil, nil)
		}
		if errCode == metadataNotFound {
			break
		}
		if err := l.makeError(errCode); err != nil {
			return nil, fmt.Errorf("idx %d: %w", idx, err)
		}

		tag := rawTag[:rawTagLen-1] // trim \0
		var item CDMetadata
		for len(tag) > 0 {
			var rawItem []byte
			rawItem, tag, _ = bytes.Cut(tag, []byte(" "))
			key, value, _ := bytes.Cut(rawItem, []byte(":"))

			switch string(key) {
			case "TRACK":
				x, err := strconv.Atoi(string(value))
				if err != nil {
					return nil, fmt.Errorf("idx %d: TRACK: %w", idx, err)
				}
				item.TrackNumber = x
			case "TYPE":
				item.Type = string(value)
			case "SUBTYPE":
				item.Subtype = string(value)
			case "FRAMES":
				x, err := strconv.Atoi(string(value))
				if err != nil {
					return nil, fmt.Errorf("idx %d: FRAMES: %w", idx, err)
				}
				item.Frames = x
			case "PREGAP":
				x, err := strconv.Atoi(string(value))
				if err != nil {
					return nil, fmt.Errorf("idx %d: PREGAP: %w", idx, err)
				}
				item.Pregap = x
			case "PGTYPE":
				item.PregapType = string(value)
			case "PGSUB":
				item.PregapSubType = string(value)
			case "POSTGAP":
				x, err := strconv.Atoi(string(value))
				if err != nil {
					return nil, fmt.Errorf("idx %d: POSTGAP: %w", idx, err)
				}
				item.Postgap = x
			}
		}

		ret = append(ret, item)
	}

	return ret, nil
}

func (l *LibCHDR) ReadHeader(f handler.File) (*FileHeader, error) {
	cgoFileHandle := osutil.NewHandle(f)
	defer cgoFileHandle.Delete()

	var header FileHeader
	if err := l.makeError(l.readHeaderCallbacks(l.callbacks, cgoFileHandle, &header)); err == nil {
		return &header, nil
	}

	// workaround until https://github.com/rtissera/libchdr/pull/146 is merged or that issue is fixed somehow
	var chdFileHandle fileHandle
	// special wrapper to avoid file closing by libchdr
	chdErrCode := l.openFileCallbacks(l.callbacks, osutil.NewHandle(&nopCloserFile{f}), fileModeRead, 0, &chdFileHandle)
	if err := l.makeError(chdErrCode); err != nil {
		return nil, fmt.Errorf("chd: open: %w", err)
	}
	defer l.close(chdFileHandle)

	// clone to not refer to C memory
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

func (f *File) init() error {
	if f.handle == 0 {
		return fs.ErrClosed
	}

	return nil
}

func (f *File) loadHunk(hunkNum int) error {
	if hunkNum == f.currentHunkNum && len(f.currentHunkData) > 0 {
		// already loaded
		return nil
	}

	// allocate on-demand only
	if len(f.currentHunkData) == 0 {
		f.currentHunkData = make([]byte, f.Header.HunkBytes)
	}

	readRes := f.lib.read(f.handle, uint32(hunkNum), &f.currentHunkData[0])
	if err := f.lib.makeError(readRes); err != nil {
		return fmt.Errorf("chd: read hunk %d: %w", hunkNum, err)
	}

	f.currentHunkNum = hunkNum
	return nil
}

func (f *File) Read(b []byte) (int, error) {
	if err := f.init(); err != nil {
		return 0, err
	}

	if f.offset >= int64(f.Header.LogicalBytes) {
		// at EOF
		return 0, io.EOF
	}

	read := 0
	newOffset := f.offset
	// either buffer is filled or file is ended
	for len(b) > 0 && newOffset < int64(f.Header.LogicalBytes) {
		// decompress hunk if needed
		desiredHunkNum := int(newOffset / int64(f.Header.HunkBytes))
		offsetInHunk := newOffset % int64(f.Header.HunkBytes)

		if desiredHunkNum < 0 || desiredHunkNum >= int(f.Header.TotalHunks) {
			break
		}

		// a small optimization to avoid excessive copying
		// if request buffer is large enough to fit whole hunk from the beginning
		// we can just read it directly into a Header
		if offsetInHunk == 0 && len(b) >= int(f.Header.HunkBytes) {
			readRes := f.lib.read(f.handle, uint32(desiredHunkNum), &b[0])
			if err := f.lib.makeError(readRes); err != nil {
				return read, fmt.Errorf("chd: direct read hunk %d: %w", desiredHunkNum, err)
			}

			read += int(f.Header.HunkBytes)
			newOffset += int64(f.Header.HunkBytes)
			b = b[f.Header.HunkBytes:]
			continue
		}

		if err := f.loadHunk(desiredHunkNum); err != nil {
			return read, err
		}

		// now just copy unpacked data to our target buffer
		copied := copy(b, f.currentHunkData[offsetInHunk:])
		read += copied
		newOffset += int64(copied)
		b = b[copied:]
	}

	f.offset = newOffset
	return read, nil
}

func (f *File) Seek(offset int64, whence int) (int64, error) {
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

	if offset < 0 || offset > int64(f.Header.LogicalBytes) {
		return 0, syscall.EINVAL
	}

	f.offset = offset
	return offset, nil
}

type fileStat struct {
	fs.FileInfo
	header     *FileHeader
	cdMetadata []CDMetadata
}

func (s *fileStat) Size() int64 {
	return int64(s.header.LogicalBytes)
}

func (s *fileStat) Mode() fs.FileMode {
	return s.FileInfo.Mode() | fs.ModeIrregular
}

func (f *File) Stat() (fs.FileInfo, error) {
	if err := f.init(); err != nil {
		return nil, err
	}
	return f.originalFileInfo, nil
}

func (f *File) Close() error {
	if f.handle == 0 {
		return fs.ErrClosed
	}

	f.cleanup.Stop()
	f.lib.close(f.handle)
	return nil
}
