//go:build nopurego || (!((android || ios || linux || darwin || windows || freebsd || netbsd) && (amd64 || arm64)) && !((android || windows) && (386 || arm)) && !(linux && (386 || arm || loong64 || ppc64le || riscv64 || s390x)))

package osutil

import "errors"

type CDecl struct{}

func LoadLibrary(name string) (uintptr, error) {
	return 0, errors.ErrUnsupported
}

func UnloadLibrary(uintptr) error {
	return errors.ErrUnsupported
}

func RegisterLibFunc(fptr any, handle uintptr, name string) {
	panic(errors.ErrUnsupported)
}

func NewCallback(fptr any) uintptr {
	panic(errors.ErrUnsupported)
}
