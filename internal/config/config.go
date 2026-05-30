package config

import (
	"log/slog"
	"os"
	"strconv"
	"time"
)

type Config struct {
	AppEnv                      string
	APIPort                     string
	WorkerMetricsPort           string
	DatabaseURL                 string
	RabbitMQURL                 string
	RabbitMQExchange            string
	SalesCreatedQueue           string
	SaleCompletedQueue          string
	SMTPHost                    string
	SMTPPort                    string
	SMTPUsername                string
	SMTPPassword                string
	SMTPFrom                    string
	RetentionEnabled            bool
	RetentionInterval           time.Duration
	RetentionPublishedOutboxAge time.Duration
	RetentionSentEmailMaxAge    time.Duration
}

func Load() Config {
	return Config{
		AppEnv:                      getEnv("APP_ENV", "development"),
		APIPort:                     getEnv("API_PORT", "8080"),
		WorkerMetricsPort:           getEnv("WORKER_METRICS_PORT", "9091"),
		DatabaseURL:                 getEnv("DATABASE_URL", "postgres://sales:sales@localhost:5432/sales_event?sslmode=disable"),
		RabbitMQURL:                 getEnv("RABBITMQ_URL", "amqp://guest:guest@localhost:5672/"),
		RabbitMQExchange:            getEnv("RABBITMQ_EXCHANGE", "sales.exchange"),
		SalesCreatedQueue:           getEnv("SALES_CREATED_QUEUE", "sales.created.queue"),
		SaleCompletedQueue:          getEnv("SALE_COMPLETED_QUEUE", "sale.completed.queue"),
		SMTPHost:                    getEnv("SMTP_HOST", ""),
		SMTPPort:                    getEnv("SMTP_PORT", "587"),
		SMTPUsername:                getEnv("SMTP_USERNAME", ""),
		SMTPPassword:                getEnv("SMTP_PASSWORD", ""),
		SMTPFrom:                    getEnv("SMTP_FROM", ""),
		RetentionEnabled:            getBoolEnv("RETENTION_ENABLED", true),
		RetentionInterval:           getDurationEnv("RETENTION_INTERVAL", 24*time.Hour),
		RetentionPublishedOutboxAge: getDurationEnv("RETENTION_PUBLISHED_OUTBOX_MAX_AGE", 30*24*time.Hour),
		RetentionSentEmailMaxAge:    getDurationEnv("RETENTION_SENT_EMAIL_MAX_AGE", 90*24*time.Hour),
	}
}

func getEnv(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	return value
}

func getBoolEnv(key string, fallback bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	parsed, err := strconv.ParseBool(value)
	if err != nil {
		slog.Warn("invalid boolean env value; using fallback", "key", key, "value", value, "fallback", fallback)
		return fallback
	}
	return parsed
}

func getDurationEnv(key string, fallback time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	parsed, err := time.ParseDuration(value)
	if err != nil {
		slog.Warn("invalid duration env value; using fallback", "key", key, "value", value, "fallback", fallback)
		return fallback
	}
	return parsed
}
