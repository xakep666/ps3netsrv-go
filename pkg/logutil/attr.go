package logutil

import (
	"fmt"
	"log/slog"
)

func ErrorAttr(err error) slog.Attr {
	return slog.Attr{
		Key:   "error",
		Value: slog.AnyValue(err),
	}
}

func StringerAttr(key string, value fmt.Stringer) slog.Attr {
	return slog.Attr{
		Key:   key,
		Value: slog.AnyValue(stringerToValuer{value}),
	}
}

type stringerToValuer struct {
	fmt.Stringer
}

var _ slog.LogValuer = (*stringerToValuer)(nil)

func (sv stringerToValuer) LogValue() slog.Value {
	return slog.StringValue(sv.String())
}
