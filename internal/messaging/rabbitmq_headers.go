package messaging

import (
	"context"

	"github.com/rabbitmq/amqp091-go"
	"github.com/varner/sales-event-project/internal/correlation"
	"github.com/varner/sales-event-project/internal/observability"
)

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
