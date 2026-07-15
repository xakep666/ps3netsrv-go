//go:build !linux

package osutil

import (
	"errors"
	"log/slog"
)

func SystemLogHandler(level slog.Level) (slog.Handler, error) {
	return nil, errors.ErrUnsupported
}
