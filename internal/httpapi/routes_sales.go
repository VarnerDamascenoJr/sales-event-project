package httpapi

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/varner/sales-event-project/internal/correlation"
	"github.com/varner/sales-event-project/internal/events"
	"github.com/varner/sales-event-project/internal/metrics"
	"github.com/varner/sales-event-project/internal/observability"
	"go.opentelemetry.io/otel/attribute"
)

func registerSalesRoutes(router *gin.Engine, deps RouterDeps) {
	router.GET("/sales-events/:salesEventId/sales", requireRoles(deps.AuthStore, RoleSupport, RoleAdmin), func(c *gin.Context) {
		page, pageSize, err := parsePagination(c)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		filter := listSalesFilter{
			SalesEventID: c.Param("salesEventId"),
			Status:       c.Query("status"),
			EventName:    c.Query("eventName"),
			Page:         page,
			PageSize:     pageSize,
		}

		response, err := listSales(c.Request.Context(), deps.Store, filter)
		if err != nil {
			status := http.StatusBadRequest
			var validationErr errValidation
			if !errors.As(err, &validationErr) {
				status = http.StatusInternalServerError
			}
			c.JSON(status, gin.H{"error": err.Error(), "availableStatuses": events.AvailableSaleStatuses})
			return
		}

		c.JSON(http.StatusOK, response)
	})

	router.GET("/sales-events/:salesEventId/sales/:saleId", requireRoles(deps.AuthStore, RoleSupport, RoleAdmin), func(c *gin.Context) {
		sale, err := getSale(c.Request.Context(), deps.Store, c.Param("salesEventId"), c.Param("saleId"))
		if err != nil {
			status := http.StatusBadRequest
			if errors.Is(err, errSalesEventNotFound) || errors.Is(err, errSaleNotFound) {
				status = http.StatusNotFound
			}

			var validationErr errValidation
			if !errors.As(err, &validationErr) && status == http.StatusBadRequest {
				status = http.StatusInternalServerError
			}
			c.JSON(status, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, sale)
	})

	router.POST("/sales", publicRateLimited(deps, func(c *gin.Context) {
		var req CreateSaleRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if err := validateSaleRequest(c.Request.Context(), deps.Store, req); err != nil {
			status := http.StatusBadRequest
			var validationErr errValidation
			if !errors.As(err, &validationErr) {
				status = http.StatusInternalServerError
			}
			c.JSON(status, gin.H{"error": err.Error()})
			return
		}

		saleID := uuid.NewString()
		correlationMetadata := requestCorrelationMetadata(c, saleID)
		requestContext := correlation.ContextWithMetadata(c.Request.Context(), correlationMetadata)
		requestContext, saleSpan := observability.StartBusinessSpan(requestContext, "sale.accept",
			attribute.String("sale.id", saleID),
			attribute.String("sales_event.id", req.SalesEventID),
			attribute.Int("sale.items.count", len(req.Items)),
		)
		var saleErr error
		defer func() {
			observability.EndSpan(saleSpan, saleErr)
		}()
		event := events.SaleCreated{
			EventID:       uuid.NewString(),
			EventType:     "SALE_CREATED",
			OccurredAt:    time.Now().UTC(),
			SaleID:        saleID,
			SalesEventID:  req.SalesEventID,
			CustomerID:    req.CustomerID,
			CustomerName:  req.CustomerName,
			CustomerEmail: req.CustomerEmail,
			Items:         make([]events.SaleItem, 0, len(req.Items)),
			Metadata:      eventCorrelationMetadata(correlationMetadata),
		}

		for _, item := range req.Items {
			event.Items = append(event.Items, events.SaleItem{
				TicketID:  item.TicketID,
				Quantity:  item.Quantity,
				UnitPrice: item.UnitPrice,
			})
		}

		if err := deps.Broker.PublishJSON(requestContext, events.SaleCreatedRoutingKey, event); err != nil {
			saleErr = err
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "could not enqueue sale"})
			return
		}

		correlation.WriteHTTPHeaders(c.Writer.Header(), correlationMetadata)
		metrics.SalesCreatedTotal.Inc()
		logFields := append(correlation.LogFields(correlationMetadata),
			"sale_id", saleID,
			"sales_event_id", req.SalesEventID,
		)
		slog.Info("sale accepted", logFields...)
		c.JSON(http.StatusAccepted, gin.H{
			"saleId": saleID,
			"status": events.SaleProcessingStatus,
		})
	}))
}
