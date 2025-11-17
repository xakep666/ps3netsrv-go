package fs

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"

	"github.com/xakep666/ps3netsrv-go/internal/handler"
)

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
	start, end sizeSectors
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
	regionsHeaderSize sizeBytes
	encryptedRegions  []region
	cip               cipher.Block // to use in ReadAt
	cbcDec            cbcMode
	iv                []byte
	offset            sizeBytes // to track where we are now without calling Seek
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
			start: sizeSectors(unencryptedRegions[i-1].End),
			end:   sizeSectors(unencryptedRegion.Start),
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
		regionsHeaderSize: sizeBytes(binary.Size(hdr) + binary.Size(unencryptedRegions)),
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

	e.offset += sizeBytes(read)
	e.clearRegionsData(readStart, b[:read])
	e.decryptData(readStart, b[:read], false)
	return read, nil
}

func (e *EncryptedISO) ReadAt(b []byte, off int64) (int, error) {
	read, err := e.privateFile.ReadAt(b, off)
	if err != nil || read == 0 {
		return read, err
	}

	e.clearRegionsData(sizeBytes(off), b[:read])
	e.decryptData(sizeBytes(off), b[:read], true)
	return read, nil
}

func (e *EncryptedISO) Seek(offset int64, whence int) (int64, error) {
	newOffset, err := e.privateFile.Seek(offset, whence)
	if err != nil {
		return newOffset, err
	}

	e.offset = sizeBytes(newOffset)
	return newOffset, nil
}

func (e *EncryptedISO) clearRegionsData(start sizeBytes, data []byte) {
	if start >= e.regionsHeaderSize || !e.clearRegions {
		return
	}

	for i := sizeBytes(0); i < e.regionsHeaderSize-start && i < sizeBytes(len(data)); i++ {
		data[i] = 0
	}
}

func (e *EncryptedISO) decryptData(start sizeBytes, data []byte, cloneCBC bool) {
	end := start + sizeBytes(len(data))
	for _, region := range e.encryptedRegions {
		if region.end <= start.sectors() || region.start > end.sectors() { // not covered
			continue
		}

		startSector := max(region.start, start.floorSectors())
		endSector := min(region.end, end.sectors())
		for i := startSector; i < endSector; i++ {
			encryptedSpan := data[i.bytes()-start : i.next().bytes()-start]
			e.setIVForSector(i, cloneCBC).CryptBlocks(encryptedSpan, encryptedSpan)
		}
	}
}

func (e *EncryptedISO) setIVForSector(sector sizeSectors, clone bool) cipher.BlockMode {
	if !clone { // called from Read, may reuse state
		binary.BigEndian.PutUint32(e.iv[len(e.iv)-4:], uint32(sector))
		e.cbcDec.SetIV(e.iv)
		return e.cbcDec
	}

	// called from ReadAt, by convention it may be called from multiple goroutines, so we can't reuse state
	// probably should be optimized, but it's not used now
	var iv [encryptionKeySize]byte
	binary.BigEndian.PutUint32(iv[len(iv)-4:], uint32(sector))
	return cipher.NewCBCDecrypter(e.cip, iv[:])
}

// tryGetRedumpKey attempts to find encryption key for .iso image.
func tryGetRedumpKey(fsys handler.FS, requestedPath string) ([]byte, error) {
	// encryption makes sense only for .iso or .ISO file inside ps3ISO or PS3ISO directory
	ext := filepath.Ext(requestedPath)
	if strings.ToLower(ext) != isoExt {
		return nil, fs.ErrNotExist
	}

	pathElems := strings.Split(requestedPath, string(filepath.Separator))
	ps3IsoIdx := slices.IndexFunc(pathElems, func(s string) bool {
		return strings.ToLower(s) == ps3isoDir
	})
	if ps3IsoIdx < 0 {
		return nil, fs.ErrNotExist
	}

	// try .dkey file first
	keyFile, err := fsys.Open(strings.TrimSuffix(requestedPath, ext) + dkeyExt)
	if err == nil {
		defer keyFile.Close()
		return ReadKeyFile(keyFile)
	}

	// try .dkey in REDKEY directory (instead of PS3ISO)
	pathElems[ps3IsoIdx] = redkeyDir
	pathElems[len(pathElems)-1] = strings.TrimSuffix(pathElems[len(pathElems)-1], ext) + dkeyExt
	keyFile, err = fsys.Open(filepath.Join(pathElems...))
	if err == nil {
		defer keyFile.Close()
		return ReadKeyFile(keyFile)
	}

	return nil, err
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
