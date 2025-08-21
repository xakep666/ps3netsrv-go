package proto

import (
	"encoding/binary"
	"fmt"
	"io"
	"strings"
)

type Reader struct {
	io.Reader

	cmd Command
}

// ReadCommand reads a request command.
func (r *Reader) ReadCommand() (OpCode, error) {
	err := binary.Read(r, binary.BigEndian, &r.cmd)
	if err != nil {
		return r.cmd.OpCode, fmt.Errorf("binary.Read failed: %w", err)
	}

	return r.cmd.OpCode, nil
}

// ReadOpenDir used for CmdOpenDir.
func (r *Reader) ReadOpenDir() (string, error) {
	var cmd OpenDirCommand

	err := r.readCommandTail(&cmd)
	if err != nil {
		return "", fmt.Errorf("readCommandTail failed: %w", err)
	}

	dirPath, err := r.readStringN(cmd.DpLen)
	if err != nil {
		return "", fmt.Errorf("readStringN failed: %w", err)
	}

	return dirPath, nil
}

func (r *Reader) ReadStatFile() (string, error) {
	var cmd StatFileCommand

	err := r.readCommandTail(&cmd)
	if err != nil {
		return "", fmt.Errorf("readCommandTail failed: %w", err)
	}

	filePath, err := r.readStringN(cmd.FpLen)
	if err != nil {
		return "", fmt.Errorf("readStringN failed: %w", err)
	}

	return filePath, nil
}

func (r *Reader) ReadOpenFile() (string, error) {
	var cmd StatFileCommand

	err := r.readCommandTail(&cmd)
	if err != nil {
		return "", fmt.Errorf("readCommandTail failed: %w", err)
	}

	filePath, err := r.readStringN(cmd.FpLen)
	if err != nil {
		return "", fmt.Errorf("readStringN failed: %w", err)
	}

	return filePath, nil
}

func (r *Reader) ReadReadFile() (bytesToRead uint32, offset uint64, err error) {
	var cmd ReadFileCommand

	err = r.readCommandTail(&cmd)
	if err != nil {
		return 0, 0, fmt.Errorf("readCommandTail failed: %w", err)
	}

	return cmd.BytesToRead, cmd.Offset, nil
}

func (r *Reader) ReadReadFileCritical() (bytesToRead uint32, offset uint64, err error) {
	var cmd ReadFileCriticalCommand

	err = r.readCommandTail(&cmd)
	if err != nil {
		return 0, 0, fmt.Errorf("readCommandTail failed: %w", err)
	}

	return cmd.BytesToRead, cmd.Offset, nil
}

func (r *Reader) ReadReadCD2048Critical() (sectorsToRead, startSector uint32, err error) {
	var cmd ReadCD2048CriticalCommand

	err = r.readCommandTail(&cmd)
	if err != nil {
		return 0, 0, fmt.Errorf("readCommandTail failed: %w", err)
	}

	return cmd.SectorsToRead, cmd.StartSector, nil
}

func (r *Reader) ReadCreateFile() (string, error) {
	var cmd CreateFileCommand

	err := r.readCommandTail(&cmd)
	if err != nil {
		return "", fmt.Errorf("readCommandTail failed: %w", err)
	}

	filePath, err := r.readStringN(cmd.FpLen)
	if err != nil {
		return "", fmt.Errorf("readStringN failed: %w", err)
	}

	return filePath, nil
}

func (r *Reader) ReadWriteFile() (io.Reader, error) {
	var cmd WriteFileCommand

	err := r.readCommandTail(&cmd)
	if err != nil {
		return nil, fmt.Errorf("readCommandTail failed: %w", err)
	}

	return io.LimitReader(r.Reader, int64(cmd.BytesToWrite)), nil
}

func (r *Reader) ReadDeleteFile() (string, error) {
	var cmd DeleteFileCommand

	err := r.readCommandTail(&cmd)
	if err != nil {
		return "", fmt.Errorf("readCommandTail failed: %w", err)
	}

	filePath, err := r.readStringN(cmd.FpLen)
	if err != nil {
		return "", fmt.Errorf("readStringN failed: %w", err)
	}

	return filePath, nil
}

func (r *Reader) ReadMkdir() (string, error) {
	var cmd MkdirCommand

	err := r.readCommandTail(&cmd)
	if err != nil {
		return "", fmt.Errorf("readCommandTail failed: %w", err)
	}

	filePath, err := r.readStringN(cmd.DpLen)
	if err != nil {
		return "", fmt.Errorf("readStringN failed: %w", err)
	}

	return filePath, nil
}

func (r *Reader) ReadRmdir() (string, error) {
	var cmd RmdirCommand

	err := r.readCommandTail(&cmd)
	if err != nil {
		return "", fmt.Errorf("readCommandTail failed: %w", err)
	}

	filePath, err := r.readStringN(cmd.DpLen)
	if err != nil {
		return "", fmt.Errorf("readStringN failed: %w", err)
	}

	return filePath, nil
}
func (r *Reader) ReadGetDirSize() (string, error) {
	var cmd GetDirSizeCommand

	err := r.readCommandTail(&cmd)
	if err != nil {
		return "", fmt.Errorf("readCommandTail failed: %w", err)
	}

	filePath, err := r.readStringN(cmd.DpLen)
	if err != nil {
		return "", fmt.Errorf("readStringN failed: %w", err)
	}

	return filePath, nil
}

// readCommandTail reads remaining data of command.
func (r *Reader) readCommandTail(tail interface{}) error {
	_, err := binary.Decode(r.cmd.Data[:], binary.BigEndian, tail)
	if err != nil {
		return fmt.Errorf("binary.Decode failed: %w", err)
	}

	return nil
}

func (r *Reader) readStringN(size uint16) (string, error) {
	var buf strings.Builder

	_, err := io.CopyN(&buf, r, int64(size))
	if err != nil {
		return "", fmt.Errorf("io.ReadFull failed: %w", err)
	}

	return buf.String(), nil
}
