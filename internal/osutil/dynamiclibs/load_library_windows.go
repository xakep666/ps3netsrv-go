//go:build windows && !nopurego

package dynamiclibs

import (
	"path/filepath"

	"github.com/ebitengine/purego"
	"golang.org/x/sys/windows"
)

type CDecl = purego.CDecl

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

func LoadSystemLibrary(name string) (handle uintptr, err error) {
	dll := windows.NewLazySystemDLL(name)
	if err := dll.Load(); err != nil {
		return 0, err
	}

	return dll.Handle(), nil
}

func UnloadLibrary(handle uintptr) error {
	return windows.FreeLibrary(windows.Handle(handle))
}

func RegisterLibFunc(fptr any, handle uintptr, name string) {
	purego.RegisterLibFunc(fptr, handle, name)
}

func NewCallback(fptr any) uintptr {
	return purego.NewCallback(fptr)
}
