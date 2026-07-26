package systemlog

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"strings"
	"sync"

	"golang.org/x/sys/windows/svc/eventlog"
)

const logSrc = "PS3NetSrv-Go" // match naming convention of other log sources

var eventLog = sync.OnceValues(func() (*eventlog.Log, error) {
	elog, err := eventlog.Open(logSrc)
	if err != nil {
		return nil, fmt.Errorf("%w, probably system log facility not installed, use 'svc install' command", err)
	}
	return elog, nil
})

func EventLog() (*eventlog.Log, error) {
	return eventLog()
}

func Install() error {
	err := eventlog.InstallAsEventCreate(logSrc, eventlog.Info|eventlog.Warning|eventlog.Error)
	if err != nil {
		// make it recognizeable
		if strings.HasSuffix(err.Error(), "registry key already exists") {
			return fs.ErrExist
		}
		return err
	}
	return nil
}

func Uninstall() error {
	return eventlog.Remove(logSrc)
}

func SystemLogHandler(level slog.Level) (slog.Handler, error) {
	elog, err := eventLog()
	if err != nil {
		return nil, err
	}

	return newLogSinkHandler(
		func(out io.Writer) (slog.Handler, error) {
			return slog.NewTextHandler(out, &slog.HandlerOptions{
				Level: level,
				ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
					if a.Key == slog.TimeKey {
						return slog.Attr{} // remove Time attribute because it's already logged
					}

					if a.Key == slog.LevelKey {
						ll, ok := a.Value.Any().(slog.Level)
						if !ok {
							return a
						}
						switch ll {
						case slog.LevelInfo,
							slog.LevelWarn,
							slog.LevelError:
							return slog.Attr{} // remove Level attribute for known levels because it's already logged properly
						}
					}

					return a
				},
			}), nil
		},
		new(strings.Builder),
		(*strings.Builder).String,
		logSinkFunc[string](func(_ context.Context, rec slog.Record, formatted string) (err error) {
			switch rec.Level {
			case slog.LevelInfo:
				return elog.Info(1, formatted)
			case slog.LevelWarn:
				return elog.Warning(1, formatted)
			case slog.LevelError:
				return elog.Error(1, formatted)
			default:
				return elog.Info(2, formatted) // for levels natively not supported by Windows EventLog using other event id
			}
		}),
	)
}
