package fs

import (
	"encoding"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"time"
	stdUnicode "unicode"

	"golang.org/x/text/encoding/unicode"
)

// ISO 9660 Overview
// https://wiki.osdev.org/ISO_9660

const (
	sectorSize                                sizeBytes = 0x800
	systemAreaSize                                      = sectorSize * 16
	standardIdentifier                                  = "CD001"
	volumeDescriptorBodySize                            = sectorSize - 7
	primaryVolumeDirectoryIdentifierMaxLength           = 31 // ECMA-119 7.6.3
	primaryVolumeFileIdentifierMaxLength                = 30 // ECMA-119 7.5
	pathTableItemsLimit                                 = 0x10000

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

func (s sizeSectors) bytes() sizeBytes { return sectorSize * sizeBytes(s) }

type sizeBytes int64

func (b sizeBytes) sectors() sizeSectors {
	sectors := b / sectorSize
	if b%sectorSize > 0 {
		sectors++
	}

	return sizeSectors(sectors)
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

var _ encoding.BinaryMarshaler = &volumeDescriptorHeader{}

func (vdh volumeDescriptorHeader) MarshalBinary() ([]byte, error) {
	data := make([]byte, 7)
	data[0] = vdh.Type
	data[6] = vdh.Version
	copy(data[1:6], vdh.Identifier[:])
	return data, nil
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
	ApplicationUsed               [512]byte
}

var _ encoding.BinaryMarshaler = primaryVolumeDescriptorBody{}

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

var _ encoding.BinaryMarshaler = directoryEntry{}

func (de directoryEntry) Size() sizeBytes {
	identifierLen := len(de.Identifier)
	idPaddingLen := (identifierLen + 1) % 2
	totalLen := 33 + identifierLen + idPaddingLen + len(de.SystemUse)

	return sizeBytes(totalLen)
}

// MarshalBinary encodes a directoryEntry to binary form
func (de directoryEntry) MarshalBinary() ([]byte, error) {
	identifierLen := len(de.Identifier)
	idPaddingLen := (identifierLen + 1) % 2
	totalLen := de.Size()
	if totalLen > 255 {
		return nil, fmt.Errorf("identifier %q is too long", de.Identifier)
	}

	data := make([]byte, totalLen)

	data[0] = byte(totalLen)
	data[1] = de.ExtendedAttributeRecordLength

	writeInt32LSBMSB(data[2:10], uint32(de.ExtentLocation))
	writeInt32LSBMSB(data[10:18], uint32(de.ExtentLength))
	de.RecordingDateTime.MarshalBinary(data[18:25])
	data[25] = de.FileFlags
	data[26] = de.InterleaveSize
	data[27] = de.InterleaveSkip
	writeInt16LSBMSB(data[28:32], de.VolumeSequenceNumber)
	data[32] = byte(identifierLen)
	copy(data[33:33+identifierLen], de.Identifier)

	copy(data[33+identifierLen+idPaddingLen:totalLen], de.SystemUse)

	return data, nil
}

// Clone creates a copy of the directoryEntry
func (de *directoryEntry) Clone() directoryEntry {
	newDE := directoryEntry{
		ExtendedAttributeRecordLength: de.ExtendedAttributeRecordLength,
		ExtentLocation:                de.ExtentLocation,
		ExtentLength:                  de.ExtentLength,
		RecordingDateTime:             de.RecordingDateTime,
		FileFlags:                     de.FileFlags,
		InterleaveSize:                de.InterleaveSize,
		InterleaveSkip:                de.InterleaveSkip,
		VolumeSequenceNumber:          de.VolumeSequenceNumber,
		Identifier:                    de.Identifier,
		SystemUse:                     make([]byte, len(de.SystemUse)),
	}
	copy(newDE.SystemUse, de.SystemUse)
	return newDE
}

type pathTableEntry struct {
	ExtendedAttributeRecordLength byte
	DirLocation                   sizeSectors
	ParentDirNumber               int16
	DirIdentifier                 stringD1
}

func (e pathTableEntry) Size() sizeBytes {
	totalLen := sizeBytes(8)

	idLen := byte(len(e.DirIdentifier))
	totalLen += sizeBytes(idLen)

	if idLen%2 > 0 {
		totalLen += 1
	}

	return totalLen
}

func (e pathTableEntry) MarshalBinary(order binary.ByteOrder) ([]byte, error) {
	ret := make([]byte, 8)
	ret[0] = byte(len(e.DirIdentifier))
	ret[1] = e.ExtendedAttributeRecordLength
	order.PutUint32(ret[2:6], uint32(e.DirLocation))
	order.PutUint16(ret[6:8], uint16(e.ParentDirNumber))

	ret = append(ret, e.DirIdentifier[:ret[0]]...)

	// add extra null byte for odd length
	if ret[0]%2 > 0 {
		ret = append(ret, 0)
	}

	return ret, nil
}

// MarshalBinary encodes the primaryVolumeDescriptorBody to its binary form
func (pvd primaryVolumeDescriptorBody) MarshalBinary() ([]byte, error) {
	output := make([]byte, sectorSize)

	copy(output[8:40], pvd.SystemIdentifier)

	copy(output[40:72], pvd.VolumeIdentifier)

	writeInt32LSBMSB(output[80:88], uint32(pvd.VolumeSpaceSize))
	copy(output[88:120], pvd.EscapeSequences) // for joliet
	writeInt16LSBMSB(output[120:124], uint16(pvd.VolumeSetSize))
	writeInt16LSBMSB(output[128:132], uint16(pvd.LogicalBlockSize))
	writeInt32LSBMSB(output[132:140], uint32(pvd.PathTableSize))

	binary.LittleEndian.PutUint32(output[140:144], uint32(pvd.TypeLPathTableLoc))
	binary.LittleEndian.PutUint32(output[144:148], uint32(pvd.OptTypeLPathTableLoc))
	binary.BigEndian.PutUint32(output[148:152], uint32(pvd.TypeMPathTableLoc))
	binary.BigEndian.PutUint32(output[152:156], uint32(pvd.OptTypeMPathTableLoc))

	binaryRDE, err := pvd.RootDirectoryEntry.MarshalBinary()
	if err != nil {
		return nil, err
	}
	copy(output[156:190], binaryRDE)

	copy(output[190:318], pvd.VolumeSetIdentifier)
	copy(output[318:446], pvd.PublisherIdentifier)
	copy(output[446:574], pvd.DataPreparerIdentifier)
	copy(output[574:702], pvd.ApplicationIdentifier)
	copy(output[702:740], pvd.CopyrightFileIdentifier)
	copy(output[740:776], pvd.AbstractFileIdentifier)
	copy(output[776:813], pvd.BibliographicFileIdentifier)

	d, err := pvd.VolumeCreationDateAndTime.MarshalBinary()
	if err != nil {
		return nil, err
	}
	copy(output[813:830], d)

	d, err = pvd.VolumeModificationDateAndTime.MarshalBinary()
	if err != nil {
		return nil, err
	}
	copy(output[830:847], d)

	d, err = pvd.VolumeExpirationDateAndTime.MarshalBinary()
	if err != nil {
		return nil, err
	}
	copy(output[847:864], d)

	d, err = pvd.VolumeEffectiveDateAndTime.MarshalBinary()
	if err != nil {
		return nil, err
	}
	copy(output[864:881], d)

	output[881] = pvd.FileStructureVersion
	output[882] = 0
	copy(output[883:1395], pvd.ApplicationUsed[:])

	return output, nil
}

type volumeDescriptor struct {
	Header  volumeDescriptorHeader
	Primary *primaryVolumeDescriptorBody
}

var _ encoding.BinaryMarshaler = &volumeDescriptor{}

func (vd volumeDescriptor) Type() byte {
	return vd.Header.Type
}

// MarshalBinary encodes a volumeDescriptor to binary form
func (vd volumeDescriptor) MarshalBinary() ([]byte, error) {
	var output []byte
	var err error

	switch vd.Header.Type {
	case volumeTypeBoot:
		return nil, errors.New("boot volumes are not yet supported")
	case volumeTypePartition:
		return nil, errors.New("partition volumes are not yet supported")
	case volumeTypePrimary, volumeTypeSupplementary:
		if output, err = vd.Primary.MarshalBinary(); err != nil {
			return nil, err
		}
	case volumeTypeTerminator:
		output = make([]byte, sectorSize)
	}

	data, err := vd.Header.MarshalBinary()
	if err != nil {
		return nil, err
	}

	copy(output[0:7], data)

	return output, nil
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

var _ encoding.BinaryMarshaler = &volumeDescriptorTimestamp{}

// MarshalBinary encodes the timestamp into a binary form
func (ts *volumeDescriptorTimestamp) MarshalBinary() ([]byte, error) {
	// for empty value return zeroes
	if *ts == (volumeDescriptorTimestamp{}) {
		return make([]byte, 17), nil
	}

	formatted := fmt.Sprintf("%04d%02d%02d%02d%02d%02d%02d", ts.Year, ts.Month, ts.Day, ts.Hour, ts.Minute, ts.Second, ts.Hundredth)
	formattedBytes := append([]byte(formatted), byte(ts.Offset))
	if len(formattedBytes) != 17 {
		return nil, fmt.Errorf("volumeDescriptorTimestamp.MarshalBinary: the formatted timestamp is %d bytes long", len(formatted))
	}
	return formattedBytes, nil
}

// recordingTimestamp represents a time and date format
// that can be encoded according to ECMA-119 9.1.5
type recordingTimestamp time.Time

// MarshalBinary encodes the recordingTimestamp in its binary form to a buffer
// of the length of 7 or more bytes
func (ts recordingTimestamp) MarshalBinary(dst []byte) {
	_ = dst[6] // early bounds check to guarantee safety of writes below
	t := time.Time(ts)
	year, month, day := t.Date()
	hour, min, sec := t.Clock()
	_, secOffset := t.Zone()
	secondsInAQuarter := 60 * 15
	offsetInQuarters := secOffset / secondsInAQuarter
	dst[0] = byte(year - 1900)
	dst[1] = byte(month)
	dst[2] = byte(day)
	dst[3] = byte(hour)
	dst[4] = byte(min)
	dst[5] = byte(sec)
	dst[6] = byte(offsetInQuarters)
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

// writeInt32LSBMSB writes a 32-bit integer in both byte orders, as defined in ECMA-119 7.3.3
func writeInt32LSBMSB(dst []byte, value uint32) {
	_ = dst[7] // early bounds check to guarantee safety of writes below
	binary.LittleEndian.PutUint32(dst[0:4], value)
	binary.BigEndian.PutUint32(dst[4:8], value)
}

// writeInt16LSBMSB writes a 16-bit integer in both byte orders, as defined in ECMA-119 7.2.3
func writeInt16LSBMSB(dst []byte, value uint16) {
	_ = dst[3] // early bounds check to guarantee safety of writes below
	binary.LittleEndian.PutUint16(dst[0:2], value)
	binary.BigEndian.PutUint16(dst[2:4], value)
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
