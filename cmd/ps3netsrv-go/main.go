package main

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"

	"github.com/alecthomas/kong"
	"github.com/xakep666/ps3netsrv-go/internal/kongini"
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

	Version kong.VersionFlag `help:"Show application version info."`
	Config  kong.ConfigFlag  `help:"Load configuration from file." env:"PS3NETSRV_CONFIG_FILE"`
}

func main() {
	var app app
	ctx := kong.Parse(&app,
		kong.Name("ps3netsrv-go"),
		kong.Description("Alternative ps3netsrv implementation for installing games over network."),
		kong.Configuration(kongini.Loader, configLocations()...),
		kong.Vars{
			"version": fmt.Sprintf("%s (commit '%s' at '%s' build by '%s')", version, commit, date, builtBy),
		},
		kong.UsageOnError(),
		kong.TypeMapper(reflect.TypeOf((*writeFile)(nil)), kong.MapperFunc(writeFileMapper)),
	)
	ctx.FatalIfErrorf(ctx.Run())
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
