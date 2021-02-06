package main

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	_ "net/http/pprof"
	"time"

	"github.com/alecthomas/kong"
	"github.com/spf13/afero"
	"go.uber.org/zap"

	"github.com/xakep666/ps3netsrv-go/pkg/bufferpool"
	"github.com/xakep666/ps3netsrv-go/pkg/fs"
	"github.com/xakep666/ps3netsrv-go/pkg/server"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
	builtBy = "unknown"
)

type config struct {
	ListenAddr            string        `help:"Main server listen address." default:":38008"`
	Debug                 bool          `help:"Enable debug log messages."`
	JSONLog               bool          `help:"Output log messages in json format."`
	DebugServerListenAddr string        `help:"Enables debug server (with pprof) if provided."`
	Root                  string        `help:"Root directory with games." type:"existingdir" default:"."`
	ReadTimeout           time.Duration `help:"Timeout for incoming commands. Connection will be closed on expiration." default:"10m"`
	WriteTimeout          time.Duration `help:"Timeout for outgoing data. Connection will be closed on expiration." default:"10s"`
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
	)
	ctx.FatalIfErrorf(app.Run())
}

func (cfg *config) logger() (*zap.Logger, error) {
	var logCfg zap.Config
	if cfg.Debug {
		logCfg = zap.NewDevelopmentConfig()
	} else {
		logCfg = zap.NewProductionConfig()
		logCfg.Encoding = ""
	}

	if cfg.JSONLog {
		logCfg.Encoding = "json"
		logCfg.EncoderConfig = zap.NewProductionEncoderConfig()
	} else {
		logCfg.Encoding = "console"
		logCfg.EncoderConfig = zap.NewDevelopmentEncoderConfig()
	}

	return logCfg.Build()
}

func (cfg *config) debugServer(log *zap.Logger) {
	if cfg.DebugServerListenAddr == "" {
		return
	}

	log.Info("Debug sever listening...", zap.String("addr", cfg.DebugServerListenAddr))

	go http.ListenAndServe(cfg.DebugServerListenAddr, nil)
}

func (cfg *config) Run() error {
	log, err := cfg.logger()
	if err != nil {
		return fmt.Errorf("logger setup failed: %w", err)
	}

	cfg.debugServer(log)

	socket, err := net.Listen("tcp", cfg.ListenAddr)
	if err != nil {
		return fmt.Errorf("listen failed: %w", err)
	}

	log.Info("Listening...", zap.Stringer("addr", socket.Addr()))

	var bufPool httputil.BufferPool
	if cfg.BufferSize > 0 {
		bufPool = bufferpool.NewBufferPool(cfg.BufferSize)
	}

	s := server.Server{
		Log: log,
		Handler: &Handler{
			Log: log,
			Fs:  &fs.FS{afero.NewBasePathFs(afero.NewOsFs(), cfg.Root)},
		},
		BufferPool:   bufPool,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
	}

	if err := s.Serve(socket); err != nil {
		return fmt.Errorf("serve failed: %w", err)
	}

	return nil
}
