package systemlog

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"runtime"
	"unsafe"

	"github.com/xakep666/ps3netsrv-go/internal/osutil/macoslibs"
)

// SystemLogHandler initializes native logging handler via syslog(3) libc call.
// We're using it instead of modern 'os_log' (unified logging interface)
// because that one is a macro which requires to use CGo or external library.
// It is possible to pull and call '__os_log_impl' but it's internal api that can break at any moment.
// syslog(3) is a POSIX compatible method that calls 'os_log' under the hood on macOS.
func SystemLogHandler(level slog.Level) (slog.Handler, error) {
	libc, err := macoslibs.GetLibc()
	if err != nil {
		return nil, err
	}

	libc.OpenLog("com.xakep666.ps3netsrv-go\x00", macoslibs.LOG_PID, macoslibs.LOG_USER)

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
						case slog.LevelDebug,
							slog.LevelInfo,
							slog.LevelWarn,
							slog.LevelError:
							return slog.Attr{} // remove Level attribute for known levels because it's already logged properly
						}
					}

					return a
				},
			}), nil
		},
		new(bytes.Buffer),
		func(b *bytes.Buffer) []byte {
			_ = b.WriteByte(0) // finalize string with 0-byte to make it C string
			return b.Bytes()
		},
		logSinkFunc[[]byte](func(_ context.Context, rec slog.Record, formatted []byte) (err error) {
			defer func() {
				if r := recover(); r != nil {
					err = fmt.Errorf("syslog panic: %v", err)
				}
			}()
			priority := macoslibs.LOG_NOTICE
			switch rec.Level {
			case slog.LevelDebug:
				priority = macoslibs.LOG_DEBUG
			case slog.LevelInfo:
				priority = macoslibs.LOG_INFO
			case slog.LevelWarn:
				priority = macoslibs.LOG_WARNING
			case slog.LevelError:
				priority = macoslibs.LOG_ERR
			}

			libc.Syslog1(priority, "%s\x00", uintptr(unsafe.Pointer(unsafe.SliceData(formatted))))
			runtime.KeepAlive(formatted)

			return nil
		}),
	)
}
