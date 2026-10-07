package httpapi

import (
	"encoding/json"
	"log/slog"
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

		payload, err := json.Marshal(document)
		if err != nil {
			slog.ErrorContext(c.Request.Context(), "marshal analytics export failed",
				"route", "/analytics/export",
				"sales_event_id", filter.SalesEventID,
				"limit", filter.Limit,
				"error", err,
			)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "serialize analytics export failed"})
			return
		}

		c.Data(http.StatusOK, "application/json", payload)
	})
}
