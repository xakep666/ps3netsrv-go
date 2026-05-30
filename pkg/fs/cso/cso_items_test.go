package cso_test

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/xakep666/ps3netsrv-go/pkg/fs/cso"
)

func TestIndexEntryCache(t *testing.T) {
	const entriesCount = 5
	var indexEntries []uint32
	for i := range uint32(entriesCount) {
		indexEntries = append(indexEntries, i+10)
	}
	hdr := &cso.Header{
		Magic:      cso.CISOMagic,
		Version:    1,
		BlockSize:  2048,
		HeaderSize: cso.AdvisedHeaderSize,
	}
	hdr.UncompressedSize = uint64(hdr.BlockSize) * (entriesCount - 1)

	data, err := binary.Append(nil, binary.LittleEndian, hdr)
	require.NoError(t, err)

	data, err = binary.Append(data, binary.LittleEndian, indexEntries)
	require.NoError(t, err)

	t.Run("forward reading", func(t *testing.T) {
		buf := newReadTracker(data)
		const maxEntries = 2
		iec := cso.NewIndexEntryCache(buf, hdr, maxEntries)

		var prevReadCount int
		for i := range entriesCount {
			value, err := iec.ValueOf(i)
			require.NoError(t, err)
			require.Equal(t, uint32(i+10), value)

			if i%maxEntries == 0 {
				require.Equal(t, prevReadCount+1, buf.ReadCount, "read must occur to fill cache, item %d", i)
				prevReadCount = buf.ReadCount
			} else {
				require.Equal(t, prevReadCount, buf.ReadCount, "read must not occur for cached values, item %d", i)
			}
		}
	})

	t.Run("random access", func(t *testing.T) {
		buf := newReadTracker(data)
		const maxEntries = 2
		iec := cso.NewIndexEntryCache(buf, hdr, maxEntries)

		type testCase struct {
			idx        int
			readOccurs bool
		}

		var prevReadCount int
		for _, tc := range []testCase{
			{2, true},
			{3, false},
			{4, true},
			{0, true},
			{1, false},
			{4, true},
		} {
			value, err := iec.ValueOf(tc.idx)
			require.NoError(t, err)
			require.Equal(t, uint32(tc.idx+10), value)

			if tc.readOccurs {
				require.Equal(t, prevReadCount+1, buf.ReadCount, "read must occur to fill cache, item %d", tc.idx)
				prevReadCount = buf.ReadCount
			} else {
				require.Equal(t, prevReadCount, buf.ReadCount, "read must not occur for cached values, item %d", tc.idx)
			}
		}
	})

	t.Run("out of bounds", func(t *testing.T) {
		buf := newReadTracker(data)
		const maxEntries = 2
		iec := cso.NewIndexEntryCache(buf, hdr, maxEntries)

		_, err := iec.ValueOf(-1)
		require.Error(t, err)

		_, err = iec.ValueOf(entriesCount + 1)
		require.Error(t, err)
	})
}

type readTracker struct {
	*bytes.Reader
	ReadCount int
}

func newReadTracker(data []byte) *readTracker {
	return &readTracker{Reader: bytes.NewReader(data)}
}

func (m *readTracker) Read(b []byte) (int, error) {
	m.ReadCount++
	return m.Reader.Read(b)
}
