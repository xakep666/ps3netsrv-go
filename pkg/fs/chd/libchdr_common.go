package chd

import (
	"crypto/md5"
	"crypto/sha1"
	"errors"
	"fmt"
	"io/fs"
	"structs"
)

type CompressionCodec uint32

const (
	CompressionNone CompressionCodec = 0

	// v1-v4 codecs
	CompressionZlib     CompressionCodec = 1
	CompressionZlibPlus CompressionCodec = 2
	CompressionAV       CompressionCodec = 3

	// v5 codecs
	CompressionCodecZlib    CompressionCodec = ('z' << 24) | ('l' << 16) | ('i' << 8) | 'b'
	CompressionCodecLZMA    CompressionCodec = ('l' << 24) | ('z' << 16) | ('m' << 8) | 'a'
	CompressionCodecHuffman CompressionCodec = ('h' << 24) | ('u' << 16) | ('f' << 8) | 'f'
	CompressionCodecFLAC    CompressionCodec = ('f' << 24) | ('l' << 16) | ('a' << 8) | 'c'
	CompressionCodecZstd    CompressionCodec = ('z' << 24) | ('s' << 16) | ('t' << 8) | 'd'
	// v5 codecs w/ CD frontend
	CompressionCodecCDZlib CompressionCodec = ('c' << 24) | ('d' << 16) | ('z' << 8) | 'l'
	CompressionCodecCDLZMA CompressionCodec = ('c' << 24) | ('d' << 16) | ('l' << 8) | 'z'
	CompressionCodecCDFLAC CompressionCodec = ('c' << 24) | ('d' << 16) | ('f' << 8) | 'l'
	CompressionCodecCDZstd CompressionCodec = ('c' << 24) | ('d' << 16) | ('z' << 8) | 's'
)

// FileHeader holds some basic CHD file metadata.
type FileHeader struct {
	_ structs.HostLayout

	Length       uint32 // of header in file
	Version      uint32
	Flags        uint32
	Compression  [4]CompressionCodec
	HunkBytes    uint32 // this is how much you should allocate for reading
	TotalHunks   uint32 // this is used to limit amount of reads
	LogicalBytes uint64 // uncompressed size of source file
	MetaOffset   uint64
	MapOffset    uint64
	MD5          [md5.Size]byte
	ParentMD5    [md5.Size]byte
	SHA1         [sha1.Size]byte // with metadata
	RawSHA1      [sha1.Size]byte // of original file
	ParentSHA1   [sha1.Size]byte
	UnitBytes    uint32 // actually an uncompressed sector size
	UnitCount    uint64 // total amount of sectors
	HunkCount    uint32

	// unexported to avoid raw access by user
	mapEntryBytes uint32
	rawMap        uintptr

	// don't used anymore
	obsoleteCylinders uint32
	obsoleteHeads     uint32
	obsoleteSectors   uint32
	obsoleteHunkSize  uint32
}

func (c CompressionCodec) String() string {
	switch c {
	case CompressionNone:
		return "None"
	case CompressionZlib:
		return "Zlib"
	case CompressionZlibPlus:
		return "Zlib+"
	case CompressionAV:
		return "AV"
	case CompressionCodecZlib:
		return "zlib (Zlib)"
	case CompressionCodecLZMA:
		return "lzma (LZMA)"
	case CompressionCodecHuffman:
		return "huff (Huffman)"
	case CompressionCodecFLAC:
		return "flac (FLAC)"
	case CompressionCodecZstd:
		return "zstd (Zstd)"
	case CompressionCodecCDZlib:
		return "cdzl (CD Zlib)"
	case CompressionCodecCDLZMA:
		return "cdlz (CD LZMA)"
	case CompressionCodecCDFLAC:
		return "cdfl (CD FLAC)"
	case CompressionCodecCDZstd:
		return "cdzs (CD Zstd)"
	default:
		return fmt.Sprintf("<unknown:%d>", uint32(c))
	}
}

type errorCode uint

type Error struct {
	code    errorCode
	message string
}

func (e *Error) Error() string {
	if e.message != "" {
		return e.message
	}

	return fmt.Sprintf("chd error code %d", e.code)
}

func (e *Error) Is(other error) bool {
	otherError, ok := errors.AsType[*Error](other)
	if !ok {
		return false
	}

	return e.code == otherError.code
}

type chdFileStat struct {
	fs.FileInfo
	header *FileHeader
}

func (s *chdFileStat) Size() int64 {
	return int64(s.header.LogicalBytes)
}

func (s *chdFileStat) Mode() fs.FileMode {
	return s.FileInfo.Mode() | fs.ModeIrregular
}

type StatSysWithHeader struct {
	Header *FileHeader
	Sys    any
}

func (s *chdFileStat) Sys() any {
	return &StatSysWithHeader{
		Header: s.header,
		Sys:    s.FileInfo.Sys(),
	}
}
