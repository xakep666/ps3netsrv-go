//go:build windows

package osutil

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

func LoadLibrary(name string) (handle uintptr, err error) {
	if filepath.Ext(name) != ".dll" {
		name += ".dll"
	}

	if !filepath.IsAbs(name) {
		if exe, exeErr := os.Executable(); exeErr == nil {
			name = filepath.Join(filepath.Dir(exe), name)
		}
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
