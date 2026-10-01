package observability

import (
	"context"
	"errors"
	"strings"
	"time"

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
)

const instrumentationName = "github.com/varner/sales-event-project"

// ConfigureTelemetry configures all three telemetry signals for an application process.
func ConfigureTelemetry(ctx context.Context, service string, namespace string, environment string, endpoint string) (func(context.Context) error, error) {
	endpoint = strings.TrimRight(endpoint, "/")
	resourceAttrs := []attribute.KeyValue{
		attribute.String("service.name", service),
		attribute.String("deployment.environment.name", environment),
	}
	if strings.TrimSpace(namespace) != "" {
		resourceAttrs = append(resourceAttrs, attribute.String("service.namespace", namespace))
	}
	resource := resource.NewWithAttributes("", resourceAttrs...)

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
