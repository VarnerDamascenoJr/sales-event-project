package httpapi

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/varner/sales-event-project/internal/correlation"
	"github.com/varner/sales-event-project/internal/metrics"
	"github.com/varner/sales-event-project/internal/observability"
	"go.opentelemetry.io/otel/attribute"
)

func registerPaymentRoutes(router *gin.Engine, deps RouterDeps) {
	router.POST("/sales/:saleId/payment-intents", publicRateLimited(deps, func(c *gin.Context) {
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
}
