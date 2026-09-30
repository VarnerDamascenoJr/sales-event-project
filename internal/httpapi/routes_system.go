package httpapi

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func registerSystemRoutes(router *gin.Engine, deps RouterDeps) {
	router.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	metricsHandler := gin.WrapH(promhttp.Handler())
	if deps.MetricsProtected {
		router.GET("/metrics", requireRoles(deps.AuthStore, RoleAdmin), metricsHandler)
		return
	}
	router.GET("/metrics", metricsHandler)
}
