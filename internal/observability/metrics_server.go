package observability

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// StartMetricsServer starts health and Prometheus endpoints asynchronously.
func StartMetricsServer(host string, port string, component string) {
	server := newMetricsServer(host, port)
	go func() {
		slog.Info("metrics server listening", "component", component, "host", host, "port", port)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("metrics server failed", "component", component, "error", err)
		}
	}()
}

func newMetricsServer(host string, port string) *http.Server {
	router := gin.New()
	router.Use(gin.Recovery())
	router.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	router.GET("/metrics", gin.WrapH(promhttp.Handler()))

	return &http.Server{
		Addr:              host + ":" + port,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
	}
}
