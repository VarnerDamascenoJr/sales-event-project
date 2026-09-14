package messaging

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/rabbitmq/amqp091-go"
	"github.com/varner/sales-event-project/internal/correlation"
	"github.com/varner/sales-event-project/internal/metrics"
	"github.com/varner/sales-event-project/internal/observability"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

var (
	ErrPublishNacked   = errors.New("rabbitmq publish was not acknowledged")
	ErrPublishReturned = errors.New("rabbitmq publish was returned")
)

type amqpChannel interface {
	PublishWithContext(ctx context.Context, exchange string, key string, mandatory bool, immediate bool, msg amqp091.Publishing) error
	Qos(prefetchCount int, prefetchSize int, global bool) error
	Consume(queue string, consumer string, autoAck bool, exclusive bool, noLocal bool, noWait bool, args amqp091.Table) (<-chan amqp091.Delivery, error)
	Close() error
}

type RabbitMQ struct {
	conn                 *amqp091.Connection
	channel              amqpChannel
	exchange             string
	publishConfirmations <-chan amqp091.Confirmation
	publishReturns       <-chan amqp091.Return
	publishMu            sync.Mutex
}

type PublishReturnError struct {
	Exchange   string
	RoutingKey string
	ReplyCode  uint16
	ReplyText  string
}

func (e PublishReturnError) Error() string {
	return fmt.Sprintf("rabbitmq publish returned: exchange=%q routing_key=%q reply_code=%d reply_text=%q", e.Exchange, e.RoutingKey, e.ReplyCode, e.ReplyText)
}

func (e PublishReturnError) Unwrap() error {
	return ErrPublishReturned
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

	if err := ch.Confirm(false); err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return nil, err
	}

	confirmations := ch.NotifyPublish(make(chan amqp091.Confirmation, 1))
	returns := ch.NotifyReturn(make(chan amqp091.Return, 1))

	return &RabbitMQ{
		conn:                 conn,
		channel:              ch,
		exchange:             exchange,
		publishConfirmations: confirmations,
		publishReturns:       returns,
	}, nil
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

func (r *RabbitMQ) waitForPublishOutcome(ctx context.Context, routingKey string) error {
	if err := r.pollReturnedPublish(ctx, routingKey); err != nil {
		return err
	}

	select {
	case returned := <-r.publishReturns:
		returnErr := publishReturnError(returned)
		if err := r.waitForPublishConfirmation(ctx, routingKey); err != nil {
			return errors.Join(returnErr, err)
		}
		return returnErr
	case confirmation := <-r.publishConfirmations:
		if !confirmation.Ack {
			return fmt.Errorf("%w: exchange=%q routing_key=%q delivery_tag=%d", ErrPublishNacked, r.exchange, routingKey, confirmation.DeliveryTag)
		}
		return nil
	case <-ctx.Done():
		return fmt.Errorf("wait for rabbitmq publish confirmation: %w", ctx.Err())
	}
}

func (r *RabbitMQ) pollReturnedPublish(ctx context.Context, routingKey string) error {
	select {
	case returned := <-r.publishReturns:
		returnErr := publishReturnError(returned)
		if err := r.waitForPublishConfirmation(ctx, routingKey); err != nil {
			return errors.Join(returnErr, err)
		}
		return returnErr
	default:
		return nil
	}
}

func (r *RabbitMQ) waitForPublishConfirmation(ctx context.Context, routingKey string) error {
	select {
	case confirmation := <-r.publishConfirmations:
		if !confirmation.Ack {
			return fmt.Errorf("%w: exchange=%q routing_key=%q delivery_tag=%d", ErrPublishNacked, r.exchange, routingKey, confirmation.DeliveryTag)
		}
		return nil
	case <-ctx.Done():
		return fmt.Errorf("wait for rabbitmq publish confirmation after return: %w", ctx.Err())
	}
}

func (r *RabbitMQ) drainPublishNotifications() {
	for {
		select {
		case <-r.publishConfirmations:
		case <-r.publishReturns:
		default:
			return
		}
	}
}

func publishReturnError(returned amqp091.Return) error {
	return PublishReturnError{
		Exchange:   returned.Exchange,
		RoutingKey: returned.RoutingKey,
		ReplyCode:  returned.ReplyCode,
		ReplyText:  returned.ReplyText,
	}
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
