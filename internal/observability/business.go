package observability

import (
	"context"

	"github.com/varner/sales-event-project/internal/correlation"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	oteltrace "go.opentelemetry.io/otel/trace"
)

const (
	attrRequestID     = "request_id"
	attrCorrelationID = "correlation_id"
	attrTransactionID = "transaction_id"
)

// StartBusinessSpan creates a domain-level span enriched with portfolio correlation fields.
func StartBusinessSpan(ctx context.Context, name string, attrs ...attribute.KeyValue) (context.Context, oteltrace.Span) {
	ctx, span := otel.Tracer(instrumentationName).Start(ctx, name)
	span.SetAttributes(append(CorrelationAttributes(ctx), attrs...)...)
	return withTraceID(ctx), span
}

// CorrelationAttributes returns trace attributes that match the shared portfolio contract.
func CorrelationAttributes(ctx context.Context) []attribute.KeyValue {
	metadata, ok := correlation.FromContext(ctx)
	if !ok {
		return nil
	}

	attrs := make([]attribute.KeyValue, 0, 3)
	if metadata.RequestID != "" {
		attrs = append(attrs, attribute.String(attrRequestID, metadata.RequestID))
	}
	if metadata.CorrelationID != "" {
		attrs = append(attrs, attribute.String(attrCorrelationID, metadata.CorrelationID))
	}
	if metadata.TransactionID != "" {
		attrs = append(attrs, attribute.String(attrTransactionID, metadata.TransactionID))
	}
	return attrs
}

// EndSpan records err on span before ending it, keeping successful paths uncluttered.
func EndSpan(span oteltrace.Span, err error) {
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	span.End()
}
