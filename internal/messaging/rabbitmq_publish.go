package messaging

import (
	"context"
	"encoding/json"
	"time"

	"github.com/rabbitmq/amqp091-go"
	"github.com/varner/sales-event-project/internal/metrics"
	"github.com/varner/sales-event-project/internal/observability"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

func (r *RabbitMQ) PublishJSON(ctx context.Context, routingKey string, value any) error {
	r.publishMu.Lock()
	defer r.publishMu.Unlock()

	ctx, span := otel.Tracer("github.com/varner/sales-event-project/messaging").Start(ctx, "rabbitmq publish "+routingKey)
	span.SetAttributes(append(observability.CorrelationAttributes(ctx),
		attribute.String("messaging.system", "rabbitmq"),
		attribute.String("messaging.operation", "publish"),
		attribute.String("messaging.destination.name", r.exchange),
		attribute.String("messaging.rabbitmq.routing_key", routingKey),
	)...)
	var publishErr error
	defer func() {
		observability.EndSpan(span, publishErr)
	}()

	body, err := json.Marshal(value)
	if err != nil {
		publishErr = err
		return err
	}

	headers := publishingHeaders(ctx)

	r.drainPublishNotifications()

	if err := r.channel.PublishWithContext(ctx, r.exchange, routingKey, true, false, amqp091.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp091.Persistent,
		Timestamp:    time.Now().UTC(),
		Headers:      headers,
		Body:         body,
	}); err != nil {
		publishErr = err
		metrics.EventPublishedTotal.WithLabelValues(routingKey, "failed").Inc()
		return err
	}

	if err := r.waitForPublishOutcome(ctx, routingKey); err != nil {
		publishErr = err
		metrics.EventPublishedTotal.WithLabelValues(routingKey, "failed").Inc()
		return err
	}

	metrics.EventPublishedTotal.WithLabelValues(routingKey, "published").Inc()
	return nil
}
