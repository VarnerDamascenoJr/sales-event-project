package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/varner/sales-event-project/internal/config"
	"github.com/varner/sales-event-project/internal/database"
	"github.com/varner/sales-event-project/internal/events"
	"github.com/varner/sales-event-project/internal/httpapi"
	"github.com/varner/sales-event-project/internal/messaging"
	"github.com/varner/sales-event-project/internal/observability"
)

func main() {
	cfg := config.Load()
	observability.ConfigureLogger("sales-event-api", cfg.AppEnv)
	ctx := context.Background()

	db, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("connect postgres failed", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	broker, err := messaging.ConnectWithRetry(ctx, cfg.RabbitMQURL, cfg.RabbitMQExchange, map[string]string{
		events.SaleCreatedRoutingKey:   cfg.SalesCreatedQueue,
		events.SaleCompletedRoutingKey: cfg.SaleCompletedQueue,
	}, 20, 2*time.Second)
	if err != nil {
		slog.Error("connect rabbitmq failed", "error", err)
		os.Exit(1)
	}
	defer broker.Close()

	router := httpapi.NewRouter(httpapi.RouterDeps{Broker: broker, DB: db})
	server := &http.Server{
		Addr:              ":" + cfg.APIPort,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		slog.Info("api listening", "port", cfg.APIPort)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("api listen failed", "error", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		slog.Error("shutdown api failed", "error", err)
	}
}
