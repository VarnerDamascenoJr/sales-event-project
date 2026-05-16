package httpapi

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/varner/sales-event-project/internal/events"
	"github.com/varner/sales-event-project/internal/messaging"
	"github.com/varner/sales-event-project/internal/metrics"
)

type RouterDeps struct {
	Broker *messaging.RabbitMQ
	DB     *pgxpool.Pool
}

type CreateSaleRequest struct {
	SalesEventID string           `json:"salesEventId" binding:"required"`
	CustomerID   string           `json:"customerId" binding:"required"`
	Items        []CreateSaleItem `json:"items" binding:"required,min=1,dive"`
}

type CreateSaleItem struct {
	TicketID  string `json:"ticketId" binding:"required"`
	Quantity  int    `json:"quantity" binding:"required,min=1"`
	UnitPrice int64  `json:"unitPrice" binding:"required,min=1"`
}

func NewRouter(deps RouterDeps) *gin.Engine {
	router := gin.New()
	router.Use(gin.Recovery(), metrics.GinMiddleware())

	router.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	router.GET("/metrics", gin.WrapH(promhttp.Handler()))

	router.POST("/sales", func(c *gin.Context) {
		var req CreateSaleRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if err := validateSaleRequest(c.Request.Context(), deps.DB, req); err != nil {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
			return
		}

		saleID := uuid.NewString()
		event := events.SaleCreated{
			EventID:      uuid.NewString(),
			EventType:    "SALE_CREATED",
			OccurredAt:   time.Now().UTC(),
			SaleID:       saleID,
			SalesEventID: req.SalesEventID,
			CustomerID:   req.CustomerID,
			Items:        make([]events.SaleItem, 0, len(req.Items)),
		}

		for _, item := range req.Items {
			event.Items = append(event.Items, events.SaleItem{
				TicketID:  item.TicketID,
				Quantity:  item.Quantity,
				UnitPrice: item.UnitPrice,
			})
		}

		if err := deps.Broker.PublishJSON(c.Request.Context(), events.SaleCreatedRoutingKey, event); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "could not enqueue sale"})
			return
		}

		metrics.SalesCreatedTotal.Inc()
		c.JSON(http.StatusAccepted, gin.H{
			"saleId": saleID,
			"status": events.SaleProcessingStatus,
		})
	})

	return router
}

func validateSaleRequest(ctx context.Context, db *pgxpool.Pool, req CreateSaleRequest) error {
	var eventExists bool
	if err := db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM sales_events
			WHERE id = $1 AND status = 'PUBLISHED'
		)
	`, req.SalesEventID).Scan(&eventExists); err != nil {
		return err
	}
	if !eventExists {
		return errValidation("sales event does not exist or is not published")
	}

	for _, item := range req.Items {
		var ticketExists bool
		if err := db.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1
				FROM tickets
				WHERE id = $1
				  AND sales_event_id = $2
				  AND available_quantity >= $3
				  AND price = $4
			)
		`, item.TicketID, req.SalesEventID, item.Quantity, item.UnitPrice).Scan(&ticketExists); err != nil {
			return err
		}
		if !ticketExists {
			return errValidation("ticket does not exist, has insufficient quantity, or has a different price")
		}
	}

	return nil
}

type errValidation string

func (e errValidation) Error() string {
	return string(e)
}
