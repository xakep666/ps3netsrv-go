package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"

	"github.com/alecthomas/kong"

	"github.com/xakep666/ps3netsrv-go/internal/kongutil"
	"github.com/xakep666/ps3netsrv-go/pkg/kongini"
)

const (
	appConfigDir  = "ps3netsrv-go"
	appConfigFile = "config.ini"
)

var (
	Version = "dev"
)

type app struct {
	ServerApp  serverApp  `cmd:"" name:"server" help:"Run server."`
	DecryptApp decryptApp `cmd:"" name:"decrypt" help:"Decrypt encrypted images."`
	MakeISOApp makeISOApp `cmd:"" name:"make-iso" help:"Make ISO image from directory."`
	CHDApp     chdApp     `cmd:"" name:"chd" help:"Helpers for CHD images."`
	CSOApp     csoApp     `cmd:"" name:"cso" help:"Helpers for CSO/ZSO images."`
	ClientApp  clientApp  `cmd:"" name:"client" help:"Client for netiso protocol"`

	Version kong.VersionFlag `help:"Show application version info."`
	Config  kong.ConfigFlag  `help:"Load configuration from file." env:"PS3NETSRV_CONFIG_FILE"`
}

func main() {
	kongutil.Run(
		kong.Must(new(app),
			kong.Name("ps3netsrv-go"),
			kong.Description("Alternative ps3netsrv implementation for installing games over network."),
			kong.Configuration(kongini.Loader, configLocations()...),
			kong.Vars{
				"version": versionString(),
			},
			kong.UsageOnError(),
			kongutil.OutputFileMapper,
			kongutil.BinSizeMapper,
		),
		func(ctx context.Context, k *kong.Kong, args []string) error {
			kctx, err := k.Parse(translateArgs(args))
			if err != nil {
				return err
			}

			kctx.BindTo(ctx, (*context.Context)(nil))
			return kctx.Run()
		},
	)
}

func versionString() string {
	var sb strings.Builder
	sb.WriteString("ps3netsrv-go version ")
	sb.WriteString(Version)

	if runtimeVersion := runtime.Version(); runtimeVersion != "" {
		_, _ = fmt.Fprintf(&sb, " built with %s", runtimeVersion)
	}

	var vcsType, vcsRevision, vcsTime string
	if buildInfo, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range buildInfo.Settings {
			if setting.Key == "vcs" {
				vcsType = setting.Value
			}
			if setting.Key == "vcs.revision" {
				vcsRevision = setting.Value
			}
			if setting.Key == "vcs.time" {
				vcsTime = setting.Value
			}
		}
	}

	// for "git" vcs clip revision to 7 characters (default git behavior)
	if vcsRevision != "" {
		if vcsType == "git" {
			vcsRevision = vcsRevision[:7]
		}
		_, _ = fmt.Fprintf(&sb, " from %s", vcsRevision)
	}
	if vcsTime != "" {
		_, _ = fmt.Fprintf(&sb, " on %s", vcsTime)
	}
	return sb.String()
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

	// simulate current ps3netsrv behaviour
	dir := args[0]
	if st, err := os.Stat(dir); err == nil && st.IsDir() {
		sfoPath := filepath.Join(dir, "PS3_GAME", "PARAM.SFO")
		if st, err = os.Stat(sfoPath); err == nil && !st.IsDir() {
			// if we found PARAM.SFO file we should make ps3-mode iso
			return []string{"makeiso", "--ps3-mode", dir, dir + getIsoExt(dir)}
		}
		return []string{"makeiso", dir, dir + getIsoExt(dir)}
	}

	return args
}

func getIsoExt(name string) string {
	// if first and last letters are capital we want also capital .ISO extension
	if name[0] < 'A' || name[0] > 'Z' {
		return ".iso"
	}
	if name[len(name)-1] < 'A' || name[len(name)-1] > 'Z' {
		return ".iso"
	}
	return ".ISO"
}
