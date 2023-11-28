package main

import (
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	_ "net/http/pprof"
	"os"
	"time"

	"github.com/alecthomas/kong"
	"github.com/lmittmann/tint"
	"github.com/mattn/go-colorable"
	"github.com/mattn/go-isatty"
	"github.com/spf13/afero"
	"golang.org/x/net/netutil"

	"github.com/xakep666/ps3netsrv-go/pkg/bufferpool"
	"github.com/xakep666/ps3netsrv-go/pkg/fs"
	"github.com/xakep666/ps3netsrv-go/pkg/iprange"
	"github.com/xakep666/ps3netsrv-go/pkg/logutil"
	"github.com/xakep666/ps3netsrv-go/pkg/server"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
	builtBy = "unknown"
)

type config struct {
	Root string `help:"Root directory with games." arg:"" type:"existingdir" default:"."`

	ListenAddr            string           `help:"Main server listen address." default:":38008"`
	Debug                 bool             `help:"Enable debug log messages."`
	JSONLog               bool             `help:"Output log messages in json format."`
	DebugServerListenAddr string           `help:"Enables debug server (with pprof) if provided."`
	ReadTimeout           time.Duration    `help:"Timeout for incoming commands. Connection will be closed on expiration." default:"10m"`
	MaxClients            int              `help:"Limit amount of connected clients. Negative or zero means no limit."`
	ClientWhitelist       *iprange.IPRange `help:"Optional client IP whitelist. Formats: single IPv4/v6 ('192.168.0.2'), IPv4/v6 CIDR ('192.168.0.1/24'), IPv4 + subnet mask ('192.168.0.1/255.255.255.0), IPv4/IPv6 range ('192.168.0.1-192.168.0.255')."`
	AllowWrite            bool             `help:"Allow writing/modifying filesystem operations."`
	// default value found during debugging
	BufferSize int `help:"Size of buffer for data transfer. Change it only if you know what you doing." default:"65535"`
}

type app struct {
	config

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
	)
	ctx.FatalIfErrorf(app.Run())
}

func (cfg *config) setupLogger() {
	level := slog.LevelInfo
	if cfg.Debug {
		level = slog.LevelDebug
	}

	var handler slog.Handler
	if cfg.JSONLog {
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: level,
		})
	} else {
		handler = tint.NewHandler(colorable.NewColorable(os.Stdout), &tint.Options{
			Level:   level,
			NoColor: !isatty.IsTerminal(os.Stdout.Fd()),
		})
	}

	handler = &server.SlogContextHandler{handler}

	slog.SetDefault(slog.New(handler))
}

func (cfg *config) debugServer() {
	if cfg.DebugServerListenAddr == "" {
		return
	}

	socket, err := net.Listen("tcp", cfg.DebugServerListenAddr)
	if err != nil {
		slog.Error("Debug server start failed", logutil.ErrorAttr(err))
		os.Exit(1)
	}

	slog.Info("Debug sever listening...", "addr", logutil.ListenAddressValue(socket.Addr()))

	go http.Serve(socket, nil)
}

func (cfg *config) Run() error {
	cfg.setupLogger()

	cfg.debugServer()

	socket, err := net.Listen("tcp", cfg.ListenAddr)
	if err != nil {
		return fmt.Errorf("listen failed: %w", err)
	}

	slog.Info("Listening...", "addr", logutil.ListenAddressValue(socket.Addr()))

	var bufPool httputil.BufferPool
	if cfg.BufferSize > 0 {
		bufPool = bufferpool.NewBufferPool(cfg.BufferSize)
	}

	s := server.Server{
		Handler: &Handler{
			Fs:         &fs.FS{afero.NewBasePathFs(afero.NewOsFs(), cfg.Root)},
			AllowWrite: cfg.AllowWrite,
			BufferPool: bufPool,
		},
		BufferPool:  bufPool,
		ReadTimeout: cfg.ReadTimeout,
	}

	if cfg.MaxClients > 0 {
		socket = netutil.LimitListener(socket, cfg.MaxClients)
	}
	if cfg.ClientWhitelist != nil {
		socket = iprange.FilterListener(socket, cfg.ClientWhitelist, false)
	}

	if err := s.Serve(socket); err != nil {
		return fmt.Errorf("serve failed: %w", err)
	}

	return nil
}
