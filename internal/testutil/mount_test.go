package testutil_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xakep666/ps3netsrv-go/internal/testutil"
)

func TestMountISO(t *testing.T) {
	if !testutil.MountSupported {
		t.Skip("mount not supported on this platform")
		return
	}

	t.Logf("Root permission may be required for mounting")
	mountPath, cleanup, err := testutil.MountISO(filepath.Join("testdata", "testimg.iso"))
	require.NoError(t, err)

	t.Cleanup(func() {
		t.Helper()
		require.NoError(t, cleanup())
	})

	entries, err := os.ReadDir(mountPath)
	if assert.NoError(t, err) {
		assert.Len(t, entries, 1)
		assert.Equal(t, "test.txt", entries[0].Name())
	}

	content, err := os.ReadFile(filepath.Join(mountPath, "test.txt"))
	if assert.NoError(t, err) {
		assert.Equal(t, "hello world!!", string(content))
	}
}
