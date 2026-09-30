package httpapi

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/varner/sales-event-project/internal/analyticsdb"
	"github.com/varner/sales-event-project/internal/correlation"
	"github.com/varner/sales-event-project/internal/events"
	"github.com/varner/sales-event-project/internal/metrics"
	"github.com/varner/sales-event-project/internal/observability"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"go.opentelemetry.io/otel/attribute"
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

		requestCtx, emailSpan := observability.StartBusinessSpan(c.Request.Context(), "email.event.record",
			attribute.String("sale.id", recordReq.SaleID),
			attribute.String("email.event_type", recordReq.EventType),
			attribute.String("email.provider_event_id", recordReq.ProviderEventID),
		)
		result, err := deps.Store.RecordEmailEvent(requestCtx, recordReq)
		if err == nil {
			emailSpan.SetAttributes(observability.CorrelationAttributes(correlation.ContextWithMetadata(requestCtx, result.Metadata))...)
			emailSpan.SetAttributes(attribute.String("email.status", result.Status))
		}
		observability.EndSpan(emailSpan, err)
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

		correlation.WriteHTTPHeaders(c.Writer.Header(), result.Metadata)
		logFields := append(correlation.LogFields(result.Metadata),
			"sale_id", result.SaleID,
			"status", result.Status,
			"provider_event_id", result.ProviderEventID,
		)
		slog.Info("email event recorded", logFields...)
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

		requestCtx, paymentSpan := observability.StartBusinessSpan(c.Request.Context(), "payment.webhook.process",
			attribute.String("sale.id", paymentReq.SaleID),
			attribute.String("payment.intent.id", paymentReq.PaymentIntentID),
			attribute.String("payment.provider", paymentReq.Provider),
			attribute.String("payment.status", paymentReq.Status),
			attribute.Int("payment.amount", paymentReq.Amount),
		)
		result, err := deps.Store.ProcessPaymentWebhook(requestCtx, paymentReq)
		if err == nil {
			paymentSpan.SetAttributes(observability.CorrelationAttributes(correlation.ContextWithMetadata(requestCtx, result.Metadata))...)
			paymentSpan.SetAttributes(attribute.String("sale.status", result.SaleStatus))
		}
		observability.EndSpan(paymentSpan, err)
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

		correlation.WriteHTTPHeaders(c.Writer.Header(), result.Metadata)
		metrics.PaymentsProcessedTotal.WithLabelValues(result.Payment.Status, result.Payment.Provider).Inc()
		if result.IdempotentReplay {
			metrics.PaymentWebhookReplaysTotal.WithLabelValues(result.Payment.Status, result.Payment.Provider).Inc()
		}
		logFields := append(correlation.LogFields(result.Metadata),
			"sale_id", result.SaleID,
			"sales_event_id", result.SalesEventID,
			"payment_status", result.Payment.Status,
			"sale_status", result.SaleStatus,
			"provider", result.Payment.Provider,
			"amount", result.Payment.Amount,
			"idempotent_replay", result.IdempotentReplay,
		)
		slog.Info("payment webhook processed", logFields...)
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

		requestCtx, paymentSpan := observability.StartBusinessSpan(c.Request.Context(), "payment.intent.create",
			attribute.String("sale.id", intentReq.SaleID),
			attribute.String("payment.provider", intentReq.Provider),
			attribute.Int("payment.amount", intentReq.Amount),
		)
		intent, err := deps.Store.CreatePaymentIntent(requestCtx, intentReq)
		if err == nil {
			paymentSpan.SetAttributes(append(observability.CorrelationAttributes(correlation.ContextWithMetadata(requestCtx, intent.Metadata)),
				attribute.String("payment.intent.id", intent.ID),
			)...)
		}
		observability.EndSpan(paymentSpan, err)
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

		correlation.WriteHTTPHeaders(c.Writer.Header(), intent.Metadata)
		logFields := append(correlation.LogFields(intent.Metadata),
			"sale_id", intent.SaleID,
			"payment_intent_id", intent.ID,
			"provider", intent.Provider,
			"amount", intent.Amount,
			"status", intent.Status,
		)
		slog.Info("payment intent created", logFields...)
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

		requestCtx, paymentSpan := observability.StartBusinessSpan(c.Request.Context(), "payment.process",
			attribute.String("sale.id", paymentReq.SaleID),
			attribute.String("payment.provider", paymentReq.Provider),
			attribute.String("payment.status", paymentReq.Status),
			attribute.Int("payment.amount", paymentReq.Amount),
		)
		result, err := deps.Store.ProcessPayment(requestCtx, paymentReq)
		if err == nil {
			paymentSpan.SetAttributes(observability.CorrelationAttributes(correlation.ContextWithMetadata(requestCtx, result.Metadata))...)
			paymentSpan.SetAttributes(attribute.String("sale.status", result.SaleStatus))
		}
		observability.EndSpan(paymentSpan, err)
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

		correlation.WriteHTTPHeaders(c.Writer.Header(), result.Metadata)
		metrics.PaymentsProcessedTotal.WithLabelValues(result.Payment.Status, result.Payment.Provider).Inc()
		logFields := append(correlation.LogFields(result.Metadata),
			"sale_id", result.SaleID,
			"sales_event_id", result.SalesEventID,
			"payment_status", result.Payment.Status,
			"sale_status", result.SaleStatus,
			"provider", result.Payment.Provider,
			"amount", result.Payment.Amount,
		)
		slog.Info("payment processed", logFields...)
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

		requestCtx, checkInSpan := observability.StartBusinessSpan(c.Request.Context(), "ticket.check_in",
			attribute.String("sales_event.id", checkInReq.SalesEventID),
		)
		result, err := deps.Store.CheckInTicket(requestCtx, checkInReq)
		if err == nil {
			checkInSpan.SetAttributes(append(observability.CorrelationAttributes(correlation.ContextWithMetadata(requestCtx, result.Metadata)),
				attribute.String("sale.id", result.SaleID),
				attribute.String("ticket.issued.id", result.IssuedTicketID),
			)...)
		}
		observability.EndSpan(checkInSpan, err)
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

		correlation.WriteHTTPHeaders(c.Writer.Header(), result.Metadata)
		metrics.CheckInsCreatedTotal.Inc()
		logFields := append(correlation.LogFields(result.Metadata),
			"check_in_id", result.CheckInID,
			"issued_ticket_id", result.IssuedTicketID,
			"sales_event_id", result.SalesEventID,
			"sale_id", result.SaleID,
		)
		slog.Info("ticket checked in", logFields...)
		c.JSON(http.StatusCreated, result)
	})

	return router
}
