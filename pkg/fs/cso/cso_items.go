package cso

import (
	"encoding/binary"
	"fmt"
	"io"
)

var (
	CISOMagic = [4]byte{'C', 'I', 'S', 'O'}
	ZISOMagic = [4]byte{'Z', 'I', 'S', 'O'}
)

const AdvisedHeaderSize = 0x18 // for CSOv2 and ZSO

type Variant int

const (
	Unknown Variant = -1 + iota
	CSOv1
	CSOv2
	ZSO
)

type Header struct {
	Magic            [4]byte
	HeaderSize       uint32
	UncompressedSize uint64
	BlockSize        uint32
	Version          byte
	IndexShift       byte
	_                [2]byte
}

func ReadHeader(r io.Reader) (*Header, error) {
	var ret Header
	if err := binary.Read(r, binary.LittleEndian, &ret); err != nil {
		return nil, err
	}
	return &ret, nil
}

func (h *Header) BlocksCount() int {
	count := h.UncompressedSize / uint64(h.BlockSize)
	if h.UncompressedSize%uint64(h.BlockSize) > 0 {
		count++
	}
	return int(count)
}

func (h *Header) Variant() Variant {
	switch h.Magic {
	case CISOMagic:
		switch h.Version {
		case 1:
			return CSOv1
		case 2:
			return CSOv2
		}
	case ZISOMagic:
		switch h.Version {
		case 1:
			return ZSO
		}
	}
	return Unknown
}

type IndexEntries []byte // uint32s, values are little endian

func MakeIndexEntries(size int) IndexEntries { return make(IndexEntries, size*4) }

func (e IndexEntries) ValueOf(idx int) uint32 {
	return binary.LittleEndian.Uint32(e[idx*4:])
}

// IndexEntryCache holds some index entries in memory and updates from file on-demand
type IndexEntryCache struct {
	baseOffset int64
	f          io.ReadSeeker
	hdr        *Header

	startEntryNum int
	entries       IndexEntries
}

func NewIndexEntryCache(f io.ReadSeeker, hdr *Header, maxEntries int) *IndexEntryCache {
	return &IndexEntryCache{
		baseOffset:    int64(binary.Size(Header{})),
		f:             f,
		hdr:           hdr,
		startEntryNum: -1,
		entries:       make(IndexEntries, 0, maxEntries*4),
	}
}

func (c *IndexEntryCache) ValueOf(entryNum int) (uint32, error) {
	entriesCount := c.hdr.BlocksCount() + 1
	if entryNum < 0 || entryNum > entriesCount {
		return 0, fmt.Errorf("block %d out of bounds [0;%d)", entryNum, entriesCount)
	}

	if entryNum >= c.startEntryNum && (entryNum-c.startEntryNum)*4 < len(c.entries) {
		return c.entries.ValueOf(entryNum - c.startEntryNum), nil
	}

	_, err := c.f.Seek(c.baseOffset+int64(entryNum)*4, io.SeekStart)
	if err != nil {
		return 0, err
	}

	c.entries = c.entries[:min(cap(c.entries), (entriesCount-entryNum)*4)]
	_, err = io.ReadFull(c.f, c.entries)
	if err != nil {
		return 0, err
	}

	c.startEntryNum = entryNum

	return c.entries.ValueOf(0), nil
}

func (c *IndexEntryCache) TopBitOf(entryNum int) (bool, error) {
	val, err := c.ValueOf(entryNum)
	if err != nil {
		return false, err
	}

	return val&(1<<31) != 0, nil
}

func (c *IndexEntryCache) OffsetOf(entryNum int) (uint64, error) {
	val, err := c.ValueOf(entryNum)
	if err != nil {
		return 0, err
	}

	return uint64(val&^(1<<31)) << c.hdr.IndexShift, nil
}
