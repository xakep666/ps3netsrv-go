package systemlog

import (
	"context"
	"io"
	"log/slog"
	"sync"
)

type byteseq interface {
	~string | ~[]byte
}

type logSink[T byteseq] interface {
	Sink(ctx context.Context, rec slog.Record, formatted T) error
}

type logSinkFunc[T byteseq] func(ctx context.Context, rec slog.Record, formatted T) error

func (f logSinkFunc[T]) Sink(ctx context.Context, rec slog.Record, formatted T) error {
	return f(ctx, rec, formatted)
}

type sinkContainer interface {
	io.Writer
	Reset()
}

// logSinkHandler is a wrapper over any upstream handler that accepts [io.Writer] as sink
// this wrapper allows to access formatted log record and raw [slog.Record] at the same time which is useful for system/ffi loggers
// upstream handler must be synchronous (standard [slog.TextHandler] and [slog.JSONHandler] are)
type logSinkHandler[C sinkContainer, T byteseq] struct {
	upstream slog.Handler

	mu        *sync.Mutex
	container C
	accessor  func(C) T
	sink      logSink[T]
}

func newLogSinkHandler[C sinkContainer, T byteseq](
	makeHandler func(out io.Writer) (slog.Handler, error),
	container C,
	accessor func(C) T,
	sink logSink[T],
) (slog.Handler, error) {
	upstream, err := makeHandler(container)
	if err != nil {
		return nil, err
	}

	return &logSinkHandler[C, T]{
		upstream:  upstream,
		mu:        new(sync.Mutex),
		container: container,
		accessor:  accessor,
		sink:      sink,
	}, nil
}

func (h *logSinkHandler[C, T]) Enabled(ctx context.Context, level slog.Level) bool {
	return h.upstream.Enabled(ctx, level)
}

func (h *logSinkHandler[C, T]) Handle(ctx context.Context, rec slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if err := h.upstream.Handle(ctx, rec); err != nil {
		return err
	}

	defer h.container.Reset()

	return h.sink.Sink(ctx, rec, h.accessor(h.container))
}

func (h *logSinkHandler[C, T]) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &logSinkHandler[C, T]{
		upstream:  h.upstream.WithAttrs(attrs),
		mu:        h.mu,
		container: h.container,
		accessor:  h.accessor,
		sink:      h.sink,
	}
}

func (h *logSinkHandler[C, T]) WithGroup(name string) slog.Handler {
	return &logSinkHandler[C, T]{
		upstream:  h.upstream.WithGroup(name),
		mu:        h.mu,
		container: h.container,
		accessor:  h.accessor,
		sink:      h.sink,
	}
}
