package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/varner/sales-event-project/internal/config"
	"github.com/varner/sales-event-project/internal/database"
	"github.com/varner/sales-event-project/internal/messaging"
	"github.com/varner/sales-event-project/internal/worker"
)

func main() {
	cfg := config.Load()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	db, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("connect postgres: %v", err)
	}
	defer db.Close()

	broker, err := messaging.ConnectWithRetry(ctx, cfg.RabbitMQURL, cfg.RabbitMQExchange, cfg.SalesCreatedQueue, 20, 2*time.Second)
	if err != nil {
		log.Fatalf("connect rabbitmq: %v", err)
	}
	defer broker.Close()

	deliveries, err := broker.Consume(cfg.SalesCreatedQueue)
	if err != nil {
		log.Fatalf("consume sales queue: %v", err)
	}

	processor := worker.NewSalesProcessor(db)
	log.Printf("worker consuming queue=%s", cfg.SalesCreatedQueue)
	startMetricsServer(cfg.WorkerMetricsPort)

	for {
		select {
		case <-ctx.Done():
			log.Println("worker stopped")
			return
		case delivery, ok := <-deliveries:
			if !ok {
				log.Println("delivery channel closed")
				return
			}

			processCtx, processCancel := context.WithTimeout(ctx, 15*time.Second)
			if err := processor.Handle(processCtx, delivery); err != nil {
				processCancel()
				log.Printf("process message failed: %v", err)
				_ = delivery.Nack(false, false)
				continue
			}
			processCancel()

			if err := delivery.Ack(false); err != nil {
				log.Printf("ack message: %v", err)
			}
		}
	}
}

func startMetricsServer(port string) {
	router := gin.New()
	router.Use(gin.Recovery())
	router.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	router.GET("/metrics", gin.WrapH(promhttp.Handler()))

	server := &http.Server{
		Addr:              ":" + port,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Printf("worker metrics listening on :%s", port)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("worker metrics server: %v", err)
		}
	}()
}
