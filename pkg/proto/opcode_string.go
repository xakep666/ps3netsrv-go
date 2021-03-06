// Code generated by "stringer -type OpCode"; DO NOT EDIT.

package proto

import "strconv"

func _() {
	// An "invalid array index" compiler error signifies that the constant values have changed.
	// Re-run the stringer command to generate them again.
	var x [1]struct{}
	_ = x[CmdOpenFile-4644]
	_ = x[CmdReadFileCritical-4645]
	_ = x[CmdReadCD2048Critical-4646]
	_ = x[CmdReadFile-4647]
	_ = x[CmdCreateFile-4648]
	_ = x[CmdWriteFile-4649]
	_ = x[CmdOpenDir-4650]
	_ = x[CmdReadDirEntry-4651]
	_ = x[CmdDeleteFile-4652]
	_ = x[CmdMkdir-4653]
	_ = x[CmdRmdir-4654]
	_ = x[CmdReadDirEntryV2-4655]
	_ = x[CmdStatFile-4656]
	_ = x[CmdGetDirSize-4657]
	_ = x[CmdReadDir-4658]
	_ = x[CmdCustom0-9234]
}

const (
	_OpCode_name_0 = "CmdOpenFileCmdReadFileCriticalCmdReadCD2048CriticalCmdReadFileCmdCreateFileCmdWriteFileCmdOpenDirCmdReadDirEntryCmdDeleteFileCmdMkdirCmdRmdirCmdReadDirEntryV2CmdStatFileCmdGetDirSizeCmdReadDir"
	_OpCode_name_1 = "CmdCustom0"
)

var (
	_OpCode_index_0 = [...]uint8{0, 11, 30, 51, 62, 75, 87, 97, 112, 125, 133, 141, 158, 169, 182, 192}
)

func (i OpCode) String() string {
	switch {
	case 4644 <= i && i <= 4658:
		i -= 4644
		return _OpCode_name_0[_OpCode_index_0[i]:_OpCode_index_0[i+1]]
	case i == 9234:
		return _OpCode_name_1
	default:
		return "OpCode(" + strconv.FormatInt(int64(i), 10) + ")"
	}
}
