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

	"github.com/lmittmann/tint"
	"github.com/mattn/go-colorable"
	"github.com/mattn/go-isatty"
	"github.com/spf13/afero"
	"golang.org/x/net/netutil"
	"golang.org/x/sync/errgroup"

	"github.com/xakep666/ps3netsrv-go/pkg/bufferpool"
	"github.com/xakep666/ps3netsrv-go/pkg/fs"
	"github.com/xakep666/ps3netsrv-go/pkg/iprange"
	"github.com/xakep666/ps3netsrv-go/pkg/logutil"
	"github.com/xakep666/ps3netsrv-go/pkg/server"
)

type serverApp struct {
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

func (sapp *serverApp) setupLogger() {
	level := slog.LevelInfo
	if sapp.Debug {
		level = slog.LevelDebug
	}

	var handler slog.Handler
	if sapp.JSONLog {
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

func (sapp *serverApp) debugServer() error {
	if sapp.DebugServerListenAddr == "" {
		return nil
	}

	socket, err := net.Listen("tcp", sapp.DebugServerListenAddr)
	if err != nil {
		return fmt.Errorf("debug server listen failed: %w", err)
	}

	slog.Info("Debug sever listening...", "addr", logutil.ListenAddressValue(socket.Addr()))

	return http.Serve(socket, nil)
}

func (sapp *serverApp) server() error {
	socket, err := net.Listen("tcp", sapp.ListenAddr)
	if err != nil {
		return fmt.Errorf("listen failed: %w", err)
	}

	slog.Info("Listening...", "addr", logutil.ListenAddressValue(socket.Addr()))

	var bufPool httputil.BufferPool
	if sapp.BufferSize > 0 {
		bufPool = bufferpool.NewBufferPool(sapp.BufferSize)
	}

	s := server.Server{
		Handler: &Handler{
			Fs:         &fs.FS{afero.NewBasePathFs(afero.NewOsFs(), sapp.Root)},
			AllowWrite: sapp.AllowWrite,
			BufferPool: bufPool,
		},
		BufferPool:  bufPool,
		ReadTimeout: sapp.ReadTimeout,
	}

	if sapp.MaxClients > 0 {
		socket = netutil.LimitListener(socket, sapp.MaxClients)
	}
	if sapp.ClientWhitelist != nil {
		socket = iprange.FilterListener(socket, sapp.ClientWhitelist, false)
	}

	return s.Serve(socket)
}

func (sapp *serverApp) Run() error {
	sapp.setupLogger()

	var eg errgroup.Group
	eg.Go(sapp.debugServer)
	eg.Go(sapp.server)

	return eg.Wait()
}
