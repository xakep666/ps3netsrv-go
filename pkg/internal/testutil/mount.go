//+build !darwin,!linux,!windows

package testutil

const (
	MountSupported           = false
	MultiExtentFileSupported = false
)

func MountISO(isoPath string) (targetPath string, cleanup func() error, err error) {
	panic("not supported platform")
}
