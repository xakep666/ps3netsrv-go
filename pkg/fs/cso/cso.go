package cso

import (
	"bytes"
	"compress/flate"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"syscall"

	"github.com/pierrec/lz4/v4"
	"github.com/xakep666/ps3netsrv-go/internal/handler"
)

type decompressor func(src, dst []byte) (int, error)

type File struct {
	f handler.File

	Header       *Header
	indexEntries *IndexEntryCache

	largeBlocksUncompressed bool         // if difference between two entry offsets >= block_size than block is uncompressed
	setBitDecompressor      decompressor // if entry top bit is 1
	clearBitDecompressor    decompressor // if entry top bit is 0

	offset   int64
	isClosed bool

	tmpBuf []byte

	cachedBlockNum int
	cachedBlock    []byte
}

func NewFile(f handler.File) (*File, error) {
	hdr, err := ReadHeader(f)
	if err != nil {
		return nil, fmt.Errorf("read header: %w", err)
	}

	// number taken from https://github.com/ps2homebrew/Open-PS2-Loader/blob/db4cb1f54045c231bc14227f035502d98e76ce60/modules/isofs/zso.h#L13
	const maxIndexEntries = 257

	ret := &File{
		f:              f,
		Header:         hdr,
		indexEntries:   NewIndexEntryCache(f, hdr, maxIndexEntries),
		tmpBuf:         make([]byte, hdr.BlockSize),
		cachedBlockNum: -1,
	}

	switch hdr.Variant() {
	case CSOv1:
		ret.clearBitDecompressor = flateDecompressor()
		ret.setBitDecompressor = rawDecompressor
	case CSOv2:
		if hdr.HeaderSize != AdvisedHeaderSize {
			return nil, fmt.Errorf("invalid header size %x", hdr.HeaderSize)
		}
		ret.clearBitDecompressor = flateDecompressor()
		ret.setBitDecompressor = lz4.UncompressBlock
		ret.largeBlocksUncompressed = true
	case ZSO:
		if hdr.HeaderSize != AdvisedHeaderSize {
			return nil, fmt.Errorf("invalid header size %x", hdr.HeaderSize)
		}
		ret.clearBitDecompressor = lz4.UncompressBlock
		ret.setBitDecompressor = rawDecompressor
	default:
		return nil, fmt.Errorf("unknown variant magic(%s)/version(%d)", hdr.Magic, hdr.Version)
	}

	return ret, nil
}

type csoStat struct {
	fs.FileInfo
	hdr *Header
}

func (s *csoStat) Size() int64 {
	return int64(s.hdr.UncompressedSize)
}

func (s *csoStat) Mode() fs.FileMode {
	return s.FileInfo.Mode() | fs.ModeIrregular
}

func (f *File) Stat() (fs.FileInfo, error) {
	fi, err := f.f.Stat()
	if err != nil {
		return nil, err
	}
	return &csoStat{
		FileInfo: fi,
		hdr:      f.Header,
	}, nil
}

func (f *File) Read(b []byte) (int, error) {
	if f.isClosed {
		return 0, fs.ErrClosed
	}

	if f.offset >= int64(f.Header.UncompressedSize) {
		return 0, io.EOF
	}

	startBlock := int(f.offset / int64(f.Header.BlockSize))
	if startBlock >= f.Header.BlocksCount() {
		return -1, fmt.Errorf("start block %d excceeds block count %d", startBlock, f.Header.BlocksCount())
	}
	blocksToRead := f.getBlocksCount(startBlock, b)

	var n int

	if posInBlock := f.offset % int64(f.Header.BlockSize); posInBlock > 0 {
		// handle unaligned offsets
		if err := f.updateCachedBlock(startBlock); err != nil {
			return 0, err
		}

		read := copy(b, f.cachedBlock[posInBlock:])
		n += read
		b = b[n:]
		startBlock++
		blocksToRead--
	}

	if blocksToRead == 0 {
		f.offset += int64(n)
		return n, nil
	}

	// read compressed blocks into the end of provided buffer to reduce syscalls amount
	// compressed blocks size are always less or equal to original block size
	// so buffer overflow will not occur
	startBlockOffset, err := f.indexEntries.OffsetOf(startBlock)
	if err != nil {
		return 0, err
	}
	endBlockOffset, err := f.indexEntries.OffsetOf(startBlock + blocksToRead)
	if err != nil {
		return 0, err
	}
	readSize := endBlockOffset - startBlockOffset
	compressedBlocks := b[len(b)-int(readSize):]

	_, err = f.f.Seek(int64(startBlockOffset), io.SeekStart)
	if err != nil {
		return 0, err
	}

	_, err = io.ReadFull(f.f, compressedBlocks)
	if err != nil {
		return 0, err
	}

	for blockNum := startBlock; len(b) > 0; blockNum++ {
		blockOffset, err := f.indexEntries.OffsetOf(blockNum)
		if err != nil {
			return 0, err
		}

		nextBlockOffset, err := f.indexEntries.OffsetOf(blockNum + 1)
		if err != nil {
			return 0, err
		}

		blockSize := nextBlockOffset - blockOffset

		// copy compressed data to temporary buffer
		copy(f.tmpBuf, compressedBlocks[:blockSize])

		dec, err := f.selectDecompressor(blockNum, blockSize)
		if err != nil {
			return 0, err
		}

		read, err := dec(f.tmpBuf[:blockSize], b[:f.Header.BlockSize])
		if err != nil {
			return 0, err
		}

		n += read
		b = b[read:]
		compressedBlocks = compressedBlocks[blockSize:]
	}

	f.offset += int64(n)
	return n, nil
}

func (f *File) getBlocksCount(startBlock int, b []byte) int {
	blocksToRead := len(b) / int(f.Header.BlockSize)
	if blocksToRead == 0 && len(b) > 0 {
		blocksToRead = 1 // at least 1 block for small buffers
	}
	if startBlock+blocksToRead > f.Header.BlocksCount() {
		blocksToRead = f.Header.BlocksCount() - startBlock
	}
	return blocksToRead
}

func (f *File) updateCachedBlock(blockNum int) error {
	if f.cachedBlockNum == blockNum {
		return nil
	}

	blockOffset, err := f.indexEntries.OffsetOf(blockNum)
	if err != nil {
		return err
	}

	nextBlockOffset, err := f.indexEntries.OffsetOf(blockNum + 1)
	if err != nil {
		return err
	}

	blockSize := nextBlockOffset - blockOffset

	f.cachedBlockNum = -1

	_, err = f.f.Seek(int64(blockOffset), io.SeekStart)
	if err != nil {
		return err
	}

	if len(f.tmpBuf) < int(blockSize) {
		f.tmpBuf = make([]byte, blockSize)
	}

	_, err = io.ReadFull(f.f, f.tmpBuf)
	if err != nil {
		return err
	}

	dec, err := f.selectDecompressor(blockNum, blockSize)
	if err != nil {
		return err
	}

	if len(f.cachedBlock) < int(f.Header.BlockSize) {
		f.cachedBlock = make([]byte, len(f.cachedBlock))
	}

	_, err = dec(f.tmpBuf, f.cachedBlock)
	if err != nil {
		return err
	}

	f.cachedBlockNum = blockNum
	return nil
}

func (f *File) selectDecompressor(blockNum int, rawBlockSize uint64) (decompressor, error) {
	topBit, err := f.indexEntries.TopBitOf(blockNum)
	if err != nil {
		return nil, err
	}
	switch {
	case f.largeBlocksUncompressed && rawBlockSize >= uint64(f.Header.BlockSize):
		return rawDecompressor, nil
	case topBit:
		return f.setBitDecompressor, nil
	default:
		return f.clearBitDecompressor, nil
	}
}

func (f *File) Close() error {
	if f.isClosed {
		return fs.ErrClosed
	}

	f.isClosed = true
	return f.f.Close()
}

func (f *File) ReadDir(n int) ([]fs.DirEntry, error) {
	return nil, errors.ErrUnsupported
}

func (f *File) Seek(offset int64, whence int) (int64, error) {
	if f.isClosed {
		return 0, fs.ErrClosed
	}

	switch whence {
	case io.SeekStart:
	case io.SeekCurrent:
		offset += int64(f.offset)
	case io.SeekEnd:
		offset = int64(f.Header.UncompressedSize) - offset - 1
	default:
		return 0, syscall.EINVAL
	}

	if offset < 0 || uint64(offset) > f.Header.UncompressedSize {
		return 0, syscall.EINVAL
	}

	f.offset = offset
	return offset, nil
}

func (f *File) Name() string {
	return f.f.Name()
}

func (f *File) Unwrap() handler.File {
	return f.f
}

func rawDecompressor(src, dst []byte) (int, error) {
	return copy(dst, src), nil
}

func flateDecompressor() decompressor {
	br := new(bytes.Reader)
	zr := flate.NewReader(nil)
	return func(src, dst []byte) (int, error) {
		br.Reset(src)
		if err := zr.(flate.Resetter).Reset(br, nil); err != nil {
			return -1, err
		}

		n, err := io.ReadFull(zr, dst)
		if err != nil {
			return -1, err
		}

		if err = zr.Close(); err != nil {
			return -1, err
		}

		return n, nil
	}
}
