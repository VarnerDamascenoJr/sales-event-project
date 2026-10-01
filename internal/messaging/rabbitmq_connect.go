package messaging

import (
	"context"
	"errors"
	"time"

	"github.com/rabbitmq/amqp091-go"
)

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
