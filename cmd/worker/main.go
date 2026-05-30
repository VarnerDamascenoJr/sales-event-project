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

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/varner/sales-event-project/internal/config"
	"github.com/varner/sales-event-project/internal/database"
	"github.com/varner/sales-event-project/internal/events"
	"github.com/varner/sales-event-project/internal/messaging"
	"github.com/varner/sales-event-project/internal/metrics"
	"github.com/varner/sales-event-project/internal/notification"
	"github.com/varner/sales-event-project/internal/observability"
	"github.com/varner/sales-event-project/internal/outbox"
	"github.com/varner/sales-event-project/internal/retention"
	"github.com/varner/sales-event-project/internal/worker"
)

func main() {
	cfg := config.Load()
	observability.ConfigureLogger("sales-event-worker", cfg.AppEnv)
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

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

	outboxBroker, err := messaging.ConnectWithRetry(ctx, cfg.RabbitMQURL, cfg.RabbitMQExchange, map[string]string{
		events.SaleCreatedRoutingKey:   cfg.SalesCreatedQueue,
		events.SaleCompletedRoutingKey: cfg.SaleCompletedQueue,
	}, 20, 2*time.Second)
	if err != nil {
		slog.Error("connect rabbitmq for outbox failed", "error", err)
		os.Exit(1)
	}
	defer outboxBroker.Close()

	saleCreatedDeliveries, err := broker.Consume(cfg.SalesCreatedQueue)
	if err != nil {
		slog.Error("consume sales queue failed", "queue", cfg.SalesCreatedQueue, "error", err)
		os.Exit(1)
	}
	saleCompletedDeliveries, err := broker.Consume(cfg.SaleCompletedQueue)
	if err != nil {
		slog.Error("consume ticket delivery queue failed", "queue", cfg.SaleCompletedQueue, "error", err)
		os.Exit(1)
	}

	salesProcessor := worker.NewSalesProcessor(db)
	ticketDeliveryProcessor := worker.NewTicketDeliveryProcessor(db, notification.NewSender(notification.SMTPConfig{
		Host:     cfg.SMTPHost,
		Port:     cfg.SMTPPort,
		Username: cfg.SMTPUsername,
		Password: cfg.SMTPPassword,
		From:     cfg.SMTPFrom,
	}))
	outboxPublisher := outbox.NewPublisher(db, outboxBroker)
	retentionCleaner := retention.NewCleaner(db)
	slog.Info("worker consuming queues", "sales_created_queue", cfg.SalesCreatedQueue, "sale_completed_queue", cfg.SaleCompletedQueue)
	startMetricsServer(cfg.WorkerMetricsHost, cfg.WorkerMetricsPort)
	go outboxPublisher.Run(ctx, time.Second, 10)
	if cfg.EmailRetryEnabled {
		go ticketDeliveryProcessor.RunEmailRetries(ctx, cfg.EmailRetryInterval, 10)
	}
	if cfg.RetentionEnabled {
		go retentionCleaner.Run(ctx, cfg.RetentionInterval, retention.Policy{
			PublishedOutboxMaxAge: cfg.RetentionPublishedOutboxAge,
			SentEmailMaxAge:       cfg.RetentionSentEmailMaxAge,
		})
	}

	for {
		select {
		case <-ctx.Done():
			slog.Info("worker stopped")
			return
		case delivery, ok := <-saleCreatedDeliveries:
			if !ok {
				slog.Warn("sale created delivery channel closed")
				return
			}

			processCtx, processCancel := context.WithTimeout(ctx, 15*time.Second)
			start := time.Now()
			if err := salesProcessor.Handle(processCtx, delivery); err != nil {
				processCancel()
				slog.Error("process sale created message failed", "queue", cfg.SalesCreatedQueue, "error", err)
				_ = delivery.Nack(false, false)
				recordWorkerMessage(cfg.SalesCreatedQueue, "failed", start)
				continue
			}
			processCancel()

			if err := delivery.Ack(false); err != nil {
				slog.Error("ack message failed", "queue", cfg.SalesCreatedQueue, "error", err)
			}
			recordWorkerMessage(cfg.SalesCreatedQueue, "acked", start)
		case delivery, ok := <-saleCompletedDeliveries:
			if !ok {
				slog.Warn("sale completed delivery channel closed")
				return
			}

			processCtx, processCancel := context.WithTimeout(ctx, 30*time.Second)
			start := time.Now()
			if err := ticketDeliveryProcessor.Handle(processCtx, delivery); err != nil {
				processCancel()
				slog.Error("deliver tickets failed", "queue", cfg.SaleCompletedQueue, "error", err)
				_ = delivery.Nack(false, false)
				recordWorkerMessage(cfg.SaleCompletedQueue, "failed", start)
				continue
			}
			processCancel()

			if err := delivery.Ack(false); err != nil {
				slog.Error("ack message failed", "queue", cfg.SaleCompletedQueue, "error", err)
			}
			recordWorkerMessage(cfg.SaleCompletedQueue, "acked", start)
		}
	}
}

func recordWorkerMessage(queue string, status string, start time.Time) {
	metrics.WorkerMessagesProcessedTotal.WithLabelValues(queue, status).Inc()
	metrics.WorkerMessageDuration.WithLabelValues(queue).Observe(time.Since(start).Seconds())
}

func startMetricsServer(host string, port string) {
	router := gin.New()
	router.Use(gin.Recovery())
	router.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	router.GET("/metrics", gin.WrapH(promhttp.Handler()))

	server := &http.Server{
		Addr:              host + ":" + port,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		slog.Info("worker metrics listening", "host", host, "port", port)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("worker metrics server failed", "error", err)
		}
	}()
}
