package observability

import (
	"context"
	"errors"
	"testing"

	"github.com/varner/sales-event-project/internal/correlation"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestStartBusinessSpanAddsCorrelationAttributes(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := trace.NewTracerProvider(trace.WithSpanProcessor(recorder))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		otel.SetTracerProvider(previous)
		_ = provider.Shutdown(context.Background())
	})

	ctx := correlation.ContextWithMetadata(context.Background(), correlation.Metadata{
		RequestID:     "req-demo",
		CorrelationID: "corr-demo",
		TransactionID: "sale-demo",
	})

	_, span := StartBusinessSpan(ctx, "sale.accept", attribute.String("sale.id", "sale-demo"))
	EndSpan(span, nil)

	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected 1 ended span, got %d", len(spans))
	}

	attrs := spanAttributes(spans[0].Attributes())
	assertAttr(t, attrs, attrRequestID, "req-demo")
	assertAttr(t, attrs, attrCorrelationID, "corr-demo")
	assertAttr(t, attrs, attrTransactionID, "sale-demo")
	assertAttr(t, attrs, "sale.id", "sale-demo")
}

func TestEndSpanRecordsErrorStatus(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := trace.NewTracerProvider(trace.WithSpanProcessor(recorder))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		otel.SetTracerProvider(previous)
		_ = provider.Shutdown(context.Background())
	})

	_, span := StartBusinessSpan(context.Background(), "outbox.publish")
	EndSpan(span, errors.New("publish failed"))

	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected 1 ended span, got %d", len(spans))
	}
	if spans[0].Status().Code != codes.Error {
		t.Fatalf("expected error status, got %s", spans[0].Status().Code)
	}
	if len(spans[0].Events()) != 1 {
		t.Fatalf("expected recorded error event, got %d events", len(spans[0].Events()))
	}
}

func spanAttributes(attrs []attribute.KeyValue) map[string]string {
	values := make(map[string]string, len(attrs))
	for _, attr := range attrs {
		values[string(attr.Key)] = attr.Value.AsString()
	}
	return values
}

func assertAttr(t *testing.T, attrs map[string]string, name string, want string) {
	t.Helper()
	if got := attrs[name]; got != want {
		t.Fatalf("expected attribute %s=%q, got %q", name, want, got)
	}
}
