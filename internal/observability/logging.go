package observability

import (
	"context"
	"errors"
	"log/slog"
	"os"
)

func ConfigureLogger(service string, environment string, telemetryHandler ...slog.Handler) {
	jsonHandler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
		ReplaceAttr: func(_ []string, attr slog.Attr) slog.Attr {
			switch attr.Key {
			case "recipient", "customer_email", "email", "smtp_password", "api_key", "webhook_secret", "signature":
				attr.Value = slog.StringValue("[redacted]")
			}
			return attr
		},
	})
	handler := slog.Handler(jsonHandler)
	if len(telemetryHandler) > 0 && telemetryHandler[0] != nil {
		handler = multiHandler{handlers: []slog.Handler{jsonHandler, telemetryHandler[0]}}
	}
	handler = redactingHandler{handler: handler}

	logger := slog.New(handler).With(
		"service", service,
		"environment", environment,
	)
	slog.SetDefault(logger)
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
	switch attr.Key {
	case "recipient", "customer_email", "email", "smtp_password", "api_key", "webhook_secret", "signature":
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
	return slog.Group(attr.Key, redacted...)
}

type multiHandler struct {
	handlers []slog.Handler
}

func (h multiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, handler := range h.handlers {
		if handler.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (h multiHandler) Handle(ctx context.Context, record slog.Record) error {
	var err error
	for _, handler := range h.handlers {
		if handler.Enabled(ctx, record.Level) {
			err = errors.Join(err, handler.Handle(ctx, record))
		}
	}
	return err
}

func (h multiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	handlers := make([]slog.Handler, 0, len(h.handlers))
	for _, handler := range h.handlers {
		handlers = append(handlers, handler.WithAttrs(attrs))
	}
	return multiHandler{handlers: handlers}
}

func (h multiHandler) WithGroup(name string) slog.Handler {
	handlers := make([]slog.Handler, 0, len(h.handlers))
	for _, handler := range h.handlers {
		handlers = append(handlers, handler.WithGroup(name))
	}
	return multiHandler{handlers: handlers}
}
