package httpapi

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/mail"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/varner/sales-event-project/internal/correlation"
	"github.com/varner/sales-event-project/internal/events"
	"github.com/varner/sales-event-project/internal/metrics"
	"github.com/varner/sales-event-project/internal/observability"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
)

type RouterDeps struct {
	Broker               EventPublisher
	DB                   *pgxpool.Pool
	Store                SalesStore
	AuthStore            AuthStore
	WebhookSecret        string
	PaymentWebhookSecret string
	MetricsProtected     bool
	PublicRateLimiter    *RateLimiter
}

type EventPublisher interface {
	PublishJSON(ctx context.Context, routingKey string, value any) error
}

type SalesStore interface {
	SalesEventExists(ctx context.Context, salesEventID string) (bool, error)
	GetTicketForEvent(ctx context.Context, ticketID string, salesEventID string) (TicketReadModel, error)
	ListSales(ctx context.Context, filter listSalesFilter) (ListSalesResponse, error)
	GetSale(ctx context.Context, salesEventID string, saleID string) (SaleDetailDTO, error)
	CreatePaymentIntent(ctx context.Context, req CreatePaymentIntentStoreRequest) (PaymentIntentDTO, error)
	ProcessPayment(ctx context.Context, req ProcessPaymentRequest) (ProcessPaymentResult, error)
	ProcessPaymentWebhook(ctx context.Context, req ProcessPaymentWebhookRequest) (ProcessPaymentResult, error)
	CheckInTicket(ctx context.Context, req CheckInTicketRequest) (CheckInTicketResult, error)
	RecordEmailEvent(ctx context.Context, req RecordEmailEventRequest) (RecordEmailEventResult, error)
}

type TicketReadModel struct {
	Name              string
	Price             int
	AvailableQuantity int
}

type CreateSaleRequest struct {
	SalesEventID  string           `json:"salesEventId"`
	CustomerID    string           `json:"customerId"`
	CustomerName  string           `json:"customerName"`
	CustomerEmail string           `json:"customerEmail"`
	Status        string           `json:"status"`
	Items         []CreateSaleItem `json:"items"`
}

type CreateSaleItem struct {
	TicketID  string `json:"ticketId"`
	Quantity  int    `json:"quantity"`
	UnitPrice int    `json:"unitPrice"`
}

type CreatePaymentRequest struct {
	Amount   int    `json:"amount"`
	Provider string `json:"provider"`
	Status   string `json:"status"`
}

type CreatePaymentIntentRequest struct {
	Amount   int    `json:"amount"`
	Provider string `json:"provider"`
}

type CreateCheckInRequest struct {
	TicketCode string `json:"ticketCode"`
}

type CreateEmailEventRequest struct {
	SaleID          string    `json:"saleId"`
	EventType       string    `json:"eventType"`
	OccurredAt      time.Time `json:"occurredAt"`
	ProviderEventID string    `json:"providerEventId"`
	Reason          string    `json:"reason"`
}

type CreatePaymentWebhookRequest struct {
	PaymentIntentID   string    `json:"paymentIntentId"`
	SaleID            string    `json:"saleId"`
	Provider          string    `json:"provider"`
	Status            string    `json:"status"`
	Amount            int       `json:"amount"`
	OccurredAt        time.Time `json:"occurredAt"`
	ProviderReference string    `json:"providerReference"`
}

type CreatePaymentIntentStoreRequest struct {
	SaleID   string
	Amount   int
	Provider string
}

type ProcessPaymentRequest struct {
	SaleID   string
	Amount   int
	Provider string
	Status   string
}

type ProcessPaymentWebhookRequest struct {
	PaymentIntentID   string
	SaleID            string
	Provider          string
	Status            string
	Amount            int
	OccurredAt        time.Time
	ProviderReference string
}

type CheckInTicketRequest struct {
	SalesEventID   string
	IssuedTicketID string
	TicketCode     string
}

type RecordEmailEventRequest struct {
	SaleID          string
	EventType       string
	OccurredAt      time.Time
	ProviderEventID string
	Reason          string
}

type ProcessPaymentResult struct {
	SaleID       string     `json:"saleId"`
	SalesEventID string     `json:"salesEventId"`
	SaleStatus   string     `json:"saleStatus"`
	Payment      PaymentDTO `json:"payment"`
}

type CheckInTicketResult struct {
	CheckInID      string    `json:"checkInId"`
	IssuedTicketID string    `json:"issuedTicketId"`
	SalesEventID   string    `json:"salesEventId"`
	SaleID         string    `json:"saleId"`
	TicketID       string    `json:"ticketId"`
	TicketName     string    `json:"ticketName"`
	CustomerID     string    `json:"customerId"`
	CustomerName   string    `json:"customerName"`
	CheckedInAt    time.Time `json:"checkedInAt"`
}

type RecordEmailEventResult struct {
	SaleID          string    `json:"saleId"`
	Status          string    `json:"status"`
	ProviderEventID string    `json:"providerEventId"`
	RecordedAt      time.Time `json:"recordedAt"`
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
	CustomerName   string    `json:"customerName"`
	CustomerEmail  string    `json:"customerEmail"`
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
	CustomerName   string            `json:"customerName"`
	CustomerEmail  string            `json:"customerEmail"`
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

type PaymentIntentDTO struct {
	ID                string    `json:"id"`
	SaleID            string    `json:"saleId"`
	Status            string    `json:"status"`
	Provider          string    `json:"provider"`
	Amount            int       `json:"amount"`
	ProviderReference string    `json:"providerReference"`
	ClientSecret      string    `json:"clientSecret"`
	CreatedAt         time.Time `json:"createdAt"`
	UpdatedAt         time.Time `json:"updatedAt"`
}

type SaleItemReadDTO struct {
	TicketID   string    `json:"ticketId"`
	TicketName string    `json:"ticketName"`
	Quantity   int       `json:"quantity"`
	UnitPrice  int       `json:"unitPrice"`
	CreatedAt  time.Time `json:"createdAt"`
}

func NewRouter(deps RouterDeps) *gin.Engine {
	if deps.Store == nil && deps.DB != nil {
		deps.Store = NewPostgresSalesStore(deps.DB)
	}
	if deps.AuthStore == nil && deps.DB != nil {
		deps.AuthStore = NewPostgresAuthStore(deps.DB)
	}

	router := gin.New()
	router.Use(gin.Recovery(), correlationMiddleware(), otelgin.Middleware("sales-event-api"), observability.GinCorrelationMiddleware(), metrics.GinMiddleware())

	router.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	metricsHandler := gin.WrapH(promhttp.Handler())
	if deps.MetricsProtected {
		router.GET("/metrics", requireRoles(deps.AuthStore, RoleAdmin), metricsHandler)
	} else {
		router.GET("/metrics", metricsHandler)
	}

	router.POST("/webhooks/email-events", requireWebhookSecret(deps.WebhookSecret), func(c *gin.Context) {
		var req CreateEmailEventRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		recordReq := RecordEmailEventRequest{
			SaleID:          req.SaleID,
			EventType:       req.EventType,
			OccurredAt:      req.OccurredAt,
			ProviderEventID: req.ProviderEventID,
			Reason:          req.Reason,
		}
		if err := validateEmailEventRequest(recordReq); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		result, err := deps.Store.RecordEmailEvent(c.Request.Context(), recordReq)
		if err != nil {
			status := http.StatusBadRequest
			switch {
			case errors.Is(err, errSaleNotFound):
				status = http.StatusNotFound
			default:
				var validationErr errValidation
				if !errors.As(err, &validationErr) {
					status = http.StatusInternalServerError
				}
			}
			c.JSON(status, gin.H{"error": err.Error()})
			return
		}

		slog.Info("email event recorded",
			"sale_id", result.SaleID,
			"status", result.Status,
			"provider_event_id", result.ProviderEventID,
		)
		c.JSON(http.StatusAccepted, result)
	})

	router.POST("/webhooks/payments", requireBodyHMACSignature(deps.PaymentWebhookSecret, paymentSignatureHeader), func(c *gin.Context) {
		var req CreatePaymentWebhookRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		paymentReq := ProcessPaymentWebhookRequest{
			PaymentIntentID:   req.PaymentIntentID,
			SaleID:            req.SaleID,
			Provider:          req.Provider,
			Status:            req.Status,
			Amount:            req.Amount,
			OccurredAt:        req.OccurredAt,
			ProviderReference: req.ProviderReference,
		}
		if err := validatePaymentWebhookRequest(paymentReq); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		result, err := deps.Store.ProcessPaymentWebhook(c.Request.Context(), paymentReq)
		if err != nil {
			status := http.StatusBadRequest
			switch {
			case errors.Is(err, errSaleNotFound), errors.Is(err, errPaymentIntentNotFound):
				status = http.StatusNotFound
			case errors.Is(err, errSaleAlreadyPaid), errors.Is(err, errSaleCannotBePaid):
				status = http.StatusConflict
			default:
				var validationErr errValidation
				if !errors.As(err, &validationErr) {
					status = http.StatusInternalServerError
				}
			}
			c.JSON(status, gin.H{"error": err.Error()})
			return
		}

		metrics.PaymentsProcessedTotal.WithLabelValues(result.Payment.Status, result.Payment.Provider).Inc()
		slog.Info("payment webhook processed",
			"sale_id", result.SaleID,
			"sales_event_id", result.SalesEventID,
			"payment_status", result.Payment.Status,
			"sale_status", result.SaleStatus,
			"provider", result.Payment.Provider,
			"amount", result.Payment.Amount,
		)
		c.JSON(http.StatusAccepted, result)
	})

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

	publicRateLimited := func(handler gin.HandlerFunc) gin.HandlerFunc {
		if deps.PublicRateLimiter == nil {
			return handler
		}
		return func(c *gin.Context) {
			deps.PublicRateLimiter.Middleware()(c)
			if c.IsAborted() {
				return
			}
			handler(c)
		}
	}

	router.POST("/sales", publicRateLimited(func(c *gin.Context) {
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

	router.POST("/sales/:saleId/payment-intents", publicRateLimited(func(c *gin.Context) {
		var req CreatePaymentIntentRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		intentReq := CreatePaymentIntentStoreRequest{
			SaleID:   c.Param("saleId"),
			Amount:   req.Amount,
			Provider: req.Provider,
		}
		if err := validateCreatePaymentIntentRequest(intentReq); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		intent, err := deps.Store.CreatePaymentIntent(c.Request.Context(), intentReq)
		if err != nil {
			status := http.StatusBadRequest
			switch {
			case errors.Is(err, errSaleNotFound):
				status = http.StatusNotFound
			case errors.Is(err, errSaleAlreadyPaid), errors.Is(err, errSaleCannotBePaid):
				status = http.StatusConflict
			default:
				var validationErr errValidation
				if !errors.As(err, &validationErr) {
					status = http.StatusInternalServerError
				}
			}
			c.JSON(status, gin.H{"error": err.Error()})
			return
		}

		slog.Info("payment intent created",
			"sale_id", intent.SaleID,
			"payment_intent_id", intent.ID,
			"provider", intent.Provider,
			"amount", intent.Amount,
			"status", intent.Status,
		)
		c.JSON(http.StatusAccepted, intent)
	}))

	router.POST("/sales/:saleId/payments", requireRoles(deps.AuthStore, RolePaymentProvider, RoleAdmin), func(c *gin.Context) {
		var req CreatePaymentRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		paymentReq := ProcessPaymentRequest{
			SaleID:   c.Param("saleId"),
			Amount:   req.Amount,
			Provider: req.Provider,
			Status:   req.Status,
		}

		if err := validatePaymentRequest(paymentReq); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		result, err := deps.Store.ProcessPayment(c.Request.Context(), paymentReq)
		if err != nil {
			status := http.StatusBadRequest
			switch {
			case errors.Is(err, errSaleNotFound):
				status = http.StatusNotFound
			case errors.Is(err, errSaleAlreadyPaid), errors.Is(err, errSaleCannotBePaid):
				status = http.StatusConflict
			default:
				var validationErr errValidation
				if !errors.As(err, &validationErr) {
					status = http.StatusInternalServerError
				}
			}
			c.JSON(status, gin.H{"error": err.Error()})
			return
		}

		metrics.PaymentsProcessedTotal.WithLabelValues(result.Payment.Status, result.Payment.Provider).Inc()
		slog.Info("payment processed",
			"sale_id", result.SaleID,
			"sales_event_id", result.SalesEventID,
			"payment_status", result.Payment.Status,
			"sale_status", result.SaleStatus,
			"provider", result.Payment.Provider,
			"amount", result.Payment.Amount,
		)
		c.JSON(http.StatusAccepted, result)
	})

	router.POST("/sales-events/:salesEventId/check-ins", requireRoles(deps.AuthStore, RoleCheckIn, RoleAdmin), func(c *gin.Context) {
		var req CreateCheckInRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		checkInReq := CheckInTicketRequest{
			SalesEventID: c.Param("salesEventId"),
			TicketCode:   req.TicketCode,
		}
		if err := validateCheckInRequest(&checkInReq); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		result, err := deps.Store.CheckInTicket(c.Request.Context(), checkInReq)
		if err != nil {
			status := http.StatusBadRequest
			switch {
			case errors.Is(err, errSalesEventNotFound), errors.Is(err, errIssuedTicketNotFound):
				status = http.StatusNotFound
			case errors.Is(err, errTicketAlreadyCheckedIn), errors.Is(err, errTicketWrongEvent), errors.Is(err, errTicketCannotBeCheckedIn):
				status = http.StatusConflict
			default:
				var validationErr errValidation
				if !errors.As(err, &validationErr) {
					status = http.StatusInternalServerError
				}
			}
			c.JSON(status, gin.H{"error": err.Error()})
			return
		}

		slog.Info("ticket checked in",
			"check_in_id", result.CheckInID,
			"issued_ticket_id", result.IssuedTicketID,
			"sales_event_id", result.SalesEventID,
			"sale_id", result.SaleID,
		)
		c.JSON(http.StatusCreated, result)
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

func listSales(ctx context.Context, store SalesStore, filter listSalesFilter) (ListSalesResponse, error) {
	if err := validateSalesEventID(filter.SalesEventID); err != nil {
		return ListSalesResponse{}, err
	}
	if err := validateSaleStatusFilter(filter.Status); err != nil {
		return ListSalesResponse{}, err
	}
	if len(filter.EventName) > events.MaxTextLength {
		return ListSalesResponse{}, errValidation("eventName is too long; maximum length is 50 characters")
	}

	eventExists, err := store.SalesEventExists(ctx, filter.SalesEventID)
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

	return store.ListSales(ctx, filter)
}

func getSale(ctx context.Context, store SalesStore, salesEventID string, saleID string) (SaleDetailDTO, error) {
	if err := validateSalesEventID(salesEventID); err != nil {
		return SaleDetailDTO{}, err
	}
	if _, err := uuid.Parse(saleID); err != nil {
		return SaleDetailDTO{}, errValidation("saleId must be a valid UUID")
	}

	eventExists, err := store.SalesEventExists(ctx, salesEventID)
	if err != nil {
		return SaleDetailDTO{}, err
	}
	if !eventExists {
		return SaleDetailDTO{}, errSalesEventNotFound
	}

	return store.GetSale(ctx, salesEventID, saleID)
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

func validatePaymentRequest(req ProcessPaymentRequest) error {
	if _, err := uuid.Parse(req.SaleID); err != nil {
		return errValidation("saleId must be a valid UUID")
	}
	if req.Amount <= 0 {
		return errValidation("amount must be greater than zero")
	}
	if req.Amount > events.MaxMoneyAmountInCents {
		return errValidation(fmt.Sprintf("amount is too high; maximum is %d", events.MaxMoneyAmountInCents))
	}
	if req.Provider == "" {
		return errValidation("provider is required")
	}
	if len(req.Provider) > events.MaxTextLength {
		return errValidation("provider is too long; maximum length is 50 characters")
	}
	if req.Status == "" {
		return nil
	}
	if req.Status != events.PaymentApprovedStatus && req.Status != events.PaymentFailedStatus {
		return errValidation("status must be APPROVED or FAILED")
	}
	return nil
}

func validateCreatePaymentIntentRequest(req CreatePaymentIntentStoreRequest) error {
	if _, err := uuid.Parse(req.SaleID); err != nil {
		return errValidation("saleId must be a valid UUID")
	}
	if req.Amount <= 0 {
		return errValidation("amount must be greater than zero")
	}
	if req.Amount > events.MaxMoneyAmountInCents {
		return errValidation(fmt.Sprintf("amount is too high; maximum is %d", events.MaxMoneyAmountInCents))
	}
	if req.Provider == "" {
		return errValidation("provider is required")
	}
	if len(req.Provider) > events.MaxTextLength {
		return errValidation("provider is too long; maximum length is 50 characters")
	}
	return nil
}

func validatePaymentWebhookRequest(req ProcessPaymentWebhookRequest) error {
	if _, err := uuid.Parse(req.PaymentIntentID); err != nil {
		return errValidation("paymentIntentId must be a valid UUID")
	}
	if err := validatePaymentRequest(ProcessPaymentRequest{
		SaleID:   req.SaleID,
		Amount:   req.Amount,
		Provider: req.Provider,
		Status:   req.Status,
	}); err != nil {
		return err
	}
	if req.OccurredAt.IsZero() {
		return errValidation("occurredAt is required")
	}
	if len(req.ProviderReference) > 100 {
		return errValidation("providerReference is too long; maximum length is 100 characters")
	}
	return nil
}

func validateCheckInRequest(req *CheckInTicketRequest) error {
	if err := validateSalesEventID(req.SalesEventID); err != nil {
		return err
	}
	if req.TicketCode == "" {
		return errValidation("ticketCode is required")
	}
	if len(req.TicketCode) > 255 {
		return errValidation("ticketCode is too long; maximum length is 255 characters")
	}

	const payloadPrefix = "issued_ticket:"
	issuedTicketID := req.TicketCode
	if len(req.TicketCode) > len(payloadPrefix) && req.TicketCode[:len(payloadPrefix)] == payloadPrefix {
		issuedTicketID = req.TicketCode[len(payloadPrefix):]
	}
	if _, err := uuid.Parse(issuedTicketID); err != nil {
		return errValidation("ticketCode must contain a valid issued ticket id")
	}
	req.IssuedTicketID = issuedTicketID
	return nil
}

func validateEmailEventRequest(req RecordEmailEventRequest) error {
	if _, err := uuid.Parse(req.SaleID); err != nil {
		return errValidation("saleId must be a valid UUID")
	}
	switch req.EventType {
	case "DELIVERED", "OPENED", "CLICKED", "BOUNCED":
	default:
		return errValidation("eventType must be DELIVERED, OPENED, CLICKED, or BOUNCED")
	}
	if req.OccurredAt.IsZero() {
		return errValidation("occurredAt is required")
	}
	if len(req.ProviderEventID) > 100 {
		return errValidation("providerEventId is too long; maximum length is 100 characters")
	}
	if len(req.Reason) > 1000 {
		return errValidation("reason is too long; maximum length is 1000 characters")
	}
	return nil
}

func validateSaleRequest(ctx context.Context, store SalesStore, req CreateSaleRequest) error {
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
	if req.CustomerName == "" {
		return errValidation("customerName is required")
	}
	if len(req.CustomerName) > events.MaxCustomerNameLength {
		return errValidation("customerName is too long; maximum length is 100 characters")
	}
	if req.CustomerEmail == "" {
		return errValidation("customerEmail is required")
	}
	if len(req.CustomerEmail) > events.MaxEmailLength {
		return errValidation("customerEmail is too long; maximum length is 255 characters")
	}
	if _, err := mail.ParseAddress(req.CustomerEmail); err != nil {
		return errValidation("customerEmail must be a valid email address")
	}
	if req.Status != "" && req.Status != events.SaleProcessingStatus {
		return errValidation("status must be PROCESSING when creating a sale")
	}
	if len(req.Items) == 0 {
		return errValidation("items must contain at least one ticket")
	}

	var eventExists bool
	eventExists, err := store.SalesEventExists(ctx, req.SalesEventID)
	if err != nil {
		return err
	}
	if !eventExists {
		return errValidation("sales event does not exist")
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

		ticket, err := store.GetTicketForEvent(ctx, item.TicketID, req.SalesEventID)
		if err != nil {
			if errors.Is(err, errTicketNotFound) {
				return errValidation(fmt.Sprintf("items[%d].ticketId does not exist for this sales event", index))
			}
			return err
		}
		if item.Quantity > ticket.AvailableQuantity {
			return errValidation(fmt.Sprintf("items[%d].quantity exceeds available tickets; available quantity is %d", index, ticket.AvailableQuantity))
		}
		if item.UnitPrice != ticket.Price {
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
	errSalesEventNotFound      = errors.New("sales event does not exist")
	errSaleNotFound            = errors.New("sale does not exist for this sales event")
	errPaymentIntentNotFound   = errors.New("payment intent does not exist")
	errTicketNotFound          = errors.New("ticket does not exist for this sales event")
	errIssuedTicketNotFound    = errors.New("issued ticket does not exist")
	errTicketWrongEvent        = errors.New("issued ticket does not belong to this sales event")
	errTicketAlreadyCheckedIn  = errors.New("ticket is already checked in")
	errTicketCannotBeCheckedIn = errors.New("ticket cannot be checked in because sale is not completed")
	errSaleAlreadyPaid         = errors.New("sale is already paid")
	errSaleCannotBePaid        = errors.New("sale cannot be paid in its current status")
)
