//go:build aix || ppc64

package osutil

import "errors"

func LoadLibrary(name string) (uintptr, error) {
	return 0, errors.ErrUnsupported
}

func UnloadLibrary(uintptr) error {
	return errors.ErrUnsupported
}
