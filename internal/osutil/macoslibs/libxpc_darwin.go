package macoslibs

import (
	"sync"
	"syscall"

	"github.com/xakep666/ps3netsrv-go/internal/osutil/dynamiclibs"
)

type LibXPC struct {
	launchActivateSocket func(name string, fds **int32, fdslen *uint) uintptr
}

var getLibXPC = sync.OnceValues(func() (*LibXPC, error) {
	handle, err := dynamiclibs.LoadSystemLibrary("/usr/lib/system/libxpc.dylib")
	if err != nil {
		return nil, err
	}

	ret := new(LibXPC)
	dynamiclibs.RegisterLibFunc(&ret.launchActivateSocket, handle, "launch_activate_socket")

	return ret, nil
})

func GetLibXPC() (*LibXPC, error) {
	return getLibXPC()
}

func (l *LibXPC) LaunchActivateSocket(name string, fds **int32, fdslen *uint) error {
	ret := l.launchActivateSocket(name, fds, fdslen)
	if ret != 0 {
		return syscall.Errno(ret)
	}

	return nil
}
