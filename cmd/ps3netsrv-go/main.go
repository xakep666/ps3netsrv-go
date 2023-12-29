package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/alecthomas/kong"

	"github.com/xakep666/ps3netsrv-go/internal/kongutil"
	"github.com/xakep666/ps3netsrv-go/pkg/kongini"
)

const (
	appConfigDir  = "ps3netsrv-go"
	appConfigFile = "config.ini"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
	builtBy = "unknown"
)

type app struct {
	ServerApp  serverApp  `cmd:"" name:"server" help:"Run server."`
	DecryptApp decryptApp `cmd:"" name:"decrypt" help:"Decrypt encrypted images."`
	MakeISOApp makeISOApp `cmd:"" name:"make-iso" help:"Make ISO image from directory."`

	Version kong.VersionFlag `help:"Show application version info."`
	Config  kong.ConfigFlag  `help:"Load configuration from file." env:"PS3NETSRV_CONFIG_FILE"`
}

func main() {
	var app app
	k := kong.Must(&app,
		kong.Name("ps3netsrv-go"),
		kong.Description("Alternative ps3netsrv implementation for installing games over network."),
		kong.Configuration(kongini.Loader, configLocations()...),
		kong.Vars{
			"version": fmt.Sprintf("%s (commit '%s' at '%s' build by '%s')", version, commit, date, builtBy),
		},
		kong.UsageOnError(),
		kongutil.OutputFileMapper,
		kongutil.BinSizeMapper,
	)
	ctx, err := k.Parse(translateArgs(os.Args[1:]))
	k.FatalIfErrorf(err)
	k.FatalIfErrorf(ctx.Run())
}

func configLocations() []string {
	var ret []string
	userConfigDir, err := os.UserConfigDir()
	if err == nil {
		ret = append(ret, filepath.Join(userConfigDir, appConfigDir, appConfigFile))
	}

	ret = append(ret, appConfigFile) // search in current workdir
	return ret
}

// hack to run server if 1st arg is a path to directory
// this allows to simply drag-n-drop directory to executable
func translateArgs(args []string) []string {
	if len(args) == 0 {
		return args
	}

	if st, err := os.Stat(args[0]); err == nil && st.IsDir() {
		return append([]string{"server", "--root=" + args[0]}, args[1:]...)
	}

	return args
}
