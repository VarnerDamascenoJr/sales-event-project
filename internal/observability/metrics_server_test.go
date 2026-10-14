package observability

import (
	"context"
	"encoding/json"
	"log"
	"log/slog"
	"mime"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/expfmt"
)

func TestMetricsServerEndpoints(t *testing.T) {
	originalMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(originalMode) })

	const metricName = "observability_metrics_server_test_total"
	counter := prometheus.NewCounter(prometheus.CounterOpts{
		Name: metricName,
		Help: "Counter used to verify the metrics server endpoint.",
	})
	if err := prometheus.Register(counter); err != nil {
		t.Fatalf("register test counter: %v", err)
	}
	t.Cleanup(func() { prometheus.Unregister(counter) })
	counter.Add(2)

	server := newMetricsServer("127.0.0.1", "9091")
	if server.Addr != "127.0.0.1:9091" {
		t.Fatalf("unexpected metrics server address: %q", server.Addr)
	}
	if server.ReadHeaderTimeout != 5*time.Second {
		t.Fatalf("expected a 5s read header timeout, got %v", server.ReadHeaderTimeout)
	}
	testServer := httptest.NewServer(server.Handler)
	t.Cleanup(testServer.Close)
	client := testServer.Client()
	client.Timeout = 2 * time.Second

	t.Run("health endpoint", func(t *testing.T) {
		response, err := client.Get(testServer.URL + "/healthz")
		if err != nil {
			t.Fatalf("request health endpoint: %v", err)
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			t.Fatalf("expected health status 200, got %d", response.StatusCode)
		}
		var body map[string]string
		if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
			t.Fatalf("decode health response: %v", err)
		}
		if len(body) != 1 || body["status"] != "ok" {
			t.Fatalf("unexpected health response: %v", body)
		}
	})

	t.Run("metrics endpoint", func(t *testing.T) {
		response, err := client.Get(testServer.URL + "/metrics")
		if err != nil {
			t.Fatalf("request metrics endpoint: %v", err)
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			t.Fatalf("expected metrics status 200, got %d", response.StatusCode)
		}
		contentType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
		if err != nil || contentType != "text/plain" {
			t.Fatalf("unexpected metrics content type %q: %v", response.Header.Get("Content-Type"), err)
		}
		var parser expfmt.TextParser
		families, err := parser.TextToMetricFamilies(response.Body)
		if err != nil {
			t.Fatalf("parse Prometheus response: %v", err)
		}
		family := families[metricName]
		if family == nil || len(family.GetMetric()) != 1 {
			t.Fatalf("expected one metric in family %q, got %v", metricName, family)
		}
		if got := family.GetMetric()[0].GetCounter().GetValue(); got != 2 {
			t.Fatalf("expected counter value 2, got %v", got)
		}
	})

	t.Run("unknown endpoint", func(t *testing.T) {
		response, err := client.Get(testServer.URL + "/unknown")
		if err != nil {
			t.Fatalf("request unknown endpoint: %v", err)
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusNotFound {
			t.Fatalf("expected unknown endpoint status 404, got %d", response.StatusCode)
		}
	})
}

func TestStartMetricsServerLogsStartupFailure(t *testing.T) {
	originalLogger := slog.Default()
	originalLogWriter := log.Writer()
	originalLogFlags := log.Flags()
	t.Cleanup(func() {
		slog.SetDefault(originalLogger)
		log.SetOutput(originalLogWriter)
		log.SetFlags(originalLogFlags)
	})
	records := make(chan slog.Record, 2)
	slog.SetDefault(slog.New(metricsServerTestHandler{records: records}))

	StartMetricsServer("127.0.0.1", "invalid-port", "worker")
	for _, expected := range []struct {
		message string
		level   slog.Level
	}{
		{message: "metrics server listening", level: slog.LevelInfo},
		{message: "metrics server failed", level: slog.LevelError},
	} {
		select {
		case record := <-records:
			if record.Message != expected.message || record.Level != expected.level {
				t.Fatalf("unexpected metrics server log record: %+v", record)
			}
			attrs := map[string]any{}
			record.Attrs(func(attr slog.Attr) bool {
				attrs[attr.Key] = attr.Value.Any()
				return true
			})
			assertCapturedAttr(t, attrs, "component", "worker")
			if record.Level == slog.LevelInfo {
				assertCapturedAttr(t, attrs, "host", "127.0.0.1")
				assertCapturedAttr(t, attrs, "port", "invalid-port")
			} else if err, ok := attrs["error"].(error); !ok || err == nil {
				t.Fatalf("expected a startup error in the log, got %v", attrs["error"])
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for log record %q", expected.message)
		}
	}
}

type metricsServerTestHandler struct {
	records chan<- slog.Record
}

func (h metricsServerTestHandler) Enabled(context.Context, slog.Level) bool {
	return true
}

func (h metricsServerTestHandler) Handle(_ context.Context, record slog.Record) error {
	h.records <- record.Clone()
	return nil
}

func (h metricsServerTestHandler) WithAttrs([]slog.Attr) slog.Handler {
	return h
}

func (h metricsServerTestHandler) WithGroup(string) slog.Handler {
	return h
}
