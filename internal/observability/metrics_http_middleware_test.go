package observability

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/varner/sales-event-project/internal/metrics"
)

func TestGinMetricsMiddleware(t *testing.T) {
	originalMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(originalMode) })

	tests := []struct {
		name     string
		method   string
		route    string
		targets  []string
		path     string
		status   int
		abort    bool
		duration time.Duration
	}{
		{
			name:    "parameterized route shares labels across requests",
			method:  http.MethodGet,
			route:   "/metrics-middleware-test/sales/:id",
			targets: []string{"/metrics-middleware-test/sales/sale-123?source=web", "/metrics-middleware-test/sales/sale-456"},
			path:    "/metrics-middleware-test/sales/:id",
			status:  http.StatusAccepted,
		},
		{
			name:    "post request with created status",
			method:  http.MethodPost,
			route:   "/metrics-middleware-test/sales",
			targets: []string{"/metrics-middleware-test/sales"},
			path:    "/metrics-middleware-test/sales",
			status:  http.StatusCreated,
		},
		{
			name:    "aborted request is recorded",
			method:  http.MethodGet,
			route:   "/metrics-middleware-test/forbidden",
			targets: []string{"/metrics-middleware-test/forbidden"},
			path:    "/metrics-middleware-test/forbidden",
			status:  http.StatusForbidden,
			abort:   true,
		},
		{
			name:     "duration includes downstream processing and final error status",
			method:   http.MethodGet,
			route:    "/metrics-middleware-test/error",
			targets:  []string{"/metrics-middleware-test/error"},
			path:     "/metrics-middleware-test/error",
			status:   http.StatusInternalServerError,
			duration: 5 * time.Millisecond,
		},
		{
			name:    "unmatched request uses URL path without query string",
			method:  http.MethodGet,
			targets: []string{"/metrics-middleware-test/missing?source=web"},
			path:    "/metrics-middleware-test/missing",
			status:  http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status := strconv.Itoa(tt.status)
			t.Cleanup(func() {
				metrics.HTTPRequestTotal.DeleteLabelValues(tt.method, tt.path, status)
				metrics.HTTPRequestDuration.DeleteLabelValues(tt.method, tt.path, status)
			})
			before := readHTTPMetrics(t, tt.method, tt.path, status)
			router := gin.New()
			router.Use(GinMetricsMiddleware())
			handlerCalls := 0
			if tt.route != "" {
				router.Handle(tt.method, tt.route, func(c *gin.Context) {
					handlerCalls++
					if tt.duration > 0 {
						time.Sleep(tt.duration)
					}
					if tt.abort {
						c.AbortWithStatus(tt.status)
						return
					}
					c.Status(tt.status)
				})
			}

			for _, target := range tt.targets {
				response := httptest.NewRecorder()
				router.ServeHTTP(response, httptest.NewRequest(tt.method, target, nil))
				if response.Code != tt.status {
					t.Fatalf("expected response status %d, got %d", tt.status, response.Code)
				}
			}
			if tt.route != "" && handlerCalls != len(tt.targets) {
				t.Fatalf("expected %d downstream calls, got %d", len(tt.targets), handlerCalls)
			}

			after := readHTTPMetrics(t, tt.method, tt.path, status)
			if got := after.requests - before.requests; got != float64(len(tt.targets)) {
				t.Fatalf("expected %d requests for the method, path and status labels, got %v", len(tt.targets), got)
			}
			if got := after.durationCount - before.durationCount; got != uint64(len(tt.targets)) {
				t.Fatalf("expected %d duration observations, got %d", len(tt.targets), got)
			}
			duration := after.durationSum - before.durationSum
			if duration <= 0 {
				t.Fatalf("expected positive request duration, got %v", duration)
			}
			if want := tt.duration.Seconds() * float64(len(tt.targets)); duration < want {
				t.Fatalf("expected duration to include downstream processing of at least %v seconds, got %v", want, duration)
			}
		})
	}
}

type httpMetricsSnapshot struct {
	requests      float64
	durationCount uint64
	durationSum   float64
}

func readHTTPMetrics(t *testing.T, method string, path string, status string) httpMetricsSnapshot {
	t.Helper()
	var counter dto.Metric
	if err := metrics.HTTPRequestTotal.WithLabelValues(method, path, status).Write(&counter); err != nil {
		t.Fatalf("read HTTP request counter: %v", err)
	}
	var histogram dto.Metric
	if err := metrics.HTTPRequestDuration.WithLabelValues(method, path, status).(prometheus.Histogram).Write(&histogram); err != nil {
		t.Fatalf("read HTTP duration histogram: %v", err)
	}
	return httpMetricsSnapshot{
		requests:      counter.GetCounter().GetValue(),
		durationCount: histogram.GetHistogram().GetSampleCount(),
		durationSum:   histogram.GetHistogram().GetSampleSum(),
	}
}
