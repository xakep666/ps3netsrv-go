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

type clientApp struct {
	Address    string `help:"Target server address" required:""`
	BufferSize int64  `help:"Size of buffer for data transfer. Change it only if you know what you doing." type:"binsize" default:"64k"`

	StatCmd    clientStatCmd    `cmd:"" name:"stat" help:"Display single file/dir info"`
	ReadDirCmd clientReadDirCmd `cmd:"" name:"readdir" help:"Read directory entries"`
	DirSizeCmd clientDirSizeCmd `cmd:"" name:"dirsize" help:"Get directory size"`
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
