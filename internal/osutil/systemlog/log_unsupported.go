//go:build !linux

package systemlog

import (
	"errors"
	"log/slog"
)

func SystemLogHandler(level slog.Level) (slog.Handler, error) {
	return nil, errors.ErrUnsupported
}
