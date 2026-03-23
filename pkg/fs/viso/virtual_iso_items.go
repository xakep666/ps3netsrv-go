package viso

import (
	"cmp"
	"encoding/binary"
	"iter"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/xakep666/ps3netsrv-go/internal/handler"
	"github.com/xakep666/ps3netsrv-go/internal/iso9660"
	pkgfs "github.com/xakep666/ps3netsrv-go/pkg/fs"
)

const (
	parentDir  = ".."
	currentDir = "."
)

type directoryFile struct {
	path string // relative to base fs
	name string
	size iso9660.SizeBytes   // reported by Stat() or counted as sum for multipart files
	rLBA iso9660.SizeSectors // virtual address of file start, we use this to control boundaries during read

	modTime time.Time
}

type dirItem struct {
	path           string // relative to base fs
	name           string
	modTime        time.Time
	dirEntry       []iso9660.DirectoryEntry // must be alphabetically sort by name
	dirEntryJoliet []iso9660.DirectoryEntry // must be alphabetically sort by name
	files          []directoryFile
}

func (i dirItem) isDirectChild(of dirItem) bool {
	p, _ := filepath.Rel(i.path, of.path)
	return p == parentDir
}

func (i dirItem) findDirEntry(item *dirItem, joliet bool) *iso9660.DirectoryEntry {
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

func (l dirItemList) fixLBA(isoLBA, jolietLBA, filesLBA iso9660.SizeSectors) {
	for i := range l {
		fixDirLBA(l[i].dirEntry, isoLBA, filesLBA)
		fixDirLBA(l[i].dirEntryJoliet, jolietLBA, filesLBA)

		for j := range l[i].files {
			l[i].files[j].rLBA += filesLBA
		}
	}
}

func (l dirItemList) size(joliet bool) iso9660.SizeBytes {
	var ret iso9660.SizeBytes

	for _, item := range l {
		entries := item.dirEntry
		if joliet {
			entries = item.dirEntryJoliet
		}

		for _, entry := range entries {
			ret += entry.Size()
		}

		ret = ret.Sectors().Bytes() // directory entries of one directory aligned to sector
	}

	return ret
}

type fileItem struct {
	file handler.File // used for reduce opening count during reading

	path string              // relative to base fs
	size iso9660.SizeBytes   // reported by Stat() or counted as sum for multipart files
	rLBA iso9660.SizeSectors // virtual address of file start, we use this to control boundaries during read
}

func (i *fileItem) openOnDemand(fs pkgfs.SystemRoot) (handler.File, error) {
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

func (l dirItemList) collectFiles() []fileItem {
	var ret []fileItem

	for _, dir := range l {
		for _, file := range dir.files {
			ret = append(ret, fileItem{
				path: file.path,
				size: file.size,
				rLBA: file.rLBA,
			})
		}
	}

	// order by location to faster search during reading
	slices.SortFunc(ret, func(a, b fileItem) int {
		return cmp.Compare(a.rLBA, b.rLBA)
	})

	return ret
}

type filesList []fileItem

// filesToRead finds files that should be read by given toRead (limit) and offset (since disk start).
func (l filesList) filesToRead(toRead, offset iso9660.SizeBytes) iter.Seq[*fileItem] {
	return func(yield func(item *fileItem) bool) {
		// find a file where offset goes between start and end (aligned to sector)
		// we assume that files are not overlapping
		startFile, found := slices.BinarySearchFunc(l, offset.FloorSectors(),
			func(item fileItem, target iso9660.SizeSectors) int {
				switch {
				case target < item.rLBA: // target sector before file
					return 1
				case target >= item.rLBA+item.size.Sectors(): // target sector after file
					return -1
				default: // target sector inside file
					return 0
				}
			},
		)

		if !found {
			return
		}

		for i := startFile; i < len(l) && toRead > 0; i++ {
			file := &l[i]
			if !yield(file) {
				return
			}

			// file size may be actually not aligned to sector
			read := file.size.Sectors().Bytes() - (offset - file.rLBA.Bytes())
			toRead -= read
			offset += read
		}
	}
}

func fixDirLBA(entries []iso9660.DirectoryEntry, dirLBA, filesLBA iso9660.SizeSectors) {
	for i := range entries {
		if entries[i].FileFlags&iso9660.DirFlagDir > 0 {
			entries[i].ExtentLocation += dirLBA
		} else {
			entries[i].ExtentLocation += filesLBA
		}
	}
}

type pathTable []iso9660.PathTableEntry

func (t pathTable) fixLBA(dirLBA iso9660.SizeSectors) {
	for i := range t {
		t[i].DirLocation += dirLBA
	}
}

func (t pathTable) size() iso9660.SizeBytes {
	var ret iso9660.SizeBytes
	for _, e := range t {
		ret += e.Size()
	}

	return ret
}

//
// Special ps3 game disk sectors
//

type sectorRangeEntry struct {
	StartSector iso9660.SizeSectors
	EndSector   iso9660.SizeSectors
}

type discRangesSector []sectorRangeEntry

func (d discRangesSector) Encode(enc *iso9660.Encoder) {
	enc.AppendUint32(uint32(len(d)), binary.BigEndian)
	enc.AppendZeroes(4)

	for _, e := range d {
		enc.AppendUint32(uint32(e.StartSector), binary.BigEndian)
		enc.AppendUint32(uint32(e.EndSector), binary.BigEndian)
	}
}

type discInfoSector struct {
	ConsoleID string // max size 0x10
	ProductID string // max size 0x20
	Info      [0x1B0]byte
	Hash      [0x10]byte
}

func (d *discInfoSector) Encode(enc *iso9660.Encoder) {
	enc.AppendString(d.ConsoleID, 16, 0)
	enc.AppendString(d.ProductID, 32, ' ')
	enc.AppendZeroes(16)
	enc.AppendBytes(d.Info[:])
	enc.AppendBytes(d.Hash[:])
}

func makeIdentifier(name string, joliet bool) iso9660.StringD1 {
	if !joliet {
		name = strings.ToUpper(name)
	}

	return iso9660.MangleStringD1(name, joliet)
}
