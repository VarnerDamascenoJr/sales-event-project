package observability

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/varner/sales-event-project/internal/correlation"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// GinCorrelationMiddleware binds a request ID and trace ID to request-scoped logs.
func GinCorrelationMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := c.GetHeader(correlation.RequestIDHeader)
		if requestID == "" {
			requestID = uuid.NewString()
		}
		c.Header(correlation.RequestIDHeader, requestID)

		spanContext := oteltrace.SpanContextFromContext(c.Request.Context())
		ctx := context.WithValue(c.Request.Context(), requestIDContextKey{}, requestID)
		if spanContext.HasTraceID() {
			ctx = context.WithValue(ctx, traceIDContextKey{}, spanContext.TraceID().String())
		}
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}
