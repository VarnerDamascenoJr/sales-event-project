package observability

import (
	"context"

	"github.com/varner/sales-event-project/internal/correlation"
	oteltrace "go.opentelemetry.io/otel/trace"
)

type requestIDContextKey struct{}
type traceIDContextKey struct{}

// LogAttrs returns correlation fields suitable for slog calls made with ctx.
func LogAttrs(ctx context.Context) []any {
	metadata, hasMetadata := correlation.FromContext(ctx)
	attrs := correlation.LogFields(metadata)
	if requestID, ok := ctx.Value(requestIDContextKey{}).(string); ok && (!hasMetadata || metadata.RequestID == "") {
		attrs = append(attrs, "request_id", requestID)
	}
	if traceID, ok := ctx.Value(traceIDContextKey{}).(string); ok {
		attrs = append(attrs, "trace_id", traceID)
	}
	return attrs
}

func withTraceID(ctx context.Context) context.Context {
	spanContext := oteltrace.SpanContextFromContext(ctx)
	if !spanContext.HasTraceID() {
		return ctx
	}
	return context.WithValue(ctx, traceIDContextKey{}, spanContext.TraceID().String())
}
