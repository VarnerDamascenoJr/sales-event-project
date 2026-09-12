//go:build integration

package integration_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/varner/sales-event-project/internal/messaging"
)

const defaultRabbitMQURL = "amqp://guest:guest@localhost:5672/"

func TestRabbitMQPublishFailsForUnroutedMandatoryMessage(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	broker, err := messaging.ConnectWithRetry(ctx, rabbitMQURL(), "sales.exchange", map[string]string{}, 3, time.Second)
	if err != nil {
		t.Fatalf("connect rabbitmq: %v", err)
	}
	defer func() {
		if err := broker.Close(); err != nil {
			t.Fatalf("close rabbitmq: %v", err)
		}
	}()

	err = broker.PublishJSON(ctx, "integration.unbound", map[string]string{"event": "unrouted"})

	if !errors.Is(err, messaging.ErrPublishReturned) {
		t.Fatalf("expected unrouted publish to be returned, got %v", err)
	}
}

func rabbitMQURL() string {
	if value := os.Getenv("RABBITMQ_URL"); value != "" {
		return value
	}
	return defaultRabbitMQURL
}
