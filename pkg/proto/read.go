package proto

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"path/filepath"
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

	return filepath.FromSlash(dirPath), nil
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

	return filepath.FromSlash(filePath), nil
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

	return filepath.FromSlash(filePath), nil
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

// readCommandTail reads remaining data of command.
func (r *Reader) readCommandTail(tail interface{}) error {
	err := binary.Read(bytes.NewReader(r.cmd.Data[:]), binary.BigEndian, tail)
	if err != nil {
		return fmt.Errorf("binary.Read failed: %w", err)
	}

	return nil
}

func (r *Reader) readStringN(size uint16) (string, error) {
	buf := make([]byte, size)

	_, err := io.ReadFull(r, buf)
	if err != nil {
		return "", fmt.Errorf("io.ReadFull failed: %w", err)
	}

	return string(buf), nil
}
