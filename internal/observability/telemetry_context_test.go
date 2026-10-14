package observability

import (
	"context"
	"reflect"
	"testing"

	"github.com/varner/sales-event-project/internal/correlation"
	oteltrace "go.opentelemetry.io/otel/trace"
)

func TestLogAttrs(t *testing.T) {
	metadata := correlation.Metadata{RequestID: "req-metadata", CorrelationID: "corr-123", TransactionID: "tx-123"}
	partial := correlation.Metadata{CorrelationID: "corr-123"}
	tests := []struct {
		name string
		ctx  context.Context
		want map[string]any
	}{
		{name: "empty context", ctx: context.Background(), want: map[string]any{}},
		{
			name: "fallback request ID",
			ctx:  context.WithValue(context.Background(), requestIDContextKey{}, "req-fallback"),
			want: map[string]any{"request_id": "req-fallback"},
		},
		{
			name: "metadata takes precedence over fallback",
			ctx:  context.WithValue(correlation.ContextWithMetadata(context.Background(), metadata), requestIDContextKey{}, "req-fallback"),
			want: map[string]any{"request_id": "req-metadata", "correlation_id": "corr-123", "transaction_id": "tx-123"},
		},
		{
			name: "partial metadata uses fallback request ID",
			ctx:  context.WithValue(correlation.ContextWithMetadata(context.Background(), partial), requestIDContextKey{}, "req-fallback"),
			want: map[string]any{"request_id": "req-fallback", "correlation_id": "corr-123"},
		},
		{
			name: "trace ID",
			ctx:  context.WithValue(context.Background(), traceIDContextKey{}, "trace-123"),
			want: map[string]any{"trace_id": "trace-123"},
		},
		{
			name: "invalid value types are ignored",
			ctx:  context.WithValue(context.WithValue(context.Background(), requestIDContextKey{}, 123), traceIDContextKey{}, 456),
			want: map[string]any{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := logAttrsForTest(t, tt.ctx); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("expected log attributes %v, got %v", tt.want, got)
			}
		})
	}
}

func TestWithTraceID(t *testing.T) {
	t.Run("active span", func(t *testing.T) {
		spanContext := amqpTestSpanContext(t)
		ctx := oteltrace.ContextWithSpanContext(context.Background(), spanContext)
		ctx = context.WithValue(ctx, requestIDContextKey{}, "req-123")
		got := withTraceID(ctx)
		if got.Value(traceIDContextKey{}) != spanContext.TraceID().String() {
			t.Fatalf("unexpected trace ID in log context: %v", got.Value(traceIDContextKey{}))
		}
		if got.Value(requestIDContextKey{}) != "req-123" || !oteltrace.SpanContextFromContext(got).Equal(spanContext) {
			t.Fatal("existing context values or span context changed")
		}
	})
	t.Run("without span", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), requestIDContextKey{}, "req-123")
		if got := withTraceID(ctx); got != ctx {
			t.Fatal("expected the original context when there is no trace ID")
		}
	})
}

func logAttrsForTest(t *testing.T, ctx context.Context) map[string]any {
	t.Helper()
	attrs := LogAttrs(ctx)
	if len(attrs)%2 != 0 {
		t.Fatalf("expected key/value pairs, got %v", attrs)
	}
	values := map[string]any{}
	for i := 0; i < len(attrs); i += 2 {
		key, ok := attrs[i].(string)
		if !ok {
			t.Fatalf("expected a string attribute key, got %v", attrs[i])
		}
		if _, exists := values[key]; exists {
			t.Fatalf("duplicate log attribute %q", key)
		}
		values[key] = attrs[i+1]
	}
	return values
}
