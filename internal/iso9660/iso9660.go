package iso9660

import (
	"encoding/binary"
	"fmt"
	"strings"
	"sync"
	"time"
	stdUnicode "unicode"

	"golang.org/x/text/encoding/unicode"
)

// ISO 9660 Overview
// https://wiki.osdev.org/ISO_9660

const (
	SectorSize                 SizeBytes = 0x800
	SystemAreaSize                       = SectorSize * 16
	VolumeDescriptorHeaderSize           = 7
	VolumeDescriptorBodySize             = SectorSize - VolumeDescriptorHeaderSize
	PathTableItemsLimit                  = 0x10000

	VolumeTypeBoot          byte = 0
	VolumeTypePrimary       byte = 1
	VolumeTypeSupplementary byte = 2
	VolumeTypePartition     byte = 3
	VolumeTypeTerminator    byte = 255

	ACharacters StringA = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_!\"%&'()*+,-./:;<=>?"
	DCharacters StringD = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_"
	// ECMA-119 7.4.2.2 defines d1-characters as
	// "subject to agreement between the originator and the recipient of the volume".
	D1Characters StringD1 = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789_!\"%&'()*+,-./:;<=>?"
)

const (
	DirFlagHidden = 1 << iota
	DirFlagDir
	DirFlagAssociated
	DirFlagRecord
	DirFlagProtection
	_
	_
	DirFlagMultiExtent
)

var StandardIdentifierBytes = [5]byte{'C', 'D', '0', '0', '1'}

var (
	aCharactersSet = sync.OnceValue(func() map[rune]struct{} {
		m := make(map[rune]struct{}, len(ACharacters))
		for _, r := range ACharacters {
			m[r] = struct{}{}
		}
		return m
	})

	dCharactersSet = sync.OnceValue(func() map[rune]struct{} {
		m := make(map[rune]struct{}, len(DCharacters))
		for _, r := range DCharacters {
			m[r] = struct{}{}
		}
		return m
	})

	d1CharactersSet = sync.OnceValue(func() map[rune]struct{} {
		m := make(map[rune]struct{}, len(D1Characters))
		for _, r := range D1Characters {
			m[r] = struct{}{}
		}
		return m
	})
)

// types to minimize mixing of bytes and sector units

type SizeSectors int32

func (s SizeSectors) Next() SizeSectors { return s + 1 }

func (s SizeSectors) Prev() SizeSectors { return s - 1 }

func (s SizeSectors) Bytes() SizeBytes { return SectorSize * SizeBytes(s) }

type SizeBytes int64

// FloorSectors returns how many "full" sectors will be occupied by this amount of bytes (floor).
func (b SizeBytes) FloorSectors() SizeSectors {
	return SizeSectors(b / SectorSize)
}

// Sectors returns how many Sectors will be occupied by this amount of bytes (ceil).
func (b SizeBytes) Sectors() SizeSectors {
	sectors := b.FloorSectors()
	if b%SectorSize > 0 {
		sectors++
	}

	return sectors
}

type (
	StringA  string
	StringD  string
	StringD1 string
)

var utf16Encoder = unicode.UTF16(unicode.BigEndian, unicode.IgnoreBOM).NewEncoder() // ucs2 is utf16 actually

// VolumeDescriptorHeader represents the data in bytes 0-6
// of a Volume Descriptor as defined in ECMA-119 8.1
type VolumeDescriptorHeader struct {
	Type       byte
	Identifier [5]byte
	Version    byte
}

func (vdh VolumeDescriptorHeader) Encode(enc *Encoder) {
	enc.AppendByte(vdh.Type)
	enc.AppendBytes(vdh.Identifier[:])
	enc.AppendByte(vdh.Version)
}

// PrimaryVolumeDescriptorBody represents the data in bytes 7-2047
// of a Primary Volume Descriptor as defined in ECMA-119 8.4
type PrimaryVolumeDescriptorBody struct {
	StringPadding byte // character used for strings padding

	SystemIdentifier              StringA
	VolumeIdentifier              StringD
	VolumeSpaceSize               SizeSectors
	EscapeSequences               string
	VolumeSetSize                 SizeBytes
	VolumeSequenceNumber          uint16
	LogicalBlockSize              SizeBytes
	PathTableSize                 SizeBytes
	TypeLPathTableLoc             SizeSectors
	OptTypeLPathTableLoc          SizeSectors
	TypeMPathTableLoc             SizeSectors
	OptTypeMPathTableLoc          SizeSectors
	RootDirectoryEntry            *FixedDirectoryEntry
	VolumeSetIdentifier           StringD
	PublisherIdentifier           StringA
	DataPreparerIdentifier        StringA
	ApplicationIdentifier         StringA
	CopyrightFileIdentifier       StringD
	AbstractFileIdentifier        StringD
	BibliographicFileIdentifier   StringD
	VolumeCreationDateAndTime     VolumeDescriptorTimestamp
	VolumeModificationDateAndTime VolumeDescriptorTimestamp
	VolumeExpirationDateAndTime   VolumeDescriptorTimestamp
	VolumeEffectiveDateAndTime    VolumeDescriptorTimestamp
	FileStructureVersion          byte
	ApplicationUsed               []byte
}

// FixedDirectoryEntry is embedded into PrimaryVolumeDescriptorBody
type FixedDirectoryEntry struct {
	ExtendedAttributeRecordLength byte
	ExtentLocation                SizeSectors
	ExtentLength                  SizeBytes
	RecordingDateTime             RecordingTimestamp
	FileFlags                     byte
	InterleaveSize                byte
	InterleaveSkip                byte
	VolumeSequenceNumber          uint16
	Identifier                    StringD1
}

func (de FixedDirectoryEntry) Size() SizeBytes {
	identifierLen := len(de.Identifier)
	idPaddingLen := (identifierLen + 1) % 2
	totalLen := 33 + identifierLen + idPaddingLen

	return SizeBytes(totalLen)
}

func (de FixedDirectoryEntry) addBytes(enc *Encoder) (startPos SizeBytes) {
	identifierLen := len(de.Identifier)
	idPaddingLen := (identifierLen + 1) % 2

	startPos = enc.Size()
	enc.AppendByte(0) // reserved for size
	enc.AppendByte(de.ExtendedAttributeRecordLength)
	enc.AppendUint32LSBMSB(uint32(de.ExtentLocation))
	enc.AppendUint32LSBMSB(uint32(de.ExtentLength))
	de.RecordingDateTime.encode(enc)
	enc.AppendByte(de.FileFlags)
	enc.AppendByte(de.InterleaveSize)
	enc.AppendByte(de.InterleaveSkip)
	enc.AppendUint16LSBMSB(de.VolumeSequenceNumber)
	enc.AppendByte(byte(identifierLen))
	enc.AppendStringD1(de.Identifier, -1)
	if idPaddingLen > 0 {
		enc.AppendByte(0)
	}
	return startPos
}

func (de FixedDirectoryEntry) Encode(enc *Encoder) {
	startPos := de.addBytes(enc)
	// ensure that size method works correctly
	if de.Size() != enc.Size()-startPos {
		panic("directory entry size mismatch")
	}

	// set size
	enc.SetByteAt(byte(enc.Size()-startPos), startPos)
}

// DirectoryEntry contains data from a Directory Descriptor
// as described by ECMA-119 9.1
type DirectoryEntry struct {
	FixedDirectoryEntry
	SystemUse []byte
}

func (de DirectoryEntry) Size() SizeBytes {
	return de.FixedDirectoryEntry.Size() + SizeBytes(len(de.SystemUse))
}

func (de DirectoryEntry) Encode(enc *Encoder) {
	// if entry doesn't fit in sector, begin new sector
	if encSize := enc.Size(); de.Size() > encSize.Sectors().Bytes()-encSize {
		enc.PadLastSector()
	}

	startPos := de.FixedDirectoryEntry.addBytes(enc)

	enc.AppendBytes(de.SystemUse)
	// ensure that size method works correctly
	if de.Size() != enc.Size()-startPos {
		panic("directory entry size mismatch")
	}

	// set size
	enc.SetByteAt(byte(enc.Size()-startPos), startPos)
}

type PathTableEntry struct {
	ExtendedAttributeRecordLength byte
	DirLocation                   SizeSectors
	ParentDirNumber               int16
	DirIdentifier                 StringD1
}

func (e PathTableEntry) Size() SizeBytes {
	totalLen := SizeBytes(8)

	idLen := byte(len(e.DirIdentifier))
	totalLen += SizeBytes(idLen)

	if idLen%2 > 0 {
		totalLen += 1
	}

	return totalLen
}

func (e PathTableEntry) EncodeOrdered(enc *Encoder, order binary.AppendByteOrder) {
	start := enc.Size()
	enc.AppendByte(byte(len(e.DirIdentifier)))
	enc.AppendByte(e.ExtendedAttributeRecordLength)
	enc.AppendUint32(uint32(e.DirLocation), order)
	enc.AppendUint16(uint16(e.ParentDirNumber), order)
	enc.AppendStringD1(e.DirIdentifier, -1)

	// add extra null byte for odd length
	if len(e.DirIdentifier)%2 > 0 {
		enc.AppendByte(0)
	}

	// ensure that size method works correctly
	if e.Size() != enc.Size()-start {
		panic("path table entry size mismatch")
	}
}

func (pvd PrimaryVolumeDescriptorBody) Encode(enc *Encoder) {
	enc.AppendByte(0) // reserved
	enc.AppendString(string(pvd.SystemIdentifier), 32, pvd.StringPadding)
	enc.AppendString(string(pvd.VolumeIdentifier), 32, pvd.StringPadding)
	enc.AppendZeroes(8) // reserved
	enc.AppendUint32LSBMSB(uint32(pvd.VolumeSpaceSize))
	enc.AppendString(pvd.EscapeSequences, 32, 0) // for joliet
	enc.AppendUint16LSBMSB(uint16(pvd.VolumeSetSize))
	enc.AppendUint16LSBMSB(pvd.VolumeSequenceNumber)
	enc.AppendUint16LSBMSB(uint16(pvd.LogicalBlockSize))
	enc.AppendUint32LSBMSB(uint32(pvd.PathTableSize))

	enc.AppendUint32(uint32(pvd.TypeLPathTableLoc), binary.LittleEndian)
	enc.AppendUint32(uint32(pvd.OptTypeLPathTableLoc), binary.LittleEndian)
	enc.AppendUint32(uint32(pvd.TypeMPathTableLoc), binary.BigEndian)
	enc.AppendUint32(uint32(pvd.OptTypeMPathTableLoc), binary.BigEndian)

	enc.AppendEncodable(pvd.RootDirectoryEntry, 34)

	enc.AppendString(string(pvd.VolumeSetIdentifier), 128, pvd.StringPadding)
	enc.AppendString(string(pvd.PublisherIdentifier), 128, pvd.StringPadding)
	enc.AppendString(string(pvd.DataPreparerIdentifier), 128, pvd.StringPadding)
	enc.AppendString(string(pvd.ApplicationIdentifier), 128, pvd.StringPadding)
	enc.AppendString(string(pvd.CopyrightFileIdentifier), 37, pvd.StringPadding)
	enc.AppendString(string(pvd.AbstractFileIdentifier), 37, pvd.StringPadding)
	enc.AppendString(string(pvd.BibliographicFileIdentifier), 37, pvd.StringPadding)

	pvd.VolumeCreationDateAndTime.Encode(enc)
	pvd.VolumeModificationDateAndTime.Encode(enc)
	pvd.VolumeExpirationDateAndTime.Encode(enc)
	pvd.VolumeEffectiveDateAndTime.Encode(enc)

	enc.AppendByte(pvd.FileStructureVersion)
	enc.AppendByte(0) // reserved
	enc.AppendBytesFixed(pvd.ApplicationUsed, 512)
}

type VolumeDescriptor struct {
	Header  VolumeDescriptorHeader
	Primary *PrimaryVolumeDescriptorBody
}

func (vd VolumeDescriptor) Encode(enc *Encoder) {
	enc.AppendEncodable(vd.Header, VolumeDescriptorHeaderSize)

	switch vd.Header.Type {
	case VolumeTypeBoot:
		panic("boot volumes are not yet supported")
	case VolumeTypePartition:
		panic("partition volumes are not yet supported")
	case VolumeTypePrimary, VolumeTypeSupplementary:
		enc.AppendEncodable(vd.Primary, VolumeDescriptorBodySize)
	case VolumeTypeTerminator:
		enc.AppendZeroes(VolumeDescriptorBodySize)
	}
}

// VolumeDescriptorTimestamp represents a time and date format
// that can be encoded according to ECMA-119 8.4.26.1
type VolumeDescriptorTimestamp struct {
	Year      int
	Month     int
	Day       int
	Hour      int
	Minute    int
	Second    int
	Hundredth int
	Offset    int
}

func (ts *VolumeDescriptorTimestamp) Encode(enc *Encoder) {
	startPos := enc.Size()
	enc.AppendFormat("%04d%02d%02d%02d%02d%02d%02d", ts.Year, ts.Month, ts.Day, ts.Hour, ts.Minute, ts.Second, ts.Hundredth)
	enc.AppendByte(byte(ts.Offset))
	if encodedLen := enc.Size() - startPos; encodedLen != 17 {
		panic(fmt.Sprintf("the formatted timestamp is %d bytes long", encodedLen))
	}
}

// RecordingTimestamp represents a time and date format
// that can be encoded according to ECMA-119 9.1.5
type RecordingTimestamp time.Time

func (ts RecordingTimestamp) encode(enc *Encoder) {
	t := time.Time(ts)
	year, month, day := t.Date()
	hour, minute, sec := t.Clock()
	_, secOffset := t.Zone()
	secondsInAQuarter := 60 * 15
	offsetInQuarters := secOffset / secondsInAQuarter

	enc.AppendByte(byte(year - 1900))
	enc.AppendByte(byte(month))
	enc.AppendByte(byte(day))
	enc.AppendByte(byte(hour))
	enc.AppendByte(byte(minute))
	enc.AppendByte(byte(sec))
	enc.AppendByte(byte(offsetInQuarters))
}

// VolumeDescriptorTimestampFromTime converts time.Time to volumeDescriptorTimestamp
func VolumeDescriptorTimestampFromTime(t time.Time) VolumeDescriptorTimestamp {
	t = t.UTC()
	year, month, day := t.Date()
	hour, minute, second := t.Clock()
	hundredth := t.Nanosecond() / 10000000
	return VolumeDescriptorTimestamp{
		Year:      year,
		Month:     int(month),
		Day:       day,
		Hour:      hour,
		Minute:    minute,
		Second:    second,
		Hundredth: hundredth,
		Offset:    0, // we converted to UTC
	}
}

func MangleStringA(in string, joliet bool) StringA {
	set := aCharactersSet()
	ret := strings.Map(func(r rune) rune {
		if _, ok := set[r]; ok {
			return r
		}

		upper := stdUnicode.ToUpper(r)
		if _, ok := set[upper]; ok {
			return upper
		}

		return '_'
	}, in)

	if joliet {
		ret, _ = utf16Encoder.String(ret)
	}

	return StringA(ret)
}

func MangleStringD(in string, joliet bool) StringD {
	set := dCharactersSet()
	ret := strings.Map(func(r rune) rune {
		if _, ok := set[r]; ok {
			return r
		}

		upper := stdUnicode.ToUpper(r)
		if _, ok := set[upper]; ok {
			return upper
		}

		return '_'
	}, in)

	if joliet {
		ret, _ = utf16Encoder.String(ret)
	}

	return StringD(ret)
}

func MangleStringD1(in string, joliet bool) StringD1 {
	set := d1CharactersSet()
	ret := strings.Map(func(r rune) rune {
		if _, ok := set[r]; ok {
			return r
		}

		return '_'
	}, in)

	if joliet {
		ret, _ = utf16Encoder.String(ret)
	}

	return StringD1(ret)
}
