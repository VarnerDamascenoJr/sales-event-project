package httpapi

import (
	"net/http"
	"testing"
	"time"

	"github.com/varner/sales-event-project/internal/correlation"
)

func TestCheckInTicketReturnsCreated(t *testing.T) {
	now := time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)
	issuedTicketID := "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	metadata := correlation.Metadata{
		RequestID:     "req-sale-check-in",
		CorrelationID: "corr-sale-check-in",
		TransactionID: testSaleID,
	}
	router, _, store := newTestRouter(fakeSalesStore{
		checkInResult: CheckInTicketResult{
			CheckInID:      "cccccccc-cccc-cccc-cccc-cccccccccccc",
			IssuedTicketID: issuedTicketID,
			SalesEventID:   testSalesEventID,
			SaleID:         testSaleID,
			TicketID:       testTicketID,
			TicketName:     "General Admission",
			CustomerID:     "customer-001",
			CustomerName:   "Ada Lovelace",
			CheckedInAt:    now,
			Metadata:       metadata,
		},
	})

	response := performRequestWithAPIKey(t, router, http.MethodPost, "/sales-events/"+testSalesEventID+"/check-ins", map[string]any{
		"ticketCode": "issued_ticket:" + issuedTicketID,
	}, "dev-check-in-key")

	assertStatus(t, response, http.StatusCreated)

	var body CheckInTicketResult
	decodeResponse(t, response, &body)
	if body.IssuedTicketID != issuedTicketID || body.CheckInID == "" {
		t.Fatalf("unexpected check-in response: %+v", body)
	}
	if store.lastCheckInRequest.IssuedTicketID != issuedTicketID {
		t.Fatalf("expected issued ticket id to be parsed from QR payload, got %+v", store.lastCheckInRequest)
	}
	assertCorrelationHeaders(t, response, metadata)
}

func TestCheckInTicketRejectsInvalidTicketCode(t *testing.T) {
	router, _, _ := newTestRouter(fakeSalesStore{})

	response := performRequestWithAPIKey(t, router, http.MethodPost, "/sales-events/"+testSalesEventID+"/check-ins", map[string]any{
		"ticketCode": "issued_ticket:not-a-uuid",
	}, "dev-check-in-key")

	assertStatus(t, response, http.StatusBadRequest)
	assertJSONField(t, response, "error", "ticketCode must contain a valid issued ticket id")
}

func TestCheckInTicketRejectsDuplicate(t *testing.T) {
	router, _, _ := newTestRouter(fakeSalesStore{
		checkInErr: errTicketAlreadyCheckedIn,
	})

	response := performRequestWithAPIKey(t, router, http.MethodPost, "/sales-events/"+testSalesEventID+"/check-ins", map[string]any{
		"ticketCode": "issued_ticket:bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb",
	}, "dev-check-in-key")

	assertStatus(t, response, http.StatusConflict)
	assertJSONField(t, response, "error", "ticket is already checked in")
}
