package fs

import (
	"os"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVirtualISO(t *testing.T) {
	fs := afero.NewMemMapFs()
	afero.WriteFile(fs, "PKG/test.txt", []byte("hello world!!"), os.ModePerm)

	f, err := NewVirtualISO(fs, "PKG", false)
	require.NoError(t, err)

	out := make(disc, len(f.fsBuf))
	copy(out, f.fsBuf)

	out.appendSector([]byte("hello world!!"))

	padSectors := basePadSectors

	if extraPad := sizeBytes(len(out)).sectors() % basePadSectors; extraPad > 0 {
		padSectors += basePadSectors - extraPad
	}

	out = append(out, make([]byte, padSectors.bytes())...)

	os.WriteFile("../../test_gen.iso", out, os.ModePerm)
}

func TestSFO(t *testing.T) {
	fs := afero.NewMemMapFs()
	afero.WriteFile(fs, "param.sfo", []byte{
		0x00, 0x50, 0x53, 0x46, 0x01, 0x01, 0x00, 0x00, 0x24, 0x00, 0x00, 0x00, 0x30, 0x00, 0x00, 0x00,
		0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x04, 0x02, 0x0A, 0x00, 0x00, 0x00, 0x0F, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x54, 0x49, 0x54, 0x4C, 0x45, 0x5F, 0x49, 0x44, 0x00, 0x00, 0x00, 0x00,
		0x42, 0x4C, 0x55, 0x53, 0x31, 0x32, 0x33, 0x34, 0x35, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	}, os.ModePerm)

	f, err := fs.Open("param.sfo")
	require.NoError(t, err)

	v, err := sfoField(f, "TITLE_ID")
	if assert.NoError(t, err) {
		assert.Equal(t, "BLUS12345", v)
	}
}
