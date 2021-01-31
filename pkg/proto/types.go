// Package proto describes ps3netsrv protocol used by WebMAN MOD to interact with remote filesystem using network.
// Protocol is request-response. Any request starts from OpCode and it's size always 16 bytes including it.
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

	// CmdReadCD2048Critical reads 2048 sectors in a 2352 sectors iso.
	// Offsets and sizes in sectors. If file read fails, client is exited.
	CmdReadCD2048Critical

	// Closes the active wo file (if any) and opens+truncates or creates a new one.
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
