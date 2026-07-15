package osutil

import (
	"errors"
	"log/slog"
	"os"
	"strings"

	slogsyslog "github.com/mocheryl/slog-syslog"
	slogjournal "github.com/systemd/slog-journal"
)

// SystemLogHandler initializes systemd-journald/syslog native logging handler if it's present on system.
func SystemLogHandler(level slog.Level) (slog.Handler, error) {
	// check if systemd-journald is running
	_, err := os.Stat("/run/systemd/journal/socket")
	if err == nil {
		var lv slogjournal.LevelVar
		lv.Set(level)
		return slogjournal.NewHandler(&slogjournal.Options{
			Level: &lv,
			ReplaceGroup: func(k string) string {
				return strings.ReplaceAll(strings.ToUpper(k), "-", "_")
			},
			ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
				a.Key = strings.ReplaceAll(strings.ToUpper(a.Key), "-", "_")
				return a
			},
		})
	}

	// try local syslog as it made in stdlib
	logTypes := []string{"unixgram", "unix"}
	logPaths := []string{"/dev/log", "/var/run/syslog", "/var/run/log"}
	for _, network := range logTypes {
		for _, path := range logPaths {
			h, err := slogsyslog.New(&slogsyslog.Options{
				Level:    level,
				Network:  network,
				Address:  path,
				Facility: slogsyslog.Daemon,
			})
			if err == nil {
				return h, nil
			}
		}
	}

	return nil, errors.ErrUnsupported
}
