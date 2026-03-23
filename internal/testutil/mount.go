//go:build !darwin && !linux && !windows

package testutil

import (
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	MountSupported           = false
	MultiExtentFileSupported = false
)

func MountISO(t *testing.T, isoPath string) string {
	t.Helper()
	require.FailNow(t, "unsupported platform")
	return ""
}
