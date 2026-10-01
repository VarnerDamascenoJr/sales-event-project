package observability

import (
	"context"

	"github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel"
)

// InjectAMQPContext preserves the active W3C trace context on published messages.
func InjectAMQPContext(ctx context.Context, headers amqp091.Table) {
	otel.GetTextMapPropagator().Inject(ctx, amqpHeaderCarrier(headers))
}

// ExtractAMQPContext reconstructs a message parent context from AMQP headers.
func ExtractAMQPContext(ctx context.Context, headers amqp091.Table) context.Context {
	return otel.GetTextMapPropagator().Extract(ctx, amqpHeaderCarrier(headers))
}

type amqpHeaderCarrier amqp091.Table

func (c amqpHeaderCarrier) Get(key string) string {
	value, ok := c[key]
	if !ok {
		return ""
	}
	stringValue, _ := value.(string)
	return stringValue
}

func (c amqpHeaderCarrier) Set(key string, value string) {
	c[key] = value
}

func (c amqpHeaderCarrier) Keys() []string {
	keys := make([]string, 0, len(c))
	for key := range c {
		keys = append(keys, key)
	}
	return keys
}
