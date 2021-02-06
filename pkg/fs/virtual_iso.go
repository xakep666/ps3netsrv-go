package fs

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"github.com/spf13/afero"
	"go.uber.org/multierr"
)

const (
	ps3ModeVolumeName = "PS3VOLUME"
	consoleID         = "PlayStation3"

	multiExtentPartSize       sizeBytes   = 0xFFFFF800
	maxPartSize               sizeBytes   = 0xFFFFFFFF
	basePadSectors            sizeSectors = 0x20
	volumeDescriptorsCount    sizeSectors = 3
	discRangesSectorPlacement sizeSectors = 0
	discInfoSectorPlacement   sizeSectors = 1

	dotEntryIdentifier    = stringD1(byte(0))
	dotDotEntryIdentifier = stringD1(byte(1))
)

// ErrNotDirectory occurs when root path for VirtualISO is not a directory
var ErrNotDirectory = fmt.Errorf("not a directory")

var paramSFOPath = filepath.Join("PS3_GAME", "PARAM.SFO")

// VirtualISO is a on-the-fly generated .iso disk image.
// According to iso9660 spec image consists of:
// 1) 16 empty sectors (system area, for ps3 game mode 0 and 1 sectors set)
// 2) volume descriptors (1 per sector)
// 3) little-endian and big-endian path tables (multiple entries in one sector, table aligned to sector size)
// 4) directory entries (multiple in one sector)
// 5) files
// PS3 requires also Joliet extension. To support it here added:
// * supplementary volume descriptor (goes second)
// * duplicated path tables (same as for plain iso9660 but identifiers encoded with utf16-be w/o bom) after iso ones
// * duplicated directory entries (like for path tables) after iso ones aligned to sector size
// The main idea is to make binary representation of parts 1-4 in memory (named fsBuf next) and keep files on disk.
// So to achieve this file address space partitioned like:
// * 0 - fsBuf size: binary encoded parts 1-4 (kept inmemory)
// * fsBuf size - file1 start: file 1 (from disk)
// * file1 end - file2 end: file 2 (from disk)
// * fileN start - fileN end: file N (from disk)
// * lastFile end - volume end: padding area (zero bytes) from last file end to volume end
// To serve file we need to correctly compare address space boundaries and copy data from fsBuf
// or open and send file from disk.
// In ps3 game mode we have to parse PARAM.SFO and get TITLE_ID to create sector 1 and
// write full volume size to sector 0.
type VirtualISO struct {
	fs        afero.Fs
	root      string
	ps3Mode   bool
	createdAt time.Time

	rootDir           dirItemList // must be alphabetically sort by path
	filesSizeSectors  sizeSectors // sum of file sizes in sectors
	pathTable         pathTable   // used in network ps3 mode, testing isn't easy because most desktop OSes ignore it
	pathTableJoliet   pathTable
	volumeDescriptors [volumeDescriptorsCount]volumeDescriptor
	volumeSizeSectors sizeSectors

	isClosed     bool
	totalSize    sizeBytes // whole disc size (with files)
	padAreaStart sizeBytes
	padAreaSize  sizeBytes
	fsBuf        disc      // binary-encoded filesystem structures
	offset       sizeBytes // used during Read and Seek
}

// NewVirtualISO creates a virtual iso object from given root optionally with some data for ps3 games.
// Root path must be without ..'s.
func NewVirtualISO(fs afero.Fs, root string, ps3Mode bool) (*VirtualISO, error) {
	rootStat, err := fs.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("stat failed: %w", err)
	}

	if !rootStat.IsDir() {
		return nil, ErrNotDirectory
	}

	// add path separator to end for empty root
	if root == "" {
		root += string(os.PathSeparator)
	}

	ret := &VirtualISO{
		fs:        fs,
		root:      root,
		ps3Mode:   ps3Mode,
		createdAt: time.Now(),
	}

	if err := ret.init(); err != nil {
		return nil, err
	}

	return ret, nil
}

func (viso *VirtualISO) init() error {
	var (
		err error

		volumeName, gameCode string
	)

	if viso.ps3Mode {
		gameCode, err = viso.getTitleID()
		if err != nil {
			return fmt.Errorf("getTitleID failed: %w", err)
		}

		volumeName = ps3ModeVolumeName
	} else {
		_, volumeName = filepath.Split(viso.root)
	}

	if err := viso.buildFS(volumeName, gameCode); err != nil {
		return fmt.Errorf("build fs failed: %w", err)
	}

	return nil
}

func (viso *VirtualISO) getTitleID() (string, error) {
	f, err := viso.fs.Open(filepath.Join(viso.root, paramSFOPath))
	if err != nil {
		return "", fmt.Errorf("param.sfo open failed: %w", err)
	}

	defer f.Close()

	return sfoField(f, "TITLE_ID")
}

func (viso *VirtualISO) buildFS(volumeName, gameCode string) error {
	if err := viso.buildFSStructures(volumeName); err != nil {
		return fmt.Errorf("build fs stuctures failed: %w", err)
	}

	if err := viso.writeFSStructures(gameCode); err != nil {
		return fmt.Errorf("write fs structures failed: %w", err)
	}

	return nil
}

func (viso *VirtualISO) buildFSStructures(volumeName string) error {
	// map fs tree to rootDir
	err := viso.scanDirectory()
	if err != nil {
		return fmt.Errorf("map fs tree to rootDir failed: %w", err)
	}

	for i := range viso.rootDir {
		if err := viso.makeDirEntries(&viso.rootDir[i], false); err != nil {
			return fmt.Errorf("failed to make dir entries: %w", err)
		}
	}

	for i := range viso.rootDir {
		if err := viso.makeDirEntries(&viso.rootDir[i], true); err != nil {
			return fmt.Errorf("failed to make joliet dir entries: %w", err)
		}
	}

	viso.pathTable, err = viso.makePathTable(false)
	if err != nil {
		return fmt.Errorf("failed to make path table: %w", err)
	}

	viso.pathTableJoliet, err = viso.makePathTable(true)
	if err != nil {
		return fmt.Errorf("failed to make joliet path table: %w", err)
	}

	isoLBA := systemAreaSize.sectors() +
		volumeDescriptorsCount + // volume descriptors are 1 per sector
		1 + // empty sector after descriptors
		viso.pathTable.size().sectors()*2 + // little-endian + big-endian table
		viso.pathTableJoliet.size().sectors()*2
	jolietLBA := isoLBA + viso.rootDir.size(false).sectors()
	filesLBA := jolietLBA + viso.rootDir.size(true).sectors()

	viso.calculateSizes(filesLBA)

	viso.makeVolumeDescriptors(volumeName)

	// we need to shift LBAs (sector numbers) of all structures
	// due to space before filesystem start and volume descriptors
	viso.rootDir.fixLBA(isoLBA, jolietLBA, filesLBA)
	viso.pathTable.fixLBA(isoLBA)
	viso.pathTableJoliet.fixLBA(jolietLBA)

	return nil
}

func (viso *VirtualISO) scanDirectory() error {
	// scan directory recursively using BFS to ensure that files will be located (rLBA) sequentially
	queue := []string{viso.root} // paths

	for i := 0; i < len(queue); i++ {
		dir, err := viso.fs.Open(queue[i])
		if err != nil {
			return fmt.Errorf("dir %s open failed: %w", queue[i], err)
		}

		stat, err := dir.Stat()
		if err != nil {
			return fmt.Errorf("dir %s stat failed: %w", queue[i], err)
		}

		if !stat.IsDir() {
			return fmt.Errorf("%s is not a dir", queue[i])
		}

		viso.rootDir = append(viso.rootDir, dirItem{
			path:    queue[i],
			name:    stat.Name(),
			modTime: stat.ModTime(),
		})

		items, err := dir.Readdirnames(-1)
		if err != nil {
			return fmt.Errorf("dir %s items get failed: %w", queue[i], err)
		}

		for _, item := range items {
			fullPath := filepath.Join(queue[i], item)
			itemStat, err := viso.fs.Stat(fullPath)
			if err != nil {
				return fmt.Errorf("item %s stat failed: %w", fullPath, err)
			}

			if itemStat.IsDir() {
				queue = append(queue, fullPath)
				continue
			}

			fi := fileItem{
				path:    fullPath,
				name:    itemStat.Name(),
				size:    sizeBytes(itemStat.Size()),
				rLBA:    viso.filesSizeSectors,
				modTime: itemStat.ModTime(),
			}

			viso.rootDir[len(viso.rootDir)-1].files = append(viso.rootDir[len(viso.rootDir)-1].files, fi)

			viso.filesSizeSectors += fi.size.sectors()
		}
	}

	return nil
}

func (viso *VirtualISO) makeDirEntries(item *dirItem, joliet bool) error {
	var totalSizeBytes sizeBytes

	// '.' entry
	dotEntry := directoryEntry{
		FileFlags:            dirFlagDir,
		ExtentLocation:       viso.rootDir.size(joliet).sectors(),
		RecordingDateTime:    recordingTimestamp(item.modTime),
		VolumeSequenceNumber: 1,
		Identifier:           dotEntryIdentifier,
	}

	// '..' entry
	dotDotEntry := directoryEntry{
		FileFlags:            dirFlagDir,
		VolumeSequenceNumber: 1,
		Identifier:           dotDotEntryIdentifier,
	}

	parent := viso.rootDir.parent(*item)
	if parent != nil {
		// link parent directory
		dotDotEntry.RecordingDateTime = recordingTimestamp(parent.modTime)
		dotDotEntry.ExtentLocation = parent.dirEntry[0].ExtentLocation
		if joliet {
			dotDotEntry.ExtentLocation = parent.dirEntryJoliet[0].ExtentLocation
		}
	} else {
		dotDotEntry.RecordingDateTime = dotEntry.RecordingDateTime
	}

	if joliet {
		item.dirEntryJoliet = append(item.dirEntryJoliet, dotEntry, dotDotEntry)
	} else {
		item.dirEntry = append(item.dirEntry, dotEntry, dotDotEntry)
	}

	totalSizeBytes += dotEntry.Size() + dotDotEntry.Size()

	// file entries
	for _, fileItem := range item.files {
		parts := 1
		lba := fileItem.rLBA

		if fileItem.size > maxPartSize { // 4Gb
			parts = int(fileItem.size / multiExtentPartSize)
			if fileItem.size%multiExtentPartSize > 0 {
				// one more part if file doesn't fully fit to last extent
				parts++
			}
		}

		for i := 0; i < parts; i++ {
			entry := directoryEntry{
				RecordingDateTime:    recordingTimestamp(fileItem.modTime),
				VolumeSequenceNumber: 1,
				ExtentLocation:       lba,
			}

			switch {
			case parts == 1:
				entry.ExtentLength = fileItem.size
			case i == parts-1:
				entry.ExtentLength = fileItem.size - sizeBytes(i)*multiExtentPartSize
			default:
				entry.ExtentLength = multiExtentPartSize
				entry.FileFlags = dirFlagMultiExtent
				lba += multiExtentPartSize.sectors()
			}

			if joliet {
				entry.Identifier = makeIdentifier(fileItem.name, true)
				item.dirEntryJoliet = append(item.dirEntryJoliet, entry)
			} else {
				entry.Identifier = makeIdentifier(fileItem.name, false)
				item.dirEntry = append(item.dirEntry, entry)
			}

			totalSizeBytes += entry.Size()
		}
	}

	// add child dir entries
	for i, dirItem := range viso.rootDir {
		if i == 0 {
			continue
		}

		if !dirItem.isDirectChild(*item) {
			continue
		}

		entry := directoryEntry{
			FileFlags:            dirFlagDir,
			VolumeSequenceNumber: 1,
			RecordingDateTime:    recordingTimestamp(dirItem.modTime),
			Identifier:           makeIdentifier(dirItem.name, joliet),
		}

		if joliet {
			item.dirEntryJoliet = append(item.dirEntryJoliet, entry)
		} else {
			item.dirEntry = append(item.dirEntry, entry)
		}

		totalSizeBytes += entry.Size()
	}

	// total size must be integer number of sectors so ceil it if needed
	totalSizeBytes = totalSizeBytes.sectors().bytes()

	// set correct size to first entry
	if joliet {
		item.dirEntryJoliet[0].ExtentLength = totalSizeBytes
	} else {
		item.dirEntry[0].ExtentLength = totalSizeBytes
	}

	if parent == nil {
		// fix '..' record for root
		parentRecord := &item.dirEntry[1]
		dirEntry := item.dirEntry
		if joliet {
			parentRecord = &item.dirEntryJoliet[1]
			dirEntry = item.dirEntryJoliet
		}

		parentRecord.ExtentLength = dirEntry[0].ExtentLength
		parentRecord.ExtentLocation = dirEntry[0].ExtentLocation
	} else {
		// find child record pointing to this item in parent
		dirEntry := item.dirEntry
		if joliet {
			dirEntry = item.dirEntryJoliet
		}

		childRecord := parent.findDirEntry(item, joliet)
		childRecord.ExtentLength = dirEntry[0].ExtentLength
		childRecord.ExtentLocation = dirEntry[0].ExtentLocation
	}

	return nil
}

func (viso *VirtualISO) makePathTable(joliet bool) (pathTable, error) {
	var ret pathTable

	for i := 0; i < len(viso.rootDir) && i < pathTableItemsLimit; i++ {
		pathTableEntry := pathTableEntry{
			DirIdentifier: makeIdentifier(viso.rootDir[i].name, joliet),
		}

		if i == 0 {
			pathTableEntry.ParentDirNumber = 1
			pathTableEntry.DirIdentifier = dotEntryIdentifier
		} else {
			parentIdx := viso.rootDir.parentIdx(viso.rootDir[i])
			if parentIdx < 0 {
				return nil, fmt.Errorf("unexpectedly no parent")
			}

			pathTableEntry.ParentDirNumber = int16(parentIdx + 1)
		}

		pathTableEntry.DirLocation = viso.rootDir[i].dirEntry[0].ExtentLocation
		if joliet {
			pathTableEntry.DirLocation = viso.rootDir[i].dirEntryJoliet[0].ExtentLocation
		}

		ret = append(ret, pathTableEntry)
	}

	return ret, nil
}

func (viso *VirtualISO) calculateSizes(filesLBA sizeSectors) {
	// in sectors
	volumeSize := filesLBA + viso.filesSizeSectors
	padSectors := basePadSectors

	if extraPad := volumeSize % basePadSectors; extraPad > 0 {
		padSectors += basePadSectors - extraPad
	}

	volumeSizeWithPad := volumeSize + padSectors

	viso.volumeSizeSectors = volumeSizeWithPad

	// in bytes
	viso.totalSize = volumeSizeWithPad.bytes()
	viso.padAreaStart = volumeSize.bytes()
	viso.padAreaSize = padSectors.bytes()
}

func (viso *VirtualISO) makeVolumeDescriptors(volumeName string) {
	descriptorsLBA := systemAreaSize.sectors()
	pathTableLLBA := descriptorsLBA + volumeDescriptorsCount + 1                       // little-endian iso path table
	pathTableMLBA := pathTableLLBA + viso.pathTable.size().sectors()                   // big-endian iso path table
	pathTableJolietLLBA := pathTableMLBA + viso.pathTable.size().sectors()             // little-endian joliet path table
	pathTableJolietMLBA := pathTableJolietLLBA + viso.pathTableJoliet.size().sectors() // big-endian joliet path table

	now := time.Now()

	pvd := volumeDescriptor{
		Header: volumeDescriptorHeader{
			Type:       volumeTypePrimary,
			Identifier: standardIdentifierBytes,
			Version:    1,
		},
		Primary: &primaryVolumeDescriptorBody{
			SystemIdentifier:              mangleStrA(runtime.GOOS, false),
			VolumeIdentifier:              mangleStrD(volumeName, false),
			VolumeSpaceSize:               viso.volumeSizeSectors,
			VolumeSetSize:                 1,
			VolumeSequenceNumber:          1,
			LogicalBlockSize:              sectorSize,
			PathTableSize:                 viso.pathTable.size(),
			TypeLPathTableLoc:             pathTableLLBA,
			TypeMPathTableLoc:             pathTableMLBA,
			ApplicationIdentifier:         "ps3netsrv",
			VolumeSetIdentifier:           mangleStrD(volumeName, false),
			VolumeCreationDateAndTime:     volumeDescriptorTimestampFromTime(now),
			VolumeModificationDateAndTime: volumeDescriptorTimestampFromTime(now),
			FileStructureVersion:          1,
			RootDirectoryEntry:            &viso.rootDir[0].dirEntry[0],
		},
	}

	pvdJoliet := volumeDescriptor{
		Header: volumeDescriptorHeader{
			Type:       volumeTypeSupplementary,
			Identifier: standardIdentifierBytes,
			Version:    1,
		},
		Primary: &primaryVolumeDescriptorBody{
			SystemIdentifier:              mangleStrA(runtime.GOOS, true),
			VolumeIdentifier:              mangleStrD(volumeName, true),
			VolumeSpaceSize:               viso.volumeSizeSectors,
			EscapeSequences:               "%/@",
			VolumeSetSize:                 1,
			VolumeSequenceNumber:          1,
			LogicalBlockSize:              sectorSize,
			PathTableSize:                 viso.pathTableJoliet.size(),
			TypeLPathTableLoc:             pathTableJolietLLBA,
			TypeMPathTableLoc:             pathTableJolietMLBA,
			ApplicationIdentifier:         "ps3netsrv",
			VolumeSetIdentifier:           mangleStrD(volumeName, true),
			VolumeCreationDateAndTime:     volumeDescriptorTimestampFromTime(now),
			VolumeModificationDateAndTime: volumeDescriptorTimestampFromTime(now),
			FileStructureVersion:          1,
			RootDirectoryEntry:            &viso.rootDir[0].dirEntryJoliet[0],
		},
	}

	terminator := volumeDescriptor{
		Header: volumeDescriptorHeader{
			Type:       volumeTypeTerminator,
			Identifier: standardIdentifierBytes,
		},
	}

	viso.volumeDescriptors = [volumeDescriptorsCount]volumeDescriptor{pvd, pvdJoliet, terminator}
}

func (viso *VirtualISO) writeFSStructures(gameCode string) error {
	// 16 empty sectors
	for i := sizeSectors(0); i < systemAreaSize.sectors(); i++ {
		viso.fsBuf.appendSector(emptySector[:])
	}

	// ps3-game specific sectors
	if viso.ps3Mode {
		err := viso.fsBuf.setSectorByMarshaller(discRangesSector{{
			StartSector: 0,
			EndSector:   viso.volumeSizeSectors - 1,
		}}, discRangesSectorPlacement)
		if err != nil {
			return fmt.Errorf("failed to write ranges sector: %w", err)
		}

		infoSector := discInfoSector{
			ConsoleID: consoleID,
			ProductID: gameCode[:4] + "-" + gameCode[4:], // i.e. BCES-00104
		}

		_, err = io.ReadFull(rand.Reader, infoSector.Info[:])
		if err != nil {
			return fmt.Errorf("failed to generate info in info sector: %w", err)
		}

		_, err = io.ReadFull(rand.Reader, infoSector.Hash[:])
		if err != nil {
			return fmt.Errorf("failed to generate hash in info sector: %w", err)
		}

		if err := viso.fsBuf.setSectorByMarshaller(&infoSector, discInfoSectorPlacement); err != nil {
			return fmt.Errorf("failed to write info sector: %w", err)
		}
	}

	// volume descriptors
	for _, vd := range viso.volumeDescriptors {
		if err := viso.fsBuf.appendMarshaller(vd, true); err != nil {
			return fmt.Errorf("failed to write volume descriptor: %w", err)
		}
	}

	// empty sector
	viso.fsBuf.appendSector(emptySector[:])

	// pathTableL
	for _, e := range viso.pathTable {
		if err := viso.fsBuf.appendMarshaller(pathTableEntryMarshaller{
			pathTableEntry: e,
			order:          binary.LittleEndian,
		}, false); err != nil {
			return fmt.Errorf("failed to make little-endian path table item: %w", err)
		}
	}

	viso.fsBuf.padLastSector()

	// pathTableM
	for _, e := range viso.pathTable {
		if err := viso.fsBuf.appendMarshaller(pathTableEntryMarshaller{
			pathTableEntry: e,
			order:          binary.BigEndian,
		}, false); err != nil {
			return fmt.Errorf("failed to make little-endian path table item: %w", err)
		}
	}

	viso.fsBuf.padLastSector()

	// pathTableJolietL
	for _, e := range viso.pathTableJoliet {
		if err := viso.fsBuf.appendMarshaller(pathTableEntryMarshaller{
			pathTableEntry: e,
			order:          binary.LittleEndian,
		}, false); err != nil {
			return fmt.Errorf("failed to make little-endian joliet path table item: %w", err)
		}
	}

	viso.fsBuf.padLastSector()

	// pathTableJolietM
	for _, e := range viso.pathTableJoliet {
		if err := viso.fsBuf.appendMarshaller(pathTableEntryMarshaller{
			pathTableEntry: e,
			order:          binary.BigEndian,
		}, false); err != nil {
			return fmt.Errorf("failed to make big-endian joliet path table item: %w", err)
		}
	}

	viso.fsBuf.padLastSector()

	// iso directories
	for _, item := range viso.rootDir {
		for _, dirEntry := range item.dirEntry {
			if err := viso.fsBuf.appendMarshaller(dirEntry, false); err != nil {
				return fmt.Errorf("failed to write iso dir entry: %w", err)
			}
		}

		viso.fsBuf.padLastSector()
	}

	// joliet directories
	for _, item := range viso.rootDir {
		for _, dirEntry := range item.dirEntryJoliet {
			if err := viso.fsBuf.appendMarshaller(dirEntry, false); err != nil {
				return fmt.Errorf("failed to write joliet dir entry: %w", err)
			}
		}

		viso.fsBuf.padLastSector()
	}

	return nil
}

func (viso *VirtualISO) Read(p []byte) (int, error) {
	nw, err := viso.read(p, int64(viso.offset))

	viso.offset += sizeBytes(nw)
	return int(nw), err
}

func (viso *VirtualISO) ReadAt(p []byte, off int64) (int, error) {
	nw, err := viso.read(p, off)
	return int(nw), err
}

func (viso *VirtualISO) read(buf []byte, off int64) (int64, error) {
	if viso.isClosed {
		return 0, afero.ErrFileClosed
	}

	offset := sizeBytes(off)
	remain := sizeBytes(len(buf))
	read := int64(0)

	// at EOF
	if offset >= viso.totalSize || remain == 0 {
		return 0, io.EOF
	}

	// direct read from buffer
	if offset < viso.fsBuf.size() {
		end := offset + remain
		if end > viso.fsBuf.size() {
			end = viso.fsBuf.size()
		}

		written := copy(buf, viso.fsBuf[offset:end])
		buf = buf[written:]
		remain -= sizeBytes(written)
		read += int64(written)
		offset += sizeBytes(written)
	}

	if offset >= viso.totalSize || remain == 0 {
		return read, nil
	}

	// read files
	if offset < viso.padAreaStart {
		for _, fileItem := range viso.rootDir.filesToRead(remain, offset) {
			f, err := fileItem.openOnDemand(viso.fs)
			if err != nil {
				return read, fmt.Errorf("failed to open %s: %w", fileItem.path, err)
			}

			if offset < fileItem.rLBA.bytes() {
				return read, fmt.Errorf("file %s location (%d) greater than offset (%d)",
					fileItem.path, fileItem.rLBA.bytes(), offset)
			}

			if offset >= fileItem.rLBA.bytes()+fileItem.size.sectors().bytes() {
				return read, fmt.Errorf("offset (%d) greater than padded file %s location(%d)+size(%d)",
					offset, fileItem.path, fileItem.rLBA.bytes(), fileItem.size.sectors().bytes())
			}

			fileOffset := offset - fileItem.rLBA.bytes()

			if fileOffset < fileItem.size {
				_, err = f.Seek(int64(fileOffset), io.SeekStart)
				if err != nil {
					return read, fmt.Errorf("seek %s failed: %w", fileItem.path, err)
				}

				n, err := io.LimitReader(f, int64(remain)).Read(buf)
				if err != nil {
					return read, fmt.Errorf("read %s failed: %w", fileItem.path, err)
				}

				buf = buf[n:]
				remain -= sizeBytes(n)
				read += int64(n)
				offset += sizeBytes(n)
			}

			// fill remaining space with zeroes
			if fileItem.size%sectorSize > 0 && remain > 0 {
				toWrite := sectorSize - fileItem.size%sectorSize
				if remain < toWrite {
					remain = toWrite
				}

				n := copy(buf, emptySector[:toWrite])
				buf = buf[n:]
				remain -= sizeBytes(n)
				read += int64(n)
				offset += sizeBytes(n)
			}
		}
	}

	// read pad area
	if offset >= viso.padAreaStart && offset < viso.totalSize {
		toRead := viso.padAreaSize - (offset - viso.padAreaStart)
		if toRead == 0 {
			return read, nil
		}

		if toRead > remain {
			toRead = remain
		}

		for i := sizeBytes(0); i < remain; i++ {
			buf[i] = 0
		}

		offset += remain
		read += int64(remain)
		remain = 0
	}

	return read, nil
}

func (viso *VirtualISO) Seek(offset int64, whence int) (int64, error) {
	if viso.isClosed {
		return 0, afero.ErrFileClosed
	}

	switch whence {
	case io.SeekStart:
	case io.SeekCurrent:
		offset += int64(viso.offset)
	case io.SeekEnd:
		offset = int64(viso.totalSize) - offset - 1
	default:
		return 0, syscall.EINVAL
	}

	if offset < 0 || sizeBytes(offset) > viso.totalSize {
		return 0, afero.ErrOutOfRange
	}

	viso.offset = sizeBytes(offset)
	return offset, nil
}

func (viso *VirtualISO) Name() string {
	_, name := filepath.Split(viso.root)
	return name
}

func (viso *VirtualISO) Readdir(count int) ([]os.FileInfo, error) {
	dir, err := viso.fs.Open(viso.root)
	if err != nil {
		return nil, err
	}

	return dir.Readdir(count)
}

func (viso *VirtualISO) Readdirnames(n int) ([]string, error) {
	dir, err := viso.fs.Open(viso.root)
	if err != nil {
		return nil, err
	}

	return dir.Readdirnames(n)
}

func (viso *VirtualISO) Stat() (os.FileInfo, error) { return &virtualISOStat{viso}, nil }

func (*VirtualISO) Write([]byte) (int, error) { return 0, syscall.EPERM }

func (*VirtualISO) WriteAt([]byte, int64) (int, error) { return 0, syscall.EPERM }

func (*VirtualISO) Truncate(int64) error { return syscall.EPERM }

func (*VirtualISO) WriteString(string) (int, error) { return 0, syscall.EPERM }

func (*VirtualISO) Sync() error { return nil }

func (viso *VirtualISO) Close() error {
	if viso.isClosed {
		return nil
	}

	var errs []error
	viso.isClosed = true
	for i := range viso.rootDir {
		for j := range viso.rootDir[i].files {
			if err := viso.rootDir[i].files[j].closeOpened(); err != nil {
				errs = append(errs, fmt.Errorf("file %s close failed: %w", viso.rootDir[i].files[j].path, err))
			}
		}
	}

	return multierr.Combine(errs...)
}

type virtualISOStat struct {
	iso *VirtualISO
}

func (s virtualISOStat) Name() string { return s.iso.Name() }

func (s virtualISOStat) Size() int64 { return int64(s.iso.totalSize) }

func (s virtualISOStat) Mode() os.FileMode { return os.ModeIrregular }

func (s virtualISOStat) ModTime() time.Time { return s.iso.createdAt }

func (s virtualISOStat) IsDir() bool { return false }

func (s virtualISOStat) Sys() interface{} { return nil }
