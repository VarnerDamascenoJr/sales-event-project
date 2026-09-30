package httpapi

import (
	"net/http"
	"testing"
	"time"

	"github.com/varner/sales-event-project/internal/correlation"
	"github.com/varner/sales-event-project/internal/events"
)

func TestCreatePaymentApprovesPendingSale(t *testing.T) {
	now := time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)
	router, broker, _ := newTestRouter(fakeSalesStore{
		paymentResult: ProcessPaymentResult{
			SaleID:       testSaleID,
			SalesEventID: testSalesEventID,
			SaleStatus:   events.SaleCompletedStatus,
			Payment: PaymentDTO{
				Status:      events.PaymentApprovedStatus,
				Amount:      10000,
				Provider:    "credit_card",
				ProcessedAt: now,
			},
		},
	})

	response := performRequestWithAPIKey(t, router, http.MethodPost, "/sales/"+testSaleID+"/payments", map[string]any{
		"amount":   10000,
		"provider": "credit_card",
	}, "payment-provider-key")

	assertStatus(t, response, http.StatusAccepted)

	var body ProcessPaymentResult
	decodeResponse(t, response, &body)
	if body.SaleStatus != events.SaleCompletedStatus || body.Payment.Status != events.PaymentApprovedStatus {
		t.Fatalf("unexpected payment response: %+v", body)
	}
	if len(broker.published) != 0 {
		t.Fatalf("expected payment endpoint to rely on outbox instead of direct publish, got %d published events", len(broker.published))
	}
}

func TestCreatePaymentIntentAcceptsPendingSale(t *testing.T) {
	now := time.Date(2026, 5, 30, 15, 0, 0, 0, time.UTC)
	metadata := correlation.Metadata{
		RequestID:     "req-sale-payment-intent",
		CorrelationID: "corr-sale-payment-intent",
		TransactionID: testSaleID,
	}
	router, _, _ := newTestRouter(fakeSalesStore{
		paymentIntentResult: PaymentIntentDTO{
			ID:                "dddddddd-dddd-dddd-dddd-dddddddddddd",
			SaleID:            testSaleID,
			Status:            events.PaymentPendingStatus,
			Provider:          "credit_card",
			Amount:            10000,
			ProviderReference: "pay_test_123",
			ClientSecret:      "pi_secret",
			CreatedAt:         now,
			UpdatedAt:         now,
			Metadata:          metadata,
		},
	})

	response := performRequest(t, router, http.MethodPost, "/sales/"+testSaleID+"/payment-intents", map[string]any{
		"amount":   10000,
		"provider": "credit_card",
	})

	assertStatus(t, response, http.StatusAccepted)

	var body PaymentIntentDTO
	decodeResponse(t, response, &body)
	if body.ID == "" || body.Status != events.PaymentPendingStatus || body.ProviderReference == "" {
		t.Fatalf("unexpected payment intent response: %+v", body)
	}
	assertCorrelationHeaders(t, response, metadata)
}

func TestCreatePaymentRejectsInvalidStatus(t *testing.T) {
	router, _, _ := newTestRouter(fakeSalesStore{})

	response := performRequestWithAPIKey(t, router, http.MethodPost, "/sales/"+testSaleID+"/payments", map[string]any{
		"amount":   10000,
		"provider": "credit_card",
		"status":   "UNKNOWN",
	}, "payment-provider-key")

	assertStatus(t, response, http.StatusBadRequest)
	assertJSONField(t, response, "error", "status must be APPROVED or FAILED")
}
