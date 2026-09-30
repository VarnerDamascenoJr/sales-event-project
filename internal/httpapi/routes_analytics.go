package httpapi

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func registerAnalyticsRoutes(router *gin.Engine, deps RouterDeps) {
	router.GET("/analytics/export", requireRoles(deps.AuthStore, RoleSupport, RoleAdmin), func(c *gin.Context) {
		filter, err := parseAnalyticsExportFilter(c)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if deps.AnalyticsExporter == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "analytics exporter is not configured"})
			return
		}

		document, err := deps.AnalyticsExporter.ExportAnalytics(c.Request.Context(), filter)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "export analytics failed"})
			return
		}

		c.JSON(http.StatusOK, document)
	})
}
