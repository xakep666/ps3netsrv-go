package main

import (
	"crypto/md5"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"os"
	"text/tabwriter"

	"github.com/docker/go-units"
	"github.com/lmittmann/tint"
	"github.com/mattn/go-colorable"
	"github.com/mattn/go-isatty"
	"github.com/xakep666/ps3netsrv-go/pkg/fs/chd"
)

type chdInfoCmd struct {
	Image *os.File `arg:"" help:"Path to CHD image to inspect."`
}

func (c *chdInfoCmd) Run() error {
	slogHandler := tint.NewHandler(colorable.NewColorable(os.Stderr), &tint.Options{
		Level:   slog.LevelDebug,
		NoColor: !isatty.IsTerminal(os.Stderr.Fd()),
	})

	fi, err := c.Image.Stat()
	if err != nil {
		return err
	}

	lib, err := chd.NewLibCHDR(slog.New(slogHandler))
	if err != nil {
		return err
	}

	cf, err := lib.NewFile(c.Image)
	if err != nil {
		return err
	}
	defer cf.Close()

	hdr := cf.Header

	type kv struct {
		name      string
		formatter string
		value     any
	}
	data := []kv{
		{"Version", "%d", hdr.Version},
		{"Hunks count", "%d", hdr.TotalHunks},
		{"Bytes per hunk (uncompressed)", "0x%x", hdr.HunkBytes},
		{"Uncompressed size", "%s", units.HumanSize(float64(hdr.LogicalBytes))},
		{"On-disk size", "%s", units.HumanSize(float64(fi.Size()))},
		{"Units count (uncompressed)", "%d", hdr.UnitCount},
		{"Bytes per unit (uncompressed)", "0x%d", hdr.UnitBytes},
	}
	if hdr.MD5 != ([md5.Size]byte{}) {
		data = append(data, kv{"MD5", "%s", hex.EncodeToString(hdr.MD5[:])})
	}
	if hdr.ParentMD5 != ([md5.Size]byte{}) {
		data = append(data, kv{"Parent MD5", "%s", hex.EncodeToString(hdr.ParentMD5[:])})
	}
	if hdr.SHA1 != ([sha1.Size]byte{}) {
		data = append(data, kv{"SHA1", "%s", hex.EncodeToString(hdr.SHA1[:])})
	}
	if hdr.RawSHA1 != ([sha1.Size]byte{}) {
		data = append(data, kv{"Data SHA1", "%s", hex.EncodeToString(hdr.RawSHA1[:])})
	}
	if hdr.ParentSHA1 != ([sha1.Size]byte{}) {
		data = append(data, kv{"Parent SHA1", "%s", hex.EncodeToString(hdr.ParentSHA1[:])})
	}
	for i, compressor := range hdr.Compression {
		data = append(data, kv{fmt.Sprintf("Custom compressor %d", i), "%s", compressor})
	}
	for i := range cf.CDMetadata {
		data = append(data, kv{fmt.Sprintf("Metadata %d", i), "%s", &cf.CDMetadata[i]})
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

type chdDecompressCmd struct {
	Image        *os.File `arg:"" help:"Path to CHD image to decompress."`
	Output       *os.File `arg:"" help:"Path to output image." type:"outputfile"`
	RawCdSectors bool     `help:"Write raw sectors data ignoring metadata info if CHD image is CD-codecs encoded."`
}

func (c *chdDecompressCmd) Run() error {
	slogHandler := tint.NewHandler(colorable.NewColorable(os.Stderr), &tint.Options{
		Level:   slog.LevelError,
		NoColor: !isatty.IsTerminal(os.Stderr.Fd()),
	})

	log := slog.New(slogHandler)

	lib, err := chd.NewLibCHDR(log)
	if err != nil {
		return err
	}

	f, err := lib.NewFile(c.Image)
	if err != nil {
		return err
	}

	if !f.Header.IsCDCodesOnly() || c.RawCdSectors {
		fmt.Printf("Decompressing CHD image %s ...\n", c.Image.Name())
		_, err = io.Copy(c.Output, f)
		return err
	}

	cdFile, err := f.AsCD()
	if err != nil {
		return err
	}

	fmt.Printf("Decompressing CHD CD image %s: %d sectors %d bytes each ...\n", c.Image.Name(), cdFile.SectorsCount, cdFile.SectorDataSize)
	_, err = io.Copy(c.Output, cdFile)
	return err
}

type chdApp struct {
	CHDInfo       chdInfoCmd       `cmd:"" name:"info" help:"Inspect a CHD image and display information."`
	CHDDecompress chdDecompressCmd `cmd:"" name:"decompress" help:"Decompress CHD image."`
}
