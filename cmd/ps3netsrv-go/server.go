package main

import (
	"fmt"
	"log/slog"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"path/filepath"
	"time"

	"github.com/lmittmann/tint"
	"github.com/mattn/go-colorable"
	"github.com/mattn/go-isatty"
	"github.com/spf13/afero"
	"golang.org/x/net/netutil"
	"golang.org/x/sync/errgroup"

	"github.com/xakep666/ps3netsrv-go/internal/copier"
	"github.com/xakep666/ps3netsrv-go/internal/isroot"
	"github.com/xakep666/ps3netsrv-go/internal/logutil"
	"github.com/xakep666/ps3netsrv-go/pkg/fs"
	"github.com/xakep666/ps3netsrv-go/pkg/iprange"
	"github.com/xakep666/ps3netsrv-go/pkg/server"
)

type serverApp struct {
	Root                  string           `help:"Root directory with games." type:"existingdir" default:"." env:"PS3NETSRV_ROOT"`
	ListenAddr            string           `help:"Main server listen address." default:":38008" env:"PS3NETSRV_LISTEN_ADDR"`
	Debug                 bool             `help:"Enable debug log messages." env:"PS3NETSRV_DEBUG"`
	JSONLog               bool             `help:"Output log messages in json format." env:"PS3NETSRV_JSON_LOG"`
	DebugServerListenAddr string           `help:"Enables debug server (with pprof) if provided." env:"PS3NETSRV_DEBUG_SERVER_LISTEN_ADDR"`
	ReadTimeout           time.Duration    `help:"Timeout for incoming commands. Connection will be closed on expiration." default:"10m" env:"PS3NETSRV_READ_TIMEOUT"`
	MaxClients            int              `help:"Limit amount of connected clients. Negative or zero means no limit." env:"PS3NETSRV_MAX_CLIENTS"`
	ClientWhitelist       *iprange.IPRange `help:"Optional client IP whitelist. Formats: single IPv4/v6 ('192.168.0.2'), IPv4/v6 CIDR ('192.168.0.1/24'), IPv4 + subnet mask ('192.168.0.1/255.255.255.0), IPv4/IPv6 range ('192.168.0.1-192.168.0.255')." env:"PS3NETSRV_CLIENT_WHITELIST"`
	AllowWrite            bool             `help:"Allow writing/modifying filesystem operations." env:"PS3NETSRV_ALLOW_WRITE"`
	// default value found during debugging
	BufferSize int `help:"Size of buffer for data transfer. Change it only if you know what you doing." default:"65535" env:"PS3NETSRV_BUFFER_SIZE"`
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

func (sapp *serverApp) warnIPRange(listener net.Listener) {
	if sapp.ClientWhitelist == nil {
		return
	}

	var addrToCheck net.IP
	switch addr := listener.Addr().(type) {
	case *net.TCPAddr:
		addrToCheck = addr.IP
	case *net.UDPAddr:
		addrToCheck = addr.IP
	case *net.IPAddr:
		addrToCheck = addr.IP
	default:
		return
	}
	if addrToCheck.IsUnspecified() {
		return
	}

	if !sapp.ClientWhitelist.Contains(addrToCheck) {
		slog.Warn("Listener address is not in client whitelist. This may cause connection problems.",
			"whitelist", sapp.ClientWhitelist)
	}
}

func (sapp *serverApp) server() error {
	socket, err := net.Listen("tcp", sapp.ListenAddr)
	if err != nil {
		return fmt.Errorf("listen failed: %w", err)
	}

	sapp.warnIPRange(socket)
	slog.Info("Listening...", "addr", logutil.ListenAddressValue(socket.Addr()))

	var cop *copier.Copier
	if sapp.BufferSize > 0 {
		cop = copier.NewPooledCopier(sapp.BufferSize)
	}

	s := server.Server{
		Handler: &Handler{
			Fs:         &fs.FS{afero.NewBasePathFs(afero.NewOsFs(), sapp.Root)},
			AllowWrite: sapp.AllowWrite,
			Copier:     cop,
		},
		Copier:      cop,
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

func (sapp *serverApp) warnRoot() {
	if isroot.IsRoot() {
		if sapp.AllowWrite {
			slog.Warn("Running as root/administrator with write access is dangerous! This may damage your data!")
		} else {
			slog.Warn("Running as root/administrator is not recommended! Please run as a regular user.")
		}
	}
}

func (sapp *serverApp) warnLargeDir() {
	const maxEntries = 4096 // from ps3netsrv

	queue := []string{sapp.Root}
	scanDir := func(path string) {
		slog.Debug("Checking dir for entries limit", "dir", path)

		dir, err := os.Open(path)
		if err != nil {
			return
		}

		defer dir.Close()

		var numEntries int
		for {
			entries, err := dir.ReadDir(maxEntries)
			if err != nil {
				break
			}

			numEntries += len(entries)
			for _, entry := range entries {
				if entry.IsDir() {
					queue = append(queue, filepath.Join(path, entry.Name()))
				}
			}
		}

		if numEntries > maxEntries {
			slog.Warn("Found directory that contains too many entries. Note that WebMan Mod has a limit of entries per directory so some items may be inaccessible.",
				"dir", path, "entries", numEntries, "limit", maxEntries)
		}
	}

	for len(queue) > 0 {
		dir := queue[len(queue)-1]
		queue = queue[:len(queue)-1]

		scanDir(dir)
	}
}

func (sapp *serverApp) Run() error {
	sapp.setupLogger()
	sapp.warnRoot()
	go sapp.warnLargeDir() // asynchronously to not delay server startup

	var eg errgroup.Group
	eg.Go(sapp.debugServer)
	eg.Go(sapp.server)

	return eg.Wait()
}
