package fs

import (
	"encoding/binary"
	"fmt"
	"strings"
	"time"
	stdUnicode "unicode"

	"golang.org/x/text/encoding/unicode"
)

// ISO 9660 Overview
// https://wiki.osdev.org/ISO_9660

const (
	sectorSize                 sizeBytes = 0x800
	systemAreaSize                       = sectorSize * 16
	volumeDescriptorHeaderSize           = 7
	volumeDescriptorBodySize             = sectorSize - volumeDescriptorHeaderSize
	pathTableItemsLimit                  = 0x10000

	volumeTypeBoot          byte = 0
	volumeTypePrimary       byte = 1
	volumeTypeSupplementary byte = 2
	volumeTypePartition     byte = 3
	volumeTypeTerminator    byte = 255

	aCharacters stringA = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_!\"%&'()*+,-./:;<=>?"
	dCharacters stringD = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_"
	// ECMA-119 7.4.2.2 defines d1-characters as
	// "subject to agreement between the originator and the recipient of the volume".
	d1Characters stringD1 = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789_!\"%&'()*+,-./:;<=>?"
)

const (
	dirFlagHidden = 1 << iota
	dirFlagDir
	dirFlagAssociated
	dirFlagRecord
	dirFlagProtection
	_
	_
	dirFlagMultiExtent
)

var standardIdentifierBytes = [5]byte{'C', 'D', '0', '0', '1'}

// types to minimize mixing of bytes and sector units

type sizeSectors int32

func (s sizeSectors) next() sizeSectors { return s + 1 }

func (s sizeSectors) prev() sizeSectors { return s - 1 }

func (s sizeSectors) bytes() sizeBytes { return sectorSize * sizeBytes(s) }

type sizeBytes int64

// floorSectors returns how many "full" sectors will be occupied by this amount of bytes (floor).
func (b sizeBytes) floorSectors() sizeSectors {
	return sizeSectors(b / sectorSize)
}

// sectors returns how many sectors will be occupied by this amount of bytes (ceil).
func (b sizeBytes) sectors() sizeSectors {
	sectors := b.floorSectors()
	if b%sectorSize > 0 {
		sectors++
	}

	return sectors
}

type (
	stringA  string
	stringD  string
	stringD1 string
)

var utf16Encoder = unicode.UTF16(unicode.BigEndian, unicode.IgnoreBOM).NewEncoder() // ucs2 is utf16 actually

// volumeDescriptorHeader represents the data in bytes 0-6
// of a Volume Descriptor as defined in ECMA-119 8.1
type volumeDescriptorHeader struct {
	Type       byte
	Identifier [5]byte
	Version    byte
}

func (vdh volumeDescriptorHeader) encode(enc *iso9660encoder) {
	enc.appendByte(vdh.Type)
	enc.appendBytes(vdh.Identifier[:])
	enc.appendByte(vdh.Version)
}

// primaryVolumeDescriptorBody represents the data in bytes 7-2047
// of a Primary Volume Descriptor as defined in ECMA-119 8.4
type primaryVolumeDescriptorBody struct {
	SystemIdentifier              stringA
	VolumeIdentifier              stringD
	VolumeSpaceSize               sizeSectors
	EscapeSequences               string
	VolumeSetSize                 sizeBytes
	VolumeSequenceNumber          uint16
	LogicalBlockSize              sizeBytes
	PathTableSize                 sizeBytes
	TypeLPathTableLoc             sizeSectors
	OptTypeLPathTableLoc          sizeSectors
	TypeMPathTableLoc             sizeSectors
	OptTypeMPathTableLoc          sizeSectors
	RootDirectoryEntry            *directoryEntry
	VolumeSetIdentifier           stringD
	PublisherIdentifier           stringA
	DataPreparerIdentifier        stringA
	ApplicationIdentifier         stringA
	CopyrightFileIdentifier       stringD
	AbstractFileIdentifier        stringD
	BibliographicFileIdentifier   stringD
	VolumeCreationDateAndTime     volumeDescriptorTimestamp
	VolumeModificationDateAndTime volumeDescriptorTimestamp
	VolumeExpirationDateAndTime   volumeDescriptorTimestamp
	VolumeEffectiveDateAndTime    volumeDescriptorTimestamp
	FileStructureVersion          byte
	ApplicationUsed               []byte
}

// directoryEntry contains data from a Directory Descriptor
// as described by ECMA-119 9.1
type directoryEntry struct {
	ExtendedAttributeRecordLength byte
	ExtentLocation                sizeSectors
	ExtentLength                  sizeBytes
	RecordingDateTime             recordingTimestamp
	FileFlags                     byte
	InterleaveSize                byte
	InterleaveSkip                byte
	VolumeSequenceNumber          uint16
	Identifier                    stringD1
	SystemUse                     []byte
}

func (de directoryEntry) size() sizeBytes {
	identifierLen := len(de.Identifier)
	idPaddingLen := (identifierLen + 1) % 2
	totalLen := 33 + identifierLen + idPaddingLen + len(de.SystemUse)

	return sizeBytes(totalLen)
}

func (de directoryEntry) encode(enc *iso9660encoder) {
	identifierLen := len(de.Identifier)
	idPaddingLen := (identifierLen + 1) % 2

	startPos := enc.size()
	enc.appendByte(0) // reserved for size
	enc.appendByte(de.ExtendedAttributeRecordLength)
	enc.appendUint32LSBMSB(uint32(de.ExtentLocation))
	enc.appendUint32LSBMSB(uint32(de.ExtentLength))
	de.RecordingDateTime.encode(enc)
	enc.appendByte(de.FileFlags)
	enc.appendByte(de.InterleaveSize)
	enc.appendByte(de.InterleaveSkip)
	enc.appendUint16LSBMSB(de.VolumeSequenceNumber)
	enc.appendByte(byte(identifierLen))
	enc.appendStrD1(de.Identifier, -1)
	if idPaddingLen > 0 {
		enc.appendByte(0)
	}
	enc.appendBytes(de.SystemUse)

	// ensure that size method works correctly
	if de.size() != enc.size()-startPos {
		panic("directory entry size mismatch")
	}

	// set size
	enc.setByteAt(byte(enc.size()-startPos), startPos)
}

type pathTableEntry struct {
	ExtendedAttributeRecordLength byte
	DirLocation                   sizeSectors
	ParentDirNumber               int16
	DirIdentifier                 stringD1
}

func (e pathTableEntry) size() sizeBytes {
	totalLen := sizeBytes(8)

	idLen := byte(len(e.DirIdentifier))
	totalLen += sizeBytes(idLen)

	if idLen%2 > 0 {
		totalLen += 1
	}

	return totalLen
}

func (e pathTableEntry) encodeOrdered(enc *iso9660encoder, order binary.AppendByteOrder) {
	start := enc.size()
	enc.appendByte(byte(len(e.DirIdentifier)))
	enc.appendByte(e.ExtendedAttributeRecordLength)
	enc.appendUint32(uint32(e.DirLocation), order)
	enc.appendUint16(uint16(e.ParentDirNumber), order)
	enc.appendStrD1(e.DirIdentifier, -1)

	// add extra null byte for odd length
	if len(e.DirIdentifier)%2 > 0 {
		enc.appendByte(0)
	}

	// ensure that size method works correctly
	if e.size() != enc.size()-start {
		panic("path table entry size mismatch")
	}
}

func (pvd primaryVolumeDescriptorBody) encode(enc *iso9660encoder) {
	enc.appendByte(0) // reserved
	enc.appendStrA(pvd.SystemIdentifier, 32)
	enc.appendStrD(pvd.VolumeIdentifier, 32)
	enc.appendZeroes(8) // reserved
	enc.appendUint32LSBMSB(uint32(pvd.VolumeSpaceSize))
	enc.appendString(pvd.EscapeSequences, 32, 0) // for joliet
	enc.appendUint16LSBMSB(uint16(pvd.VolumeSetSize))
	enc.appendUint16LSBMSB(pvd.VolumeSequenceNumber)
	enc.appendUint16LSBMSB(uint16(pvd.LogicalBlockSize))
	enc.appendUint32LSBMSB(uint32(pvd.PathTableSize))

	enc.appendUint32(uint32(pvd.TypeLPathTableLoc), binary.LittleEndian)
	enc.appendUint32(uint32(pvd.OptTypeLPathTableLoc), binary.LittleEndian)
	enc.appendUint32(uint32(pvd.TypeMPathTableLoc), binary.BigEndian)
	enc.appendUint32(uint32(pvd.OptTypeMPathTableLoc), binary.BigEndian)

	enc.appendEncodable(pvd.RootDirectoryEntry, 34)

	enc.appendStrD(pvd.VolumeSetIdentifier, 128)
	enc.appendStrA(pvd.PublisherIdentifier, 128)
	enc.appendStrA(pvd.DataPreparerIdentifier, 128)
	enc.appendStrA(pvd.ApplicationIdentifier, 128)
	enc.appendStrD(pvd.CopyrightFileIdentifier, 37)
	enc.appendStrD(pvd.AbstractFileIdentifier, 37)
	enc.appendStrD(pvd.BibliographicFileIdentifier, 37)

	pvd.VolumeCreationDateAndTime.encode(enc)
	pvd.VolumeModificationDateAndTime.encode(enc)
	pvd.VolumeExpirationDateAndTime.encode(enc)
	pvd.VolumeEffectiveDateAndTime.encode(enc)

	enc.appendByte(pvd.FileStructureVersion)
	enc.appendByte(0) // reserved
	enc.appendBytesFixed(pvd.ApplicationUsed, 512)
}

type volumeDescriptor struct {
	Header  volumeDescriptorHeader
	Primary *primaryVolumeDescriptorBody
}

func (vd volumeDescriptor) encode(enc *iso9660encoder) {
	enc.appendEncodable(vd.Header, volumeDescriptorHeaderSize)

	switch vd.Header.Type {
	case volumeTypeBoot:
		panic("boot volumes are not yet supported")
	case volumeTypePartition:
		panic("partition volumes are not yet supported")
	case volumeTypePrimary, volumeTypeSupplementary:
		enc.appendEncodable(vd.Primary, volumeDescriptorBodySize)
	case volumeTypeTerminator:
		enc.appendZeroes(volumeDescriptorBodySize)
	}
}

// volumeDescriptorTimestamp represents a time and date format
// that can be encoded according to ECMA-119 8.4.26.1
type volumeDescriptorTimestamp struct {
	Year      int
	Month     int
	Day       int
	Hour      int
	Minute    int
	Second    int
	Hundredth int
	Offset    int
}

func (ts *volumeDescriptorTimestamp) encode(enc *iso9660encoder) {
	startPos := enc.size()
	enc.appendFormat("%04d%02d%02d%02d%02d%02d%02d", ts.Year, ts.Month, ts.Day, ts.Hour, ts.Minute, ts.Second, ts.Hundredth)
	enc.appendByte(byte(ts.Offset))
	if encodedLen := enc.size() - startPos; encodedLen != 17 {
		panic(fmt.Sprintf("the formatted timestamp is %d bytes long", encodedLen))
	}
}

// recordingTimestamp represents a time and date format
// that can be encoded according to ECMA-119 9.1.5
type recordingTimestamp time.Time

func (ts recordingTimestamp) encode(enc *iso9660encoder) {
	t := time.Time(ts)
	year, month, day := t.Date()
	hour, minute, sec := t.Clock()
	_, secOffset := t.Zone()
	secondsInAQuarter := 60 * 15
	offsetInQuarters := secOffset / secondsInAQuarter

	enc.appendByte(byte(year - 1900))
	enc.appendByte(byte(month))
	enc.appendByte(byte(day))
	enc.appendByte(byte(hour))
	enc.appendByte(byte(minute))
	enc.appendByte(byte(sec))
	enc.appendByte(byte(offsetInQuarters))
}

// volumeDescriptorTimestampFromTime converts time.Time to volumeDescriptorTimestamp
func volumeDescriptorTimestampFromTime(t time.Time) volumeDescriptorTimestamp {
	t = t.UTC()
	year, month, day := t.Date()
	hour, minute, second := t.Clock()
	hundredth := t.Nanosecond() / 10000000
	return volumeDescriptorTimestamp{
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

func mangleStrA(in string, joliet bool) stringA {
	ret := strings.Map(func(r rune) rune {
		for _, i := range aCharacters {
			if r == i {
				return r
			}

			if upper := stdUnicode.ToUpper(r); upper == i {
				return upper
			}
		}

		return '_'
	}, in)

	if joliet {
		ret, _ = utf16Encoder.String(ret)
	}

	return stringA(ret)
}

func mangleStrD(in string, joliet bool) stringD {
	ret := strings.Map(func(r rune) rune {
		for _, i := range dCharacters {
			if r == i {
				return r
			}

			if upper := stdUnicode.ToUpper(r); upper == i {
				return upper
			}
		}

		return '_'
	}, in)

	if joliet {
		ret, _ = utf16Encoder.String(ret)
	}

	return stringD(ret)
}

func mangleStrD1(in string, joliet bool) stringD1 {
	ret := strings.Map(func(r rune) rune {
		for _, i := range d1Characters {
			if r == i {
				return r
			}
		}

		return '_'
	}, in)

	if joliet {
		ret, _ = utf16Encoder.String(ret)
	}

	return stringD1(ret)
}
