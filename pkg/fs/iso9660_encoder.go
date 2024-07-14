package fs

import (
	"encoding/binary"
	"fmt"
)

type iso9660encoder []byte

type iso9660encodable interface {
	encode(e *iso9660encoder)
}

func (e *iso9660encoder) size() sizeBytes {
	return sizeBytes(len(*e))
}

func (e *iso9660encoder) padLastSector() {
	if extra := e.size() % sectorSize; extra > 0 {
		e.appendZeroes(sectorSize - extra)
	}
}

func (e *iso9660encoder) appendByte(b byte) {
	*e = append(*e, b)
}

func (e *iso9660encoder) setByteAt(b byte, pos sizeBytes) {
	(*e)[pos] = b
}

func (e *iso9660encoder) appendUint16(v uint16, encoding binary.AppendByteOrder) {
	*e = encoding.AppendUint16(*e, v)
}

func (e *iso9660encoder) appendUint16LSBMSB(v uint16) {
	e.appendUint16(v, binary.LittleEndian)
	e.appendUint16(v, binary.BigEndian)
}

func (e *iso9660encoder) appendUint32(v uint32, encoding binary.AppendByteOrder) {
	*e = encoding.AppendUint32(*e, v)
}

func (e *iso9660encoder) appendUint32LSBMSB(v uint32) {
	e.appendUint32(v, binary.LittleEndian)
	e.appendUint32(v, binary.BigEndian)
}

func (e *iso9660encoder) appendBytes(b []byte) {
	*e = append(*e, b...)
}

func (e *iso9660encoder) appendZeroes(size sizeBytes) {
	*e = append(*e, make([]byte, size)...)
}

func (e *iso9660encoder) appendZeroSectors(size sizeSectors) {
	e.appendZeroes(size.bytes())
}

func (e *iso9660encoder) appendBytesFixed(b []byte, fixedLen sizeBytes) {
	inLen := sizeBytes(len(b))
	if fixedLen > 0 && inLen > fixedLen {
		panic("encoded data too large")
	}

	e.appendBytes(b)
	if fixedLen <= 0 || inLen == fixedLen {
		return
	}

	e.appendZeroes(fixedLen - inLen)
}

func (e *iso9660encoder) appendFormat(format string, args ...interface{}) {
	*e = fmt.Appendf(*e, format, args...)
}

func (e *iso9660encoder) appendString(s string, fixedLen sizeBytes, padding byte) {
	sLen := sizeBytes(len(s))
	if fixedLen > 0 && sLen > fixedLen {
		panic("encoded data too large")
	}

	*e = append(*e, s...)
	if fixedLen <= 0 || sLen == fixedLen {
		return
	}

	start := e.size()
	e.appendZeroes(fixedLen - sLen)

	if padding == 0 {
		return
	}

	for i := start; i < e.size(); i++ {
		(*e)[i] = padding
	}
}

func (e *iso9660encoder) appendStrA(s stringA, fixedLen sizeBytes) {
	e.appendString(string(s), fixedLen, ' ')
}

func (e *iso9660encoder) appendStrD(s stringD, fixedLen sizeBytes) {
	e.appendString(string(s), fixedLen, ' ')
}

func (e *iso9660encoder) appendStrD1(s stringD1, fixedLen sizeBytes) {
	e.appendString(string(s), fixedLen, ' ')
}

func (e *iso9660encoder) appendEncodable(enc iso9660encodable, fixedLen sizeBytes) {
	startPos := e.size()
	enc.encode(e)
	if fixedLen < 0 {
		return
	}

	encodedLen := e.size() - startPos
	if encodedLen > fixedLen {
		panic("encoded data too large")
	}

	if encodedLen < fixedLen {
		e.appendZeroes(fixedLen - encodedLen)
	}
}
