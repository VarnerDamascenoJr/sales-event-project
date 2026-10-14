package observability

import (
	"context"
	"testing"

	"github.com/rabbitmq/amqp091-go"
	"github.com/varner/sales-event-project/internal/correlation"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	oteltrace "go.opentelemetry.io/otel/trace"
)

func TestStartMessageSpan(t *testing.T) {
	useAMQPTestPropagator(t)
	producer := amqpTestSpanContext(t)
	headers := amqp091.Table{}
	InjectAMQPContext(oteltrace.ContextWithSpanContext(context.Background(), producer), headers)
	for _, tt := range []struct {
		name      string
		headers   amqp091.Table
		hasParent bool
	}{
		{name: "producer trace context", headers: headers, hasParent: true},
		{name: "without headers"},
		{name: "invalid trace context", headers: amqp091.Table{"traceparent": "invalid"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			recorder := tracetest.NewSpanRecorder()
			provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
			previous := otel.GetTracerProvider()
			otel.SetTracerProvider(provider)
			t.Cleanup(func() {
				otel.SetTracerProvider(previous)
				if err := provider.Shutdown(context.Background()); err != nil {
					t.Errorf("shutdown test tracer provider: %v", err)
				}
			})
			ctx := correlation.ContextWithMetadata(context.Background(), correlation.Metadata{RequestID: "req-123"})
			messageCtx, span := StartMessageSpan(ctx, amqp091.Delivery{
				Headers: tt.headers, Exchange: "sales-events", RoutingKey: "sale.created",
			}, "sale.process")
			span.End()
			spans := recorder.Ended()
			if len(spans) != 1 {
				t.Fatalf("expected one consumer span, got %d", len(spans))
			}
			got := spans[0]
			if got.Name() != "sale.process" || got.InstrumentationScope().Name != instrumentationName {
				t.Fatalf("unexpected span name or instrumentation scope: %q, %q", got.Name(), got.InstrumentationScope().Name)
			}
			if !got.SpanContext().IsValid() || !oteltrace.SpanContextFromContext(messageCtx).Equal(got.SpanContext()) {
				t.Fatal("expected the returned context to contain the consumer span")
			}
			if tt.hasParent {
				if !got.Parent().Equal(producer.WithRemote(true)) || got.SpanContext().TraceID() != producer.TraceID() {
					t.Fatal("consumer span did not continue the producer trace")
				}
			} else if got.Parent().IsValid() {
				t.Fatal("unexpected parent span without valid message headers")
			}
			attrs := spanAttributes(got.Attributes())
			assertAttr(t, attrs, "messaging.system", "rabbitmq")
			assertAttr(t, attrs, "messaging.operation", "process")
			assertAttr(t, attrs, "messaging.destination.name", "sales-events")
			assertAttr(t, attrs, "messaging.rabbitmq.routing_key", "sale.created")
			logAttrs := logAttrsForTest(t, messageCtx)
			assertCapturedAttr(t, logAttrs, "request_id", "req-123")
			assertCapturedAttr(t, logAttrs, "trace_id", got.SpanContext().TraceID().String())
		})
	}
}
