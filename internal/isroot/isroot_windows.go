package isroot

import (
	"golang.org/x/sys/windows"
)

func IsRoot() bool {
	return windows.GetCurrentProcessToken().IsElevated()
}
