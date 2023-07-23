package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	_ "net/http/pprof"
	"os"
	"strconv"
	"time"

	"github.com/alecthomas/kong"
	"github.com/mattn/go-isatty"
	"github.com/rs/zerolog"
	"github.com/spf13/afero"
	"golang.org/x/net/netutil"

	"github.com/xakep666/ps3netsrv-go/pkg/bufferpool"
	"github.com/xakep666/ps3netsrv-go/pkg/fs"
	"github.com/xakep666/ps3netsrv-go/pkg/iprange"
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
	WriteTimeout          time.Duration    `help:"Timeout for outgoing data. Connection will be closed on expiration." default:"10s"`
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

func (cfg *config) logger() *zerolog.Logger {
	var output io.Writer = os.Stdout

	if !cfg.JSONLog {
		output = zerolog.NewConsoleWriter(func(w *zerolog.ConsoleWriter) {
			w.Out = os.Stdout
			w.NoColor = !isatty.IsTerminal(os.Stdout.Fd())
		})
	}

	log := zerolog.New(output).With().Timestamp().Logger()

	if cfg.Debug {
		log = log.Level(zerolog.DebugLevel)
	} else {
		log = log.Level(zerolog.InfoLevel)
	}

	return &log
}

func (cfg *config) debugServer(log *zerolog.Logger) {
	if cfg.DebugServerListenAddr == "" {
		return
	}

	socket, err := net.Listen("tcp", cfg.DebugServerListenAddr)
	if err != nil {
		log.Fatal().Err(err).Msg("Debug server start failed")
	}

	log.Info().
		Str("addr", addrToLog(socket.Addr())).
		Msg("Debug sever listening...")

	go http.Serve(socket, nil)
}

func (cfg *config) Run() error {
	log := cfg.logger()

	cfg.debugServer(log)

	socket, err := net.Listen("tcp", cfg.ListenAddr)
	if err != nil {
		return fmt.Errorf("listen failed: %w", err)
	}

	log.Info().
		Str("addr", addrToLog(socket.Addr())).
		Msg("Listening...")

	var bufPool httputil.BufferPool
	if cfg.BufferSize > 0 {
		bufPool = bufferpool.NewBufferPool(cfg.BufferSize)
	}

	s := server.Server{
		Handler: &Handler{
			Fs: &fs.FS{afero.NewBasePathFs(afero.NewOsFs(), cfg.Root)},
		},
		BufferPool:   bufPool,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		ConnContext: func(ctx context.Context, c net.Conn) context.Context {
			return log.WithContext(ctx)
		},
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
