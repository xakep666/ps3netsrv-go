package main

import (
	"fmt"
	"io"
	"os"

	"github.com/xakep666/ps3netsrv-go/pkg/fs"
)

type makeISOApp struct {
	Directory string   `arg:"" help:"Path to directory to make ISO from." type:"existingdir"`
	Target    *os.File `arg:"" help:"Path to output image." type:"outputfile"`
	PS3Mode   bool     `name:"ps3-mode" help:"Enable PS3 mode. Use to make PS3-game ISO (with specific data in first sectors) from unpacked game."`
}

func (a *makeISOApp) Run() error {
	baseFS, err := fs.NewFS(a.Directory)
	if err != nil {
		return fmt.Errorf("failed to open directory: %w", err)
	}

	viso, err := fs.NewVirtualISO(baseFS, ".", a.PS3Mode)
	if err != nil {
		return fmt.Errorf("failed to build ISO: %w", err)
	}

	defer viso.Close()

	_, err = io.Copy(a.Target, viso)
	return err
}
