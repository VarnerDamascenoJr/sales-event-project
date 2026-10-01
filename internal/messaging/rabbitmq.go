package messaging

import (
	"context"
	"sync"

	"github.com/rabbitmq/amqp091-go"
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

func (r *RabbitMQ) Close() error {
	if r.channel != nil {
		_ = r.channel.Close()
	}
	if r.conn != nil {
		return r.conn.Close()
	}
	return nil
}
