// Package proto describes ps3netsrv protocol used by WebMAN MOD to interact with remote filesystem using network.
// Protocol is request-response. Any request starts from OpCode, and it's size always 16 bytes including it.
// For some messages Command can be followed by arbitrary data which length encoded in Command.Data.
package proto

//go:generate stringer -type OpCode

// OpCode is an operation code which read firstly.
type OpCode uint16

const (
	// CmdOpenFile requests to close the active ro file (if any) and open/stat a new one.
	CmdOpenFile OpCode = 0x1224 + iota

	// CmdReadFileCritical the active ro file.
	// Offsets and sizes in bytes. If file read fails, client is exited. Only read data is returned.
	CmdReadFileCritical

	// CmdReadCD2048Critical reads 2048 sectors in 2352 sectors iso.
	// Offsets and sizes in sectors. If file read fails, client is exited.
	CmdReadCD2048Critical

	// CmdReadFile closes the active wo file (if any) and opens+truncates or creates a new one.
	CmdReadFile

	// CmdCreateFile Closes the active wo file (if any) and opens+truncates or creates a new one.
	CmdCreateFile

	// CmdWriteFile writes to the active wo file.
	// After command, data is sent it returns number of bytes written to client, -1 on error.
	CmdWriteFile

	// CmdOpenDir Closes the active directory (if any) and opens a new one.
	CmdOpenDir

	// CmdReadDirEntry reads a directory entry and returns result.
	// If no more entries or an error happens, the directory is automatically closed.
	// '.' and '..' are automatically ignored.
	CmdReadDirEntry

	// CmdDeleteFile deletes a file.
	CmdDeleteFile

	// CmdMkdir creates a directory.
	CmdMkdir

	// CmdRmdir removes a directory.
	CmdRmdir

	// CmdReadDirEntryV2 Reads a directory entry (v2) and returns result.
	// If no more entries or an error happens, the directory is automatically closed.
	CmdReadDirEntryV2

	// CmdStatFile stats a file or directory.
	CmdStatFile

	// CmdGetDirSize gets a directory size.
	CmdGetDirSize

	// CmdReadDir get complete directory contents - 2013 by deank.
	CmdReadDir

	// CmdCustom0 reserved for custom commands.
	CmdCustom0 OpCode = 0x2412
)

// Command is a generic command
type Command struct {
	OpCode OpCode
	Data   [14]byte
}

// OpenDirCommand contains data for CmdOpenDir.
type OpenDirCommand struct {
	// DpLen is a length of path to read further.
	DpLen uint16
}

// OpenDirResult is a response for CmdOpenDir.
type OpenDirResult struct {
	// Result shows if reading dir was successful (0) or not (-1).
	Result int32
}

// ReadDirEntryCommand contains data for CmdReadDir, CmdReadDirEntry.
type ReadDirEntryCommand struct {
}

// ReadDirResult is a response for CmdReadDir.
type ReadDirResult struct {
	// Size is a count of following DirEntry items.
	Size int64
}

const MaxDirEntryName = 512

// DirEntry represents a single directory entry.
type DirEntry struct {
	FileSize    int64
	ModTime     uint64
	IsDirectory bool
	Name        [MaxDirEntryName]byte
}

// ReadDirEntryResult used by CmdReadDirEntry. Instead of using a fixed-size buffer for Name as in DirEntry,
// this struct contains FilenameLen so the receiver knows how many bytes to read for the name.
type ReadDirEntryResult struct {
	FileSize    int64
	FilenameLen uint16
	IsDirectory bool
}

// StatFileCommand contains data for CmdStatFile.
type StatFileCommand struct {
	// FpLen is a length of path to read further. Path is absolute here.
	FpLen uint16
}

// StatFileResult is a result of CmdStatFile with file info.
type StatFileResult struct {
	// FileSize contains file size for files, 0 for directories and -1 for error.
	FileSize    int64
	ModTime     uint64
	AccessTime  uint64
	ChangeTime  uint64
	IsDirectory bool
}

// OpenFileCommand contains data for CmdOpenFile.
type OpenFileCommand struct {
	// FpLen is a length of path to read further. Path is absolute here.
	FpLen uint16
}

// OpenFileResult is a result of CmdOpenFile with file info.
type OpenFileResult struct {
	// FileSize contains file size or -1 for error.
	FileSize int64
	ModTime  uint64
}

// ReadFileCommand contains data for CmdReadFile.
type ReadFileCommand struct {
	_ uint16 // pad

	// BytesToRead is a limit to read.
	BytesToRead uint32

	// Offset is offset from origin of the file (io.SeekStart).
	Offset uint64
}

// ReadFileResult is a result of CmdReadFile.
type ReadFileResult struct {
	// BytesRead is a number of bytes we read from file. Followed by read bytes if greater than zero.
	BytesRead int32
}

// ReadFileCriticalCommand contains data for CmdReadFileCritical.
type ReadFileCriticalCommand ReadFileCommand

// CreateFileCommand contains data for CmdCreateFile.
type CreateFileCommand struct {
	// FpLen is a length of path to read further.
	FpLen uint16
}

// CreateFileResult is a response for CmdCreateFile.
type CreateFileResult struct {
	// Result shows if creating file was successful (0) or not (-1).
	Result int32
}

// WriteFileCommand contains data for CmdWriteFile.
type WriteFileCommand struct {
	_ uint16 // pad

	// BytesToWrite is a number of bytes to write.
	BytesToWrite uint32
}

// WriteFileResult is a response for CmdWriteFile.
type WriteFileResult struct {
	// BytesWritten contains number of written bytes or -1 on error.
	BytesWritten int32
}

// DeleteFileCommand contains data for CmdDeleteFile.
type DeleteFileCommand struct {
	// FpLen is a length of path to read further. Path is absolute here.
	FpLen uint16
}

// DeleteFileResult is a response for CmdCreateFile.
type DeleteFileResult struct {
	// Result shows if creating file was successful (0) or not (-1).
	Result int32
}

// MkdirCommand contains data for CmdMkdir.
type MkdirCommand struct {
	// DpLen is a length of path to read further. Path is absolute here.
	DpLen uint16
}

// MkdirResult is a result of CmdMkdir with file info.
type MkdirResult struct {
	// Result shows if creating directory was successful (0) or not (-1).
	Result int32
}

// RmdirCommand contains data for CmdRmdir.
type RmdirCommand struct {
	// DpLen is a length of path to read further. Path is absolute here.
	DpLen uint16
}

// RmdirResult is a result of CmdRmdir with file info.
type RmdirResult struct {
	// Result shows if removing directory was successful (0) or not (-1).
	Result int32
}

// GetDirSizeCommand contains data for CmdGetDirSize.
type GetDirSizeCommand struct {
	// DpLen is a length of path to read further. Path is absolute here.
	DpLen uint16
}

// GetDirSizeResult is a result of CmdGetDirSize with file info.
type GetDirSizeResult struct {
	// Size contains total directory size or -1 on error.
	Size int64
}
