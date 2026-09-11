package messaging

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/rabbitmq/amqp091-go"
	"github.com/varner/sales-event-project/internal/correlation"
	"github.com/varner/sales-event-project/internal/metrics"
	"github.com/varner/sales-event-project/internal/observability"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

type RabbitMQ struct {
	conn     *amqp091.Connection
	channel  *amqp091.Channel
	exchange string
}

func Connect(url, exchange string, queues map[string]string) (*RabbitMQ, error) {
	conn, err := amqp091.Dial(url)
	if err != nil {
		return nil, err
	}

	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, err
	}

	if err := ch.ExchangeDeclare(exchange, "topic", true, false, false, false, nil); err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return nil, err
	}

	if err := ch.ExchangeDeclare(exchange+".dlx", "topic", true, false, false, false, nil); err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return nil, err
	}

	if _, err := ch.QueueDeclare("sales.dlq", true, false, false, false, nil); err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return nil, err
	}

	if err := ch.QueueBind("sales.dlq", "#", exchange+".dlx", false, nil); err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return nil, err
	}

	for routingKey, queue := range queues {
		if _, err := ch.QueueDeclare(queue, true, false, false, false, amqp091.Table{
			"x-dead-letter-exchange": exchange + ".dlx",
		}); err != nil {
			_ = ch.Close()
			_ = conn.Close()
			return nil, err
		}

		if err := ch.QueueBind(queue, routingKey, exchange, false, nil); err != nil {
			_ = ch.Close()
			_ = conn.Close()
			return nil, err
		}
	}

	return &RabbitMQ{conn: conn, channel: ch, exchange: exchange}, nil
}

func ConnectWithRetry(ctx context.Context, url, exchange string, queues map[string]string, attempts int, delay time.Duration) (*RabbitMQ, error) {
	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		broker, err := Connect(url, exchange, queues)
		if err == nil {
			return broker, nil
		}

		lastErr = err
		select {
		case <-ctx.Done():
			return nil, errors.Join(ctx.Err(), lastErr)
		case <-time.After(delay):
		}
	}

	return nil, lastErr
}

func (r *RabbitMQ) PublishJSON(ctx context.Context, routingKey string, value any) error {
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

	if err := r.channel.PublishWithContext(ctx, r.exchange, routingKey, false, false, amqp091.Publishing{
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

	metrics.EventPublishedTotal.WithLabelValues(routingKey, "published").Inc()
	return nil
}

func publishingHeaders(ctx context.Context) amqp091.Table {
	headers := amqp091.Table{}
	observability.InjectAMQPContext(ctx, headers)
	if metadata, ok := correlation.FromContext(ctx); ok {
		if metadata.RequestID != "" {
			headers[correlation.AMQPRequestIDHeader] = metadata.RequestID
		}
		if metadata.CorrelationID != "" {
			headers[correlation.AMQPCorrelationIDHeader] = metadata.CorrelationID
		}
		if metadata.TransactionID != "" {
			headers[correlation.AMQPTransactionIDHeader] = metadata.TransactionID
		}
	}
	return headers
}

func (r *RabbitMQ) Consume(queue string) (<-chan amqp091.Delivery, error) {
	if err := r.channel.Qos(10, 0, false); err != nil {
		return nil, err
	}

	return r.channel.Consume(queue, "", false, false, false, false, nil)
}

func (r *RabbitMQ) Close() error {
	if r.channel != nil {
		_ = r.channel.Close()
	}
	if r.conn != nil {
		return r.conn.Close()
	}
	return nil
}
