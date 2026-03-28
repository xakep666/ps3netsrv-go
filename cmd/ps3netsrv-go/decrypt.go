package main

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"

	"github.com/xakep666/ps3netsrv-go/pkg/fs/encryptediso"
	"github.com/xakep666/ps3netsrv-go/pkg/fs/iso3k3y"
)

type decrypt3k3yCmd struct {
	Image  *os.File `arg:"" help:"Path to 3k3y image to decrypt."`
	Output *os.File `arg:"" help:"Path to output image." type:"outputfile"`
}

func (c *decrypt3k3yCmd) Run() error {
	key, err := iso3k3y.Test3k3yImage(c.Image)
	if err != nil {
		return fmt.Errorf("failed to get 3k3y image key: %w", err)
	}
	if len(key) == 0 {
		return fmt.Errorf("image is not encrypted")
	}

	imageWrapped, err := encryptediso.NewEncryptedISO(c.Image, key, true)
	if err != nil {
		return err
	}

	fi, err := imageWrapped.Stat()
	if err != nil {
		return err
	}

	p := mpb.New(mpb.WithOutput(os.Stderr), mpb.WithRefreshRate(180*time.Millisecond))

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

	_, err = io.Copy(c.Output, bar.ProxyReader(imageWrapped))
	if err != nil {
		return err
	}
	p.Wait()

	return nil
}

type decryptRedumpCmd struct {
	Image  *os.File `arg:"" help:"Path to redump image to decrypt."`
	Key    *os.File `arg:"" help:"Path to key"`
	Output *os.File `arg:"" help:"Path to output image." type:"outputfile"`
}

func (c *decryptRedumpCmd) Run() error {
	key, err := encryptediso.ReadKeyFile(c.Key)
	if err != nil {
		return fmt.Errorf("key read failed: %w", err)
	}

	imageWrapped, err := encryptediso.NewEncryptedISO(c.Image, key, true)
	if err != nil {
		return err
	}

	fi, err := imageWrapped.Stat()
	if err != nil {
		return err
	}

	p := mpb.New(mpb.WithOutput(os.Stderr), mpb.WithRefreshRate(180*time.Millisecond))

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

	_, err = io.Copy(c.Output, bar.ProxyReader(imageWrapped))
	if err != nil {
		return err
	}
	p.Wait()

	return nil
}

type decryptApp struct {
	Decrypt3k3y   decrypt3k3yCmd   `cmd:"" name:"3k3y" help:"Decrypt 3k3y image."`
	DecryptRedump decryptRedumpCmd `cmd:"" name:"redump" help:"Decrypt Redump image."`
}
