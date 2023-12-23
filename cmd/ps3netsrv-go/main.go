package main

import (
	"fmt"
	"reflect"

	"github.com/alecthomas/kong"
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
}

func main() {
	var app app
	ctx := kong.Parse(&app,
		kong.Name("ps3netsrv-go"),
		kong.Description("Alternative ps3netsrv implementation for installing games over network."),
		kong.Vars{
			"version": fmt.Sprintf("%s (commit '%s' at '%s' build by '%s')", version, commit, date, builtBy),
		},
		kong.UsageOnError(),
		kong.TypeMapper(reflect.TypeOf((*writeFile)(nil)), kong.MapperFunc(writeFileMapper)),
	)
	ctx.FatalIfErrorf(ctx.Run())
}
