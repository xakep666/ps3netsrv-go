package fs

import (
	"encoding/binary"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/afero"
)

const (
	parentDir  = ".."
	currentDir = "."
)

var emptySector [sectorSize]byte

type fileItem struct {
	path string // relative to base fs
	name string
	size sizeBytes   // reported by Stat() or counted as sum for multipart files
	rLBA sizeSectors // virtual address of file start, we use this to control boundaries during read

	modTime time.Time

	file afero.File // used for reduce opening count during reading
}

func (i *fileItem) openOnDemand(fs afero.Fs) (afero.File, error) {
	if i.file != nil {
		return i.file, nil
	}

	f, err := fs.Open(i.path)
	if err != nil {
		return nil, err
	}

	i.file = f
	return f, nil
}

func (i *fileItem) closeOpened() error {
	if i.file != nil {
		err := i.file.Close()
		i.file = nil
		return err
	}

	return nil
}

type dirItem struct {
	path           string // relative to base fs
	name           string
	modTime        time.Time
	dirEntry       []directoryEntry // must be alphabetically sort by name
	dirEntryJoliet []directoryEntry // must be alphabetically sort by name
	files          []fileItem
}

func (i dirItem) isDirectChild(of dirItem) bool {
	p, _ := filepath.Rel(i.path, of.path)
	return p == parentDir
}

func (i dirItem) findDirEntry(item *dirItem, joliet bool) *directoryEntry {
	entries := i.dirEntry
	if joliet {
		entries = i.dirEntryJoliet
	}

	identifier := makeIdentifier(item.name, joliet)

	for i := range entries {
		if entries[i].Identifier == identifier {
			return &entries[i]
		}
	}

	return nil
}

type dirItemList []dirItem

func (l dirItemList) parent(item dirItem) *dirItem {
	for i := range l {
		if item.isDirectChild(l[i]) {
			return &l[i]
		}
	}

	return nil
}

func (l dirItemList) parentIdx(item dirItem) int {
	for i := range l {
		if item.isDirectChild(l[i]) {
			return i
		}
	}

	return -1
}

func (l dirItemList) fixLBA(isoLBA, jolietLBA, filesLBA sizeSectors) {
	for i := range l {
		fixDirLBA(l[i].dirEntry, isoLBA, filesLBA)
		fixDirLBA(l[i].dirEntryJoliet, jolietLBA, filesLBA)

		for j := range l[i].files {
			l[i].files[j].rLBA += filesLBA
		}
	}
}

func (l dirItemList) size(joliet bool) sizeBytes {
	var ret sizeBytes

	for _, item := range l {
		entries := item.dirEntry
		if joliet {
			entries = item.dirEntryJoliet
		}

		for _, entry := range entries {
			ret += entry.size()
		}

		ret = ret.sectors().bytes() // directory entries of one directory aligned to sector
	}

	return ret
}

// filesToRead finds files that should be read by given toRead (limit) and offset (since disk start).
func (l dirItemList) filesToRead(toRead, offset sizeBytes) []*fileItem {
	var (
		ret                 []*fileItem
		startDir, startFile int
	)

	// find a file where offset goes between start and end (aligned to sector)
	offsetSectors := offset.sectors()
firstFileLoop:
	for i := range l {
		for j := range l[i].files {
			file := &l[i].files[j]

			if offsetSectors >= file.rLBA && offsetSectors < (file.rLBA+file.size.sectors()) {
				ret = append(ret, file)
				startDir = i
				startFile = j
				// we already "read" some bytes of file
				read := file.size.sectors().bytes() - (offset - file.rLBA.bytes())
				toRead -= read
				offset += read
				break firstFileLoop
			}
		}
	}

	if len(ret) == 0 {
		return ret // no files found so nothing to do further
	}

loop:
	for i := startDir; i < len(l); i++ {
		startDirFile := 0
		if i == startDir {
			startDirFile = startFile + 1
		}

		for j := startDirFile; j < len(l[i].files); j++ {
			if toRead <= 0 {
				break loop
			}

			file := &l[i].files[j]
			ret = append(ret, file)
			read := file.size.sectors().bytes() // aligned to sector
			toRead -= read
			offset += read
		}
	}

	return ret
}

func fixDirLBA(entries []directoryEntry, dirLBA, filesLBA sizeSectors) {
	for i := 0; i < len(entries); i++ {
		if entries[i].FileFlags&dirFlagDir > 0 {
			entries[i].ExtentLocation += dirLBA
		} else {
			entries[i].ExtentLocation += filesLBA
		}
	}
}

type pathTable []pathTableEntry

func (t pathTable) fixLBA(dirLBA sizeSectors) {
	for i := 0; i < len(t); i++ {
		t[i].DirLocation += dirLBA
	}
}

func (t pathTable) size() sizeBytes {
	var ret sizeBytes
	for _, e := range t {
		ret += e.size()
	}

	return ret
}

//
// Special ps3 game disk sectors
//

type sectorRangeEntry struct {
	StartSector sizeSectors
	EndSector   sizeSectors
}

type discRangesSector []sectorRangeEntry

func (d discRangesSector) encode(enc *iso9660encoder) {
	enc.appendUint32(uint32(len(d)), binary.BigEndian)
	enc.appendZeroes(4)

	for _, e := range d {
		enc.appendUint32(uint32(e.StartSector), binary.BigEndian)
		enc.appendUint32(uint32(e.EndSector), binary.BigEndian)
	}
}

type discInfoSector struct {
	ConsoleID string // max size 0x10
	ProductID string // max size 0x20
	Info      [0x1B0]byte
	Hash      [0x10]byte
}

func (d *discInfoSector) encode(enc *iso9660encoder) {
	enc.appendString(d.ConsoleID, 16, ' ')
	enc.appendString(d.ProductID, 32, ' ')
	enc.appendZeroes(16)
	enc.appendBytes(d.Info[:])
	enc.appendBytes(d.Hash[:])
}

func makeIdentifier(name string, joliet bool) stringD1 {
	if !joliet {
		name = strings.ToUpper(name)
	}

	return mangleStrD1(name, joliet)
}
