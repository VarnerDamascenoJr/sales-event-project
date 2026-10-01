package observability

import (
	"context"
	"log/slog"
)

type correlationHandler struct {
	handler slog.Handler
}

func (h correlationHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.handler.Enabled(ctx, level)
}

func (h correlationHandler) Handle(ctx context.Context, record slog.Record) error {
	attrs := LogAttrs(ctx)
	for index := 0; index < len(attrs); index += 2 {
		record.AddAttrs(slog.Any(attrs[index].(string), attrs[index+1]))
	}
	return h.handler.Handle(ctx, record)
}

func (h correlationHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return correlationHandler{handler: h.handler.WithAttrs(attrs)}
}

func (h correlationHandler) WithGroup(name string) slog.Handler {
	return correlationHandler{handler: h.handler.WithGroup(name)}
}
