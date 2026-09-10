package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/varner/sales-event-project/internal/config"
	"github.com/varner/sales-event-project/internal/database"
	"github.com/varner/sales-event-project/internal/notification"
	"github.com/varner/sales-event-project/internal/observability"
	"github.com/varner/sales-event-project/internal/worker"
)

func main() {
	cfg := config.Load()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	shutdownTelemetry := func(context.Context) error { return nil }
	if cfg.OTelEnabled {
		var err error
		shutdownTelemetry, err = observability.ConfigureTelemetry(ctx, "sales-event-email-retry-worker", cfg.AppEnv, cfg.OTelEndpoint)
		if err != nil {
			slog.Error("configure telemetry failed", "error", err)
			os.Exit(1)
		}
	} else {
		observability.ConfigureLogger("sales-event-email-retry-worker", cfg.AppEnv)
	}
	defer func() { _ = shutdownTelemetry(context.Background()) }()

	db, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("connect postgres failed", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	processor := worker.NewTicketDeliveryProcessor(db, notification.NewSender(notification.SMTPConfig{
		Host:     cfg.SMTPHost,
		Port:     cfg.SMTPPort,
		Username: cfg.SMTPUsername,
		Password: cfg.SMTPPassword,
		From:     cfg.SMTPFrom,
	}))

	observability.StartMetricsServer(cfg.EmailRetryMetricsHost, cfg.EmailRetryMetricsPort, "email-retry-worker")
	if !cfg.EmailRetryEnabled {
		slog.Info("email retry worker disabled")
		<-ctx.Done()
		slog.Info("email retry worker stopped")
		return
	}

	slog.Info("email retry worker started", "interval", cfg.EmailRetryInterval)
	processor.RunEmailRetries(ctx, cfg.EmailRetryInterval, 10)
}
