package server

import (
	"context"
	"log/slog"

	"github.com/xakep666/ps3netsrv-go/internal/logutil"
)

// SlogContextHandler wraps slog.Handler to inject attributes from Context.
type SlogContextHandler[StateT any] struct {
	slog.Handler
}

func (h *SlogContextHandler[StateT]) Handle(ctx context.Context, rec slog.Record) error {
	if sctx, ok := ctx.(*Context[StateT]); ok {
		rec.AddAttrs(logutil.StringerAttr("remote", sctx.RemoteAddr))
	}

	return h.Handler.Handle(ctx, rec)
}

func (h *SlogContextHandler[StateT]) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &SlogContextHandler[StateT]{Handler: h.Handler.WithAttrs(attrs)}
}

func (h *SlogContextHandler[StateT]) WithGroup(name string) slog.Handler {
	return &SlogContextHandler[StateT]{Handler: h.Handler.WithGroup(name)}
}
