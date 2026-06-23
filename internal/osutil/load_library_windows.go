//go:build windows

package osutil

import (
	"path/filepath"

	"golang.org/x/sys/windows"
)

func LoadLibrary(name string) (handle uintptr, err error) {
	if filepath.Ext(name) != ".dll" {
		name += ".dll"
	}

	flags := uintptr(0)
	if filepath.IsAbs(name) {
		flags = windows.LOAD_WITH_ALTERED_SEARCH_PATH
	}

	h, err := windows.LoadLibraryEx(
		name,
		windows.Handle(0),
		flags,
	)
	return uintptr(h), err
}

func UnloadLibrary(handle uintptr) error {
	return windows.FreeLibrary(windows.Handle(handle))
}
