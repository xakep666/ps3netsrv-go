//go:build windows

package osutil

import (
	"path/filepath"

	"golang.org/x/sys/windows"
)

func LoadLibrary(name string) (handle uintptr, err error) {
	if !filepath.IsAbs(name) && filepath.Ext(name) != ".dll" {
		name += ".dll"
	}

	h, err := windows.LoadLibraryEx(
		name,
		windows.Handle(0),
		windows.LOAD_LIBRARY_SEARCH_DEFAULT_DIRS|
			windows.LOAD_LIBRARY_SEARCH_DLL_LOAD_DIR,
	)
	return uintptr(h), err
}

func UnloadLibrary(handle uintptr) error {
	return windows.FreeLibrary(windows.Handle(handle))
}
