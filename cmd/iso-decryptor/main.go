package main

import (
	"fmt"
	"github.com/alecthomas/kong"
	"github.com/spf13/afero"
	"io"
	"os"

	"github.com/xakep666/ps3netsrv-go/pkg/fs"
)

type decrypt3k3yCmd struct {
	Image  string `arg:"" help:"Path to 3k3y image to decrypt." type:"path"`
	Output string `arg:"" help:"Path to output image." type:"path"`
}

func (c *decrypt3k3yCmd) Run() error {
	img, err := afero.OsFs{}.Open(c.Image)
	if err != nil {
		return fmt.Errorf("open image failed: %w", err)
	}
	defer img.Close()

	_, err = afero.OsFs{}.Stat(c.Output)
	if err == nil {
		return fmt.Errorf("output file already exists")
	}

	out, err := afero.OsFs{}.OpenFile(c.Output, os.O_WRONLY|os.O_CREATE, os.ModePerm)
	if err != nil {
		return fmt.Errorf("create output file failed: %w", err)
	}
	defer out.Close()

	key, err := fs.Test3k3yImage(img)
	if err != nil {
		return fmt.Errorf("failed to get 3k3y image key: %w", err)
	}
	if len(key) == 0 {
		return fmt.Errorf("image is not encrypted")
	}

	imageWrapped, err := fs.NewEncryptedISO(img, key, true)
	if err != nil {
		return err
	}

	fmt.Printf("Decrypting 3k3y image %s ...\n", c.Image)

	_, err = io.Copy(out, imageWrapped)
	return err
}

type decryptRedumpCmd struct {
	Image  string `arg:"" help:"Path to redump image to decrypt." type:"path"`
	Key    string `arg:"" help:"Path to key" type:"path"`
	Output string `arg:"" help:"Path to output image." type:"path"`
}

func (c *decryptRedumpCmd) Run() error {
	img, err := afero.OsFs{}.Open(c.Image)
	if err != nil {
		return fmt.Errorf("open image failed: %w", err)
	}
	defer img.Close()

	_, err = afero.OsFs{}.Stat(c.Output)
	if err == nil {
		return fmt.Errorf("output file already exists")
	}

	out, err := afero.OsFs{}.OpenFile(c.Output, os.O_WRONLY|os.O_CREATE, os.ModePerm)
	if err != nil {
		return fmt.Errorf("create output file failed: %w", err)
	}
	defer out.Close()

	keyFile, err := afero.OsFs{}.Open(c.Key)
	if err != nil {
		return fmt.Errorf("key file open failed: %w", err)
	}
	defer keyFile.Close()

	key, err := fs.ReadKeyFile(keyFile)
	if err != nil {
		return fmt.Errorf("key read failed: %w", err)
	}

	imageWrapped, err := fs.NewEncryptedISO(img, key, true)
	if err != nil {
		return err
	}

	fmt.Printf("Decrypting Redump image %s ...\n", c.Image)

	_, err = io.Copy(out, imageWrapped)
	return err
}

type app struct {
	Decrypt3k3y   decrypt3k3yCmd   `cmd:"" name:"3k3y" help:"Decrypt 3k3y image."`
	DecryptRedump decryptRedumpCmd `cmd:"" name:"redump" help:"Decrypt Redump image."`
}

func main() {
	var app app
	ctx := kong.Parse(&app,
		kong.Name("iso-decryptor"),
		kong.Description("Decrypt Redump/3k3y encrypted iso images."),
		kong.UsageOnError(),
	)
	ctx.FatalIfErrorf(ctx.Run())
}
