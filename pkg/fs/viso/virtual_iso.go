package viso

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/xakep666/ps3netsrv-go/internal/iso9660"
	pkgfs "github.com/xakep666/ps3netsrv-go/pkg/fs"
)

const (
	ps3ModeVolumeName = "PS3VOLUME"
	consoleID         = "PlayStation3"

	multiExtentPartSize    iso9660.SizeBytes   = 0xFFFFF800
	maxPartSize            iso9660.SizeBytes   = 0xFFFFFFFF
	basePadSectors         iso9660.SizeSectors = 0x20
	volumeDescriptorsCount iso9660.SizeSectors = 3

	dotEntryIdentifier    = iso9660.StringD1(byte(0))
	dotDotEntryIdentifier = iso9660.StringD1(byte(1))
)

// ErrNotDirectory occurs when root path for VirtualISO is not a directory
var ErrNotDirectory = fmt.Errorf("not a directory")

var paramSFOPath = filepath.Join("PS3_GAME", "PARAM.SFO")

// VirtualISO is an on-the-fly generated .iso disk image.
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
	ctx       context.Context
	fs        *pkgfs.FS
	root      string
	ps3Mode   bool
	createdAt time.Time

	rootDir           dirItemList         // must be alphabetically sort by path
	filesSizeSectors  iso9660.SizeSectors // sum of file sizes in sectors
	pathTable         pathTable           // used in network ps3 mode, testing isn't easy because most desktop OSes ignore it
	pathTableJoliet   pathTable
	volumeDescriptors [volumeDescriptorsCount]iso9660.VolumeDescriptor
	volumeSizeSectors iso9660.SizeSectors

	isClosed     bool
	totalSize    iso9660.SizeBytes // whole disc size (with files)
	padAreaStart iso9660.SizeBytes
	padAreaSize  iso9660.SizeBytes
	fsBuf        iso9660.Encoder   // binary-encoded filesystem structures
	files        filesList         // ordered by location list of files to read from fs
	offset       iso9660.SizeBytes // used during Read and Seek
}

// NewVirtualISO creates a virtual iso object from given root optionally with some data for ps3 games.
// Root path must be without ..'s.
func NewVirtualISO(ctx context.Context, fsys *pkgfs.FS, root string, ps3Mode bool) (*VirtualISO, error) {
	rootStat, err := fsys.Stat(ctx, root)
	if err != nil {
		return nil, fmt.Errorf("stat failed: %w", err)
	}

	if !rootStat.IsDir() {
		return nil, ErrNotDirectory
	}

	ret := &VirtualISO{
		fs:        fsys,
		ctx:       context.WithoutCancel(ctx),
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

	viso.ctx = nil

	return nil
}

func (viso *VirtualISO) getTitleID() (string, error) {
	f, err := viso.fs.Open(viso.ctx, filepath.Join(viso.root, paramSFOPath))
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

	isoLBA := iso9660.SystemAreaSize.Sectors() +
		volumeDescriptorsCount + // volume descriptors are 1 per sector
		1 + // empty sector after descriptors
		viso.pathTable.size().Sectors()*2 + // little-endian + big-endian table
		viso.pathTableJoliet.size().Sectors()*2
	jolietLBA := isoLBA + viso.rootDir.size(false).Sectors()
	filesLBA := jolietLBA + viso.rootDir.size(true).Sectors()

	viso.calculateSizes(filesLBA)

	viso.makeVolumeDescriptors(volumeName)

	// we need to shift LBAs (sector numbers) of all structures
	// due to space before filesystem start and volume descriptors
	viso.rootDir.fixLBA(isoLBA, jolietLBA, filesLBA)
	viso.pathTable.fixLBA(isoLBA)
	viso.pathTableJoliet.fixLBA(jolietLBA)

	// finally collect flat ordered by rLBA files list
	viso.files = viso.rootDir.collectFiles()

	return nil
}

func (viso *VirtualISO) scanDirectory() error {
	// scan directory recursively using BFS to ensure that files will be located (rLBA) sequentially
	queue := []string{viso.root} // paths

	processDirectory := func(path string) error {
		dir, err := viso.fs.Open(viso.ctx, path)
		if err != nil {
			return fmt.Errorf("dir %s open failed: %w", path, err)
		}

		defer dir.Close()

		stat, err := dir.Stat()
		if err != nil {
			return fmt.Errorf("dir %s stat failed: %w", path, err)
		}

		if !stat.IsDir() {
			return fmt.Errorf("%s is not a dir", path)
		}

		dirItem := dirItem{
			path:    path,
			name:    stat.Name(),
			modTime: stat.ModTime(),
		}

		items, err := dir.ReadDir(-1)
		if err != nil {
			return fmt.Errorf("dir %s items get failed: %w", path, err)
		}

		slices.SortFunc(items, func(a, b fs.DirEntry) int {
			return strings.Compare(a.Name(), b.Name())
		})

		for _, item := range items {
			fullPath := filepath.Join(path, item.Name())
			itemStat, err := viso.fs.Stat(viso.ctx, fullPath)
			if err != nil {
				return fmt.Errorf("item %s stat failed: %w", fullPath, err)
			}

			if itemStat.IsDir() {
				queue = append(queue, fullPath)
				continue
			}

			fi := directoryFile{
				path:    fullPath,
				name:    itemStat.Name(),
				size:    iso9660.SizeBytes(itemStat.Size()),
				rLBA:    viso.filesSizeSectors,
				modTime: itemStat.ModTime(),
			}

			dirItem.files = append(dirItem.files, fi)
			viso.filesSizeSectors += fi.size.Sectors()
		}

		viso.rootDir = append(viso.rootDir, dirItem)
		return nil
	}

	for len(queue) > 0 {
		dir := queue[0]
		queue = queue[1:]

		if err := processDirectory(dir); err != nil {
			return err
		}
	}

	return nil
}

func (viso *VirtualISO) makeDirEntries(item *dirItem, joliet bool) error {
	var totalSizeBytes iso9660.SizeBytes

	// '.' entry
	dotEntry := iso9660.DirectoryEntry{
		FixedDirectoryEntry: iso9660.FixedDirectoryEntry{
			FileFlags:            iso9660.DirFlagDir,
			ExtentLocation:       viso.rootDir.size(joliet).Sectors(),
			RecordingDateTime:    iso9660.RecordingTimestamp(item.modTime.UTC()),
			VolumeSequenceNumber: 1,
			Identifier:           dotEntryIdentifier,
		},
		SystemUse: dirEntryPad,
	}

	// '..' entry
	dotDotEntry := iso9660.DirectoryEntry{
		FixedDirectoryEntry: iso9660.FixedDirectoryEntry{
			FileFlags:            iso9660.DirFlagDir,
			VolumeSequenceNumber: 1,
			Identifier:           dotDotEntryIdentifier,
		},
		SystemUse: dirEntryPad,
	}

	parent := viso.rootDir.parent(*item)
	if parent != nil {
		// link parent directory
		dotDotEntry.RecordingDateTime = iso9660.RecordingTimestamp(parent.modTime.UTC())
		dotDotEntry.ExtentLength = parent.dirEntry[0].ExtentLength
		dotDotEntry.ExtentLocation = parent.dirEntry[0].ExtentLocation
		if joliet {
			dotDotEntry.ExtentLength = parent.dirEntryJoliet[0].ExtentLength
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

		for i := range parts {
			entry := iso9660.DirectoryEntry{
				FixedDirectoryEntry: iso9660.FixedDirectoryEntry{
					Identifier:           makeIdentifier(fileItem.name+";1", joliet),
					RecordingDateTime:    iso9660.RecordingTimestamp(fileItem.modTime.UTC()),
					VolumeSequenceNumber: 1,
					ExtentLocation:       lba,
				},
				SystemUse: dirEntryPad,
			}

			switch {
			case parts == 1:
				entry.ExtentLength = fileItem.size
			case i == parts-1:
				entry.ExtentLength = fileItem.size - iso9660.SizeBytes(i)*multiExtentPartSize
			default:
				entry.ExtentLength = multiExtentPartSize
				entry.FileFlags = iso9660.DirFlagMultiExtent
				lba += multiExtentPartSize.Sectors()
			}

			if joliet {
				item.dirEntryJoliet = append(item.dirEntryJoliet, entry)
			} else {
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

		entry := iso9660.DirectoryEntry{
			FixedDirectoryEntry: iso9660.FixedDirectoryEntry{
				FileFlags:            iso9660.DirFlagDir,
				VolumeSequenceNumber: 1,
				RecordingDateTime:    iso9660.RecordingTimestamp(dirItem.modTime.UTC()),
				Identifier:           makeIdentifier(dirItem.name, joliet),
			},
			SystemUse: dirEntryPad,
		}

		if joliet {
			item.dirEntryJoliet = append(item.dirEntryJoliet, entry)
		} else {
			item.dirEntry = append(item.dirEntry, entry)
		}

		totalSizeBytes += entry.Size()
	}

	// total size must be integer number of sectors so ceil it if needed
	totalSizeBytes = totalSizeBytes.Sectors().Bytes()

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

	for i := 0; i < len(viso.rootDir) && i < iso9660.PathTableItemsLimit; i++ {
		pathTableEntry := iso9660.PathTableEntry{
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

func (viso *VirtualISO) calculateSizes(filesLBA iso9660.SizeSectors) {
	// in sectors
	volumeSize := filesLBA + viso.filesSizeSectors
	padSectors := basePadSectors

	if extraPad := volumeSize % basePadSectors; extraPad > 0 {
		padSectors += basePadSectors - extraPad
	}

	volumeSizeWithPad := volumeSize + padSectors

	viso.volumeSizeSectors = volumeSizeWithPad

	// in bytes
	viso.totalSize = volumeSizeWithPad.Bytes()
	viso.padAreaStart = volumeSize.Bytes()
	viso.padAreaSize = padSectors.Bytes()
}

func (viso *VirtualISO) makeVolumeDescriptors(volumeName string) {
	descriptorsLBA := iso9660.SystemAreaSize.Sectors()
	pathTableLLBA := descriptorsLBA + volumeDescriptorsCount + 1                       // little-endian iso path table
	pathTableMLBA := pathTableLLBA + viso.pathTable.size().Sectors()                   // big-endian iso path table
	pathTableJolietLLBA := pathTableMLBA + viso.pathTable.size().Sectors()             // little-endian joliet path table
	pathTableJolietMLBA := pathTableJolietLLBA + viso.pathTableJoliet.size().Sectors() // big-endian joliet path table

	now := time.Now()

	pvd := iso9660.VolumeDescriptor{
		Header: iso9660.VolumeDescriptorHeader{
			Type:       iso9660.VolumeTypePrimary,
			Identifier: iso9660.StandardIdentifierBytes,
			Version:    1,
		},
		Primary: &iso9660.PrimaryVolumeDescriptorBody{
			StringPadding:             ' ',
			VolumeIdentifier:          iso9660.MangleStringD(volumeName, false),
			VolumeSpaceSize:           viso.volumeSizeSectors,
			VolumeSetSize:             1,
			VolumeSequenceNumber:      1,
			LogicalBlockSize:          iso9660.SectorSize,
			PathTableSize:             viso.pathTable.size(),
			TypeLPathTableLoc:         pathTableLLBA,
			TypeMPathTableLoc:         pathTableMLBA,
			VolumeSetIdentifier:       iso9660.MangleStringD(volumeName, false),
			VolumeCreationDateAndTime: iso9660.VolumeDescriptorTimestampFromTime(now),
			FileStructureVersion:      1,
			RootDirectoryEntry:        &viso.rootDir[0].dirEntry[0].FixedDirectoryEntry,
		},
	}

	pvdJoliet := iso9660.VolumeDescriptor{
		Header: iso9660.VolumeDescriptorHeader{
			Type:       iso9660.VolumeTypeSupplementary,
			Identifier: iso9660.StandardIdentifierBytes,
			Version:    1,
		},
		Primary: &iso9660.PrimaryVolumeDescriptorBody{
			StringPadding:             0,
			VolumeIdentifier:          iso9660.MangleStringD(volumeName, true),
			VolumeSpaceSize:           viso.volumeSizeSectors,
			EscapeSequences:           "%/@",
			VolumeSetSize:             1,
			VolumeSequenceNumber:      1,
			LogicalBlockSize:          iso9660.SectorSize,
			PathTableSize:             viso.pathTableJoliet.size(),
			TypeLPathTableLoc:         pathTableJolietLLBA,
			TypeMPathTableLoc:         pathTableJolietMLBA,
			VolumeSetIdentifier:       iso9660.MangleStringD(volumeName, true),
			VolumeCreationDateAndTime: iso9660.VolumeDescriptorTimestampFromTime(now),
			FileStructureVersion:      1,
			RootDirectoryEntry:        &viso.rootDir[0].dirEntryJoliet[0].FixedDirectoryEntry,
		},
	}

	terminator := iso9660.VolumeDescriptor{
		Header: iso9660.VolumeDescriptorHeader{
			Type:       iso9660.VolumeTypeTerminator,
			Identifier: iso9660.StandardIdentifierBytes,
		},
	}

	viso.volumeDescriptors = [volumeDescriptorsCount]iso9660.VolumeDescriptor{pvd, pvdJoliet, terminator}
}

func (viso *VirtualISO) writeFSStructures(gameCode string) error {
	// ps3-game specific sectors
	emptySectorsNeeded := iso9660.SystemAreaSize.Sectors()
	if viso.ps3Mode {
		emptySectorsNeeded -= 2

		viso.fsBuf.AppendEncodable(discRangesSector{{
			StartSector: 0,
			EndSector:   viso.volumeSizeSectors - 1,
		}}, iso9660.SectorSize)

		infoSector := discInfoSector{
			ConsoleID: consoleID,
			ProductID: gameCode[:4] + "-" + gameCode[4:], // i.e. BCES-00104
		}

		_, err := io.ReadFull(rand.Reader, infoSector.Info[:])
		if err != nil {
			return fmt.Errorf("failed to generate info in info sector: %w", err)
		}

		_, err = io.ReadFull(rand.Reader, infoSector.Hash[:])
		if err != nil {
			return fmt.Errorf("failed to generate hash in info sector: %w", err)
		}

		viso.fsBuf.AppendEncodable(&infoSector, iso9660.SectorSize)
	}

	// empty sectors
	viso.fsBuf.AppendZeroSectors(emptySectorsNeeded)

	// volume descriptors
	for _, vd := range viso.volumeDescriptors {
		viso.fsBuf.AppendEncodable(vd, iso9660.SectorSize)
	}

	// empty sector
	viso.fsBuf.AppendZeroSectors(1)

	// pathTableL
	for _, e := range viso.pathTable {
		e.EncodeOrdered(&viso.fsBuf, binary.LittleEndian)
	}

	viso.fsBuf.PadLastSector()

	// pathTableM
	for _, e := range viso.pathTable {
		e.EncodeOrdered(&viso.fsBuf, binary.BigEndian)
	}

	viso.fsBuf.PadLastSector()

	// pathTableJolietL
	for _, e := range viso.pathTableJoliet {
		e.EncodeOrdered(&viso.fsBuf, binary.LittleEndian)
	}

	viso.fsBuf.PadLastSector()

	// pathTableJolietM
	for _, e := range viso.pathTableJoliet {
		e.EncodeOrdered(&viso.fsBuf, binary.BigEndian)
	}

	viso.fsBuf.PadLastSector()

	// iso directories
	for _, item := range viso.rootDir {
		for _, dirEntry := range item.dirEntry {
			dirEntry.Encode(&viso.fsBuf)
		}

		viso.fsBuf.PadLastSector()
	}

	// joliet directories
	for _, item := range viso.rootDir {
		for _, dirEntry := range item.dirEntryJoliet {
			dirEntry.Encode(&viso.fsBuf)
		}

		viso.fsBuf.PadLastSector()
	}

	return nil
}

func (viso *VirtualISO) Read(buf []byte) (read int, err error) {
	if viso.isClosed {
		return 0, fs.ErrClosed
	}

	offset := iso9660.SizeBytes(viso.offset)
	remain := iso9660.SizeBytes(len(buf))

	defer func() {
		if err == nil && read > 0 {
			viso.offset += iso9660.SizeBytes(read)
		}
	}()

	// at EOF
	if offset >= viso.totalSize || remain == 0 {
		return 0, io.EOF
	}

	// direct read from buffer
	if offset < viso.fsBuf.Size() {
		end := min(offset+remain, viso.fsBuf.Size())
		written := copy(buf, viso.fsBuf[offset:end])
		buf = buf[written:]
		remain -= iso9660.SizeBytes(written)
		read += written
		offset += iso9660.SizeBytes(written)
	}

	if offset >= viso.totalSize || remain == 0 {
		return read, nil
	}

	// read files
	if offset < viso.padAreaStart {
		for fileItem := range viso.files.filesToRead(remain, offset) {
			if offset < fileItem.rLBA.Bytes() {
				return read, fmt.Errorf("file %s location (%d) greater than offset (%d)",
					fileItem.path, fileItem.rLBA.Bytes(), offset)
			}

			if offset >= fileItem.rLBA.Bytes()+fileItem.size.Sectors().Bytes() {
				return read, fmt.Errorf("offset (%d) greater than padded file %s location(%d)+size(%d)",
					offset, fileItem.path, fileItem.rLBA.Bytes(), fileItem.size.Sectors().Bytes())
			}

			f, err := fileItem.openOnDemand(viso.ctx, viso.fs)
			if err != nil {
				return read, fmt.Errorf("failed to open %s: %w", fileItem.path, err)
			}

			fileOffset := offset - fileItem.rLBA.Bytes()

			if fileOffset < fileItem.size {
				_, err = f.Seek(int64(fileOffset), io.SeekStart)
				if err != nil {
					return read, fmt.Errorf("seek %s failed: %w", fileItem.path, err)
				}

				// ReadFull because sometimes one Read may be not enough.
				// We want to read minimum between overall remaining amount of bytes and amount of bytes needed to reach EOF.
				n, err := io.ReadFull(f, buf[:min(remain, fileItem.size-fileOffset)])
				if err != nil {
					return read, fmt.Errorf("read %s failed: %w", fileItem.path, err)
				}

				buf = buf[n:]
				remain -= iso9660.SizeBytes(n)
				read += n
				offset += iso9660.SizeBytes(n)
			}

			// fill remaining space with zeroes
			if fileItem.size%iso9660.SectorSize > 0 && remain > 0 {
				toWrite := iso9660.SectorSize - fileItem.size%iso9660.SectorSize
				if remain < toWrite {
					remain = toWrite
				}

				for i := range toWrite {
					buf[i] = 0
				}
				buf = buf[toWrite:]
				remain -= toWrite
				read += int(toWrite)
				offset += toWrite
			}
		}
	}

	// read pad area
	if offset >= viso.padAreaStart && offset < viso.totalSize {
		toRead := viso.padAreaSize - (offset - viso.padAreaStart)
		if toRead == 0 {
			return read, nil
		}

		for i := range remain {
			buf[i] = 0
		}

		offset += remain
		read += int(remain)
		remain = 0
	}

	return read, nil
}

func (viso *VirtualISO) Seek(offset int64, whence int) (int64, error) {
	if viso.isClosed {
		return 0, fs.ErrClosed
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

	if offset < 0 || iso9660.SizeBytes(offset) > viso.totalSize {
		return 0, syscall.EINVAL
	}

	viso.offset = iso9660.SizeBytes(offset)
	return offset, nil
}

func (viso *VirtualISO) Name() string {
	return viso.root
}

func (viso *VirtualISO) ReadDir(count int) ([]fs.DirEntry, error) {
	return nil, errors.ErrUnsupported
}

func (viso *VirtualISO) Stat() (fs.FileInfo, error) {
	return &virtualISOStat{
		name:      filepath.Base(viso.root),
		totalSize: int64(viso.totalSize),
		modTime:   viso.createdAt,
	}, nil
}

func (viso *VirtualISO) Close() error {
	if viso.isClosed {
		return nil
	}

	var errs []error
	viso.isClosed = true
	for i := range viso.files {
		if err := viso.files[i].closeOpened(); err != nil {
			errs = append(errs, fmt.Errorf("file %s close failed: %w", viso.files[i].path, err))
		}
	}

	return errors.Join(errs...)
}

type virtualISOStat struct {
	name      string
	totalSize int64
	modTime   time.Time
}

func (s virtualISOStat) Name() string { return s.name }

func (s virtualISOStat) Size() int64 { return int64(s.totalSize) }

func (s virtualISOStat) Mode() fs.FileMode { return fs.ModeIrregular }

func (s virtualISOStat) ModTime() time.Time { return s.modTime }

func (s virtualISOStat) IsDir() bool { return false }

func (s virtualISOStat) Sys() any { return nil }
