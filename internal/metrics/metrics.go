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

	EventPublishedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "events_published_total",
		Help: "Total number of application events published.",
	}, []string{"routing_key", "status"})

	OutboxEventsProcessedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "outbox_events_processed_total",
		Help: "Total number of outbox events processed by the publisher.",
	}, []string{"event_type", "status"})

	PaymentsProcessedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "payments_processed_total",
		Help: "Total number of payments processed by the API.",
	}, []string{"status", "provider"})

	PaymentWebhookReplaysTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "payment_webhook_replays_total",
		Help: "Total number of idempotent payment webhook replays.",
	}, []string{"status", "provider"})

	WorkerMessagesProcessedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "worker_messages_processed_total",
		Help: "Total number of worker messages processed.",
	}, []string{"queue", "status"})

	WorkerMessageDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "worker_message_processing_duration_seconds",
		Help:    "Worker message processing duration in seconds.",
		Buckets: prometheus.DefBuckets,
	}, []string{"queue"})

	TicketDeliveryTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "ticket_delivery_total",
		Help: "Total number of ticket delivery attempts.",
	}, []string{"status"})

	IssuedTicketsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "issued_tickets_total",
		Help: "Total number of issued tickets generated.",
	})

	CheckInsCreatedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "check_ins_created_total",
		Help: "Total number of successful ticket check-ins.",
	})

	RetentionDeletedRowsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "retention_deleted_rows_total",
		Help: "Total number of rows deleted by retention cleanup.",
	}, []string{"table"})
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
