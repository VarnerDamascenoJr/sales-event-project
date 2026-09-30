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

func registerWebhookRoutes(router *gin.Engine, deps RouterDeps) {
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
}
