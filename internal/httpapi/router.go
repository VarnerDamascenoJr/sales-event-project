package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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
	SalesEventID string           `json:"salesEventId"`
	CustomerID   string           `json:"customerId"`
	Status       string           `json:"status"`
	Items        []CreateSaleItem `json:"items"`
}

type CreateSaleItem struct {
	TicketID  string `json:"ticketId"`
	Quantity  int    `json:"quantity"`
	UnitPrice int    `json:"unitPrice"`
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
			status := http.StatusBadRequest
			var validationErr errValidation
			if !errors.As(err, &validationErr) {
				status = http.StatusInternalServerError
			}
			c.JSON(status, gin.H{"error": err.Error()})
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
	if req.SalesEventID == "" {
		return errValidation("salesEventId is required")
	}
	if len(req.SalesEventID) > events.MaxTextLength {
		return errValidation("salesEventId is too long; maximum length is 50 characters")
	}
	if req.CustomerID == "" {
		return errValidation("customerId is required")
	}
	if len(req.CustomerID) > events.MaxTextLength {
		return errValidation("customerId is too long; maximum length is 50 characters")
	}
	if req.Status != "" && req.Status != events.SaleProcessingStatus {
		return errValidation("status must be PROCESSING when creating a sale")
	}
	if len(req.Items) == 0 {
		return errValidation("items must contain at least one ticket")
	}

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

	totalAmount := 0
	for index, item := range req.Items {
		if item.TicketID == "" {
			return errValidation(fmt.Sprintf("items[%d].ticketId is required", index))
		}
		if len(item.TicketID) > events.MaxTextLength {
			return errValidation(fmt.Sprintf("items[%d].ticketId is too long; maximum length is 50 characters", index))
		}
		if item.Quantity < 0 {
			return errValidation(fmt.Sprintf("items[%d].quantity cannot be negative", index))
		}
		if item.Quantity == 0 {
			return errValidation(fmt.Sprintf("items[%d].quantity must be greater than zero", index))
		}
		if item.Quantity > events.MaxTicketQuantity {
			return errValidation(fmt.Sprintf("items[%d].quantity is too high; maximum is %d", index, events.MaxTicketQuantity))
		}
		if item.UnitPrice < 0 {
			return errValidation(fmt.Sprintf("items[%d].unitPrice cannot be negative", index))
		}
		if item.UnitPrice == 0 {
			return errValidation(fmt.Sprintf("items[%d].unitPrice must be greater than zero", index))
		}
		if item.UnitPrice > events.MaxMoneyAmountInCents {
			return errValidation(fmt.Sprintf("items[%d].unitPrice is too high; maximum is %d", index, events.MaxMoneyAmountInCents))
		}
		if totalAmount > events.MaxMoneyAmountInCents-(item.Quantity*item.UnitPrice) {
			return errValidation(fmt.Sprintf("sale total amount is too high; maximum is %d", events.MaxMoneyAmountInCents))
		}
		totalAmount += item.Quantity * item.UnitPrice

		var ticketName string
		var ticketPrice int
		var availableQuantity int
		if err := db.QueryRow(ctx, `
			SELECT name, price, available_quantity
			FROM tickets
			WHERE id = $1 AND sales_event_id = $2
		`, item.TicketID, req.SalesEventID).Scan(&ticketName, &ticketPrice, &availableQuantity); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return errValidation(fmt.Sprintf("items[%d].ticketId does not exist for this sales event", index))
			}
			return err
		}
		if item.Quantity > availableQuantity {
			return errValidation(fmt.Sprintf("items[%d].quantity exceeds available tickets; available quantity is %d", index, availableQuantity))
		}
		if item.UnitPrice != ticketPrice {
			return errValidation(fmt.Sprintf("items[%d].unitPrice does not match ticket price", index))
		}
	}

	return nil
}

type errValidation string

func (e errValidation) Error() string {
	return string(e)
}
