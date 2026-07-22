//go:build !windows && !nopurego && (((android || ios || linux || darwin || freebsd || netbsd) && (amd64 || arm64)) || (android && (386 || arm)) || (linux && (386 || arm || loong64 || ppc64le || riscv64 || s390x)))

package osutil

import (
	"path/filepath"
	"runtime"

	"github.com/ebitengine/purego"
)

type CDecl = purego.CDecl

func LoadLibrary(name string) (handle uintptr, err error) {
	dlExt := ".so"
	if runtime.GOOS == "darwin" {
		dlExt = ".dylib"
	}
	if !filepath.IsAbs(name) && filepath.Ext(name) != dlExt {
		name += dlExt
	}

	// RTLD_NOW to gather as much errors as possible right now
	return purego.Dlopen(name, purego.RTLD_NOW|purego.RTLD_LOCAL)
}

func LoadSystemLibrary(name string) (handle uintptr, err error) {
	return purego.Dlopen(name, purego.RTLD_NOW|purego.RTLD_GLOBAL)
}

func UnloadLibrary(handle uintptr) error {
	return purego.Dlclose(handle)
}

func RegisterLibFunc(fptr any, handle uintptr, name string) {
	purego.RegisterLibFunc(fptr, handle, name)
}

func NewCallback(fptr any) uintptr {
	return purego.NewCallback(fptr)
}
