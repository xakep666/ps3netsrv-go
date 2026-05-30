package main

import (
	"fmt"
	"io"
	"os"
	"text/tabwriter"
	"time"

	"github.com/docker/go-units"
	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"
	"github.com/xakep666/ps3netsrv-go/pkg/fs/cso"
)

type csoInfoCmd struct {
	Image *os.File `arg:"" help:"Path to CSO/ZSO image to inspect."`
}

func (c *csoInfoCmd) Run() error {
	fi, err := c.Image.Stat()
	if err != nil {
		return err
	}

	hdr, err := cso.ReadHeader(c.Image)
	if err != nil {
		return err
	}

	type kv struct {
		name      string
		formatter string
		value     any
	}
	data := []kv{
		{"Magic", "%s", hdr.Magic},
		{"Version", "%d", hdr.Version},
		{"Blocks count", "%d", hdr.BlocksCount()},
		{"Bytes per block (uncompressed)", "0x%x", hdr.BlockSize},
		{"Index shift", "%d", hdr.IndexShift},
		{"Uncompressed size", "%s", units.HumanSize(float64(hdr.UncompressedSize))},
		{"On-disk size", "%s", units.HumanSize(float64(fi.Size()))},
	}
	tw := tabwriter.NewWriter(os.Stdout, 10, 0, 2, ' ', 0)
	for _, d := range data {
		_, err := fmt.Fprintf(tw, "%s:\t"+d.formatter+"\n", d.name, d.value)
		if err != nil {
			return err
		}
	}
	return tw.Flush()
}

type csoDecompressCmd struct {
	Image  *os.File `arg:"" help:"Path to CSO/ZSO image to decompress."`
	Output *os.File `arg:"" help:"Path to output image." type:"outputfile"`
}

func (c *csoDecompressCmd) Run() error {
	f, err := cso.NewFile(c.Image)
	if err != nil {
		return err
	}

	p := mpb.New(mpb.WithOutput(os.Stderr), mpb.WithRefreshRate(180*time.Millisecond))
	builder := mpb.BarStyle().Rbound("|")
	opts := []mpb.BarOption{
		mpb.PrependDecorators(
			decor.Counters(decor.SizeB1024(0), "% .2f / % .2f"),
		),
		mpb.AppendDecorators(
			decor.EwmaETA(decor.ET_STYLE_GO, 30),
			decor.Name(" ] "),
			decor.EwmaSpeed(decor.SizeB1024(0), "% .2f", 30),
		),
	}

	bar := p.New(int64(f.Header.UncompressedSize), builder, opts...)
	_, err = io.Copy(c.Output, bar.ProxyReader(f))
	if err != nil {
		return err
	}
	p.Wait()
	return nil
}

type csoApp struct {
	CSOInfo       csoInfoCmd       `cmd:"" name:"info" help:"Inspect a CSO/ZSO image and display information."`
	CSODecompress csoDecompressCmd `cmd:"" name:"decompress" help:"Decompress CSO/ZSO image."`
}
