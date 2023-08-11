package main

import (
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	_ "net/http/pprof"
	"os"
	"strconv"
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

	slog.Info("Debug sever listening...", "addr", addrToLog(socket.Addr()))

	go http.Serve(socket, nil)
}

func (cfg *config) Run() error {
	cfg.setupLogger()

	cfg.debugServer()

	socket, err := net.Listen("tcp", cfg.ListenAddr)
	if err != nil {
		return fmt.Errorf("listen failed: %w", err)
	}

	slog.Info("Listening...", "addr", addrToLog(socket.Addr()))

	var bufPool httputil.BufferPool
	if cfg.BufferSize > 0 {
		bufPool = bufferpool.NewBufferPool(cfg.BufferSize)
	}

	s := server.Server{
		Handler: &Handler{
			Fs: &fs.FS{afero.NewBasePathFs(afero.NewOsFs(), cfg.Root)},
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

func addrToLog(addr net.Addr) string {
	tcpAddr, isTcpAddr := addr.(*net.TCPAddr)
	if !isTcpAddr {
		return addr.String()
	}

	// if bound to all, print first non-localhost ip
	// ipv6 with zone handled separately
	isV4Any := tcpAddr.IP.Equal(net.IPv4zero)
	isV6Any := tcpAddr.IP.Equal(net.IPv6unspecified)
	if isV4Any || (isV6Any && tcpAddr.Zone == "") {
		ifaddrs, err := net.InterfaceAddrs()
		if err != nil {
			return addr.String()
		}

		if foundAddr := firstSuitableIfaddr(ifaddrs, isV4Any); foundAddr != "" {
			return net.JoinHostPort(foundAddr, strconv.Itoa(tcpAddr.Port))
		}
	}

	// for zoned addr try to get interface by name
	if isV6Any && tcpAddr.Zone != "" {
		iface, err := net.InterfaceByName(tcpAddr.Zone)
		if err != nil {
			return addr.String()
		}

		ifaddrs, err := iface.Addrs()
		if err != nil {
			return addr.String()
		}

		if foundAddr := firstSuitableIfaddr(ifaddrs, isV4Any); foundAddr != "" {
			return net.JoinHostPort(foundAddr, strconv.Itoa(tcpAddr.Port))
		}
	}

	return addr.String()
}

func firstSuitableIfaddr(ifaddrs []net.Addr, skipV6 bool) string {
	for _, ifaddr := range ifaddrs {
		ipNet, isIPNet := ifaddr.(*net.IPNet)
		if !isIPNet {
			continue
		}

		// skip loopback
		if ipNet.IP.IsLoopback() {
			continue
		}

		// skip v6 for v4 bound
		if skipV6 && len(ipNet.IP) == net.IPv6len {
			continue
		}

		return ipNet.IP.String()
	}

	return ""
}
