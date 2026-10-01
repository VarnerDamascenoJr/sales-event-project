package observability

import (
	"log/slog"
	"os"
)

func ConfigureLogger(service string, environment string, telemetryHandler ...slog.Handler) {
	jsonHandler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
		ReplaceAttr: func(_ []string, attr slog.Attr) slog.Attr {
			return redactAttr(attr)
		},
	})
	handler := slog.Handler(jsonHandler)
	if len(telemetryHandler) > 0 && telemetryHandler[0] != nil {
		handler = multiHandler{handlers: []slog.Handler{jsonHandler, telemetryHandler[0]}}
	}
	handler = redactingHandler{handler: handler}
	handler = correlationHandler{handler: handler}

	logger := slog.New(handler).With(
		"service", service,
		"environment", environment,
	)
	slog.SetDefault(logger)
}
