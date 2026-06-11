package chd

import (
	"bytes"
	"crypto/md5"
	"crypto/sha1"
	"encoding/binary"
	"fmt"
	"io"
	"iter"
	"math"

	"github.com/xakep666/ps3netsrv-go/internal/ioutil"
)

/***************************************************************************

    Compressed Hunks of Data header format. All numbers are stored in
    Motorola (big-endian) byte ordering. The header is 76 (V1) or 80 (V2)
    bytes long.

    V1 header:

    [  0] char   tag[8];        // 'MComprHD'
    [  8] uint32_t length;        // length of header (including tag and length fields)
    [ 12] uint32_t version;       // drive format version
    [ 16] uint32_t flags;         // flags (see below)
    [ 20] uint32_t compression;   // compression type
    [ 24] uint32_t hunksize;      // 512-byte sectors per hunk
    [ 28] uint32_t totalhunks;    // total # of hunks represented
    [ 32] uint32_t cylinders;     // number of cylinders on hard disk
    [ 36] uint32_t heads;         // number of heads on hard disk
    [ 40] uint32_t sectors;       // number of sectors on hard disk
    [ 44] uint8_t  md5[16];       // MD5 checksum of raw data
    [ 60] uint8_t  parentmd5[16]; // MD5 checksum of parent file
    [ 76] (V1 header length)

    V2 header:

    [  0] char   tag[8];        // 'MComprHD'
    [  8] uint32_t length;        // length of header (including tag and length fields)
    [ 12] uint32_t version;       // drive format version
    [ 16] uint32_t flags;         // flags (see below)
    [ 20] uint32_t compression;   // compression type
    [ 24] uint32_t hunksize;      // seclen-byte sectors per hunk
    [ 28] uint32_t totalhunks;    // total # of hunks represented
    [ 32] uint32_t cylinders;     // number of cylinders on hard disk
    [ 36] uint32_t heads;         // number of heads on hard disk
    [ 40] uint32_t sectors;       // number of sectors on hard disk
    [ 44] uint8_t  md5[16];       // MD5 checksum of raw data
    [ 60] uint8_t  parentmd5[16]; // MD5 checksum of parent file
    [ 76] uint32_t seclen;        // number of bytes per sector
    [ 80] (V2 header length)

    V3 header:

    [  0] char   tag[8];        // 'MComprHD'
    [  8] uint32_t length;        // length of header (including tag and length fields)
    [ 12] uint32_t version;       // drive format version
    [ 16] uint32_t flags;         // flags (see below)
    [ 20] uint32_t compression;   // compression type
    [ 24] uint32_t totalhunks;    // total # of hunks represented
    [ 28] uint64_t logicalbytes;  // logical size of the data (in bytes)
    [ 36] uint64_t metaoffset;    // offset to the first blob of metadata
    [ 44] uint8_t  md5[16];       // MD5 checksum of raw data
    [ 60] uint8_t  parentmd5[16]; // MD5 checksum of parent file
    [ 76] uint32_t hunkbytes;     // number of bytes per hunk
    [ 80] uint8_t  sha1[20];      // SHA1 checksum of raw data
    [100] uint8_t  parentsha1[20];// SHA1 checksum of parent file
    [120] (V3 header length)

    V4 header:

    [  0] char   tag[8];        // 'MComprHD'
    [  8] uint32_t length;        // length of header (including tag and length fields)
    [ 12] uint32_t version;       // drive format version
    [ 16] uint32_t flags;         // flags (see below)
    [ 20] uint32_t compression;   // compression type
    [ 24] uint32_t totalhunks;    // total # of hunks represented
    [ 28] uint64_t logicalbytes;  // logical size of the data (in bytes)
    [ 36] uint64_t metaoffset;    // offset to the first blob of metadata
    [ 44] uint32_t hunkbytes;     // number of bytes per hunk
    [ 48] uint8_t  sha1[20];      // combined raw+meta SHA1
    [ 68] uint8_t  parentsha1[20];// combined raw+meta SHA1 of parent
    [ 88] uint8_t  rawsha1[20];   // raw data SHA1
    [108] (V4 header length)

    Flags:
        0x00000001 - set if this drive has a parent
        0x00000002 - set if this drive allows writes

   =========================================================================

    V5 header:

    [  0] char   tag[8];        // 'MComprHD'
    [  8] uint32_t length;        // length of header (including tag and length fields)
    [ 12] uint32_t version;       // drive format version
    [ 16] uint32_t compressors[4];// which custom compressors are used?
    [ 32] uint64_t logicalbytes;  // logical size of the data (in bytes)
    [ 40] uint64_t mapoffset;     // offset to the map
    [ 48] uint64_t metaoffset;    // offset to the first blob of metadata
    [ 56] uint32_t hunkbytes;     // number of bytes per hunk (512k maximum)
    [ 60] uint32_t unitbytes;     // number of bytes per unit within each hunk
    [ 64] uint8_t  rawsha1[20];   // raw data SHA1
    [ 84] uint8_t  sha1[20];      // combined raw+meta SHA1
    [104] uint8_t  parentsha1[20];// combined raw+meta SHA1 of parent
    [124] (V5 header length)

    If parentsha1 != 0, we have a parent (no need for flags)
    If compressors[0] == 0, we are uncompressed (including maps)

    V5 uncompressed map format:

    [  0] uint32_t offset;        // starting offset / hunk size

    V5 compressed map format header:

    [  0] uint32_t length;        // length of compressed map
    [  4] UINT48 datastart;     // offset of first block
    [ 10] uint16_t crc;           // crc-16 of the map
    [ 12] uint8_t lengthbits;     // bits used to encode complength
    [ 13] uint8_t hunkbits;       // bits used to encode self-refs
    [ 14] uint8_t parentunitbits; // bits used to encode parent unit refs
    [ 15] uint8_t reserved;       // future use
    [ 16] (compressed header length)

    Each compressed map entry, once expanded, looks like:

    [  0] uint8_t compression;    // compression type
    [  1] UINT24 complength;    // compressed length
    [  4] UINT48 offset;        // offset
    [ 10] uint16_t crc;           // crc-16 of the data

***************************************************************************/

const (
	chdV1HeaderSize = 76
	chdV2HeaderSize = 80
	chdV3HeaderSize = 120
	chdV4HeaderSize = 108
	chdV5HeaderSize = 124

	chdMaxHeaderSize = chdV5HeaderSize

	chdMagic = "MComprHD"

	chdV1SectorSize = 512

	chdMaxHunkSize = 128 * 1024 * 1024
	chdMaxFileSize = 1024 * 1024 * 1024 * 1024

	chdMetadataHeaderSize = 16
	chdMaxMetadataEntries = 65536

	chdCDMaxSectorData  = 2352
	chdCDMaxSubcodeData = 96
	cdMetadataOldTag    = ('C' << 24) | ('H' << 16) | ('C' << 8) | 'D'
	cdMetadataTag       = ('C' << 24) | ('H' << 16) | ('T' << 8) | 'R'
	cdMetadataTag2      = ('C' << 24) | ('H' << 16) | ('T' << 8) | '2'
)

type MetadataEntryHeader struct {
	Offset uint64
	Next   uint64
	Prev   uint64
	Length uint32
	Tag    uint32
	Flags  byte
}

// ReadHeader is a pure-go (without libchdr) function to get header from file.
func ReadHeader(f io.ReadSeeker) (*FileHeader, error) {
	buf := make([]byte, chdMaxHeaderSize)
	err := ioutil.FillBuffer(f, 0, buf)
	if err != nil {
		return nil, err
	}

	if string(buf[:len(chdMagic)]) != chdMagic {
		return nil, fmt.Errorf("unexpected chd magic %x", buf[:len(chdMagic)])
	}

	headerSize := binary.BigEndian.Uint32(buf[8:])
	version := binary.BigEndian.Uint32(buf[12:])

	var ret *FileHeader

	switch version {
	case 1, 2:
		var (
			secLen             uint32
			expectedHeaderSize uint32
		)
		if version == 1 {
			secLen = chdV1SectorSize
			expectedHeaderSize = chdV1HeaderSize
		} else {
			secLen = binary.BigEndian.Uint32(buf[76:])
			expectedHeaderSize = chdV2HeaderSize
		}
		if headerSize != expectedHeaderSize {
			return nil, fmt.Errorf("invalid v%d header size: %d", version, expectedHeaderSize)
		}

		ret = &FileHeader{
			Length:  headerSize,
			Version: version,
			Flags:   binary.BigEndian.Uint32(buf[16:]),
			Compression: [4]CompressionCodec{
				0: CompressionCodec(binary.BigEndian.Uint32(buf[20:])),
			},
			obsoleteHunkSize:  binary.BigEndian.Uint32(buf[24:]),
			TotalHunks:        binary.BigEndian.Uint32(buf[28:]),
			obsoleteCylinders: binary.BigEndian.Uint32(buf[32:]),
			obsoleteHeads:     binary.BigEndian.Uint32(buf[36:]),
			obsoleteSectors:   binary.BigEndian.Uint32(buf[40:]),
			MD5:               [md5.Size]byte(buf[44 : 44+md5.Size]),
			ParentMD5:         [md5.Size]byte(buf[60 : 60+md5.Size]),
		}

		hunkBytes := uint64(secLen) * uint64(ret.obsoleteHunkSize)
		ret.LogicalBytes = uint64(ret.obsoleteCylinders) * uint64(ret.obsoleteHeads) * uint64(ret.obsoleteSectors) * chdV1HeaderSize
		if hunkBytes == 0 || hunkBytes > math.MaxUint32 { // overflow check
			return nil, fmt.Errorf("invalid hunkBytes: %d", hunkBytes)
		}
		ret.HunkBytes = uint32(hunkBytes)
		ret.UnitBytes, err = guessUnitBytes(f, ret)
		if err != nil {
			return nil, fmt.Errorf("guess unit bytes: %w", err)
		}
		ret.UnitCount = ret.LogicalBytes + uint64(ret.UnitBytes) - 1
	case 3:
		if headerSize != chdV3HeaderSize {
			return nil, fmt.Errorf("invalid v3 header size: %d", headerSize)
		}

		ret = &FileHeader{
			Length:  headerSize,
			Version: 3,
			Flags:   binary.BigEndian.Uint32(buf[16:]),
			Compression: [4]CompressionCodec{
				0: CompressionCodec(binary.BigEndian.Uint32(buf[20:])),
			},
			TotalHunks:   binary.BigEndian.Uint32(buf[24:]),
			LogicalBytes: binary.BigEndian.Uint64(buf[28:]),
			MetaOffset:   binary.BigEndian.Uint64(buf[36:]),
			MD5:          [md5.Size]byte(buf[44 : 44+md5.Size]),
			ParentMD5:    [md5.Size]byte(buf[60 : 60+md5.Size]),
			HunkBytes:    binary.BigEndian.Uint32(buf[76:]),
			SHA1:         [sha1.Size]byte(buf[80 : 80+sha1.Size]),
			ParentSHA1:   [sha1.Size]byte(buf[100 : 100+sha1.Size]),
		}
		ret.UnitBytes, err = guessUnitBytes(f, ret)
		if err != nil {
			return nil, fmt.Errorf("guess unit bytes: %w", err)
		}
		ret.UnitCount = (ret.LogicalBytes + uint64(ret.UnitBytes) - 1) / uint64(ret.UnitBytes)
	case 4:
		if headerSize != chdV4HeaderSize {
			return nil, fmt.Errorf("invalid v4 header size: %d", headerSize)
		}

		ret = &FileHeader{
			Length:  headerSize,
			Version: 4,
			Flags:   binary.BigEndian.Uint32(buf[16:]),
			Compression: [4]CompressionCodec{
				0: CompressionCodec(binary.BigEndian.Uint32(buf[20:])),
			},
			TotalHunks:   binary.BigEndian.Uint32(buf[24:]),
			LogicalBytes: binary.BigEndian.Uint64(buf[28:]),
			MetaOffset:   binary.BigEndian.Uint64(buf[36:]),
			HunkBytes:    binary.BigEndian.Uint32(buf[44:]),
			SHA1:         [sha1.Size]byte(buf[48 : 48+sha1.Size]),
			ParentSHA1:   [sha1.Size]byte(buf[68 : 68+sha1.Size]),
			RawSHA1:      [sha1.Size]byte(buf[88 : 88+sha1.Size]),
		}
		ret.UnitBytes, err = guessUnitBytes(f, ret)
		if err != nil {
			return nil, fmt.Errorf("guess unit bytes: %w", err)
		}
		ret.UnitCount = (ret.LogicalBytes + uint64(ret.UnitBytes) - 1) / uint64(ret.UnitBytes)
	case 5:
		if headerSize != chdV5HeaderSize {
			return nil, fmt.Errorf("invalid v5 header size: %d", headerSize)
		}

		ret = &FileHeader{
			Length:  headerSize,
			Version: 5,
			Compression: [4]CompressionCodec{
				0: CompressionCodec(binary.BigEndian.Uint32(buf[16:])),
				1: CompressionCodec(binary.BigEndian.Uint32(buf[20:])),
				2: CompressionCodec(binary.BigEndian.Uint32(buf[24:])),
				3: CompressionCodec(binary.BigEndian.Uint32(buf[28:])),
			},
			LogicalBytes: binary.BigEndian.Uint64(buf[32:]),
			MapOffset:    binary.BigEndian.Uint64(buf[40:]),
			MetaOffset:   binary.BigEndian.Uint64(buf[48:]),
			HunkBytes:    binary.BigEndian.Uint32(buf[56:]),
			UnitBytes:    binary.BigEndian.Uint32(buf[60:]),
			RawSHA1:      [sha1.Size]byte(buf[64 : 64+sha1.Size]),
			SHA1:         [sha1.Size]byte(buf[84 : 84+sha1.Size]),
			ParentSHA1:   [sha1.Size]byte(buf[104 : 104+sha1.Size]),
		}

		if ret.HunkBytes == 0 {
			return nil, fmt.Errorf("zero hunk bytes")
		}
		ret.HunkCount = uint32((ret.LogicalBytes + uint64(ret.HunkBytes) - 1) / uint64(ret.HunkBytes))

		if ret.UnitBytes == 0 {
			return nil, fmt.Errorf("zero unit bytes")
		}
		ret.UnitCount = (ret.LogicalBytes + uint64(ret.UnitBytes) - 1) / uint64(ret.UnitBytes)

		if ret.Compression[0] != CompressionNone {
			ret.mapEntryBytes = 12
		} else {
			ret.mapEntryBytes = 4
		}

		ret.TotalHunks = ret.HunkCount
	default:
		return nil, fmt.Errorf("unexpected chd version %d", version)
	}

	if ret.Version <= 4 {
		if (ret.Flags & 0xfffffffc) != 0 {
			return nil, fmt.Errorf("invalid flags: %x", ret.Flags)
		}

		switch ret.Compression[0] {
		case CompressionNone, CompressionZlib, CompressionZlibPlus, CompressionAV:
		default:
			return nil, fmt.Errorf("invalid compression codec: %d", ret.Compression[0])
		}

		if ret.HunkBytes == 0 || ret.HunkBytes >= 65536*256 {
			return nil, fmt.Errorf("invalid hunk size %d", ret.HunkBytes)
		}

		if ret.TotalHunks == 0 {
			return nil, fmt.Errorf("zero total hunks")
		}

		if (ret.Flags&0x1) != 0 && ret.ParentMD5 == ([md5.Size]byte{}) && ret.ParentSHA1 == ([sha1.Size]byte{}) {
			return nil, fmt.Errorf("parent sha1 or md5 required, 'has parent' flag present")
		}

		if ret.Version >= 3 && (ret.obsoleteCylinders != 0 || ret.obsoleteHeads != 0 ||
			ret.obsoleteSectors != 0 || ret.obsoleteHunkSize != 0) {
			return nil, fmt.Errorf("obsolete fields must not present for v3 and later")
		}

		if ret.Version < 3 && (ret.obsoleteCylinders == 0 || ret.obsoleteHeads == 0 ||
			ret.obsoleteSectors == 0 || ret.obsoleteHunkSize == 0) {
			return nil, fmt.Errorf("obsolete fields must present for pre-v3")
		}
	}

	if ret.HunkBytes >= chdMaxHunkSize || uint64(ret.HunkBytes)*uint64(ret.TotalHunks) >= chdMaxFileSize {
		return nil, fmt.Errorf("large hunk bytes (%d) x total hunks (%d)", ret.HunkBytes, ret.TotalHunks)
	}

	return ret, nil
}

func IterateMetadata(f io.ReadSeeker, hdr *FileHeader) (iter.Seq[MetadataEntryHeader], func() error) {
	pErr := new(error)
	return func(yield func(MetadataEntryHeader) bool) {
			ret := MetadataEntryHeader{
				Offset: hdr.MetaOffset,
			}
			buf := make([]byte, chdMetadataHeaderSize)

			for range chdMaxMetadataEntries {
				if ret.Offset == 0 {
					return
				}

				if err := ioutil.FillBuffer(f, int64(ret.Offset), buf); err != nil {
					*pErr = err
					return
				}

				ret.Tag = binary.BigEndian.Uint32(buf[0:])
				ret.Length = binary.BigEndian.Uint32(buf[4:])
				ret.Next = binary.BigEndian.Uint64(buf[8:])

				ret.Flags = byte(ret.Length >> 24)
				ret.Length &= 0x00ffffff

				if !yield(ret) {
					return
				}

				ret.Prev = ret.Offset
				ret.Offset = ret.Next
			}

			*pErr = fmt.Errorf("too many metadata entries")
		},
		func() error {
			return *pErr
		}
}

func (hdr *MetadataEntryHeader) ReadValue(f io.ReadSeeker, buf []byte) (int, error) {
	n := min(len(buf), int(hdr.Length))
	if err := ioutil.FillBuffer(f, int64(hdr.Offset+chdMetadataHeaderSize), buf); err != nil {
		return 0, err
	}
	return n, nil
}

func guessUnitBytes(f io.ReadSeeker, hdr *FileHeader) (uint32, error) {
	mdIter, mdErr := IterateMetadata(f, hdr)
	md := make([]byte, 512)
	var idx int
	for hdr := range mdIter {
		switch hdr.Tag {
		case ('G' << 24) | ('D' << 16) | ('D' << 8) | 'D': // hard disk image, extract from metadata
			n, err := hdr.ReadValue(f, md)
			if err != nil {
				return 0, fmt.Errorf("meta idx %d read: %w", idx, err)
			}

			var cyls, heads, secs, bps int
			_, err = fmt.Fscanf(
				bytes.NewReader(md[:n]),
				"CYLS:%d,HEADS:%d,SECS:%d,BPS:%d",
				&cyls, &heads, &secs, &bps,
			)
			if err != nil {
				return 0, fmt.Errorf("decode hdd metadata %q", md[:n])
			}

			return uint32(bps), err
		case cdMetadataOldTag,
			cdMetadataTag,
			cdMetadataTag2,
			('C' << 24) | ('H' << 16) | ('G' << 8) | 'T', // GDROM old
			('C' << 24) | ('H' << 16) | ('G' << 8) | 'D': // GDROM
			return chdCDMaxSectorData + chdCDMaxSubcodeData, nil
		}
		idx++
	}
	if err := mdErr(); err != nil {
		return 0, err
	}

	// for versions 3 and older return special value
	if hdr.Version < 3 {
		if hdr.obsoleteHunkSize != 0 {
			return hdr.HunkBytes / hdr.obsoleteHunkSize, nil
		} else {
			return 0, nil
		}
	}

	return hdr.HunkBytes, nil
}
