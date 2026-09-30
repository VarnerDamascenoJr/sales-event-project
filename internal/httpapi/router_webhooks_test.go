package httpapi

import (
	"net/http"
	"testing"
	"time"

	"github.com/varner/sales-event-project/internal/correlation"
	"github.com/varner/sales-event-project/internal/events"
)

func TestPaymentWebhookProcessesApprovedPayment(t *testing.T) {
	now := time.Date(2026, 5, 30, 16, 0, 0, 0, time.UTC)
	metadata := correlation.Metadata{
		RequestID:     "req-sale-webhook",
		CorrelationID: "corr-sale-webhook",
		TransactionID: testSaleID,
	}
	router, _, _ := newTestRouter(fakeSalesStore{
		paymentWebhookResult: ProcessPaymentResult{
			SaleID:       testSaleID,
			SalesEventID: testSalesEventID,
			SaleStatus:   events.SaleCompletedStatus,
			Payment: PaymentDTO{
				Status:      events.PaymentApprovedStatus,
				Amount:      10000,
				Provider:    "credit_card",
				ProcessedAt: now,
			},
			Metadata: metadata,
		},
	})

	payload := map[string]any{
		"paymentIntentId":   "dddddddd-dddd-dddd-dddd-dddddddddddd",
		"saleId":            testSaleID,
		"provider":          "credit_card",
		"status":            events.PaymentApprovedStatus,
		"amount":            10000,
		"occurredAt":        now,
		"providerReference": "pay_test_123",
	}
	response := performSignedPaymentWebhook(t, router, payload)

	assertStatus(t, response, http.StatusAccepted)

	var body ProcessPaymentResult
	decodeResponse(t, response, &body)
	if body.SaleStatus != events.SaleCompletedStatus || body.Payment.Status != events.PaymentApprovedStatus {
		t.Fatalf("unexpected payment webhook response: %+v", body)
	}
	assertCorrelationHeaders(t, response, metadata)
}

func TestPaymentWebhookRejectsInvalidSignature(t *testing.T) {
	router, _, _ := newTestRouter(fakeSalesStore{})

	response := performRequestWithHeaders(t, router, http.MethodPost, "/webhooks/payments", map[string]any{
		"paymentIntentId": "dddddddd-dddd-dddd-dddd-dddddddddddd",
		"saleId":          testSaleID,
		"provider":        "credit_card",
		"status":          events.PaymentApprovedStatus,
		"amount":          10000,
		"occurredAt":      time.Now().UTC(),
	}, map[string]string{
		paymentSignatureHeader: "wrong-signature",
	})

	assertStatus(t, response, http.StatusUnauthorized)
	assertJSONField(t, response, "error", "webhook signature is invalid")
}

func TestEmailWebhookRecordsOpenedEvent(t *testing.T) {
	now := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	router, _, store := newTestRouter(fakeSalesStore{
		emailEventResult: RecordEmailEventResult{
			SaleID:          testSaleID,
			Status:          "OPENED",
			ProviderEventID: "provider-event-123",
			RecordedAt:      now,
		},
	})

	response := performRequestWithHeaders(t, router, http.MethodPost, "/webhooks/email-events", map[string]any{
		"saleId":          testSaleID,
		"eventType":       "OPENED",
		"occurredAt":      now,
		"providerEventId": "provider-event-123",
	}, map[string]string{
		webhookSecretHeader: "test-webhook-secret",
	})

	assertStatus(t, response, http.StatusAccepted)

	var body RecordEmailEventResult
	decodeResponse(t, response, &body)
	if body.Status != "OPENED" || body.ProviderEventID != "provider-event-123" {
		t.Fatalf("unexpected webhook response: %+v", body)
	}
	if store.lastEmailEventRequest.EventType != "OPENED" {
		t.Fatalf("expected webhook request to reach store, got %+v", store.lastEmailEventRequest)
	}
}

func TestEmailWebhookRejectsInvalidSecret(t *testing.T) {
	router, _, _ := newTestRouter(fakeSalesStore{})

	response := performRequestWithHeaders(t, router, http.MethodPost, "/webhooks/email-events", map[string]any{
		"saleId":     testSaleID,
		"eventType":  "OPENED",
		"occurredAt": time.Now().UTC(),
	}, map[string]string{
		webhookSecretHeader: "wrong-secret",
	})

	assertStatus(t, response, http.StatusUnauthorized)
	assertJSONField(t, response, "error", "webhook secret is invalid")
}

func TestEmailWebhookRejectsInvalidEventType(t *testing.T) {
	router, _, _ := newTestRouter(fakeSalesStore{})

	response := performRequestWithHeaders(t, router, http.MethodPost, "/webhooks/email-events", map[string]any{
		"saleId":     testSaleID,
		"eventType":  "UNKNOWN",
		"occurredAt": time.Now().UTC(),
	}, map[string]string{
		webhookSecretHeader: "test-webhook-secret",
	})

	assertStatus(t, response, http.StatusBadRequest)
	assertJSONField(t, response, "error", "eventType must be DELIVERED, OPENED, CLICKED, or BOUNCED")
}
