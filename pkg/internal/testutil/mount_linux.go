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

// IOCTL consts
const (
	loopSetFd      = 0x4C00
	loopCtlGetFree = 0x4C82
	loopClrFd      = 0x4C01
)

// syscalls will return an errno type (which implements error) for all calls,
// including success (errno 0). We only care about non-zero errnos.
func errnoIsErr(err error) error {
	if err.(syscall.Errno) != 0 {
		return err
	}
	return nil
}

// Given a handle to a Loopback device (such as /dev/loop0), and a handle
// to the image to loop mount (such as a squashfs or ext4fs image), preform
// the required call to loop the image to the provided block device.
func loop(loopbackDevice, image *os.File) error {
	_, _, err := syscall.Syscall(
		syscall.SYS_IOCTL,
		loopbackDevice.Fd(),
		loopSetFd,
		image.Fd(),
	)
	return errnoIsErr(err)
}

// Given a handle to the Loopback device (such as /dev/loop0), preform the
// required call to the image to unloop the file.
func unloop(loopbackDevice *os.File) error {
	_, _, err := syscall.Syscall(syscall.SYS_IOCTL, loopbackDevice.Fd(), loopClrFd, 0)
	return errnoIsErr(err)
}

// Get the next loopback device that isn't used. Under the hood this will ask
// loop-control for the LOOP_CTL_GET_FREE value, and interpolate that into
// the conventional GNU/Linux naming scheme for loopback devices, and os.Open
// that path.
func nextLoopDevice() (*os.File, error) {
	loopInt, err := nextUnallocatedLoop()
	if err != nil {
		return nil, err
	}
	return os.Open(fmt.Sprintf("/dev/loop%d", loopInt))
}

// Return the integer of the next loopback device we can use by calling
// loop-control with the LOOP_CTL_GET_FREE ioctl.
func nextUnallocatedLoop() (int, error) {
	fd, err := os.OpenFile("/dev/loop-control", os.O_RDONLY, 0644)
	if err != nil {
		return 0, err
	}
	defer fd.Close()
	index, _, err := syscall.Syscall(syscall.SYS_IOCTL, fd.Fd(), loopCtlGetFree, 0)
	return int(index), errnoIsErr(err)
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

	lo, err := nextLoopDevice()
	if err != nil {
		return "", nil, err
	}

	if err := loop(lo, isoFile); err != nil {
		lo.Close()
		return "", nil, fmt.Errorf("loop failed: %w", err)
	}

	if err := syscall.Mount(lo.Name(), tempDir, "iso9660", syscall.MS_RDONLY, ""); err != nil {
		unloop(lo)
		lo.Close()
		return "", nil, fmt.Errorf("mount failed: %w", err)
	}

	return tempDir, func() error {
		defer os.Remove(tempDir)
		defer lo.Close()

		if err := syscall.Unmount(targetPath, 0); err != nil {
			return fmt.Errorf("loop unmount failed: %w", err)
		}

		if err := unloop(lo); err != nil {
			return fmt.Errorf("unloop failed: %w", err)
		}

		return nil
	}, nil
}
