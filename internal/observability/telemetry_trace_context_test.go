package observability

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	oteltrace "go.opentelemetry.io/otel/trace"
)

func TestTraceContextRoundTrip(t *testing.T) {
	previousPropagator := otel.GetTextMapPropagator()
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() {
		otel.SetTextMapPropagator(previousPropagator)
	})

	provider := sdktrace.NewTracerProvider()
	t.Cleanup(func() {
		_ = provider.Shutdown(context.Background())
	})

	ctx, span := provider.Tracer("observability-test").Start(context.Background(), "publish")
	defer span.End()

	traceContext := TraceContext(ctx)
	if traceContext == "" {
		t.Fatal("expected trace context to be serialized")
	}

	restored := ContextWithTraceContext(context.Background(), traceContext)
	originalSpanContext := oteltrace.SpanContextFromContext(ctx)
	restoredSpanContext := oteltrace.SpanContextFromContext(restored)
	if !restoredSpanContext.IsValid() {
		t.Fatal("expected restored span context to be valid")
	}
	if restoredSpanContext.TraceID() != originalSpanContext.TraceID() {
		t.Fatalf("expected trace id %s, got %s", originalSpanContext.TraceID(), restoredSpanContext.TraceID())
	}
}
