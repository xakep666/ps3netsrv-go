package client

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/xakep666/ps3netsrv-go/internal/ioutil"
	"github.com/xakep666/ps3netsrv-go/pkg/proto"
)

type Client struct {
	conn     net.Conn
	copier   *ioutil.Copier
	isClosed *atomic.Bool
}

func NewClient(ctx context.Context, copier *ioutil.Copier, addr string) (*Client, error) {
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}

	return &Client{
		conn:     conn,
		copier:   copier,
		isClosed: new(atomic.Bool),
	}, nil
}

func NewClientFromConn(copier *ioutil.Copier, conn net.Conn) (*Client, error) {
	return &Client{
		conn:     conn,
		copier:   copier,
		isClosed: new(atomic.Bool),
	}, nil
}

func (c *Client) OpenFile(ctx context.Context, path string) (proto.OpenFileResult, error) {
	stop, err := c.applyContext(ctx)
	if err != nil {
		return proto.OpenFileResult{}, err
	}
	defer stop()

	err = c.writeRequest(ctx,
		proto.CmdOpenFile,
		proto.OpenFileCommand{FpLen: uint16(len(path))},
		path,
	)
	if err != nil {
		return proto.OpenFileResult{}, fmt.Errorf("make request: %w", err)
	}

	var resp proto.OpenFileResult
	if err = c.readResponse(ctx, &resp); err != nil {
		return proto.OpenFileResult{}, fmt.Errorf("read response: %w", err)
	}

	if resp.FileSize < 0 {
		return proto.OpenFileResult{}, fmt.Errorf("received unsuccessful response")
	}

	return resp, nil
}

func (c *Client) StatFile(ctx context.Context, path string) (*proto.StatFileResult, error) {
	stop, err := c.applyContext(ctx)
	if err != nil {
		return nil, err
	}
	defer stop()

	err = c.writeRequest(ctx,
		proto.CmdStatFile,
		proto.StatFileCommand{FpLen: uint16(len(path))},
		path,
	)
	if err != nil {
		return nil, fmt.Errorf("make request: %w", err)
	}

	var resp proto.StatFileResult
	if err = c.readResponse(ctx, &resp); err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.FileSize < 0 {
		return nil, fmt.Errorf("received unsuccessful response")
	}

	return &resp, nil
}

func (c *Client) ReadFileCritical(ctx context.Context, bytesToRead uint32, offset uint64, target io.Writer) error {
	stop, err := c.applyContext(ctx)
	if err != nil {
		return err
	}
	defer stop()

	err = c.writeRequest(ctx,
		proto.CmdReadFileCritical,
		proto.ReadFileCriticalCommand{BytesToRead: bytesToRead, Offset: offset},
		nil,
	)
	if err != nil {
		return fmt.Errorf("make request: %w", err)
	}

	n, err := c.copier.CopyN(target, c.conn, int64(bytesToRead))
	if err != nil {
		_ = c.Close()
		return fmt.Errorf("copy: %w", ioError(ctx, err))
	}
	if n != int64(bytesToRead) {
		_ = c.Close()
		return fmt.Errorf("expected %d bytes, got %d", bytesToRead, n)
	}

	return nil
}

func (c *Client) ReadFile(ctx context.Context, bytesToRead uint32, offset uint64, target io.Writer) error {
	stop, err := c.applyContext(ctx)
	if err != nil {
		return err
	}
	defer stop()

	err = c.writeRequest(ctx,
		proto.CmdReadFile,
		proto.ReadFileCommand{BytesToRead: bytesToRead, Offset: offset},
		nil,
	)
	if err != nil {
		return fmt.Errorf("make request: %w", err)
	}

	var resp proto.ReadFileResult
	if err = c.readResponse(ctx, &resp); err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	if resp.BytesRead < 0 {
		return fmt.Errorf("received unsuccessful response")
	}

	_, err = c.copier.CopyN(target, c.conn, int64(resp.BytesRead))
	if err != nil {
		return fmt.Errorf("copy: %w", err)
	}

	return nil
}

func (c *Client) ReadCD2048Critical(ctx context.Context, sectorsToRead, startSector uint32, target io.Writer) error {
	stop, err := c.applyContext(ctx)
	if err != nil {
		return err
	}
	defer stop()

	err = c.writeRequest(ctx,
		proto.CmdReadCD2048Critical,
		proto.ReadCD2048CriticalCommand{SectorsToRead: sectorsToRead, StartSector: startSector},
		nil,
	)
	if err != nil {
		return fmt.Errorf("make request: %w", err)
	}

	bytesToRead := int64(sectorsToRead) * 2048
	n, err := c.copier.CopyN(target, c.conn, bytesToRead)
	if err != nil {
		_ = c.Close()
		return fmt.Errorf("copy: %w", ioError(ctx, err))
	}
	if n != bytesToRead {
		_ = c.Close()
		return fmt.Errorf("expected %d bytes, got %d", bytesToRead, n)
	}

	return nil
}

func (c *Client) OpenDir(ctx context.Context, path string) error {
	stop, err := c.applyContext(ctx)
	if err != nil {
		return err
	}
	defer stop()

	err = c.writeRequest(ctx,
		proto.CmdOpenDir,
		proto.OpenDirCommand{DpLen: uint16(len(path))},
		path,
	)
	if err != nil {
		return fmt.Errorf("make request: %w", err)
	}

	var resp proto.OpenDirResult
	if err = c.readResponse(ctx, &resp); err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.Result < 0 {
		return fmt.Errorf("received unsuccessful response")
	}

	return nil
}

type DirEntry struct {
	proto.ReadDirEntryResult
	Name string
}

func (c *Client) ReadDirEntry(ctx context.Context) (DirEntry, error) {
	stop, err := c.applyContext(ctx)
	if err != nil {
		return DirEntry{}, err
	}
	defer stop()

	err = c.writeRequest(ctx,
		proto.CmdReadDirEntry,
		proto.ReadDirEntryCommand{},
		nil,
	)
	if err != nil {
		return DirEntry{}, fmt.Errorf("make request: %w", err)
	}

	var resp proto.ReadDirEntryResult
	if err = c.readResponse(ctx, &resp); err != nil {
		return DirEntry{}, fmt.Errorf("read response: %w", err)
	}
	if resp.FileSize < 0 {
		return DirEntry{}, io.EOF
	}

	var sb strings.Builder
	sb.Grow(int(resp.FilenameLen))
	_, err = c.copier.CopyN(&sb, c.conn, int64(resp.FilenameLen))
	if err != nil {
		return DirEntry{}, fmt.Errorf("copy filename: %w", ioError(ctx, err))
	}

	return DirEntry{
		ReadDirEntryResult: resp,
		Name:               sb.String(),
	}, nil
}

type DirEntryV2 struct {
	proto.ReadDirEntryV2Result
	Name string
}

func (c *Client) ReadDirEntryV2(ctx context.Context) (DirEntryV2, error) {
	stop, err := c.applyContext(ctx)
	if err != nil {
		return DirEntryV2{}, err
	}
	defer stop()

	err = c.writeRequest(ctx,
		proto.CmdReadDirEntryV2,
		proto.ReadDirEntryCommand{},
		nil,
	)
	if err != nil {
		return DirEntryV2{}, fmt.Errorf("make request: %w", err)
	}

	var resp proto.ReadDirEntryV2Result
	if err = c.readResponse(ctx, &resp); err != nil {
		return DirEntryV2{}, fmt.Errorf("read response: %w", err)
	}
	if resp.FileSize < 0 {
		return DirEntryV2{}, io.EOF
	}

	var sb strings.Builder
	sb.Grow(int(resp.FilenameLen))
	_, err = c.copier.CopyN(&sb, c.conn, int64(resp.FilenameLen))
	if err != nil {
		return DirEntryV2{}, fmt.Errorf("copy filename: %w", ioError(ctx, err))
	}

	return DirEntryV2{
		ReadDirEntryV2Result: resp,
		Name:                 sb.String(),
	}, nil
}

func (c *Client) ReadDir(ctx context.Context) ([]proto.DirEntry, error) {
	stop, err := c.applyContext(ctx)
	if err != nil {
		return nil, err
	}
	defer stop()

	err = c.writeRequest(ctx,
		proto.CmdReadDir,
		proto.ReadDirEntryCommand{},
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("make request: %w", err)
	}

	var resp proto.ReadDirResult
	if err = c.readResponse(ctx, &resp); err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.Size < 0 {
		return nil, fmt.Errorf("received unsuccessful response")
	}

	ret := make([]proto.DirEntry, resp.Size)
	for i := range resp.Size {
		if err = c.readResponse(ctx, &ret[i]); err != nil {
			return nil, fmt.Errorf("read dir entry: %w", err)
		}
	}

	return ret, nil
}

func (c *Client) CreateFile(ctx context.Context, path string) error {
	stop, err := c.applyContext(ctx)
	if err != nil {
		return err
	}
	defer stop()

	err = c.writeRequest(ctx,
		proto.CmdCreateFile,
		proto.CreateFileCommand{FpLen: uint16(len(path))},
		path,
	)
	if err != nil {
		return fmt.Errorf("make request: %w", err)
	}

	var resp proto.CreateFileResult
	if err = c.readResponse(ctx, &resp); err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.Result < 0 {
		return fmt.Errorf("received unsuccessful response")
	}

	return nil
}

func (c *Client) WriteFile(ctx context.Context, chunkSize uint32, from io.Reader) error {
	stop, err := c.applyContext(ctx)
	if err != nil {
		return err
	}
	defer stop()

	var buf bytes.Buffer
	buf.Grow(int(chunkSize))

	for {
		n, err := buf.ReadFrom(io.LimitReader(from, int64(chunkSize)))
		if err != nil {
			return fmt.Errorf("read from reader: %w", err)
		}
		if n == 0 {
			return nil
		}

		err = c.writeRequest(ctx,
			proto.CmdWriteFile,
			proto.WriteFileCommand{BytesToWrite: uint32(n)},
			&buf,
		)
		if err != nil {
			return fmt.Errorf("make request: %w", err)
		}

		var resp proto.CreateFileResult
		if err = c.readResponse(ctx, &resp); err != nil {
			return fmt.Errorf("read response: %w", err)
		}

		if resp.Result < 0 {
			return fmt.Errorf("received unsuccessful response")
		}
	}
}

func (c *Client) DeleteFile(ctx context.Context, path string) error {
	stop, err := c.applyContext(ctx)
	if err != nil {
		return err
	}
	defer stop()

	err = c.writeRequest(ctx,
		proto.CmdDeleteFile,
		proto.DeleteFileCommand{FpLen: uint16(len(path))},
		path,
	)
	if err != nil {
		return fmt.Errorf("make request: %w", err)
	}

	var resp proto.DeleteFileResult
	if err = c.readResponse(ctx, &resp); err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.Result < 0 {
		return fmt.Errorf("received unsuccessful response")
	}

	return nil
}

func (c *Client) MkDir(ctx context.Context, path string) error {
	stop, err := c.applyContext(ctx)
	if err != nil {
		return err
	}
	defer stop()

	err = c.writeRequest(ctx,
		proto.CmdMkdir,
		proto.MkdirCommand{DpLen: uint16(len(path))},
		path,
	)
	if err != nil {
		return fmt.Errorf("make request: %w", err)
	}

	var resp proto.MkdirResult
	if err = c.readResponse(ctx, &resp); err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.Result < 0 {
		return fmt.Errorf("received unsuccessful response")
	}

	return nil
}

func (c *Client) RmDir(ctx context.Context, path string) error {
	stop, err := c.applyContext(ctx)
	if err != nil {
		return err
	}
	defer stop()

	err = c.writeRequest(ctx,
		proto.CmdRmdir,
		proto.RmdirCommand{DpLen: uint16(len(path))},
		path,
	)
	if err != nil {
		return fmt.Errorf("make request: %w", err)
	}

	var resp proto.RmdirResult
	if err = c.readResponse(ctx, &resp); err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.Result < 0 {
		return fmt.Errorf("received unsuccessful response")
	}

	return nil
}

func (c *Client) GetDirSize(ctx context.Context, path string) (int64, error) {
	stop, err := c.applyContext(ctx)
	if err != nil {
		return -1, err
	}
	defer stop()

	err = c.writeRequest(ctx,
		proto.CmdGetDirSize,
		proto.GetDirSizeCommand{DpLen: uint16(len(path))},
		path,
	)
	if err != nil {
		return -1, fmt.Errorf("make request: %w", err)
	}

	var resp proto.GetDirSizeResult
	if err = c.readResponse(ctx, &resp); err != nil {
		return -1, fmt.Errorf("read response: %w", err)
	}

	if resp.Size < 0 {
		return resp.Size, fmt.Errorf("received unsuccessful response")
	}

	return resp.Size, nil
}

func (c *Client) CloseFile(ctx context.Context) error {
	_, err := c.OpenFile(ctx, "CLOSEFILE")
	return err
}

func (c *Client) Close() error {
	if c.isClosed.CompareAndSwap(false, true) {
		return c.conn.Close()
	}
	return nil
}

func (c *Client) applyContext(ctx context.Context) (func(), error) {
	if c.isClosed.Load() {
		return func() {}, fs.ErrClosed
	}

	if ctx.Done() == nil {
		return func() {}, nil
	}

	deadline, hasDeadline := ctx.Deadline()
	if hasDeadline {
		if err := c.conn.SetDeadline(deadline); err != nil {
			return nil, err
		}
	}
	stop := context.AfterFunc(ctx, func() {
		c.conn.SetDeadline(time.Now()) // trigger read/write deadline exceeded error
	})
	return func() {
		if stop() {
			c.conn.SetDeadline(time.Time{}) // reset deadline
		}
	}, nil
}

func (c *Client) writeRequest(ctx context.Context, opCode proto.OpCode, command, trailer any) error {
	cmd := proto.Command{OpCode: opCode}
	_, err := binary.Encode(cmd.Data[:], binary.BigEndian, command)
	if err != nil {
		return fmt.Errorf("encode command: %w", err)
	}

	var buf bytes.Buffer
	size := binary.Size(cmd)
	if trailer != nil {
		size += binary.Size(trailer)
	}
	buf.Grow(size)

	err = binary.Write(&buf, binary.BigEndian, cmd)
	if err != nil {
		return fmt.Errorf("write command to buffer: %w", err)
	}

	if trailer == nil {
		_, err = buf.WriteTo(c.conn)
		if err != nil {
			return fmt.Errorf("write request: %w", ioError(ctx, err))
		}
		return nil
	}

	// if it's string or []byte, write directly
	switch trailerT := trailer.(type) {
	case string:
		if _, err = buf.WriteString(trailerT); err != nil {
			return fmt.Errorf("write string to buffer: %w", err)
		}
	case []byte:
		if _, err = buf.Write(trailerT); err != nil {
			return fmt.Errorf("write bytes to buffer: %w", err)
		}
	case *bytes.Buffer:
		_, err = buf.WriteTo(c.conn)
		if err != nil {
			return fmt.Errorf("write request: %w", ioError(ctx, err))
		}

		if _, err = trailerT.WriteTo(c.conn); err != nil {
			return fmt.Errorf("write trailer to conn: %w", ioError(ctx, err))
		}
		return nil
	default:
		err = binary.Write(&buf, binary.BigEndian, trailer)
		if err != nil {
			return fmt.Errorf("write trailer to buffer: %w", err)
		}
	}

	_, err = buf.WriteTo(c.conn)
	if err != nil {
		return fmt.Errorf("write request: %w", ioError(ctx, err))
	}

	return nil
}

func (c *Client) readResponse(ctx context.Context, target any) error {
	return ioError(ctx, binary.Read(c.conn, binary.BigEndian, target))
}

func ioError(ctx context.Context, err error) error {
	if ctx == nil {
		return err
	}

	ctxErr := ctx.Err()
	if errors.Is(err, os.ErrDeadlineExceeded) && ctxErr != nil {
		return ctxErr
	}
	return err
}
