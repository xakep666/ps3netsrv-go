package encryptediso

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"

	"github.com/xakep666/ps3netsrv-go/internal/handler"
	"github.com/xakep666/ps3netsrv-go/internal/iso9660"
)

type privateFile = handler.File

const (
	encryptionKeySize = 16

	isoExt    = ".iso"
	dkeyExt   = ".dkey"
	ps3isoDir = "ps3iso"
	redkeyDir = "REDKEY"
)

var (
	// keyData1 is a base encryption key for image.
	// Key used to decrypt image is a result of its encryption with ivData1 in CBC mode.
	keyData1 = [encryptionKeySize]byte{0x38, 0x0b, 0xcf, 0x0b, 0x53, 0x45, 0x5b, 0x3c, 0x78, 0x17, 0xab, 0x4f, 0xa3, 0xba, 0x90, 0xed}
	ivData1  = [encryptionKeySize]byte{0x69, 0x47, 0x47, 0x72, 0xaf, 0x6f, 0xda, 0xb3, 0x42, 0x74, 0x3a, 0xef, 0xaa, 0x18, 0x62, 0x87}
)

type cbcMode interface {
	cipher.BlockMode
	SetIV(iv []byte)
}

type region struct {
	start, end iso9660.SizeSectors
}

type unencryptedRegionsHeader struct {
	Count uint32
	_     uint32 // pad
}

type unencryptedRegion struct {
	Start, End uint32
}

// EncryptedISO is a wrapper to decrypt encrypted images on-the-fly.
// Redump (and 3k3y dump) is not completely encrypted.
// It's consist from "regions" (one or more sectors). Only odd regions are encrypted.
// Region map placed in the beginning of the image.
// Write operations are blocked due to complexity of implementation.
// See https://www.psdevwiki.com/ps3/Bluray_disc#Encryption for details.
type EncryptedISO struct {
	privateFile

	clearRegions      bool
	regionsHeaderSize iso9660.SizeBytes
	encryptedRegions  []region
	cbcDec            cbcMode
	iv                []byte
	offset            iso9660.SizeBytes // to track where we are now without calling Seek
}

// NewEncryptedISO wraps File to EncryptedISO with provided "data1" key.
// "clearRegions" defines if regions header should be zeroed during reads, some software can't handle non-clear header.
func NewEncryptedISO(f handler.File, data1 []byte, clearRegions bool) (*EncryptedISO, error) {
	// force seek to start for convenience
	_, err := f.Seek(0, io.SeekStart)
	if err != nil {
		return nil, fmt.Errorf("seek start failed: %w", err)
	}

	var hdr unencryptedRegionsHeader
	err = binary.Read(f, binary.BigEndian, &hdr)
	if err != nil {
		return nil, fmt.Errorf("read unencrypted regions count failed: %w", err)
	}

	unencryptedRegions := make([]unencryptedRegion, hdr.Count)
	err = binary.Read(f, binary.BigEndian, unencryptedRegions)
	if err != nil {
		return nil, fmt.Errorf("read region map: %w", err)
	}

	_, err = f.Seek(0, io.SeekStart)
	if err != nil {
		return nil, fmt.Errorf("seek start failed: %w", err)
	}

	if hdr.Count < 2 { // minimum 1 encrypted region
		return nil, fmt.Errorf("unexpected unencrypted regions count (%d)", hdr.Count)
	}
	if unencryptedRegions[0].Start != 0 {
		return nil, fmt.Errorf("region 0 start is not zero (%#x)", unencryptedRegions[0].Start)
	}

	var prevRegionEnd uint32
	encryptedRegions := make([]region, hdr.Count-1)
	for i, unencryptedRegion := range unencryptedRegions {
		// some sanity checks: region "borders" must increase monotonically
		if unencryptedRegion.End <= unencryptedRegion.Start {
			return nil, fmt.Errorf("region %d: end (%#x) less than start (%#x)",
				i, unencryptedRegion.End, unencryptedRegion.Start)
		}
		if unencryptedRegion.Start < prevRegionEnd {
			return nil, fmt.Errorf("region %d: start (%#x) less than previous region end (%#x)",
				i, unencryptedRegion.End, prevRegionEnd)
		}
		prevRegionEnd = unencryptedRegion.End

		if i == 0 {
			continue
		}

		// encrypted region placed between previous unencrypted region and current unencrypted region
		encryptedRegions = append(encryptedRegions, region{
			start: iso9660.SizeSectors(unencryptedRegions[i-1].End),
			end:   iso9660.SizeSectors(unencryptedRegion.Start),
		})
	}

	var isoKey [encryptionKeySize]byte
	if err = deriveISOKey(isoKey[:], data1); err != nil {
		return nil, fmt.Errorf("derive iso key failed: %w", err)
	}

	cip, err := aes.NewCipher(isoKey[:])
	if err != nil {
		return nil, err
	}

	var iv [encryptionKeySize]byte
	return &EncryptedISO{
		clearRegions:      clearRegions,
		regionsHeaderSize: iso9660.SizeBytes(binary.Size(hdr) + binary.Size(unencryptedRegions)),
		privateFile:       f,
		encryptedRegions:  encryptedRegions,
		cbcDec:            cipher.NewCBCDecrypter(cip, iv[:]).(cbcMode),
		iv:                iv[:],
	}, nil
}

func (e *EncryptedISO) Read(b []byte) (int, error) {
	readStart := e.offset

	read, err := e.privateFile.Read(b)
	if err != nil || read == 0 {
		return read, err
	}

	e.offset += iso9660.SizeBytes(read)
	e.clearRegionsData(readStart, b[:read])
	e.decryptData(readStart, b[:read])
	return read, nil
}

func (e *EncryptedISO) Seek(offset int64, whence int) (int64, error) {
	newOffset, err := e.privateFile.Seek(offset, whence)
	if err != nil {
		return newOffset, err
	}

	e.offset = iso9660.SizeBytes(newOffset)
	return newOffset, nil
}

func (e *EncryptedISO) clearRegionsData(start iso9660.SizeBytes, data []byte) {
	if start >= e.regionsHeaderSize || !e.clearRegions {
		return
	}

	for i := iso9660.SizeBytes(0); i < e.regionsHeaderSize-start && i < iso9660.SizeBytes(len(data)); i++ {
		data[i] = 0
	}
}

func (e *EncryptedISO) decryptData(start iso9660.SizeBytes, data []byte) {
	end := start + iso9660.SizeBytes(len(data))
	for _, region := range e.encryptedRegions {
		if region.end <= start.Sectors() || region.start > end.Sectors() { // not covered
			continue
		}

		startSector := max(region.start, start.FloorSectors())
		endSector := min(region.end, end.Sectors())
		for i := startSector; i < endSector; i++ {
			encryptedSpan := data[i.Bytes()-start : i.Next().Bytes()-start]
			e.setIVForSector(i).CryptBlocks(encryptedSpan, encryptedSpan)
		}
	}
}

func (e *EncryptedISO) setIVForSector(sector iso9660.SizeSectors) cipher.BlockMode {
	binary.BigEndian.PutUint32(e.iv[len(e.iv)-4:], uint32(sector))
	e.cbcDec.SetIV(e.iv)
	return e.cbcDec
}

func deriveISOKey(targetKey, data1Key []byte) error {
	cip, err := aes.NewCipher(keyData1[:])
	if err != nil {
		return err
	}

	cipher.NewCBCEncrypter(cip, ivData1[:]).CryptBlocks(targetKey, data1Key)
	return nil
}

// ReadKeyFile reads key file and decodes hex-encoded key.
func ReadKeyFile(f io.Reader) ([]byte, error) {
	var key [encryptionKeySize]byte
	_, err := io.ReadFull(hex.NewDecoder(f), key[:])
	return key[:], err
}
