package chd

import (
	"crypto/md5"
	"crypto/sha1"
	"errors"
	"fmt"
	"io/fs"
	"runtime"
	"strings"
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
	CompressionCDFrontendMask CompressionCodec = ('c' << 24) | ('d' << 16)
	CompressionCodecCDZlib    CompressionCodec = CompressionCDFrontendMask | ('z' << 8) | 'l'
	CompressionCodecCDLZMA    CompressionCodec = CompressionCDFrontendMask | ('l' << 8) | 'z'
	CompressionCodecCDFLAC    CompressionCodec = CompressionCDFrontendMask | ('f' << 8) | 'l'
	CompressionCodecCDZstd    CompressionCodec = CompressionCDFrontendMask | ('z' << 8) | 's'
)

type MapCompressionType uint8

const (
	// # of codec in FileHeader.Compression
	MapCompressionType0 MapCompressionType = iota
	MapCompressionType1
	MapCompressionType2
	MapCompressionType3
	MapCompressionTypeNone // uncompressed
	MapCompressionTypeSelf // reference to another hunk
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

func (c CompressionCodec) IsCD() bool {
	return (c & 0xFFFF0000) == CompressionCDFrontendMask
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

func (h *FileHeader) IsCDCodesOnly() bool {
	return h.Compression[0].IsCD() && // first elem must always be not none
		(h.Compression[1].IsCD() || h.Compression[1] == CompressionNone) &&
		(h.Compression[2].IsCD() || h.Compression[2] == CompressionNone) &&
		(h.Compression[3].IsCD() || h.Compression[3] == CompressionNone)
}

type CDMetadata struct {
	TrackNumber   int
	Type          string
	Subtype       string
	Frames        int
	Pregap        int
	PregapType    string
	PregapSubType string
	Postgap       int
}

func (m *CDMetadata) IsAudio() bool {
	return strings.EqualFold(m.Type, "AUDIO")
}

func (m *CDMetadata) IsData() bool {
	return !m.IsAudio()
}

func (m *CDMetadata) SectorDataSize() int {
	switch strings.ToUpper(m.Type) {
	case "AUDIO":
		return 2352
	case "MODE1":
		return 2048
	case "MODE1_RAW":
		return 2352
	case "MODE1/2048":
		return 2048
	case "MODE1/2352":
		return 2352
	case "MODE2":
		return 2336
	case "MODE2_RAW":
		return 2352
	case "MODE2/2336":
		return 2336
	case "MODE2/2352":
		return 2352
	case "MODE2_FORM1":
		return 2048
	case "MODE2/2048":
		return 2048
	case "MODE2_FORM2":
		return 2328
	case "MODE2/2324":
		return 2324
	default:
		return 2352
	}
}

func (m *CDMetadata) String() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "TRACK:%d ", m.TrackNumber)
	fmt.Fprintf(&sb, "TYPE:%s ", m.Type)
	fmt.Fprintf(&sb, "SUBTYPE:%s ", m.Subtype)
	fmt.Fprintf(&sb, "FRAMES:%d ", m.Frames)
	fmt.Fprintf(&sb, "PREGAP:%d ", m.Pregap)
	fmt.Fprintf(&sb, "PGTYPE:%s ", m.PregapType)
	fmt.Fprintf(&sb, "PGSUB:%s ", m.PregapType)
	fmt.Fprintf(&sb, "POSTGAP:%d ", m.Postgap)
	return sb.String()
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

type fileHandle uintptr

type File struct {
	Header     *FileHeader
	CDMetadata []CDMetadata

	lib              *LibCHDR
	originalName     string
	originalFileInfo fs.FileInfo
	handle           fileHandle
	cleanup          runtime.Cleanup

	offset          int64
	currentHunkNum  int
	currentHunkData []byte
}

func (*File) ReadDir(int) ([]fs.DirEntry, error) {
	return nil, errors.ErrUnsupported
}

func (f *File) Name() string {
	return f.originalName
}

type privateFile = File

// CDFile is a wrapper around File that helps to properly decode it if cd-codecs are used.
type CDFile struct {
	*privateFile
	Size           int64
	SectorsCount   int64
	SectorDataSize int

	offset int64
}

func (f *File) AsCD() (*CDFile, error) {
	if !f.Header.IsCDCodesOnly() {
		return nil, fmt.Errorf("file must use only cd codecs")
	}

	if len(f.CDMetadata) == 0 {
		return nil, fmt.Errorf("cd metadata must present for cd file")
	}

	// check that sector data size is consistent across all tracks
	var totalFrames int64
	var sectorDataSize int
	for _, md := range f.CDMetadata {
		totalFrames += int64(md.Frames)

		if sectorDataSize == 0 {
			sectorDataSize = md.SectorDataSize()
			continue
		}
		if sectorDataSize != md.SectorDataSize() {
			return nil, fmt.Errorf("inconsistent sector size across metadata, first is %d, got %d",
				sectorDataSize, md.SectorDataSize())
		}
	}

	return &CDFile{
		privateFile:    f,
		Size:           int64(sectorDataSize) * int64(totalFrames),
		SectorsCount:   totalFrames,
		SectorDataSize: sectorDataSize,
	}, nil
}
