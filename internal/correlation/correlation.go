package correlation

import (
	"context"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/rabbitmq/amqp091-go"
)

const (
	RequestIDHeader     = "X-Request-ID"
	CorrelationIDHeader = "X-Correlation-ID"
	TransactionIDHeader = "X-Transaction-ID"

	AMQPRequestIDHeader     = "x-request-id"
	AMQPCorrelationIDHeader = "x-correlation-id"
	AMQPTransactionIDHeader = "x-transaction-id"
)

type Metadata struct {
	RequestID     string
	CorrelationID string
	TransactionID string
}

type contextKey struct{}

func FromHTTPHeader(header http.Header) Metadata {
	return Metadata{
		RequestID:     firstNonBlank(header.Get(RequestIDHeader), newID("req")),
		CorrelationID: firstNonBlank(header.Get(CorrelationIDHeader), newID("corr")),
		TransactionID: clean(header.Get(TransactionIDHeader)),
	}
}

func FromAMQPHeaders(headers amqp091.Table) Metadata {
	return Metadata{
		RequestID:     clean(amqpHeaderString(headers, AMQPRequestIDHeader)),
		CorrelationID: clean(amqpHeaderString(headers, AMQPCorrelationIDHeader)),
		TransactionID: clean(amqpHeaderString(headers, AMQPTransactionIDHeader)),
	}
}

func WithTransactionID(metadata Metadata, transactionID string) Metadata {
	metadata.TransactionID = firstNonBlank(transactionID, metadata.TransactionID)
	return metadata
}

func Merge(primary Metadata, fallback Metadata) Metadata {
	return Metadata{
		RequestID:     firstNonBlank(primary.RequestID, fallback.RequestID),
		CorrelationID: firstNonBlank(primary.CorrelationID, fallback.CorrelationID),
		TransactionID: firstNonBlank(primary.TransactionID, fallback.TransactionID),
	}
}

func ContextWithMetadata(ctx context.Context, metadata Metadata) context.Context {
	return context.WithValue(ctx, contextKey{}, metadata)
}

func FromContext(ctx context.Context) (Metadata, bool) {
	metadata, ok := ctx.Value(contextKey{}).(Metadata)
	return metadata, ok
}

func WriteHTTPHeaders(header http.Header, metadata Metadata) {
	if metadata.RequestID != "" {
		header.Set(RequestIDHeader, metadata.RequestID)
	}
	if metadata.CorrelationID != "" {
		header.Set(CorrelationIDHeader, metadata.CorrelationID)
	}
	if metadata.TransactionID != "" {
		header.Set(TransactionIDHeader, metadata.TransactionID)
	}
}

func LogFields(metadata Metadata) []any {
	fields := make([]any, 0, 6)
	if metadata.RequestID != "" {
		fields = append(fields, "request_id", metadata.RequestID)
	}
	if metadata.CorrelationID != "" {
		fields = append(fields, "correlation_id", metadata.CorrelationID)
	}
	if metadata.TransactionID != "" {
		fields = append(fields, "transaction_id", metadata.TransactionID)
	}
	return fields
}

func clean(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return value
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if cleaned := clean(value); cleaned != "" {
			return cleaned
		}
	}
	return ""
}

func newID(prefix string) string {
	return prefix + "_" + uuid.NewString()
}

func amqpHeaderString(headers amqp091.Table, name string) string {
	if headers == nil {
		return ""
	}

	value, ok := headers[name]
	if !ok {
		return ""
	}

	switch typed := value.(type) {
	case string:
		return typed
	case []byte:
		return string(typed)
	default:
		return ""
	}
}
