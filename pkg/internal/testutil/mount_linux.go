package testutil

import (
	"fmt"
	"io/ioutil"
	"os"
	"syscall"
)

const (
	MountSupported           = true
	MultiExtentFileSupported = true
)

const loopControl = "/dev/loop-control"

func ioctl(fd uintptr, req uint, arg uintptr) (err error) {
	_, _, e1 := syscall.Syscall(syscall.SYS_IOCTL, fd, uintptr(req), arg)
	if e1 != 0 {
		return e1
	}

	return
}

func MountISO(isoPath string) (targetPath string, cleanup func() error, err error) {
	tempDir, err := ioutil.TempDir("", "ps3netsrv_iso")
	if err != nil {
		return "", nil, fmt.Errorf("failed to make temp dir: %w", err)
	}

	isoFile, err := os.OpenFile(isoPath, os.O_RDONLY, os.ModePerm)
	if err != nil {
		return "", nil, fmt.Errorf("iso open failed: %w", err)
	}

	defer isoFile.Close()

	loopDevNumber, err := freeLoopNr()
	if err != nil {
		return "", nil, fmt.Errorf("loop find failed: %w", err)
	}

	loopPath := fmt.Sprintf("/dev/loop%d", loopDevNumber)

	loopDev, err := os.OpenFile(loopPath, os.O_RDWR, os.ModeDevice)
	if err != nil {
		return "", nil, fmt.Errorf("loop open failed: %w", err)
	}

	defer loopDev.Close()

	const (
		loopSetFD   = 0x4C00
		loopClearFD = 0x4C01
	)

	if err := ioctl(loopDev.Fd(), loopSetFD, isoFile.Fd()); err != nil {
		return "", nil, fmt.Errorf("loop setup failed: %w", err)
	}

	if err := syscall.Mount(loopPath, isoPath, "iso9660", syscall.MS_RDONLY, ""); err != nil {
		return "", nil, fmt.Errorf("failed to mount loop device: %w", err)
	}

	return tempDir, func() error {
		defer os.Remove(tempDir)

		if err := syscall.Unmount(loopPath, 0); err != nil {
			return fmt.Errorf("loop unmount failed: %w", err)
		}

		loopDev, err := os.OpenFile(loopPath, os.O_RDWR, os.ModeDevice)
		if err != nil {
			return fmt.Errorf("loop open failed: %w", err)
		}

		defer loopDev.Close()

		if err := ioctl(loopDev.Fd(), loopClearFD, 0); err != nil {
			return fmt.Errorf("loop clear failed: %w", err)
		}

		return nil
	}, nil
}

func freeLoopNr() (loopNr uintptr, err error) {
	const loopCtlGetFree = 0x4C82

	loopctl, err := os.OpenFile(loopControl, os.O_RDWR, os.ModeDevice)
	if err != nil {
		return 0, fmt.Errorf("open loop-control failed: %w", err)
	}

	defer loopctl.Close()

	_, _, nr := syscall.Syscall(syscall.SYS_IOCTL, loopctl.Fd(), loopCtlGetFree, 0)
	if nr == -1 {
		return 0, fmt.Errorf("no free loop devices")
	}

	return uintptr(nr), nil
}
