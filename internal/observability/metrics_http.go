package observability

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/varner/sales-event-project/internal/metrics"
)

func GinMetricsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		route := c.FullPath()
		if route == "" {
			route = c.Request.URL.Path
		}

		status := strconv.Itoa(c.Writer.Status())
		metrics.HTTPRequestTotal.WithLabelValues(c.Request.Method, route, status).Inc()
		metrics.HTTPRequestDuration.WithLabelValues(c.Request.Method, route, status).Observe(time.Since(start).Seconds())
	}
}
