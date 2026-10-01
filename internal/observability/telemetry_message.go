package observability

import (
	"context"

	"github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// StartMessageSpan creates a consumer span linked to the message producer context.
func StartMessageSpan(ctx context.Context, delivery amqp091.Delivery, operation string) (context.Context, oteltrace.Span) {
	parent := ExtractAMQPContext(ctx, delivery.Headers)
	messageCtx, span := otel.Tracer(instrumentationName).Start(parent, operation)
	span.SetAttributes(
		attribute.String("messaging.system", "rabbitmq"),
		attribute.String("messaging.operation", "process"),
		attribute.String("messaging.destination.name", delivery.Exchange),
		attribute.String("messaging.rabbitmq.routing_key", delivery.RoutingKey),
	)
	return withTraceID(messageCtx), span
}
