package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
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

type ListSalesResponse struct {
	Page     int               `json:"page"`
	PageSize int               `json:"pageSize"`
	Total    int               `json:"total"`
	Data     []SaleListItemDTO `json:"data"`
}

type SaleListItemDTO struct {
	ID             string    `json:"id"`
	SalesEventID   string    `json:"salesEventId"`
	SalesEventName string    `json:"salesEventName"`
	CustomerID     string    `json:"customerId"`
	Status         string    `json:"status"`
	TotalAmount    int       `json:"totalAmount"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

type SaleDetailDTO struct {
	ID             string            `json:"id"`
	SalesEventID   string            `json:"salesEventId"`
	SalesEventName string            `json:"salesEventName"`
	CustomerID     string            `json:"customerId"`
	Status         string            `json:"status"`
	TotalAmount    int               `json:"totalAmount"`
	Payment        PaymentDTO        `json:"payment"`
	Items          []SaleItemReadDTO `json:"items"`
	CreatedAt      time.Time         `json:"createdAt"`
	UpdatedAt      time.Time         `json:"updatedAt"`
}

type PaymentDTO struct {
	Status      string    `json:"status"`
	Amount      int       `json:"amount"`
	Provider    string    `json:"provider"`
	ProcessedAt time.Time `json:"processedAt"`
}

type SaleItemReadDTO struct {
	TicketID   string    `json:"ticketId"`
	TicketName string    `json:"ticketName"`
	Quantity   int       `json:"quantity"`
	UnitPrice  int       `json:"unitPrice"`
	CreatedAt  time.Time `json:"createdAt"`
}

func NewRouter(deps RouterDeps) *gin.Engine {
	router := gin.New()
	router.Use(gin.Recovery(), metrics.GinMiddleware())

	router.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	router.GET("/metrics", gin.WrapH(promhttp.Handler()))

	router.GET("/sales-events/:salesEventId/sales", func(c *gin.Context) {
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

		response, err := listSales(c.Request.Context(), deps.DB, filter)
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

	router.GET("/sales-events/:salesEventId/sales/:saleId", func(c *gin.Context) {
		sale, err := getSale(c.Request.Context(), deps.DB, c.Param("salesEventId"), c.Param("saleId"))
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

type listSalesFilter struct {
	SalesEventID string
	Status       string
	EventName    string
	Page         int
	PageSize     int
}

func parsePagination(c *gin.Context) (int, int, error) {
	page := 1
	pageSize := 20

	if rawPage := c.Query("page"); rawPage != "" {
		parsedPage, err := strconv.Atoi(rawPage)
		if err != nil {
			return 0, 0, errValidation("page must be a valid integer")
		}
		page = parsedPage
	}

	if rawPageSize := c.Query("pageSize"); rawPageSize != "" {
		parsedPageSize, err := strconv.Atoi(rawPageSize)
		if err != nil {
			return 0, 0, errValidation("pageSize must be a valid integer")
		}
		pageSize = parsedPageSize
	}

	if page < 1 {
		return 0, 0, errValidation("page must be greater than or equal to 1")
	}
	if pageSize < 1 {
		return 0, 0, errValidation("pageSize must be greater than or equal to 1")
	}
	if pageSize > 100 {
		return 0, 0, errValidation("pageSize must be less than or equal to 100")
	}

	return page, pageSize, nil
}

func listSales(ctx context.Context, db *pgxpool.Pool, filter listSalesFilter) (ListSalesResponse, error) {
	if err := validateSalesEventID(filter.SalesEventID); err != nil {
		return ListSalesResponse{}, err
	}
	if err := validateSaleStatusFilter(filter.Status); err != nil {
		return ListSalesResponse{}, err
	}
	if len(filter.EventName) > events.MaxTextLength {
		return ListSalesResponse{}, errValidation("eventName is too long; maximum length is 50 characters")
	}

	eventExists, err := salesEventExists(ctx, db, filter.SalesEventID)
	if err != nil {
		return ListSalesResponse{}, err
	}
	if !eventExists {
		return ListSalesResponse{
			Page:     filter.Page,
			PageSize: filter.PageSize,
			Total:    0,
			Data:     []SaleListItemDTO{},
		}, nil
	}

	offset := (filter.Page - 1) * filter.PageSize

	var total int
	if err := db.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM sales s
		JOIN sales_events se ON se.id = s.sales_event_id
		WHERE s.sales_event_id = $1
		  AND ($2 = '' OR s.status = $2)
		  AND ($3 = '' OR se.name ILIKE '%' || $3 || '%')
	`, filter.SalesEventID, filter.Status, filter.EventName).Scan(&total); err != nil {
		return ListSalesResponse{}, err
	}

	rows, err := db.Query(ctx, `
		SELECT s.id, s.sales_event_id, se.name, s.customer_id, s.status, s.total_amount, s.created_at, s.updated_at
		FROM sales s
		JOIN sales_events se ON se.id = s.sales_event_id
		WHERE s.sales_event_id = $1
		  AND ($2 = '' OR s.status = $2)
		  AND ($3 = '' OR se.name ILIKE '%' || $3 || '%')
		ORDER BY s.created_at DESC
		LIMIT $4 OFFSET $5
	`, filter.SalesEventID, filter.Status, filter.EventName, filter.PageSize, offset)
	if err != nil {
		return ListSalesResponse{}, err
	}
	defer rows.Close()

	data := make([]SaleListItemDTO, 0)
	for rows.Next() {
		var sale SaleListItemDTO
		if err := rows.Scan(
			&sale.ID,
			&sale.SalesEventID,
			&sale.SalesEventName,
			&sale.CustomerID,
			&sale.Status,
			&sale.TotalAmount,
			&sale.CreatedAt,
			&sale.UpdatedAt,
		); err != nil {
			return ListSalesResponse{}, err
		}
		data = append(data, sale)
	}
	if err := rows.Err(); err != nil {
		return ListSalesResponse{}, err
	}

	return ListSalesResponse{
		Page:     filter.Page,
		PageSize: filter.PageSize,
		Total:    total,
		Data:     data,
	}, nil
}

func getSale(ctx context.Context, db *pgxpool.Pool, salesEventID string, saleID string) (SaleDetailDTO, error) {
	if err := validateSalesEventID(salesEventID); err != nil {
		return SaleDetailDTO{}, err
	}
	if _, err := uuid.Parse(saleID); err != nil {
		return SaleDetailDTO{}, errValidation("saleId must be a valid UUID")
	}

	eventExists, err := salesEventExists(ctx, db, salesEventID)
	if err != nil {
		return SaleDetailDTO{}, err
	}
	if !eventExists {
		return SaleDetailDTO{}, errSalesEventNotFound
	}

	var sale SaleDetailDTO
	if err := db.QueryRow(ctx, `
		SELECT s.id, s.sales_event_id, se.name, s.customer_id, s.status, s.total_amount,
		       p.status, p.amount, p.provider, p.processed_at,
		       s.created_at, s.updated_at
		FROM sales s
		JOIN sales_events se ON se.id = s.sales_event_id
		JOIN payments p ON p.sale_id = s.id
		WHERE s.sales_event_id = $1
		  AND s.id = $2
	`, salesEventID, saleID).Scan(
		&sale.ID,
		&sale.SalesEventID,
		&sale.SalesEventName,
		&sale.CustomerID,
		&sale.Status,
		&sale.TotalAmount,
		&sale.Payment.Status,
		&sale.Payment.Amount,
		&sale.Payment.Provider,
		&sale.Payment.ProcessedAt,
		&sale.CreatedAt,
		&sale.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return SaleDetailDTO{}, errSaleNotFound
		}
		return SaleDetailDTO{}, err
	}

	rows, err := db.Query(ctx, `
		SELECT si.ticket_id, t.name, si.quantity, si.unit_price, si.created_at
		FROM sale_items si
		JOIN tickets t ON t.id = si.ticket_id
		WHERE si.sale_id = $1
		ORDER BY si.created_at ASC
	`, saleID)
	if err != nil {
		return SaleDetailDTO{}, err
	}
	defer rows.Close()

	sale.Items = make([]SaleItemReadDTO, 0)
	for rows.Next() {
		var item SaleItemReadDTO
		if err := rows.Scan(&item.TicketID, &item.TicketName, &item.Quantity, &item.UnitPrice, &item.CreatedAt); err != nil {
			return SaleDetailDTO{}, err
		}
		sale.Items = append(sale.Items, item)
	}
	if err := rows.Err(); err != nil {
		return SaleDetailDTO{}, err
	}

	return sale, nil
}

func validateSalesEventID(salesEventID string) error {
	if salesEventID == "" {
		return errValidation("salesEventId is required")
	}
	if _, err := uuid.Parse(salesEventID); err != nil {
		return errValidation("salesEventId must be a valid UUID")
	}
	return nil
}

func validateSaleStatusFilter(status string) error {
	if status == "" {
		return nil
	}
	for _, availableStatus := range events.AvailableSaleStatuses {
		if status == availableStatus {
			return nil
		}
	}
	return errValidation("status is invalid")
}

func salesEventExists(ctx context.Context, db *pgxpool.Pool, salesEventID string) (bool, error) {
	var exists bool
	err := db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM sales_events
			WHERE id = $1
		)
	`, salesEventID).Scan(&exists)
	return exists, err
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

var (
	errSalesEventNotFound = errors.New("sales event does not exist")
	errSaleNotFound       = errors.New("sale does not exist for this sales event")
)
