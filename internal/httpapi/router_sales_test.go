package httpapi

import (
	"net/http"
	"testing"
	"time"

	"github.com/varner/sales-event-project/internal/correlation"
	"github.com/varner/sales-event-project/internal/events"
)

func TestCreateSalePublishesEvent(t *testing.T) {
	router, broker, _ := newTestRouter(fakeSalesStore{
		salesEventExists: true,
		ticket: TicketReadModel{
			Name:              "General Admission",
			Price:             10000,
			AvailableQuantity: 10,
		},
	})

	response := performRequest(t, router, http.MethodPost, "/sales", map[string]any{
		"salesEventId":  testSalesEventID,
		"customerId":    "customer-001",
		"customerName":  "Ada Lovelace",
		"customerEmail": "ada@example.com",
		"items": []map[string]any{
			{
				"ticketId":  testTicketID,
				"quantity":  2,
				"unitPrice": 10000,
			},
		},
	})

	if response.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d: %s", http.StatusAccepted, response.Code, response.Body.String())
	}
	if len(broker.published) != 1 {
		t.Fatalf("expected one published event, got %d", len(broker.published))
	}
	if broker.published[0].routingKey != events.SaleCreatedRoutingKey {
		t.Fatalf("expected routing key %q, got %q", events.SaleCreatedRoutingKey, broker.published[0].routingKey)
	}
	if response.Header().Get(correlation.RequestIDHeader) == "" {
		t.Fatal("expected response to include request id")
	}
	if response.Header().Get(correlation.CorrelationIDHeader) == "" {
		t.Fatal("expected response to include correlation id")
	}
	if response.Header().Get(correlation.TransactionIDHeader) == "" {
		t.Fatal("expected response to include transaction id")
	}

	event, ok := broker.published[0].value.(events.SaleCreated)
	if !ok {
		t.Fatalf("expected published value to be events.SaleCreated, got %T", broker.published[0].value)
	}
	if event.SalesEventID != testSalesEventID || event.CustomerID != "customer-001" || event.CustomerEmail != "ada@example.com" {
		t.Fatalf("published event has unexpected payload: %+v", event)
	}
}

func TestCreateSalePropagatesCorrelationMetadata(t *testing.T) {
	router, broker, _ := newTestRouter(fakeSalesStore{
		salesEventExists: true,
		ticket: TicketReadModel{
			Name:              "General Admission",
			Price:             10000,
			AvailableQuantity: 10,
		},
	})

	response := performRequestWithHeaders(t, router, http.MethodPost, "/sales", map[string]any{
		"salesEventId":  testSalesEventID,
		"customerId":    "customer-001",
		"customerName":  "Ada Lovelace",
		"customerEmail": "ada@example.com",
		"items": []map[string]any{
			{
				"ticketId":  testTicketID,
				"quantity":  2,
				"unitPrice": 10000,
			},
		},
	}, map[string]string{
		correlation.RequestIDHeader:     "req-demo",
		correlation.CorrelationIDHeader: "corr-demo",
	})

	assertStatus(t, response, http.StatusAccepted)
	if len(broker.published) != 1 {
		t.Fatalf("expected one published event, got %d", len(broker.published))
	}

	var body struct {
		SaleID string `json:"saleId"`
	}
	decodeResponse(t, response, &body)

	if response.Header().Get(correlation.RequestIDHeader) != "req-demo" {
		t.Fatalf("expected request id header to be preserved, got %q", response.Header().Get(correlation.RequestIDHeader))
	}
	if response.Header().Get(correlation.CorrelationIDHeader) != "corr-demo" {
		t.Fatalf("expected correlation id header to be preserved, got %q", response.Header().Get(correlation.CorrelationIDHeader))
	}
	if response.Header().Get(correlation.TransactionIDHeader) != body.SaleID {
		t.Fatalf("expected transaction id header to be sale id %q, got %q", body.SaleID, response.Header().Get(correlation.TransactionIDHeader))
	}

	event := broker.published[0].value.(events.SaleCreated)
	if event.Metadata.RequestID != "req-demo" {
		t.Fatalf("expected event request id to be propagated, got %q", event.Metadata.RequestID)
	}
	if event.Metadata.CorrelationID != "corr-demo" || event.Metadata.TransactionID != body.SaleID {
		t.Fatalf("unexpected event metadata: %+v", event.Metadata)
	}
	if broker.published[0].metadata.CorrelationID != "corr-demo" {
		t.Fatalf("expected broker context metadata to be propagated, got %+v", broker.published[0].metadata)
	}
	if broker.published[0].metadata.TransactionID != body.SaleID {
		t.Fatalf("expected broker context transaction id to be sale id %q, got %+v", body.SaleID, broker.published[0].metadata)
	}
}

func TestCreateSaleRejectsNegativeQuantity(t *testing.T) {
	router, broker, _ := newTestRouter(fakeSalesStore{
		salesEventExists: true,
		ticket: TicketReadModel{
			Price:             10000,
			AvailableQuantity: 10,
		},
	})

	response := performRequest(t, router, http.MethodPost, "/sales", map[string]any{
		"salesEventId":  testSalesEventID,
		"customerId":    "customer-001",
		"customerName":  "Ada Lovelace",
		"customerEmail": "ada@example.com",
		"items": []map[string]any{
			{
				"ticketId":  testTicketID,
				"quantity":  -1,
				"unitPrice": 10000,
			},
		},
	})

	assertStatus(t, response, http.StatusBadRequest)
	assertJSONField(t, response, "error", "items[0].quantity cannot be negative")
	if len(broker.published) != 0 {
		t.Fatalf("expected no published events, got %d", len(broker.published))
	}
}

func TestListSalesRejectsInvalidStatusWithAvailableStatuses(t *testing.T) {
	router, _, _ := newTestRouter(fakeSalesStore{salesEventExists: true})

	response := performRequestWithAPIKey(t, router, http.MethodGet, "/sales-events/"+testSalesEventID+"/sales?status=INVALID", nil, "support-key")

	assertStatus(t, response, http.StatusBadRequest)
	assertJSONField(t, response, "error", "status is invalid")

	var body struct {
		AvailableStatuses []string `json:"availableStatuses"`
	}
	decodeResponse(t, response, &body)
	if len(body.AvailableStatuses) != 3 || body.AvailableStatuses[0] != events.SalePendingPaymentStatus || body.AvailableStatuses[1] != events.SaleCompletedStatus || body.AvailableStatuses[2] != events.SaleFailedStatus {
		t.Fatalf("unexpected available statuses: %+v", body.AvailableStatuses)
	}
}

func TestListSalesReturnsEmptyPageWhenSalesEventDoesNotExist(t *testing.T) {
	router, _, _ := newTestRouter(fakeSalesStore{salesEventExists: false})

	response := performRequestWithAPIKey(t, router, http.MethodGet, "/sales-events/"+testSalesEventID+"/sales?page=2&pageSize=5", nil, "support-key")

	assertStatus(t, response, http.StatusOK)

	var body ListSalesResponse
	decodeResponse(t, response, &body)
	if body.Page != 2 || body.PageSize != 5 || body.Total != 0 || len(body.Data) != 0 {
		t.Fatalf("unexpected response: %+v", body)
	}
}

func TestListSalesReturnsPage(t *testing.T) {
	now := time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)
	store := fakeSalesStore{
		salesEventExists: true,
		listResponse: ListSalesResponse{
			Page:     1,
			PageSize: 10,
			Total:    1,
			Data: []SaleListItemDTO{
				{
					ID:             testSaleID,
					SalesEventID:   testSalesEventID,
					SalesEventName: "Backend Moderno Conference",
					CustomerID:     "customer-001",
					Status:         events.SaleCompletedStatus,
					TotalAmount:    10000,
					CreatedAt:      now,
					UpdatedAt:      now,
				},
			},
		},
	}
	router, _, _ := newTestRouter(store)

	response := performRequestWithAPIKey(t, router, http.MethodGet, "/sales-events/"+testSalesEventID+"/sales?page=1&pageSize=10&status=COMPLETED&eventName=Backend", nil, "support-key")

	assertStatus(t, response, http.StatusOK)

	var body ListSalesResponse
	decodeResponse(t, response, &body)
	if body.Total != 1 || len(body.Data) != 1 || body.Data[0].ID != testSaleID {
		t.Fatalf("unexpected response: %+v", body)
	}
}

func TestGetSaleReturnsNotFoundWhenSalesEventDoesNotExist(t *testing.T) {
	router, _, _ := newTestRouter(fakeSalesStore{salesEventExists: false})

	response := performRequestWithAPIKey(t, router, http.MethodGet, "/sales-events/"+testSalesEventID+"/sales/"+testSaleID, nil, "support-key")

	assertStatus(t, response, http.StatusNotFound)
	assertJSONField(t, response, "error", "sales event does not exist")
}

func TestGetSaleReturnsNotFoundWhenSaleDoesNotExist(t *testing.T) {
	router, _, _ := newTestRouter(fakeSalesStore{
		salesEventExists: true,
		getSaleErr:       errSaleNotFound,
	})

	response := performRequestWithAPIKey(t, router, http.MethodGet, "/sales-events/"+testSalesEventID+"/sales/"+testSaleID, nil, "support-key")

	assertStatus(t, response, http.StatusNotFound)
	assertJSONField(t, response, "error", "sale does not exist for this sales event")
}

func TestGetSaleReturnsDetail(t *testing.T) {
	now := time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)
	router, _, _ := newTestRouter(fakeSalesStore{
		salesEventExists: true,
		sale: SaleDetailDTO{
			ID:             testSaleID,
			SalesEventID:   testSalesEventID,
			SalesEventName: "Backend Moderno Conference",
			CustomerID:     "customer-001",
			Status:         events.SaleCompletedStatus,
			TotalAmount:    10000,
			Payment: PaymentDTO{
				Status:      events.PaymentApprovedStatus,
				Amount:      10000,
				Provider:    "simulation",
				ProcessedAt: now,
			},
			Items: []SaleItemReadDTO{
				{
					TicketID:   testTicketID,
					TicketName: "General Admission",
					Quantity:   1,
					UnitPrice:  10000,
					CreatedAt:  now,
				},
			},
			CreatedAt: now,
			UpdatedAt: now,
		},
	})

	response := performRequestWithAPIKey(t, router, http.MethodGet, "/sales-events/"+testSalesEventID+"/sales/"+testSaleID, nil, "support-key")

	assertStatus(t, response, http.StatusOK)

	var body SaleDetailDTO
	decodeResponse(t, response, &body)
	if body.ID != testSaleID || body.Payment.Status != events.PaymentApprovedStatus || len(body.Items) != 1 {
		t.Fatalf("unexpected response: %+v", body)
	}
}
