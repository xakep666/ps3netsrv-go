package socketactivation

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"syscall"
	"unsafe"

	"github.com/xakep666/ps3netsrv-go/internal/osutil/macoslibs"
)

func ActivationListeners(name string) ([]net.Listener, error) {
	files, err := activationFiles(name)
	if err != nil {
		return nil, err
	}

	listeners := make([]net.Listener, 0, len(files))
	for _, file := range files {
		if lis, err := net.FileListener(file); err == nil {
			listeners = append(listeners, lis)
		}
	}

	return listeners, nil
}

func activationFiles(name string) ([]*os.File, error) {
	const maxFDs = 1 << 20

	libc, err := macoslibs.GetLibc()
	if err != nil {
		return nil, err
	}

	libxpc, err := macoslibs.GetLibXPC()
	if err != nil {
		return nil, err
	}

	var (
		fdsRaw *int32
		fdsLen uint
	)

	defer func() {
		if fdsRaw != nil {
			libc.Free(unsafe.Pointer(fdsRaw))
		}
	}()

	err = libxpc.LaunchActivateSocket(name, &fdsRaw, &fdsLen)
	if fdsLen > maxFDs {
		err = syscall.EINVAL
	}
	if err != nil {
		return nil, fmt.Errorf("launch_activate_socket: %w", err)
	}

	ret := make([]*os.File, 0, fdsLen)
	for i, fd := range unsafe.Slice(fdsRaw, fdsLen) {
		ret = append(ret, os.NewFile(uintptr(fd), name+":"+strconv.Itoa(i)))
	}

	return ret, nil
}
