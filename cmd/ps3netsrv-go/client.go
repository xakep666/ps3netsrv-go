package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"iter"
	"os"
	"os/signal"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/docker/go-units"
	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"

	"github.com/xakep666/ps3netsrv-go/internal/ioutil"
	"github.com/xakep666/ps3netsrv-go/pkg/client"
)

type clientStatCmd struct {
	Path string `arg:"" help:"Path on server to stat"`
}

func (c *clientStatCmd) Run(client *client.Client) error {
	sigCtx, sigDone := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer sigDone()

	info, err := client.StatFile(sigCtx, c.Path)
	if err != nil {
		return err
	}

	var buf bytes.Buffer
	fmt.Fprintf(&buf, "Path: %s\n", c.Path)
	fmt.Fprintf(&buf, "Size: %s (%d)\n", units.HumanSize(float64(info.FileSize)), info.FileSize)
	fmt.Fprintf(&buf, "Mod time: %s (%d)\n", time.Unix(int64(info.ModTime), 0).Format(time.Stamp), info.ModTime)
	fmt.Fprintf(&buf, "Access time: %s (%d)\n", time.Unix(int64(info.AccessTime), 0).Format(time.Stamp), info.AccessTime)
	fmt.Fprintf(&buf, "Change time: %s (%d)\n", time.Unix(int64(info.ChangeTime), 0).Format(time.Stamp), info.ChangeTime)
	fmt.Fprintf(&buf, "Is directory: %t\n", info.IsDirectory)

	_, err = buf.WriteTo(os.Stdout)
	return err
}

type clientReadDirCmd struct {
	Path   string `arg:"" help:"Path to target directory on server"`
	Method string `enum:"v1,v2,full" help:"Method to use: v1 - send ReadDirEntry commands, v2 - send ReadDirEntryV2 commands, full - send ReadDir command." default:"v2"`
}

func (c *clientReadDirCmd) Run(client *client.Client) error {
	sigCtx, sigDone := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer sigDone()

	if err := client.OpenDir(sigCtx, c.Path); err != nil {
		return fmt.Errorf("open dir: %w", err)
	}

	defer func() {
		_ = client.CloseFile(sigCtx)
	}()

	tw := tabwriter.NewWriter(os.Stdout, 10, 0, 2, ' ', 0)
	switch c.Method {
	case "v1":
		_, err := io.WriteString(tw, "Name\tIs directory\tSize (in bytes)\n")
		if err != nil {
			return err
		}

		iter, errp := dirEntriesV1(sigCtx, client)
		for item := range iter {
			_, err := fmt.Fprintf(tw, "%s\t%t\t%s (%d)\n",
				item.Name,
				item.IsDirectory,
				units.HumanSize(float64(item.FileSize)), item.FileSize,
			)
			if err != nil {
				return err
			}
		}
		if err := errp(); err != nil {
			return err
		}
	case "v2":
		_, err := io.WriteString(tw, "Name\tIs directory\tSize (in bytes)\tMod time (unix)\tChange time (unix)\tAccess time (unix)\n")
		if err != nil {
			return err
		}

		iter, errp := dirEntriesV2(sigCtx, client)
		for item := range iter {
			_, err := fmt.Fprintf(tw, "%s\t%t\t%s (%d)\t%s (%d)\t%s (%d)\t%s (%d)\n",
				item.Name,
				item.IsDirectory,
				units.HumanSize(float64(item.FileSize)), item.FileSize,
				time.Unix(int64(item.ModTime), 0).Format(time.Stamp), item.ModTime,
				time.Unix(int64(item.ChangeTime), 0).Format(time.Stamp), item.ChangeTime,
				time.Unix(int64(item.AccessTime), 0).Format(time.Stamp), item.AccessTime,
			)
			if err != nil {
				return err
			}
		}
		if err := errp(); err != nil {
			return err
		}
	case "full":
		_, err := io.WriteString(tw, "Name\tIs directory\tSize (in bytes)\n")
		if err != nil {
			return err
		}

		items, err := client.ReadDir(sigCtx)
		if err != nil {
			return err
		}

		for _, item := range items {
			var name []byte
			nulIdx := bytes.IndexRune(item.Name[:], 0)
			if nulIdx < 0 {
				name = item.Name[:]
			}
			if nulIdx > 0 {
				name = item.Name[:nulIdx]
			}

			_, err := fmt.Fprintf(tw, "%s\t%t\t%s (%d)\n",
				name,
				item.IsDirectory,
				units.HumanSize(float64(item.FileSize)), item.FileSize,
			)
			if err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("unknown method: %q", c.Method)
	}

	return tw.Flush()
}

type clientDirSizeCmd struct {
	Path string `arg:"" help:"Path to target directory on server"`
}

func (c *clientDirSizeCmd) Run(client *client.Client) error {
	sigCtx, sigDone := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer sigDone()

	size, err := client.GetDirSize(sigCtx, c.Path)
	if err != nil {
		return err
	}

	_, err = fmt.Printf("Size (in bytes): %s (%d)\n", units.HumanSize(float64(size)), size)
	return err
}

type clientMkdirCmd struct {
	Path string `arg:"" help:"Path to target directory on server"`
}

func (c *clientMkdirCmd) Run(client *client.Client) error {
	sigCtx, sigDone := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer sigDone()

	return client.MkDir(sigCtx, c.Path)
}

type clientRmdirCmd struct {
	Path string `arg:"" help:"Path to target directory on server"`
}

func (c *clientRmdirCmd) Run(client *client.Client) error {
	sigCtx, sigDone := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer sigDone()

	return client.RmDir(sigCtx, c.Path)
}

type clientRmCmd struct {
	Path string `arg:"" help:"Path to target file on server"`
}

func (c *clientRmCmd) Run(client *client.Client) error {
	sigCtx, sigDone := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer sigDone()

	return client.DeleteFile(sigCtx, c.Path)
}

type clientReadCmd struct {
	Path   string             `arg:"" help:"Path to file on server"`
	Target targetFileWithMode `embed:""`

	NonCritical bool   `help:"Use ReadFile command instead of ReadFileCritical command"`
	BlockSize   uint32 `help:"Size of single file block (chunk) read from server in one request" type:"binsize" default:"64k"`
	Seek        int    `help:"Skip N blocks before start reading" placeholder:"N"`
	Count       int    `help:"Read N blocks. Read until EOF if not specified." placeholder:"N"`
}

func (c *clientReadCmd) Run(client *client.Client) error {
	sigCtx, sigDone := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer sigDone()

	openResult, err := client.OpenFile(sigCtx, c.Path)
	if err != nil {
		return err
	}

	defer func() {
		_ = client.CloseFile(sigCtx)
	}()

	offset := min(int64(c.Seek)*int64(c.BlockSize), 0)
	count := min(int64(c.Count)*int64(c.BlockSize), 0)
	if offset+count > openResult.FileSize {
		return fmt.Errorf("blocksize(%d)*(seek(%d)+count(%d)) are greater than file size (%d)",
			c.BlockSize, c.Seek, c.Count, openResult.FileSize,
		)
	}

	count = max(count, openResult.FileSize-offset)

	targetFile, err := c.Target.open()
	if err != nil {
		return err
	}

	p := mpb.NewWithContext(sigCtx, mpb.WithOutput(os.Stderr), mpb.WithRefreshRate(180*time.Millisecond))

	bar := p.New(count,
		mpb.BarStyle().Rbound("|"),
		mpb.PrependDecorators(
			decor.Counters(decor.SizeB1024(0), "% .2f / % .2f"),
		),
		mpb.AppendDecorators(
			// EWMA decorators doesn't work with proxy writer now
			decor.AverageETA(decor.ET_STYLE_GO),
			decor.Name(" ] "),
			decor.AverageSpeed(decor.SizeB1024(0), "% .2f"),
		),
	)

	fmt.Fprintf(os.Stderr, "Reading file %q from server\n", c.Path)
	target := bar.ProxyWriter(targetFile)
	for count > 0 {
		readBytes := min(c.BlockSize, uint32(count))
		if c.NonCritical {
			err = client.ReadFile(sigCtx, readBytes, uint64(offset), target)
		} else {
			err = client.ReadFileCritical(sigCtx, readBytes, uint64(offset), target)
		}
		if err != nil {
			return err
		}

		count -= int64(readBytes)
		offset += int64(readBytes)
	}

	p.Wait()

	return target.Close()
}

type clientReadCd2048Cmd struct {
	Path   string             `arg:"" help:"Path to file on server"`
	Target targetFileWithMode `embed:""`

	BlockSize uint32 `help:"Amount of sectors per single read request" default:"1"`
	Seek      uint32 `help:"Skip N sectors before start reading" placeholder:"N"`
	Count     uint32 `help:"Read N sectors. Read until EOF if not specified." placeholder:"N" required:"true"`
}

func (c *clientReadCd2048Cmd) Run(client *client.Client) error {
	sigCtx, sigDone := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer sigDone()

	openResult, err := client.OpenFile(sigCtx, c.Path)
	if err != nil {
		return err
	}

	defer func() {
		_ = client.CloseFile(sigCtx)
	}()

	offset := c.Seek
	count := c.Count

	const sectorSize = 2048
	if sectorSize*int64(offset+count) > openResult.FileSize { // not precise validation
		return fmt.Errorf("blocksize(%d)*(seek(%d)+count(%d)) are greater than file size (%d)",
			sectorSize, c.Seek, c.Count, openResult.FileSize,
		)
	}

	targetFile, err := c.Target.open()
	if err != nil {
		return err
	}

	p := mpb.NewWithContext(sigCtx, mpb.WithOutput(os.Stderr), mpb.WithRefreshRate(180*time.Millisecond))

	bar := p.New(int64(count)*sectorSize,
		mpb.BarStyle().Rbound("|"),
		mpb.PrependDecorators(
			decor.Counters(decor.SizeB1024(0), "% .2f / % .2f"),
		),
		mpb.AppendDecorators(
			// EWMA decorators doesn't work with proxy writer now
			decor.AverageETA(decor.ET_STYLE_GO),
			decor.Name(" ] "),
			decor.AverageSpeed(decor.SizeB1024(0), "% .2f"),
		),
	)

	target := bar.ProxyWriter(targetFile)
	fmt.Fprintf(os.Stderr, "Reading file %q from server in cd-2048 mode\n", c.Path)
	for count > 0 {
		err := client.ReadCD2048Critical(sigCtx, c.BlockSize, offset, target)
		if err != nil {
			return err
		}

		count -= c.BlockSize
		offset += c.BlockSize
	}

	p.Wait()

	return target.Close()
}

type clientWriteCmd struct {
	Source     *os.File `arg:"" help:"Local file to send"`
	TargetPath string   `arg:"" help:"Path on server to place a file"`

	BlockSize uint32 `help:"Size of single file block (chunk) sent to server in one request" type:"binsize" default:"64k"`
	Seek      int    `help:"Skip N blocks before start reading" placeholder:"N"`
	Count     int    `help:"Read N blocks. Read until EOF if not specified." placeholder:"N"`
}

func (c *clientWriteCmd) Run(client *client.Client) error {
	sigCtx, sigDone := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer sigDone()

	fi, err := c.Source.Stat()
	if err != nil {
		return err
	}

	offset := min(int64(c.Seek)*int64(c.BlockSize), 0)
	count := min(int64(c.Count)*int64(c.BlockSize), 0)
	if offset+count > fi.Size() {
		return fmt.Errorf("blocksize(%d)*(seek(%d)+count(%d)) are greater than file size (%d)",
			c.BlockSize, c.Seek, c.Count, fi.Size(),
		)
	}

	count = max(count, fi.Size()-offset)

	if offset > 0 {
		_, err = c.Source.Seek(offset, io.SeekStart)
		if err != nil {
			return err
		}
	}

	if err = client.CreateFile(sigCtx, c.TargetPath); err != nil {
		return err
	}

	p := mpb.NewWithContext(sigCtx, mpb.WithOutput(os.Stderr), mpb.WithRefreshRate(180*time.Millisecond))

	bar := p.New(count,
		mpb.BarStyle().Rbound("|"),
		mpb.PrependDecorators(
			decor.Counters(decor.SizeB1024(0), "% .2f / % .2f"),
		),
		mpb.AppendDecorators(
			decor.EwmaETA(decor.ET_STYLE_GO, 30),
			decor.Name(" ] "),
			decor.EwmaSpeed(decor.SizeB1024(0), "% .2f", 30),
		),
	)

	fmt.Fprintf(os.Stderr, "Sending file %q\n", c.Source.Name())
	err = client.WriteFile(sigCtx, c.BlockSize, bar.ProxyReader(io.LimitReader(c.Source, count)))
	if err != nil {
		return err
	}

	p.Wait()
	return nil
}

type targetFileWithMode struct {
	TargetPath string `arg:"" help:"Path to target (local) file, '-' for stdout"`
	TargetMode string `enum:"exclude,truncate,append" help:"Target file open mode: exclude - command fails if file already exists, truncate - truncates already existing file, append - appends to already existing file" default:"exclude"`
}

func (t *targetFileWithMode) open() (*os.File, error) {
	if t.TargetPath == "-" {
		return os.Stdout, nil
	}
	switch t.TargetMode {
	case "exclude":
		return os.OpenFile(t.TargetPath, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o644)
	case "truncate":
		return os.OpenFile(t.TargetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	case "append":
		return os.OpenFile(t.TargetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	default:
		return nil, fmt.Errorf("unknown mode %q", t.TargetPath)
	}
}

func dirEntriesV1(ctx context.Context, c *client.Client) (iter.Seq[client.DirEntry], func() error) {
	errp := new(error)
	return func(yield func(client.DirEntry) bool) {
			for {
				item, err := c.ReadDirEntry(ctx)
				if err != nil {
					if !errors.Is(err, io.EOF) {
						*errp = err
					}
					return
				}

				if !yield(item) {
					return
				}
			}
		}, func() error {
			return *errp
		}
}

func dirEntriesV2(ctx context.Context, c *client.Client) (iter.Seq[client.DirEntryV2], func() error) {
	errp := new(error)
	return func(yield func(client.DirEntryV2) bool) {
			for {
				item, err := c.ReadDirEntryV2(ctx)
				if err != nil {
					if !errors.Is(err, io.EOF) {
						*errp = err
					}
					return
				}

				if !yield(item) {
					return
				}
			}
		}, func() error {
			return *errp
		}
}

type clientApp struct {
	Address    string `help:"Target server address" required:""`
	BufferSize int64  `help:"Size of buffer for data transfer. Change it only if you know what you doing." type:"binsize" default:"64k"`

	StatCmd       clientStatCmd       `cmd:"" name:"stat" help:"Display single file/dir info"`
	ReadDirCmd    clientReadDirCmd    `cmd:"" name:"readdir" help:"Read directory entries"`
	DirSizeCmd    clientDirSizeCmd    `cmd:"" name:"dirsize" help:"Get directory size"`
	MkdirCmd      clientMkdirCmd      `cmd:"" name:"mkdir" help:"Create a directory"`
	RmdirCmd      clientRmdirCmd      `cmd:"" name:"rmdir" help:"Remove a directory"`
	RmCmd         clientRmCmd         `cmd:"" name:"rm" help:"Remove a file"`
	ReadCmd       clientReadCmd       `cmd:"" name:"read" help:"Copy file from server to local machine"`
	ReadCd2048Cmd clientReadCd2048Cmd `cmd:"" name:"read-cd2048" help:"Copy file from server to local machine using ReadCD2048 command (PSX mode)"`
	WriteCmd      clientWriteCmd      `cmd:"" name:"write" help:"Copy local file to server"`
}

func (c *clientApp) ProvideClient() (*client.Client, error) {
	sigCtx, sigDone := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer sigDone()

	var cop *ioutil.Copier
	if c.BufferSize > 0 {
		cop = ioutil.NewPooledCopier(c.BufferSize)
	} else {
		cop = ioutil.NewCopier()
	}

	return client.NewClient(sigCtx, cop, c.Address)
}
