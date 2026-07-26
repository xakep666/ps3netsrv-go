package kongutil

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime/debug"

	"github.com/alecthomas/kong"
	"golang.org/x/sys/windows/svc"
)

var errShuttingDown = fmt.Errorf("svc: shutting down")

func Run(app *kong.Kong, run RunFn) {
	isSvc, err := svc.IsWindowsService()
	if err != nil {
		panic(err)
	}
	if !isSvc {
		commonRun(app, run)
		return
	}

	err = svc.Run(app.Model.Name, &kongRunService{
		app: app,
		run: run,
	})
	if err != nil {
		// this is system error and should never happen
		panic(err)
	}
}

type kongRunService struct {
	app *kong.Kong
	run RunFn
}

func (ks *kongRunService) Execute(args []string, r <-chan svc.ChangeRequest, s chan<- svc.Status) (svcSpecificEC bool, exitCode uint32) {
	s <- svc.Status{
		State: svc.StartPending,
	}

	if len(args) <= 1 {
		args = os.Args // use cmdline args by default unless they're overriden with service start args
	}

	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	errc := make(chan error)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				// format panic output as it done if no recovery used
				errc <- fmt.Errorf("panic %v\n%s", r, debug.Stack())
			}
		}()

		errc <- ks.run(ctx, ks.app, args[1:])
	}()

	s <- svc.Status{
		State:   svc.Running,
		Accepts: svc.AcceptStop | svc.AcceptPreShutdown | svc.AcceptShutdown,
	}

	for {
		select {
		case err := <-errc:
			return svcErrToCode(err)
		case req := <-r:
			switch req.Cmd {
			case svc.Interrogate:
				s <- req.CurrentStatus
			case svc.Stop: // manual stop
				cancel(errShuttingDown)
				s <- svc.Status{State: svc.StopPending}
				// timeout is not necessary, controlled by service manager limits
				return svcErrToCode(<-errc)
			case svc.PreShutdown: // going to power off
				cancel(errShuttingDown)
				s <- svc.Status{State: svc.StopPending}
			case svc.Shutdown: // powering off
				// timeout is not necessary because Windows can kill service by itself
				// HKEY_LOCAL_MACHINE\SYSTEM\CurrentControlSet\Control\WaitToKillServiceTimeout
				return svcErrToCode(<-errc)
			}
		}
	}
}

func svcErrToCode(err error) (bool, uint32) {
	if err == nil {
		return false, 0
	}

	if errors.Is(err, errShuttingDown) {
		return false, 0 // may be returned, but it's expected
	}

	type exitCoder interface {
		error
		kong.ExitCoder
	}

	if kExitCoder, ok := errors.AsType[exitCoder](err); ok {
		return true, uint32(kExitCoder.ExitCode()) // specific for Kong
	}

	return false, 1 // generic error
}
