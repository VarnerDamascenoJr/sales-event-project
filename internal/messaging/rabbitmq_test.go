package messaging

import (
	"context"
	"errors"
	"testing"

	"github.com/rabbitmq/amqp091-go"
	"github.com/varner/sales-event-project/internal/correlation"
)

func TestPublishingHeadersIncludesCorrelationMetadata(t *testing.T) {
	metadata := correlation.Metadata{
		RequestID:     "req-demo",
		CorrelationID: "corr-demo",
		TransactionID: "sale-demo",
	}
	ctx := correlation.ContextWithMetadata(context.Background(), metadata)

	headers := publishingHeaders(ctx)

	if headers[correlation.AMQPRequestIDHeader] != metadata.RequestID {
		t.Fatalf("expected request id header %q, got %q", metadata.RequestID, headers[correlation.AMQPRequestIDHeader])
	}
	if headers[correlation.AMQPCorrelationIDHeader] != metadata.CorrelationID {
		t.Fatalf("expected correlation id header %q, got %q", metadata.CorrelationID, headers[correlation.AMQPCorrelationIDHeader])
	}
	if headers[correlation.AMQPTransactionIDHeader] != metadata.TransactionID {
		t.Fatalf("expected transaction id header %q, got %q", metadata.TransactionID, headers[correlation.AMQPTransactionIDHeader])
	}
}

func TestPublishingHeadersOmitEmptyCorrelationValues(t *testing.T) {
	ctx := correlation.ContextWithMetadata(context.Background(), correlation.Metadata{
		CorrelationID: "corr-demo",
	})

	headers := publishingHeaders(ctx)

	if _, ok := headers[correlation.AMQPRequestIDHeader]; ok {
		t.Fatal("expected empty request id to be omitted")
	}
	if headers[correlation.AMQPCorrelationIDHeader] != "corr-demo" {
		t.Fatalf("expected correlation id header, got %q", headers[correlation.AMQPCorrelationIDHeader])
	}
	if _, ok := headers[correlation.AMQPTransactionIDHeader]; ok {
		t.Fatal("expected empty transaction id to be omitted")
	}
}

func TestPublishJSONRequiresBrokerConfirmation(t *testing.T) {
	channel := &fakePublishChannel{}
	broker, confirmations, _ := newTestRabbitMQ(channel)
	channel.afterPublish = func() {
		confirmations <- amqp091.Confirmation{DeliveryTag: 1, Ack: true}
	}

	if err := broker.PublishJSON(context.Background(), "sale.completed", map[string]string{"event": "ok"}); err != nil {
		t.Fatalf("publish json: %v", err)
	}

	if !channel.mandatory {
		t.Fatal("expected publish to require routing with mandatory=true")
	}
	if channel.immediate {
		t.Fatal("expected immediate=false")
	}
}

func TestPublishJSONFailsWhenBrokerNacks(t *testing.T) {
	channel := &fakePublishChannel{}
	broker, confirmations, _ := newTestRabbitMQ(channel)
	channel.afterPublish = func() {
		confirmations <- amqp091.Confirmation{DeliveryTag: 2, Ack: false}
	}

	err := broker.PublishJSON(context.Background(), "sale.completed", map[string]string{"event": "ok"})

	if !errors.Is(err, ErrPublishNacked) {
		t.Fatalf("expected nack error, got %v", err)
	}
}

func TestPublishJSONFailsWhenMessageIsReturned(t *testing.T) {
	channel := &fakePublishChannel{}
	broker, confirmations, returns := newTestRabbitMQ(channel)
	channel.afterPublish = func() {
		returns <- amqp091.Return{
			Exchange:   "sales.exchange",
			RoutingKey: "sale.failed",
			ReplyCode:  312,
			ReplyText:  "NO_ROUTE",
		}
		confirmations <- amqp091.Confirmation{DeliveryTag: 3, Ack: true}
	}

	err := broker.PublishJSON(context.Background(), "sale.failed", map[string]string{"event": "failed"})

	if !errors.Is(err, ErrPublishReturned) {
		t.Fatalf("expected returned publish error, got %v", err)
	}
	var returnErr PublishReturnError
	if !errors.As(err, &returnErr) {
		t.Fatalf("expected PublishReturnError, got %T", err)
	}
	if returnErr.RoutingKey != "sale.failed" || returnErr.ReplyCode != 312 {
		t.Fatalf("unexpected return error: %+v", returnErr)
	}
}

func newTestRabbitMQ(channel *fakePublishChannel) (*RabbitMQ, chan amqp091.Confirmation, chan amqp091.Return) {
	confirmations := make(chan amqp091.Confirmation, 1)
	returns := make(chan amqp091.Return, 1)
	return &RabbitMQ{
		channel:              channel,
		exchange:             "sales.exchange",
		publishConfirmations: confirmations,
		publishReturns:       returns,
	}, confirmations, returns
}

type fakePublishChannel struct {
	mandatory    bool
	immediate    bool
	afterPublish func()
}

func (c *fakePublishChannel) PublishWithContext(_ context.Context, _ string, _ string, mandatory bool, immediate bool, _ amqp091.Publishing) error {
	c.mandatory = mandatory
	c.immediate = immediate
	if c.afterPublish != nil {
		c.afterPublish()
	}
	return nil
}

func (c *fakePublishChannel) Qos(_ int, _ int, _ bool) error {
	return nil
}

func (c *fakePublishChannel) Consume(_ string, _ string, _ bool, _ bool, _ bool, _ bool, _ amqp091.Table) (<-chan amqp091.Delivery, error) {
	return make(chan amqp091.Delivery), nil
}

func (c *fakePublishChannel) Close() error {
	return nil
}
