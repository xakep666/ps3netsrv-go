package testutil

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"unicode"
)

const (
	MountSupported           = true
	MultiExtentFileSupported = false
)

func MountISO(isoPath string) (targetPath string, cleanup func() error, err error) {
	tempDir, err := os.MkdirTemp("", "ps3netsrv_iso")
	if err != nil {
		return "", nil, fmt.Errorf("failed to make temp dir: %w", err)
	}

	devPath, err := exec.Command("hdiutil", "attach", "-nomount", isoPath).Output()
	if err != nil {
		_ = os.Remove(tempDir)

		return "", nil, fmt.Errorf("hdiutil error: %w", err)
	}

	devPath = bytes.TrimFunc(devPath, unicode.IsSpace)

	detach := func() error {
		defer os.Remove(tempDir)

		out, err := exec.Command("hdiutil", "detach", string(devPath)).CombinedOutput()
		if err != nil {
			return fmt.Errorf("hdiutil error: %s: %w", bytes.Trim(out, "\n\t"), err)
		}

		return nil
	}

	out, err := exec.Command("osascript", "-e",
		fmt.Sprintf(
			`do shell script "mount_cd9660 -o nobrowse -er %s %s" with prompt "Mount ISO" with administrator privileges`,
			devPath, tempDir,
		),
	).CombinedOutput()
	if err != nil {
		_ = detach()

		return "", nil, fmt.Errorf("mount error: %s: %w", out, err)
	}

	return tempDir, detach, nil
}
