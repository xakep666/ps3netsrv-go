package testutil

import (
	"fmt"
	"os"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"
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

func MountISO(t *testing.T, isoPath string) string {
	t.Helper()

	tempDir, err := os.MkdirTemp(t.TempDir(), "ps3netsrv_iso")
	require.NoError(t, err, "failed to make temp dir")

	isoFile, err := os.OpenFile(isoPath, os.O_RDONLY, os.ModePerm)
	require.NoError(t, err, "failed to open iso")

	t.Cleanup(func() {
		t.Helper()
		require.NoError(t, isoFile.Close())
	})

	lo, err := nextLoopDevice()
	require.NoError(t, err, "get next loop device")

	t.Cleanup(func() {
		t.Helper()
		require.NoError(t, lo.Close())
	})

	require.NoError(t, loop(lo, isoFile), "loop failed")
	t.Cleanup(func() {
		t.Helper()
		require.NoError(t, unloop(lo))
	})

	require.NoError(t, unix.Mount(lo.Name(), tempDir, "iso9660", unix.MS_RDONLY, ""), "mount failed")
	t.Cleanup(func() {
		t.Helper()
		require.NoError(t, unix.Unmount(tempDir, 0))
	})

	return tempDir
}
