package testutil

import (
	"fmt"
	"unsafe"

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

func MountISO(isoPath string) (targetPath string, cleanup func() error, err error) {
	var handle windows.Handle

	pathPtr, err := windows.UTF16PtrFromString(isoPath)
	if err != nil {
		return "", nil, fmt.Errorf("failed to make path pointer: %w", err)
	}

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
	if err != nil {
		return "", nil, fmt.Errorf("failed open virtual disk: %w", err)
	}

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
	if err != nil {
		return "", nil, fmt.Errorf("failed attach virtual disk: %w", err)
	}

	var path [windows.MAX_PATH / 2]uint16

	_, _, err = getVirtualDiskPhysicalPath.Call(
		// Handle,
		uintptr(handle),
		// DiskPathSizeInBytes
		windows.MAX_PATH,
		// DiskPath
		uintptr(unsafe.Pointer(&path[0])),
	)
	if err != nil {
		return "", nil, fmt.Errorf("failed to get disk path: %w", err)
	}

	return windows.UTF16ToString(path[:]), func() error {
		defer windows.Close(handle)

		_, _, err := detachVirtualDisk.Call(
			// Handle
			uintptr(handle),
			// Flags
			0,
			// ProviderSpecificFlags
			0,
		)

		return err
	}, nil
}
