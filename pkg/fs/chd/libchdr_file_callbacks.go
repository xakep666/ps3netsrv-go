//go:build !aix && !ppc64

package chd

import (
	"errors"
	"io"
	"log/slog"
	"math"
	"structs"
	"syscall"
	"unsafe"

	"github.com/ebitengine/purego"

	"github.com/xakep666/ps3netsrv-go/internal/handler"
	"github.com/xakep666/ps3netsrv-go/internal/logutil"
	"github.com/xakep666/ps3netsrv-go/internal/osutil"
)

// fileCallbacks are called by libchdr itself to outsource i/o operations
type fileCallbacks struct {
	_ structs.HostLayout

	fsize  uintptr
	fread  uintptr
	fclose uintptr
	fseek  uintptr
}

func newFileCallbacks(log *slog.Logger) *fileCallbacks {
	// *osutil.Handle used to propagate a Go value through C code without it's being garbage-collected
	return &fileCallbacks{
		fsize: purego.NewCallback(func(_ purego.CDecl, userdata *osutil.Handle) uint64 {
			f := userdata.Value().(handler.File)
			ret, err := f.Seek(0, io.SeekEnd)
			if err != nil {
				log.Error("chd: seek/fsize failed", logutil.ErrorAttr(err), slog.String("name", f.Name()))
				return math.MaxUint64
			}
			return uint64(ret)
		}),
		fread: purego.NewCallback(func(_ purego.CDecl, target *byte, size, count int64, userdata *osutil.Handle) int64 {
			f := userdata.Value().(handler.File)
			n, err := f.Read(unsafe.Slice(target, count*size))
			switch {
			case errors.Is(err, nil):
				return int64(n)
			case errors.Is(err, io.EOF):
				return 0
			default:
				log.Error("chd: read failed", logutil.ErrorAttr(err), slog.String("name", f.Name()))
				return int64(extractSysErrCode(err))
			}
		}),
		fclose: purego.NewCallback(func(_ purego.CDecl, userdata *osutil.Handle) int {
			defer userdata.Delete()

			f := userdata.Value().(handler.File)
			err := f.Close()
			if err != nil {
				log.Error("chd: close failed", logutil.ErrorAttr(err), slog.String("name", f.Name()))
				return extractSysErrCode(err)
			}
			return 0
		}),
		fseek: purego.NewCallback(func(_ purego.CDecl, userdata *osutil.Handle, offset int64, whence int) int {
			f := userdata.Value().(handler.File)
			_, err := f.Seek(offset, whence)
			if err != nil {
				log.Error("chd: seek failed", logutil.ErrorAttr(err), slog.String("name", f.Name()))
				return extractSysErrCode(err)
			}
			return 0
		}),
	}
}

func extractSysErrCode(err error) int {
	if err == nil {
		return 0
	}

	sysErr, ok := errors.AsType[syscall.Errno](err)
	if ok {
		return -int(sysErr)
	}
	return -1
}
