package httpapi

import (
	"github.com/gin-gonic/gin"
	"github.com/varner/sales-event-project/internal/analyticsdb"
	"github.com/varner/sales-event-project/internal/metrics"
	"github.com/varner/sales-event-project/internal/observability"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
)

func NewRouter(deps RouterDeps) *gin.Engine {
	if deps.Store == nil && deps.DB != nil {
		deps.Store = NewPostgresSalesStore(deps.DB)
	}
	if deps.AuthStore == nil && deps.DB != nil {
		deps.AuthStore = NewPostgresAuthStore(deps.DB)
	}
	if deps.AnalyticsExporter == nil && deps.DB != nil {
		deps.AnalyticsExporter = analyticsdb.NewExporter(deps.DB)
	}

	router := gin.New()
	router.Use(gin.Recovery(), correlationMiddleware(), otelgin.Middleware("sales-event-api"), observability.GinCorrelationMiddleware(), metrics.GinMiddleware())

	registerSystemRoutes(router, deps)
	registerWebhookRoutes(router, deps)
	registerSalesRoutes(router, deps)
	registerPaymentRoutes(router, deps)
	registerAnalyticsRoutes(router, deps)
	registerCheckInRoutes(router, deps)

	return router
}
