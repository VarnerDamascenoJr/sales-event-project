package observability

import (
	"context"
	"log/slog"
)

var sensitiveLogAttrs = map[string]struct{}{
	"api_key":        {},
	"customer_email": {},
	"email":          {},
	"recipient":      {},
	"signature":      {},
	"smtp_password":  {},
	"webhook_secret": {},
}

type redactingHandler struct {
	handler slog.Handler
}

func (h redactingHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.handler.Enabled(ctx, level)
}

func (h redactingHandler) Handle(ctx context.Context, record slog.Record) error {
	redacted := slog.NewRecord(record.Time, record.Level, record.Message, record.PC)
	record.Attrs(func(attr slog.Attr) bool {
		redacted.AddAttrs(redactAttr(attr))
		return true
	})
	return h.handler.Handle(ctx, redacted)
}

func (h redactingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	redacted := make([]slog.Attr, 0, len(attrs))
	for _, attr := range attrs {
		redacted = append(redacted, redactAttr(attr))
	}
	return redactingHandler{handler: h.handler.WithAttrs(redacted)}
}

func (h redactingHandler) WithGroup(name string) slog.Handler {
	return redactingHandler{handler: h.handler.WithGroup(name)}
}

func redactAttr(attr slog.Attr) slog.Attr {
	if _, ok := sensitiveLogAttrs[attr.Key]; ok {
		return slog.String(attr.Key, "[redacted]")
	}

	if attr.Value.Kind() != slog.KindGroup {
		return attr
	}

	group := attr.Value.Group()
	redacted := make([]slog.Attr, 0, len(group))
	for _, groupAttr := range group {
		redacted = append(redacted, redactAttr(groupAttr))
	}
	args := make([]any, 0, len(redacted))
	for _, attr := range redacted {
		args = append(args, attr)
	}
	return slog.Group(attr.Key, args...)
}
