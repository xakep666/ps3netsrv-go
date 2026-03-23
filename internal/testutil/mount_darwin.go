package testutil

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"unicode"

	"github.com/stretchr/testify/require"
)

const (
	MountSupported           = true
	MultiExtentFileSupported = false
)

func MountISO(t *testing.T, isoPath string) string {
	t.Helper()

	tempDir, err := os.MkdirTemp(t.TempDir(), "ps3netsrv_iso")
	require.NoError(t, err, "failed to make temp dir")

	devPath, err := exec.Command("hdiutil", "attach", "-nomount", isoPath).Output()
	require.NoErrorf(t, err, "hdutil error")

	devPath = bytes.TrimFunc(devPath, unicode.IsSpace)

	t.Cleanup(func() {
		t.Helper()

		out, err := exec.Command("hdiutil", "detach", string(devPath)).CombinedOutput()
		require.NoErrorf(t, err, "hdiutil error: %w", bytes.Trim(out, "\n\t"))
	})

	out, err := exec.Command("osascript", "-e",
		fmt.Sprintf(
			`do shell script "mount_cd9660 -o nobrowse -er %s %s" with prompt "Mount ISO" with administrator privileges`,
			devPath, tempDir,
		),
	).CombinedOutput()
	require.NoErrorf(t, err, "mount error: %s", out)

	return tempDir
}
