package osutil

import (
	"sync"
	"unsafe"
)

type Libc struct {
	free func(unsafe.Pointer)
}

var getLibc = sync.OnceValues(func() (*Libc, error) {
	handle, err := LoadSystemLibrary("/usr/lib/libSystem.B.dylib")
	if err != nil {
		return nil, err
	}

	ret := new(Libc)
	RegisterLibFunc(&ret.free, handle, "free")

	return ret, nil
})

func GetLibc() (*Libc, error) {
	return getLibc()
}

func (l *Libc) Free(ptr unsafe.Pointer) {
	l.free(ptr)
}
