package kongutil

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/alecthomas/kong"
	"github.com/mattn/go-isatty"
)

type RunFn func(ctx context.Context, app *kong.Kong, args []string) error

func commonRun(app *kong.Kong, run RunFn) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	context.AfterFunc(ctx, func() {
		if isatty.IsTerminal(os.Stdout.Fd()) {
			// for terminal sessions print this log to help user if shutdown is stuck
			slog.Info("Shutting down... Press Ctrl-C again for force-shutdown")
		} else {
			slog.Info("Shutting down...")
		}
		// unregister custom signal handler, default is force-exit
		stop()
	})

	app.FatalIfErrorf(run(ctx, app, os.Args[1:]))
}
