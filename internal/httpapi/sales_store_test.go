package httpapi

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/varner/sales-event-project/internal/correlation"
	"github.com/varner/sales-event-project/internal/events"
)

func TestPaymentOutboxPayloadIncludesCorrelationMetadata(t *testing.T) {
	metadata := correlation.Metadata{
		RequestID:     "req-demo",
		CorrelationID: "corr-demo",
		TransactionID: testSaleID,
	}

	payload, err := paymentOutboxPayload("event-demo", testSalesEventID, ProcessPaymentRequest{
		SaleID:   testSaleID,
		Amount:   10000,
		Provider: "unit-test",
		Status:   events.PaymentApprovedStatus,
	}, events.PaymentApprovedStatus, events.SaleCompletedStatus, time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC), metadata)
	if err != nil {
		t.Fatalf("build payment outbox payload: %v", err)
	}

	var event events.SaleCompleted
	if err := json.Unmarshal(payload, &event); err != nil {
		t.Fatalf("decode payment outbox payload: %v", err)
	}

	if event.Metadata.RequestID != metadata.RequestID {
		t.Fatalf("expected request id %q, got %q", metadata.RequestID, event.Metadata.RequestID)
	}
	if event.Metadata.CorrelationID != metadata.CorrelationID {
		t.Fatalf("expected correlation id %q, got %q", metadata.CorrelationID, event.Metadata.CorrelationID)
	}
	if event.Metadata.TransactionID != metadata.TransactionID {
		t.Fatalf("expected transaction id %q, got %q", metadata.TransactionID, event.Metadata.TransactionID)
	}
}
