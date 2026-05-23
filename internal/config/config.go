package config

import (
	"os"
)

type Config struct {
	AppEnv             string
	APIPort            string
	WorkerMetricsPort  string
	DatabaseURL        string
	RabbitMQURL        string
	RabbitMQExchange   string
	SalesCreatedQueue  string
	SaleCompletedQueue string
	SMTPHost           string
	SMTPPort           string
	SMTPUsername       string
	SMTPPassword       string
	SMTPFrom           string
}

func Load() Config {
	return Config{
		AppEnv:             getEnv("APP_ENV", "development"),
		APIPort:            getEnv("API_PORT", "8080"),
		WorkerMetricsPort:  getEnv("WORKER_METRICS_PORT", "9091"),
		DatabaseURL:        getEnv("DATABASE_URL", "postgres://sales:sales@localhost:5432/sales_event?sslmode=disable"),
		RabbitMQURL:        getEnv("RABBITMQ_URL", "amqp://guest:guest@localhost:5672/"),
		RabbitMQExchange:   getEnv("RABBITMQ_EXCHANGE", "sales.exchange"),
		SalesCreatedQueue:  getEnv("SALES_CREATED_QUEUE", "sales.created.queue"),
		SaleCompletedQueue: getEnv("SALE_COMPLETED_QUEUE", "sale.completed.queue"),
		SMTPHost:           getEnv("SMTP_HOST", ""),
		SMTPPort:           getEnv("SMTP_PORT", "587"),
		SMTPUsername:       getEnv("SMTP_USERNAME", ""),
		SMTPPassword:       getEnv("SMTP_PASSWORD", ""),
		SMTPFrom:           getEnv("SMTP_FROM", ""),
	}
}

func getEnv(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	return value
}
