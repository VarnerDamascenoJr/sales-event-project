package observability

import (
	"context"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/log/global"
	logspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	metricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	tracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	"google.golang.org/protobuf/proto"
)

func TestConfigureTelemetryExportsAllSignals(t *testing.T) {
	for _, namespace := range []string{"portfolio", " \t "} {
		t.Run("namespace="+namespace, func(t *testing.T) {
			collector := newTelemetryTestCollector(t)
			shutdown := configureTelemetryForTest(t, namespace, collector.server.URL+"///")
			ctx, span := StartBusinessSpan(context.Background(), "telemetry-test-span")
			traceID := span.SpanContext().TraceID().String()
			span.End()
			counter, err := otel.Meter("observability-test").Int64Counter("observability.test.requests")
			if err != nil {
				t.Fatalf("create test metric: %v", err)
			}
			counter.Add(ctx, 3)
			slog.InfoContext(ctx, "telemetry test log", "email", "buyer@example.com")

			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := shutdown(shutdownCtx); err != nil {
				t.Fatalf("shutdown and flush telemetry: %v", err)
			}

			traces := waitForTelemetryExport(t, collector.traces, "traces").GetResourceSpans()
			if len(traces) != 1 || len(traces[0].GetScopeSpans()) != 1 || len(traces[0].GetScopeSpans()[0].GetSpans()) != 1 {
				t.Fatalf("expected one exported span, got %v", traces)
			}
			assertTelemetryResource(t, traces[0].GetResource(), namespace)
			exportedSpan := traces[0].GetScopeSpans()[0].GetSpans()[0]
			if exportedSpan.GetName() != "telemetry-test-span" || hex.EncodeToString(exportedSpan.GetTraceId()) != traceID {
				t.Fatalf("unexpected exported span: %v", exportedSpan)
			}

			metrics := waitForTelemetryExport(t, collector.metrics, "metrics").GetResourceMetrics()
			foundCounter := false
			for _, resourceMetrics := range metrics {
				assertTelemetryResource(t, resourceMetrics.GetResource(), namespace)
				for _, scope := range resourceMetrics.GetScopeMetrics() {
					for _, metric := range scope.GetMetrics() {
						if metric.GetName() == "observability.test.requests" {
							points := metric.GetSum().GetDataPoints()
							if len(points) != 1 || points[0].GetAsInt() != 3 {
								t.Fatalf("expected counter value 3, got %v", points)
							}
							foundCounter = true
						}
					}
				}
			}
			if !foundCounter {
				t.Fatal("configured meter provider did not export the test counter")
			}

			logs := waitForTelemetryExport(t, collector.logs, "logs").GetResourceLogs()
			if len(logs) != 1 || len(logs[0].GetScopeLogs()) != 1 || len(logs[0].GetScopeLogs()[0].GetLogRecords()) != 1 {
				t.Fatalf("expected one exported log record, got %v", logs)
			}
			assertTelemetryResource(t, logs[0].GetResource(), namespace)
			record := logs[0].GetScopeLogs()[0].GetLogRecords()[0]
			if record.GetBody().GetStringValue() != "telemetry test log" || hex.EncodeToString(record.GetTraceId()) != traceID {
				t.Fatalf("unexpected exported log record: %v", record)
			}
			attrs := map[string]string{}
			for _, attr := range record.GetAttributes() {
				attrs[attr.GetKey()] = attr.GetValue().GetStringValue()
			}
			assertAttr(t, attrs, "email", "[redacted]")
			assertAttr(t, attrs, "service", "sales-event-api")
			assertAttr(t, attrs, "environment", "test")
		})
	}
}

func TestConfigureTelemetryShutdownReportsCancellation(t *testing.T) {
	collector := newTelemetryTestCollector(t)
	shutdown := configureTelemetryForTest(t, "", collector.server.URL)
	counter, err := otel.Meter("observability-test").Int64Counter("observability.test.requests")
	if err != nil {
		t.Fatalf("create test metric: %v", err)
	}
	counter.Add(context.Background(), 1)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := shutdown(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected shutdown to report context cancellation, got %v", err)
	}
}

func configureTelemetryForTest(t *testing.T, namespace string, endpoint string) func(context.Context) error {
	t.Helper()
	t.Setenv("OTEL_TRACES_SAMPLER", "always_on")
	captureLoggerStdout(t)
	previousTracer := otel.GetTracerProvider()
	previousMeter := otel.GetMeterProvider()
	previousPropagator := otel.GetTextMapPropagator()
	previousLogger := global.GetLoggerProvider()
	t.Cleanup(func() {
		otel.SetTracerProvider(previousTracer)
		otel.SetMeterProvider(previousMeter)
		otel.SetTextMapPropagator(previousPropagator)
		global.SetLoggerProvider(previousLogger)
	})
	shutdown, err := ConfigureTelemetry(context.Background(), "sales-event-api", namespace, "test", endpoint)
	if err != nil {
		t.Fatalf("configure telemetry: %v", err)
	}
	if shutdown == nil {
		t.Fatal("expected a telemetry shutdown function")
	}
	didShutdown := false
	t.Cleanup(func() {
		if !didShutdown {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := shutdown(ctx); err != nil {
				t.Errorf("cleanup telemetry providers: %v", err)
			}
		}
	})
	if global.GetLoggerProvider() == previousLogger {
		t.Fatal("expected the global logger provider to be replaced")
	}
	fields := otel.GetTextMapPropagator().Fields()
	slices.Sort(fields)
	if !slices.Equal(fields, []string{"baggage", "traceparent", "tracestate"}) {
		t.Fatalf("expected W3C trace and baggage propagation, got fields %v", fields)
	}
	return func(ctx context.Context) error {
		didShutdown = true
		return shutdown(ctx)
	}
}

type telemetryTestCollector struct {
	server  *httptest.Server
	traces  chan *tracepb.ExportTraceServiceRequest
	metrics chan *metricspb.ExportMetricsServiceRequest
	logs    chan *logspb.ExportLogsServiceRequest
}

func newTelemetryTestCollector(t *testing.T) *telemetryTestCollector {
	t.Helper()
	collector := &telemetryTestCollector{
		traces:  make(chan *tracepb.ExportTraceServiceRequest, 16),
		metrics: make(chan *metricspb.ExportMetricsServiceRequest, 16),
		logs:    make(chan *logspb.ExportLogsServiceRequest, 16),
	}
	collector.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("unexpected collector request method: %s", r.Method)
			http.Error(w, "expected POST", http.StatusMethodNotAllowed)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read OTLP request: %v", err)
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		var message proto.Message
		switch r.URL.Path {
		case "/v1/traces":
			message = &tracepb.ExportTraceServiceRequest{}
		case "/v1/metrics":
			message = &metricspb.ExportMetricsServiceRequest{}
		case "/v1/logs":
			message = &logspb.ExportLogsServiceRequest{}
		default:
			t.Errorf("unexpected collector path: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		if err := proto.Unmarshal(body, message); err != nil {
			t.Errorf("decode OTLP request: %v", err)
			http.Error(w, "invalid protobuf", http.StatusBadRequest)
			return
		}
		switch message := message.(type) {
		case *tracepb.ExportTraceServiceRequest:
			collector.traces <- message
		case *metricspb.ExportMetricsServiceRequest:
			collector.metrics <- message
		case *logspb.ExportLogsServiceRequest:
			collector.logs <- message
		}
		w.Header().Set("Content-Type", "application/x-protobuf")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(collector.server.Close)
	return collector
}

func waitForTelemetryExport[T any](t *testing.T, requests <-chan T, signal string) T {
	t.Helper()
	select {
	case request := <-requests:
		return request
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for OTLP %s export", signal)
		var zero T
		return zero
	}
}

func assertTelemetryResource(t *testing.T, resource *resourcepb.Resource, namespace string) {
	t.Helper()
	attrs := map[string]string{}
	for _, attr := range resource.GetAttributes() {
		attrs[attr.GetKey()] = attr.GetValue().GetStringValue()
	}
	assertAttr(t, attrs, "service.name", "sales-event-api")
	assertAttr(t, attrs, "deployment.environment.name", "test")
	if strings.TrimSpace(namespace) != "" {
		assertAttr(t, attrs, "service.namespace", namespace)
	} else if _, exists := attrs["service.namespace"]; exists {
		t.Fatal("blank namespace should not be exported as a resource attribute")
	}
}
