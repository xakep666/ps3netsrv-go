package iso3k3y

import (
	"errors"
	"fmt"
	"io"

	"github.com/xakep666/ps3netsrv-go/internal/handler"
	"github.com/xakep666/ps3netsrv-go/internal/ioutil"
	"github.com/xakep666/ps3netsrv-go/internal/iso9660"
)

const encryptionKeySize = 16

const (
	_3k3yMaskedDataBegin iso9660.SizeBytes = 0xF70
	_3k3yMaskedDataSize  iso9660.SizeBytes = 256
	_3k3yMaskedDataEnd                     = _3k3yMaskedDataBegin + _3k3yMaskedDataSize

	_3k3yWatermarkPlacement iso9660.SizeBytes = 0x0 // relative to _3k3yMaskedDataBegin
	_3k3yWatermarkSize      iso9660.SizeBytes = 16
	_3k3yWatermarkEnd                         = _3k3yWatermarkPlacement + _3k3yWatermarkSize

	_3k3yEncryptionKeyPlacement iso9660.SizeBytes = 0x10 // relative to _3k3yMaskedDataBegin
	_3k3yEncryptionKeyEnd                         = _3k3yEncryptionKeyPlacement + encryptionKeySize
)

var (
	// _3k3yDecWatermark appears in decrypted 3k3y image (only watermark masking applied).
	_3k3yDecWatermark = [_3k3yWatermarkSize]byte{0x45, 0x6E, 0x63, 0x72, 0x79, 0x70, 0x74, 0x65, 0x64, 0x20, 0x33, 0x4B, 0x20, 0x42, 0x4C, 0x44}

	// _3k3yEncWatermark appears in encrypted 3k3y image.
	_3k3yEncWatermark = [_3k3yWatermarkSize]byte{0x44, 0x6E, 0x63, 0x72, 0x79, 0x70, 0x74, 0x65, 0x64, 0x20, 0x33, 0x4B, 0x20, 0x42, 0x4C, 0x44}
)

var ErrNot3k3y = fmt.Errorf("not 3k3y image")

type privateFile = handler.File

// ISO3k3y is a simple wrapper to remove 3k3y data on-the-fly during reads.
// Write operations are blocked because here we don't know how to handle them correctly.
type ISO3k3y struct {
	privateFile

	offset iso9660.SizeBytes
}

// NewISO3k3y wraps File to ISO3k3y.
func NewISO3k3y(f handler.File) (*ISO3k3y, error) {
	curOffset, err := f.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, fmt.Errorf("get current offset failed: %w", err)
	}

	return &ISO3k3y{
		privateFile: f,

		offset: iso9660.SizeBytes(curOffset),
	}, nil
}

func (iso *ISO3k3y) Read(b []byte) (int, error) {
	readStart := iso.offset

	read, err := iso.privateFile.Read(b)
	if err != nil || read == 0 {
		return read, err
	}

	iso.offset += iso9660.SizeBytes(read)
	iso.clear3k3yData(readStart, b[:read])
	return read, nil
}

func (*ISO3k3y) clear3k3yData(start iso9660.SizeBytes, data []byte) {
	end := start + iso9660.SizeBytes(len(data))
	if start >= _3k3yMaskedDataEnd || end < _3k3yMaskedDataBegin {
		return
	}

	for i := _3k3yMaskedDataBegin - start; i < min(_3k3yMaskedDataEnd, end)-start; i++ {
		data[i] = 0
	}
}

func (iso *ISO3k3y) Seek(offset int64, whence int) (int64, error) {
	newOffset, err := iso.privateFile.Seek(offset, whence)
	if err != nil {
		return newOffset, err
	}

	iso.offset = iso9660.SizeBytes(newOffset)
	return newOffset, nil
}

func (iso *ISO3k3y) Unwrap() handler.File {
	return iso.privateFile
}

// Test3k3yImage performs checks if it is 3k3y image and returns ErrNot3k3y if not.
// If key is not empty then image is encrypted.
func Test3k3yImage(f io.ReadSeeker) ([]byte, error) {
	var data [_3k3yMaskedDataSize]byte

	err := ioutil.FillBuffer(f, int64(_3k3yMaskedDataBegin), data[:])
	switch {
	case errors.Is(err, nil):
		// pass
	case errors.Is(err, io.EOF), errors.Is(err, io.ErrUnexpectedEOF):
		return nil, ErrNot3k3y
	default:
		return nil, err
	}

	watermark := (*[_3k3yWatermarkSize]byte)(data[_3k3yWatermarkPlacement:_3k3yWatermarkEnd])
	switch *watermark {
	case _3k3yEncWatermark:
		return data[_3k3yEncryptionKeyPlacement:_3k3yEncryptionKeyEnd], nil
	case _3k3yDecWatermark:
		return nil, nil
	default:
		return nil, ErrNot3k3y
	}
}
