package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/alecthomas/kong"
	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"

	"github.com/xakep666/ps3netsrv-go/pkg/fs"
	"github.com/xakep666/ps3netsrv-go/pkg/fs/viso"
)

type makeISOApp struct {
	Directory string   `arg:"" help:"Path to directory to make ISO from." type:"existingdir"`
	Target    *os.File `arg:"" help:"Path to output image." type:"outputfile"`
	PS3Mode   bool     `name:"ps3-mode" help:"Enable PS3 mode. Use to make PS3-game ISO (with specific data in first sectors) from unpacked game."`
}

func (a *makeISOApp) Run(ctx context.Context, k *kong.Kong) error {
	viso, err := viso.NewVirtualISO(ctx, fs.NewFS(fs.NewRelaxedSystemRoot(a.Directory), nil, nil), ".", a.PS3Mode)
	if err != nil {
		return fmt.Errorf("failed to build ISO: %w", err)
	}

	defer viso.Close()

	fi, err := viso.Stat()
	if err != nil {
		return err
	}

	p := mpb.New(mpb.WithOutput(k.Stderr), mpb.WithRefreshRate(180*time.Millisecond))

	bar := p.New(fi.Size(),
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

	pr, err := bar.ProxyReader(viso)
	if err != nil {
		return err
	}

	_, err = io.Copy(a.Target, pr)
	if err != nil {
		return err
	}
	p.Wait()
	return nil
}
