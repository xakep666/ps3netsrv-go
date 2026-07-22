package macoslibs

import (
	"sync"
	"unsafe"

	"github.com/xakep666/ps3netsrv-go/internal/osutil/dynamiclibs"
)

type Libc struct {
	free func(unsafe.Pointer)
}

var getLibc = sync.OnceValues(func() (*Libc, error) {
	handle, err := dynamiclibs.LoadSystemLibrary("/usr/lib/libSystem.B.dylib")
	if err != nil {
		return nil, err
	}

	ret := new(Libc)
	dynamiclibs.RegisterLibFunc(&ret.free, handle, "free")

	return ret, nil
})

func GetLibc() (*Libc, error) {
	return getLibc()
}

func (l *Libc) Free(ptr unsafe.Pointer) {
	l.free(ptr)
}
