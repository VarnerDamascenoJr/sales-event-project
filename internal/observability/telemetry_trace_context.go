package observability

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

// TraceContext serializes the W3C traceparent header for durable asynchronous handoffs.
func TraceContext(ctx context.Context) string {
	headers := map[string]string{}
	otel.GetTextMapPropagator().Inject(ctx, propagation.MapCarrier(headers))
	return headers["traceparent"]
}

// ContextWithTraceContext restores a persisted W3C traceparent header.
func ContextWithTraceContext(ctx context.Context, traceContext string) context.Context {
	if traceContext == "" {
		return ctx
	}
	return otel.GetTextMapPropagator().Extract(ctx, propagation.MapCarrier{"traceparent": traceContext})
}
