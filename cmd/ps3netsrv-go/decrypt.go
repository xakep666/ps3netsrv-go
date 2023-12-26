package main

import (
	"fmt"
	"io"
	"os"
	"reflect"

	"github.com/alecthomas/kong"

	"github.com/xakep666/ps3netsrv-go/pkg/fs"
)

type decrypt3k3yCmd struct {
	Image  *os.File   `arg:"" help:"Path to 3k3y image to decrypt."`
	Output *writeFile `arg:"" help:"Path to output image."`
}

func (c *decrypt3k3yCmd) Run() error {
	key, err := fs.Test3k3yImage(c.Image)
	if err != nil {
		return fmt.Errorf("failed to get 3k3y image key: %w", err)
	}
	if len(key) == 0 {
		return fmt.Errorf("image is not encrypted")
	}

	imageWrapped, err := fs.NewEncryptedISO(c.Image, key, true)
	if err != nil {
		return err
	}

	fmt.Printf("Decrypting 3k3y image %s ...\n", c.Image.Name())

	_, err = io.Copy(c.Output, imageWrapped)
	return err
}

type decryptRedumpCmd struct {
	Image  *os.File   `arg:"" help:"Path to redump image to decrypt."`
	Key    *os.File   `arg:"" help:"Path to key"`
	Output *writeFile `arg:"" help:"Path to output image."`
}

func (c *decryptRedumpCmd) Run() error {
	key, err := fs.ReadKeyFile(c.Key)
	if err != nil {
		return fmt.Errorf("key read failed: %w", err)
	}

	imageWrapped, err := fs.NewEncryptedISO(c.Image, key, true)
	if err != nil {
		return err
	}

	fmt.Printf("Decrypting Redump image %s ...\n", c.Image.Name())

	_, err = io.Copy(c.Output, imageWrapped)
	return err
}

type decryptApp struct {
	Decrypt3k3y   decrypt3k3yCmd   `cmd:"" name:"3k3y" help:"Decrypt 3k3y image."`
	DecryptRedump decryptRedumpCmd `cmd:"" name:"redump" help:"Decrypt Redump image."`
}

type writeFile struct {
	*os.File
}

func writeFileMapper(dctx *kong.DecodeContext, target reflect.Value) error {
	var path string
	err := dctx.Scan.PopValueInto("string", &path)
	if err != nil {
		return err
	}
	path = kong.ExpandPath(path)

	if path == "-" {
		target.Set(reflect.ValueOf(&writeFile{os.Stdout}))
		return nil
	}

	_, err = os.Stat(path)
	if err == nil {
		return fmt.Errorf("target file already exists")
	}

	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE, os.ModePerm)
	if err != nil {
		return err
	}

	target.Set(reflect.ValueOf(&writeFile{f}))

	return nil
}
