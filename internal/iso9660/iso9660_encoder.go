package iso9660

import (
	"encoding/binary"
	"fmt"
)

type Encoder []byte

type Encodable interface {
	Encode(e *Encoder)
}

func (e *Encoder) Size() SizeBytes {
	return SizeBytes(len(*e))
}

func (e *Encoder) PadLastSector() {
	if extra := e.Size() % SectorSize; extra > 0 {
		e.AppendZeroes(SectorSize - extra)
	}
}

func (e *Encoder) AppendByte(b byte) {
	*e = append(*e, b)
}

func (e *Encoder) SetByteAt(b byte, pos SizeBytes) {
	(*e)[pos] = b
}

func (e *Encoder) AppendUint16(v uint16, encoding binary.AppendByteOrder) {
	*e = encoding.AppendUint16(*e, v)
}

func (e *Encoder) AppendUint16LSBMSB(v uint16) {
	e.AppendUint16(v, binary.LittleEndian)
	e.AppendUint16(v, binary.BigEndian)
}

func (e *Encoder) AppendUint32(v uint32, encoding binary.AppendByteOrder) {
	*e = encoding.AppendUint32(*e, v)
}

func (e *Encoder) AppendUint32LSBMSB(v uint32) {
	e.AppendUint32(v, binary.LittleEndian)
	e.AppendUint32(v, binary.BigEndian)
}

func (e *Encoder) AppendBytes(b []byte) {
	*e = append(*e, b...)
}

func (e *Encoder) AppendZeroes(size SizeBytes) {
	*e = append(*e, make([]byte, size)...)
}

func (e *Encoder) AppendZeroSectors(size SizeSectors) {
	e.AppendZeroes(size.Bytes())
}

func (e *Encoder) AppendBytesFixed(b []byte, fixedLen SizeBytes) {
	inLen := SizeBytes(len(b))
	if fixedLen > 0 && inLen > fixedLen {
		panic("encoded data too large")
	}

	e.AppendBytes(b)
	if fixedLen <= 0 || inLen == fixedLen {
		return
	}

	e.AppendZeroes(fixedLen - inLen)
}

func (e *Encoder) AppendFormat(format string, args ...any) {
	*e = fmt.Appendf(*e, format, args...)
}

func (e *Encoder) AppendString(s string, fixedLen SizeBytes, padding byte) {
	sLen := SizeBytes(len(s))
	if fixedLen > 0 && sLen > fixedLen {
		panic("encoded data too large")
	}

	*e = append(*e, s...)
	if fixedLen <= 0 || sLen == fixedLen {
		return
	}

	start := e.Size()
	e.AppendZeroes(fixedLen - sLen)

	if padding == 0 {
		return
	}

	for i := start; i < e.Size(); i++ {
		(*e)[i] = padding
	}
}

func (e *Encoder) AppendStringA(s StringA, fixedLen SizeBytes) {
	e.AppendString(string(s), fixedLen, ' ')
}

func (e *Encoder) AppendStringD(s StringD, fixedLen SizeBytes) {
	e.AppendString(string(s), fixedLen, ' ')
}

func (e *Encoder) AppendStringD1(s StringD1, fixedLen SizeBytes) {
	e.AppendString(string(s), fixedLen, ' ')
}

func (e *Encoder) AppendEncodable(enc Encodable, fixedLen SizeBytes) {
	startPos := e.Size()
	enc.Encode(e)
	if fixedLen < 0 {
		return
	}

	encodedLen := e.Size() - startPos
	if encodedLen > fixedLen {
		panic("encoded data too large")
	}

	if encodedLen < fixedLen {
		e.AppendZeroes(fixedLen - encodedLen)
	}
}
