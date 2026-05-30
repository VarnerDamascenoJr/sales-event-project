package observability

import (
	"log/slog"
	"os"
)

func ConfigureLogger(service string, environment string) {
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
		ReplaceAttr: func(_ []string, attr slog.Attr) slog.Attr {
			switch attr.Key {
			case "recipient", "customer_email", "email", "smtp_password", "api_key", "webhook_secret", "signature":
				attr.Value = slog.StringValue("[redacted]")
			}
			return attr
		},
	})

	logger := slog.New(handler).With(
		"service", service,
		"environment", environment,
	)
	slog.SetDefault(logger)
}
