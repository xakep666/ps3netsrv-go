package fs

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"

	"github.com/spf13/afero"
)

var sfoMagic = [...]byte{0, 'P', 'S', 'F'}

type sfoHeader struct {
	Magic             [4]byte
	Version           [4]byte
	KeyTableStart     uint32
	DataTableStart    uint32
	TableEntriesCount uint32
}

type sfoIndexTableEntry struct {
	KeyOffset  uint16 // relative to key table start (i.e. 0 for first key)
	DataFormat uint16
	DataLen    uint32
	DataMaxLen uint32
	DataOffset uint32 // relative to data table start
}

// sfoField returns provided field from param.sfo file.
// See https://psdevwiki.com/ps3/PARAM.SFO for file format.
func sfoField(f afero.File, field string) (string, error) {
	var hdr sfoHeader

	if err := binary.Read(f, binary.LittleEndian, &hdr); err != nil {
		return "", fmt.Errorf("sfo header read failed: %w", err)
	}

	if hdr.Magic != sfoMagic {
		return "", fmt.Errorf("bad sfo magic: %s", hdr.Magic)
	}

	var idxEntry *sfoIndexTableEntry

	for i := uint32(0); i < hdr.TableEntriesCount; i++ {
		var e sfoIndexTableEntry

		indexEntryOff := binary.Size(hdr) + int(i)*binary.Size(e)

		if _, err := f.Seek(int64(indexEntryOff), io.SeekStart); err != nil {
			return "", fmt.Errorf("seek to index entry %d failed: %w", i, err)
		}

		if err := binary.Read(f, binary.LittleEndian, &e); err != nil {
			return "", fmt.Errorf("failed to parse index table entry: %w", err)
		}

		keyOff := hdr.KeyTableStart + uint32(e.KeyOffset)

		_, err := f.Seek(int64(keyOff), io.SeekStart)
		if err != nil {
			return "", fmt.Errorf("failed to seek to key at %d: %w", keyOff, err)
		}

		key, err := bufio.NewReader(f).ReadBytes(0)
		if err != nil {
			return "", fmt.Errorf("failed to read key at %d: %w", keyOff, err)
		}

		if string(key[:len(key)-1]) == field {
			idxEntry = &e
			break
		}
	}

	if idxEntry == nil {
		return "", fmt.Errorf("field was not found")
	}

	off := int64(hdr.DataTableStart) + int64(idxEntry.DataOffset)

	_, err := f.Seek(off, io.SeekStart)
	if err != nil {
		return "", fmt.Errorf("failed to seek to key at %d: %w", off, err)
	}

	ret := make([]byte, idxEntry.DataLen)

	_, err = io.ReadFull(f, ret)
	if err != nil {
		return "", fmt.Errorf("failed to read value: %w", err)
	}

	return string(ret[:len(ret)-1]), nil
}
