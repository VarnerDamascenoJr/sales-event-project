package metrics

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	HTTPRequestTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "Total number of HTTP requests.",
	}, []string{"method", "path", "status"})

	HTTPRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_request_duration_seconds",
		Help:    "HTTP request duration in seconds.",
		Buckets: prometheus.DefBuckets,
	}, []string{"method", "path", "status"})

	SalesCreatedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "sales_created_total",
		Help: "Total number of sales accepted by the API.",
	})

	WorkerSalesProcessedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "worker_sales_processed_total",
		Help: "Total number of sales processed by workers.",
	}, []string{"status"})

	WorkerProcessingDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "worker_processing_duration_seconds",
		Help:    "Worker sale processing duration in seconds.",
		Buckets: prometheus.DefBuckets,
	})
)

func GinMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		route := c.FullPath()
		if route == "" {
			route = c.Request.URL.Path
		}

		status := strconv.Itoa(c.Writer.Status())
		HTTPRequestTotal.WithLabelValues(c.Request.Method, route, status).Inc()
		HTTPRequestDuration.WithLabelValues(c.Request.Method, route, status).Observe(time.Since(start).Seconds())
	}
}
