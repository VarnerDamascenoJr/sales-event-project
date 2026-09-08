package observability

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	oteltrace "go.opentelemetry.io/otel/trace"
)

const instrumentationName = "github.com/varner/sales-event-project"

// ConfigureTelemetry configures all three telemetry signals for an application process.
func ConfigureTelemetry(ctx context.Context, service string, environment string, endpoint string) (func(context.Context) error, error) {
	endpoint = strings.TrimRight(endpoint, "/")
	resource := resource.NewWithAttributes("",
		attribute.String("service.name", service),
		attribute.String("deployment.environment.name", environment),
	)

	traceExporter, err := otlptracehttp.New(ctx, otlptracehttp.WithEndpointURL(endpoint+"/v1/traces"))
	if err != nil {
		return nil, err
	}
	metricExporter, err := otlpmetrichttp.New(ctx, otlpmetrichttp.WithEndpointURL(endpoint+"/v1/metrics"))
	if err != nil {
		return nil, errors.Join(traceExporter.Shutdown(ctx), err)
	}
	logExporter, err := otlploghttp.New(ctx, otlploghttp.WithEndpointURL(endpoint+"/v1/logs"))
	if err != nil {
		return nil, errors.Join(traceExporter.Shutdown(ctx), metricExporter.Shutdown(ctx), err)
	}

	tracerProvider := sdktrace.NewTracerProvider(sdktrace.WithBatcher(traceExporter), sdktrace.WithResource(resource))
	meterProvider := metric.NewMeterProvider(
		metric.WithReader(metric.NewPeriodicReader(metricExporter, metric.WithInterval(5*time.Second))),
		metric.WithResource(resource),
	)
	loggerProvider := log.NewLoggerProvider(
		log.WithProcessor(log.NewBatchProcessor(logExporter)),
		log.WithResource(resource),
	)

	otel.SetTracerProvider(tracerProvider)
	otel.SetMeterProvider(meterProvider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	global.SetLoggerProvider(loggerProvider)
	ConfigureLogger(service, environment, otelslog.NewHandler(instrumentationName, otelslog.WithLoggerProvider(loggerProvider)))

	return func(shutdownCtx context.Context) error {
		return errors.Join(
			loggerProvider.Shutdown(shutdownCtx),
			meterProvider.Shutdown(shutdownCtx),
			tracerProvider.Shutdown(shutdownCtx),
		)
	}, nil
}

// GinCorrelationMiddleware binds a request ID and trace ID to request-scoped logs.
func GinCorrelationMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := c.GetHeader("X-Request-ID")
		if requestID == "" {
			requestID = uuid.NewString()
		}
		c.Header("X-Request-ID", requestID)

		spanContext := oteltrace.SpanContextFromContext(c.Request.Context())
		ctx := context.WithValue(c.Request.Context(), requestIDContextKey{}, requestID)
		if spanContext.HasTraceID() {
			ctx = context.WithValue(ctx, traceIDContextKey{}, spanContext.TraceID().String())
		}
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

type requestIDContextKey struct{}
type traceIDContextKey struct{}

// LogAttrs returns correlation fields suitable for slog calls made with ctx.
func LogAttrs(ctx context.Context) []any {
	attrs := make([]any, 0, 2)
	if requestID, ok := ctx.Value(requestIDContextKey{}).(string); ok {
		attrs = append(attrs, "request_id", requestID)
	}
	if traceID, ok := ctx.Value(traceIDContextKey{}).(string); ok {
		attrs = append(attrs, "trace_id", traceID)
	}
	return attrs
}

// InjectAMQPContext preserves the active W3C trace context on published messages.
func InjectAMQPContext(ctx context.Context, headers amqp091.Table) {
	otel.GetTextMapPropagator().Inject(ctx, amqpHeaderCarrier(headers))
}

// ExtractAMQPContext reconstructs a message parent context from AMQP headers.
func ExtractAMQPContext(ctx context.Context, headers amqp091.Table) context.Context {
	return otel.GetTextMapPropagator().Extract(ctx, amqpHeaderCarrier(headers))
}

type amqpHeaderCarrier amqp091.Table

func (c amqpHeaderCarrier) Get(key string) string {
	value, ok := c[key]
	if !ok {
		return ""
	}
	stringValue, _ := value.(string)
	return stringValue
}

func (c amqpHeaderCarrier) Set(key string, value string) {
	c[key] = value
}

func (c amqpHeaderCarrier) Keys() []string {
	keys := make([]string, 0, len(c))
	for key := range c {
		keys = append(keys, key)
	}
	return keys
}

// StartMessageSpan creates a consumer span linked to the message producer context.
func StartMessageSpan(ctx context.Context, delivery amqp091.Delivery, operation string) (context.Context, oteltrace.Span) {
	parent := ExtractAMQPContext(ctx, delivery.Headers)
	messageCtx, span := otel.Tracer(instrumentationName).Start(parent, operation)
	return withTraceID(messageCtx), span
}

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

func withTraceID(ctx context.Context) context.Context {
	spanContext := oteltrace.SpanContextFromContext(ctx)
	if !spanContext.HasTraceID() {
		return ctx
	}
	return context.WithValue(ctx, traceIDContextKey{}, spanContext.TraceID().String())
}
