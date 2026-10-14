package observability

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/varner/sales-event-project/internal/correlation"
	oteltrace "go.opentelemetry.io/otel/trace"
)

func TestGinCorrelationMiddleware(t *testing.T) {
	originalMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(originalMode) })
	for _, tt := range []struct {
		name      string
		requestID string
		withTrace bool
	}{
		{name: "supplied request ID", requestID: "req-123"},
		{name: "generated request ID"},
		{name: "supplied request ID with trace", requestID: "req-123", withTrace: true},
		{name: "generated request ID with trace", withTrace: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.WithValue(context.Background(), requestIDContextKey{}, "old-request")
			spanContext := amqpTestSpanContext(t)
			if tt.withTrace {
				ctx = oteltrace.ContextWithSpanContext(ctx, spanContext)
			}
			router := gin.New()
			router.Use(GinCorrelationMiddleware())
			var attrs map[string]any
			calls := 0
			router.GET("/correlation-test", func(c *gin.Context) {
				calls++
				attrs = logAttrsForTest(t, c.Request.Context())
				c.Status(http.StatusAccepted)
			})
			request := httptest.NewRequest(http.MethodGet, "/correlation-test", nil).WithContext(ctx)
			if tt.requestID != "" {
				request.Header.Set(correlation.RequestIDHeader, tt.requestID)
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != http.StatusAccepted || calls != 1 {
				t.Fatalf("expected downstream handler to run once with status 202, got status %d and %d calls", response.Code, calls)
			}
			requestID := response.Header().Get(correlation.RequestIDHeader)
			if tt.requestID != "" {
				if requestID != tt.requestID {
					t.Fatalf("expected response request ID %q, got %q", tt.requestID, requestID)
				}
			} else if _, err := uuid.Parse(requestID); err != nil {
				t.Fatalf("expected a generated UUID request ID, got %q: %v", requestID, err)
			}
			assertCapturedAttr(t, attrs, "request_id", requestID)
			if tt.withTrace {
				assertCapturedAttr(t, attrs, "trace_id", spanContext.TraceID().String())
			} else if _, exists := attrs["trace_id"]; exists {
				t.Fatal("unexpected trace ID without an active span")
			}
		})
	}
}
