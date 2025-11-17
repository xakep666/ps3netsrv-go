package testutil

import (
	"fmt"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

const (
	MountSupported           = true
	MultiExtentFileSupported = true
)

// syscalls will return an errno type (which implements error) for all calls,
// including success (errno 0). We only care about non-zero errnos.
func errnoIsErr(err syscall.Errno) error {
	if err != 0 {
		return err
	}
	return nil
}

// Given a handle to a Loopback device (such as /dev/loop0), and a handle
// to the image to loop mount (such as a squashfs or ext4fs image), preform
// the required call to loop the image to the provided block device.
func loop(loopbackDevice, image *os.File) error {
	_, _, err := unix.Syscall(
		unix.SYS_IOCTL,
		loopbackDevice.Fd(),
		unix.LOOP_SET_FD,
		image.Fd(),
	)
	return errnoIsErr(err)
}

// Given a handle to the Loopback device (such as /dev/loop0), preform the
// required call to the image to unloop the file.
func unloop(loopbackDevice *os.File) error {
	_, _, err := unix.Syscall(unix.SYS_IOCTL, loopbackDevice.Fd(), unix.LOOP_CLR_FD, 0)
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
	fd, err := unix.Open("/dev/loop-control", os.O_RDONLY, 0644)
	if err != nil {
		return 0, err
	}
	defer unix.Close(fd)
	index, _, sysErr := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), unix.LOOP_CTL_GET_FREE, 0)
	return int(index), errnoIsErr(sysErr)
}

func MountISO(isoPath string) (targetPath string, cleanup func() error, err error) {
	tempDir, err := os.MkdirTemp("", "ps3netsrv_iso")
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

	if err := unix.Mount(lo.Name(), tempDir, "iso9660", unix.MS_RDONLY, ""); err != nil {
		unloop(lo)
		lo.Close()
		return "", nil, fmt.Errorf("mount failed: %w", err)
	}

	return tempDir, func() error {
		defer os.Remove(tempDir)
		defer lo.Close()

		if err := unix.Unmount(targetPath, 0); err != nil {
			return fmt.Errorf("loop unmount failed: %w", err)
		}

		if err := unloop(lo); err != nil {
			return fmt.Errorf("unloop failed: %w", err)
		}

		return nil
	}, nil
}
