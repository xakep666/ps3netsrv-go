package fs_test

import (
	"crypto/rand"
	"crypto/sha256"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xakep666/ps3netsrv-go/internal/testutil"
	"github.com/xakep666/ps3netsrv-go/pkg/fs"
)

func TestMakeFullImage(t *testing.T) {
	if !testutil.MountSupported {
		t.Skip("mount not supported on this platform")
		return
	}

	if !testutil.MultiExtentFileSupported {
		t.Log("Multiextent files not supported on this platform")
	}

	const (
		isoRoot     = "iso_root"
		bigFileSize = 4*1024*1024*1024 + // 4Gb
			200*1024*1024 + 133 // 200Mb + a bit
	)

	var bigFileHash []byte

	baseFS, err := fs.NewFS(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { _ = baseFS.Close() })

	require.NoError(t, baseFS.Mkdir(isoRoot, os.ModePerm))
	require.NoError(t,
		baseFS.WriteFile(filepath.Join(isoRoot, "test.txt"), []byte("hello world"), os.ModePerm),
	)
	require.NoError(t, baseFS.Mkdir(filepath.Join(isoRoot, "dir1"), os.ModePerm))
	require.NoError(t,
		baseFS.WriteFile(filepath.Join(isoRoot, "dir1", "A.TXT"), []byte("a content"), os.ModePerm),
	)
	require.NoError(t, baseFS.Mkdir(filepath.Join(isoRoot, "dir1", "DIR2"), os.ModePerm))
	require.NoError(t,
		baseFS.WriteFile(filepath.Join(isoRoot, "dir1", "DIR2", "b.txt"), []byte("b content"), os.ModePerm),
	)
	require.NoError(t,
		baseFS.WriteFile(filepath.Join(isoRoot, "dir1", "c.txt"), []byte("c content"), os.ModePerm),
	)

	var multiSectorFile [3123]byte
	_, err = io.ReadFull(rand.Reader, multiSectorFile[:])
	require.NoError(t, err)

	require.NoError(t, baseFS.Mkdir(filepath.Join(isoRoot, "dir2"), os.ModePerm))
	require.NoError(t,
		baseFS.WriteFile(filepath.Join(isoRoot, "dir2", "multisector.bin"), multiSectorFile[:], os.ModePerm),
	)

	if !testing.Short() {
		t.Logf("Generate big file to test multiextent file")
		f, err := baseFS.Create(filepath.Join(isoRoot, "dir2", "big.bin"))
		require.NoError(t, err)

		hw := sha256.New()

		n, err := io.CopyN(io.MultiWriter(f, hw), rand.Reader, bigFileSize)
		require.NoError(t, err)
		require.Equal(t, int64(bigFileSize), n)

		require.NoError(t, f.Close())
		bigFileHash = hw.Sum(nil)

		t.Logf("Generated file hash: %x", bigFileHash)
	}

	viso, err := fs.NewVirtualISO(baseFS, isoRoot, false)
	require.NoError(t, err)

	isoFile, err := os.CreateTemp("", "test_gen*.iso")
	require.NoError(t, err)

	t.Logf("Created ISO file at %s", isoFile.Name())

	t.Cleanup(func() {
		if !t.Failed() {
			require.NoError(t, os.Remove(isoFile.Name()))
		} else {
			t.Logf("Not removing file so it can be debugged")
		}
	})

	written, err := io.Copy(isoFile, viso)
	if assert.NoError(t, err) {
		assert.Zero(t, written%0x800, "total size must be a multiply of sector size")
	}

	require.NoError(t, isoFile.Close())

	t.Logf("Root permission may be required for mounting")
	mountPath, unmount, err := testutil.MountISO(isoFile.Name())
	require.NoError(t, err)

	t.Cleanup(func() { require.NoError(t, unmount()) })

	t.Run("test.txt", func(t *testing.T) {
		content, err := os.ReadFile(filepath.Join(mountPath, "test.txt"))
		if assert.NoError(t, err) {
			assert.Equal(t, "hello world", string(content))
		}
	})

	t.Run("dir1/A.TXT", func(t *testing.T) {
		content, err := os.ReadFile(filepath.Join(mountPath, "dir1", "A.TXT"))
		if assert.NoError(t, err) {
			assert.Equal(t, "a content", string(content))
		}
	})

	t.Run("dir1/DIR2/b.txt", func(t *testing.T) {
		content, err := os.ReadFile(filepath.Join(mountPath, "dir1", "DIR2", "b.txt"))
		if assert.NoError(t, err) {
			assert.Equal(t, "b content", string(content))
		}
	})

	t.Run("dir1/c.txt", func(t *testing.T) {
		content, err := os.ReadFile(filepath.Join(mountPath, "dir1", "c.txt"))
		if assert.NoError(t, err) {
			assert.Equal(t, "c content", string(content))
		}
	})

	t.Run("dir2/multisector.bin", func(t *testing.T) {
		content, err := os.ReadFile(filepath.Join(mountPath, "dir2", "multisector.bin"))
		if assert.NoError(t, err) {
			assert.Equal(t, multiSectorFile[:], content)
		}
	})

	if !testing.Short() && testutil.MultiExtentFileSupported {
		t.Run("dir2/big.bin", func(t *testing.T) {
			stat, err := os.Stat(filepath.Join(mountPath, "dir2", "big.bin"))
			if assert.NoError(t, err) {
				assert.Equal(t, int64(bigFileSize), stat.Size())
			}

			hw := sha256.New()
			f, err := os.Open(filepath.Join(mountPath, "dir2", "big.bin"))
			require.NoError(t, err)

			_, err = io.Copy(hw, f)
			require.NoError(t, err)

			require.NoError(t, f.Close())

			assert.Equal(t, bigFileHash, hw.Sum(nil))
		})
	}
}
