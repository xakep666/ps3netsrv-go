package testutil

import (
	"testing"
	"unsafe"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
)

const (
	MountSupported           = true
	MultiExtentFileSupported = true
)

var (
	virtdiskDLL = windows.NewLazySystemDLL("virtdisk.dll")

	openVirtualDisk            = virtdiskDLL.NewProc("OpenVirtualDisk")
	attachVirtualDisk          = virtdiskDLL.NewProc("AttachVirtualDisk")
	getVirtualDiskPhysicalPath = virtdiskDLL.NewProc("GetVirtualDiskPhysicalPath")
	detachVirtualDisk          = virtdiskDLL.NewProc("DetachVirtualDisk")
)

type openParameters struct {
	DeviceID uint32
	GUID     windows.GUID
}

type attachParameters struct {
	Version int
}

func MountISO(t *testing.T, isoPath string) string {
	t.Helper()
	var handle windows.Handle

	pathPtr, err := windows.UTF16PtrFromString(isoPath)
	require.NoError(t, err, "failed to make path pointer")

	_, _, err = openVirtualDisk.Call(
		// VirtualStorageType
		uintptr(unsafe.Pointer(&openParameters{
			DeviceID: 1, // iso
		})),
		// Path
		uintptr(unsafe.Pointer(pathPtr)),
		// VirtualDiskAccessMask
		0x003f0000, // VIRTUAL_DISK_ACCESS_ALL
		// Flags
		0,
		// Parameters
		0,
		// Handle
		uintptr(unsafe.Pointer(&handle)),
	)
	require.NoError(t, err, "failed to open virtual disk")

	_, _, err = attachVirtualDisk.Call(
		// Handle
		uintptr(handle),
		// SecurityDescriptor
		0,
		// Flags
		1, // ATTACH_VIRTUAL_DISK_FLAG_READ_ONLY
		// ProviderSpecificFlags
		0,
		// ProviderSpecificFlags
		0,
		// Parameters,
		uintptr(unsafe.Pointer(&attachParameters{
			Version: 1,
		})),
		// Overlapped
		0,
	)
	require.NoError(t, err, "failed to attach virtual disk")

	var path [windows.MAX_PATH / 2]uint16

	_, _, err = getVirtualDiskPhysicalPath.Call(
		// Handle,
		uintptr(handle),
		// DiskPathSizeInBytes
		windows.MAX_PATH,
		// DiskPath
		uintptr(unsafe.Pointer(&path[0])),
	)
	require.NoError(t, err, "failed to get disk path")

	t.Cleanup(func() {
		t.Helper()

		_, _, err := detachVirtualDisk.Call(
			// Handle
			uintptr(handle),
			// Flags
			0,
			// ProviderSpecificFlags
			0,
		)
		assert.NoError(t, err, "failed to detach virtual disk")

		require.NoError(t, windows.Close(handle))
	})

	return windows.UTF16ToString(path[:])
}
