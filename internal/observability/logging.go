package observability

import (
	"log/slog"
	"os"
)

func ConfigureLogger(service string, environment string) {
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})

	logger := slog.New(handler).With(
		"service", service,
		"environment", environment,
	)
	slog.SetDefault(logger)
}
