//go:build !windows && !aix && !ppc64

package osutil

import (
	"path/filepath"
	"runtime"

	"github.com/ebitengine/purego"
)

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

func UnloadLibrary(handle uintptr) error {
	return purego.Dlclose(handle)
}
