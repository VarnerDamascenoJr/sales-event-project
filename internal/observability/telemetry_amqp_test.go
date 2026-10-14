package observability

import (
	"context"
	"reflect"
	"slices"
	"testing"

	"github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/baggage"
	"go.opentelemetry.io/otel/propagation"
	oteltrace "go.opentelemetry.io/otel/trace"
)

func TestAMQPContextRoundTrip(t *testing.T) {
	useAMQPTestPropagator(t)
	spanContext := amqpTestSpanContext(t)
	ctx := oteltrace.ContextWithSpanContext(context.Background(), spanContext)
	values, err := baggage.Parse("tenant=tenant-123")
	if err != nil {
		t.Fatalf("parse test baggage: %v", err)
	}
	ctx = baggage.ContextWithBaggage(ctx, values)
	headers := amqp091.Table{"message_type": "SALE_CREATED", "attempt": int32(2)}

	InjectAMQPContext(ctx, headers)
	want := amqp091.Table{
		"message_type": "SALE_CREATED",
		"attempt":      int32(2),
		"traceparent":  "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
		"tracestate":   "vendor=value",
		"baggage":      "tenant=tenant-123",
	}
	if !reflect.DeepEqual(headers, want) {
		t.Fatalf("expected AMQP headers %v, got %v", want, headers)
	}

	consumerCtx := context.WithValue(context.Background(), requestIDContextKey{}, "req-123")
	restored := ExtractAMQPContext(consumerCtx, headers)
	if got := oteltrace.SpanContextFromContext(restored); !got.Equal(spanContext.WithRemote(true)) {
		t.Fatalf("expected the remote producer span context, got %+v", got)
	}
	if got := baggage.FromContext(restored).Member("tenant").Value(); got != "tenant-123" {
		t.Fatalf("expected baggage tenant=tenant-123, got %q", got)
	}
	if got := restored.Value(requestIDContextKey{}); got != "req-123" {
		t.Fatalf("expected consumer context values to be preserved, got %v", got)
	}
}

func TestInjectAMQPContextWithoutTracePreservesHeaders(t *testing.T) {
	useAMQPTestPropagator(t)
	headers := amqp091.Table{"message_type": "SALE_CREATED"}
	InjectAMQPContext(context.Background(), headers)
	if want := (amqp091.Table{"message_type": "SALE_CREATED"}); !reflect.DeepEqual(headers, want) {
		t.Fatalf("expected existing headers without trace headers, got %v", headers)
	}
}

func TestExtractAMQPContextPreservesParentForInvalidHeaders(t *testing.T) {
	useAMQPTestPropagator(t)
	spanContext := amqpTestSpanContext(t)
	ctx := oteltrace.ContextWithSpanContext(context.Background(), spanContext)
	ctx = context.WithValue(ctx, requestIDContextKey{}, "req-123")
	tests := []struct {
		name    string
		headers amqp091.Table
	}{
		{name: "nil headers"},
		{name: "empty headers", headers: amqp091.Table{}},
		{name: "missing traceparent", headers: amqp091.Table{"message_type": "SALE_CREATED"}},
		{name: "malformed traceparent", headers: amqp091.Table{"traceparent": "invalid"}},
		{name: "numeric traceparent", headers: amqp091.Table{"traceparent": int32(123)}},
		{name: "byte traceparent", headers: amqp091.Table{"traceparent": []byte("00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			restored := ExtractAMQPContext(ctx, tt.headers)
			if got := oteltrace.SpanContextFromContext(restored); !got.Equal(spanContext) {
				t.Fatalf("expected the original span context to be preserved, got %+v", got)
			}
			if got := restored.Value(requestIDContextKey{}); got != "req-123" {
				t.Fatalf("expected parent context values to be preserved, got %v", got)
			}
		})
	}
}

func TestAMQPHeaderCarrierGet(t *testing.T) {
	carrier := amqpHeaderCarrier{
		"string": "value",
		"empty":  "",
		"number": int32(123),
		"bytes":  []byte("value"),
		"nil":    nil,
	}
	for _, tt := range []struct {
		key  string
		want string
	}{
		{key: "string", want: "value"},
		{key: "empty"},
		{key: "number"},
		{key: "bytes"},
		{key: "nil"},
		{key: "missing"},
	} {
		t.Run(tt.key, func(t *testing.T) {
			if got := carrier.Get(tt.key); got != tt.want {
				t.Fatalf("expected header %q=%q, got %q", tt.key, tt.want, got)
			}
		})
	}
}

func TestAMQPHeaderCarrierSetUpdatesOriginalTable(t *testing.T) {
	headers := amqp091.Table{"traceparent": "old", "attempt": int32(2)}
	carrier := amqpHeaderCarrier(headers)
	carrier.Set("traceparent", "new")
	carrier.Set("tracestate", "vendor=value")
	want := amqp091.Table{
		"traceparent": "new",
		"tracestate":  "vendor=value",
		"attempt":     int32(2),
	}
	if !reflect.DeepEqual(headers, want) {
		t.Fatalf("expected original table headers %v, got %v", want, headers)
	}
}

func TestAMQPHeaderCarrierKeys(t *testing.T) {
	tests := []struct {
		name    string
		headers amqp091.Table
		want    []string
	}{
		{name: "nil table"},
		{name: "empty table", headers: amqp091.Table{}},
		{
			name:    "all header names regardless of value type",
			headers: amqp091.Table{"traceparent": "value", "attempt": int32(2), "opaque": []byte("value")},
			want:    []string{"attempt", "opaque", "traceparent"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keys := amqpHeaderCarrier(tt.headers).Keys()
			slices.Sort(keys)
			if !slices.Equal(keys, tt.want) {
				t.Fatalf("expected header keys %v, got %v", tt.want, keys)
			}
		})
	}
}

func useAMQPTestPropagator(t *testing.T) {
	t.Helper()
	previous := otel.GetTextMapPropagator()
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	t.Cleanup(func() { otel.SetTextMapPropagator(previous) })
}

func amqpTestSpanContext(t *testing.T) oteltrace.SpanContext {
	t.Helper()
	traceID, err := oteltrace.TraceIDFromHex("4bf92f3577b34da6a3ce929d0e0e4736")
	if err != nil {
		t.Fatalf("parse test trace ID: %v", err)
	}
	spanID, err := oteltrace.SpanIDFromHex("00f067aa0ba902b7")
	if err != nil {
		t.Fatalf("parse test span ID: %v", err)
	}
	state, err := oteltrace.ParseTraceState("vendor=value")
	if err != nil {
		t.Fatalf("parse test trace state: %v", err)
	}
	return oteltrace.NewSpanContext(oteltrace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: oteltrace.FlagsSampled,
		TraceState: state,
	})
}
