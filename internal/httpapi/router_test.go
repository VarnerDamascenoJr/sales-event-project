package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/varner/sales-event-project/internal/events"
)

const (
	testSalesEventID = "11111111-1111-1111-1111-111111111111"
	testSaleID       = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	testTicketID     = "22222222-2222-2222-2222-222222222222"
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

	event, ok := broker.published[0].value.(events.SaleCreated)
	if !ok {
		t.Fatalf("expected published value to be events.SaleCreated, got %T", broker.published[0].value)
	}
	if event.SalesEventID != testSalesEventID || event.CustomerID != "customer-001" || event.CustomerEmail != "ada@example.com" {
		t.Fatalf("published event has unexpected payload: %+v", event)
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

	response := performRequest(t, router, http.MethodGet, "/sales-events/"+testSalesEventID+"/sales?status=INVALID", nil)

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

	response := performRequest(t, router, http.MethodGet, "/sales-events/"+testSalesEventID+"/sales?page=2&pageSize=5", nil)

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

	response := performRequest(t, router, http.MethodGet, "/sales-events/"+testSalesEventID+"/sales?page=1&pageSize=10&status=COMPLETED&eventName=Backend", nil)

	assertStatus(t, response, http.StatusOK)

	var body ListSalesResponse
	decodeResponse(t, response, &body)
	if body.Total != 1 || len(body.Data) != 1 || body.Data[0].ID != testSaleID {
		t.Fatalf("unexpected response: %+v", body)
	}
}

func TestGetSaleReturnsNotFoundWhenSalesEventDoesNotExist(t *testing.T) {
	router, _, _ := newTestRouter(fakeSalesStore{salesEventExists: false})

	response := performRequest(t, router, http.MethodGet, "/sales-events/"+testSalesEventID+"/sales/"+testSaleID, nil)

	assertStatus(t, response, http.StatusNotFound)
	assertJSONField(t, response, "error", "sales event does not exist")
}

func TestGetSaleReturnsNotFoundWhenSaleDoesNotExist(t *testing.T) {
	router, _, _ := newTestRouter(fakeSalesStore{
		salesEventExists: true,
		getSaleErr:       errSaleNotFound,
	})

	response := performRequest(t, router, http.MethodGet, "/sales-events/"+testSalesEventID+"/sales/"+testSaleID, nil)

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

	response := performRequest(t, router, http.MethodGet, "/sales-events/"+testSalesEventID+"/sales/"+testSaleID, nil)

	assertStatus(t, response, http.StatusOK)

	var body SaleDetailDTO
	decodeResponse(t, response, &body)
	if body.ID != testSaleID || body.Payment.Status != events.PaymentApprovedStatus || len(body.Items) != 1 {
		t.Fatalf("unexpected response: %+v", body)
	}
}

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

	response := performRequest(t, router, http.MethodPost, "/sales/"+testSaleID+"/payments", map[string]any{
		"amount":   10000,
		"provider": "credit_card",
	})

	assertStatus(t, response, http.StatusAccepted)

	var body ProcessPaymentResult
	decodeResponse(t, response, &body)
	if body.SaleStatus != events.SaleCompletedStatus || body.Payment.Status != events.PaymentApprovedStatus {
		t.Fatalf("unexpected payment response: %+v", body)
	}
	if len(broker.published) != 1 {
		t.Fatalf("expected one published event, got %d", len(broker.published))
	}
	if broker.published[0].routingKey != events.SaleCompletedRoutingKey {
		t.Fatalf("expected routing key %q, got %q", events.SaleCompletedRoutingKey, broker.published[0].routingKey)
	}
	event, ok := broker.published[0].value.(events.SaleCompleted)
	if !ok {
		t.Fatalf("expected published value to be events.SaleCompleted, got %T", broker.published[0].value)
	}
	if event.SaleID != testSaleID || event.SalesEventID != testSalesEventID {
		t.Fatalf("published event has unexpected payload: %+v", event)
	}
}

func TestCreatePaymentRejectsInvalidStatus(t *testing.T) {
	router, _, _ := newTestRouter(fakeSalesStore{})

	response := performRequest(t, router, http.MethodPost, "/sales/"+testSaleID+"/payments", map[string]any{
		"amount":   10000,
		"provider": "credit_card",
		"status":   "UNKNOWN",
	})

	assertStatus(t, response, http.StatusBadRequest)
	assertJSONField(t, response, "error", "status must be APPROVED or FAILED")
}

func newTestRouter(store fakeSalesStore) (*gin.Engine, *fakePublisher, *fakeSalesStore) {
	gin.SetMode(gin.TestMode)

	broker := &fakePublisher{}
	storeRef := &store
	router := NewRouter(RouterDeps{
		Broker: broker,
		Store:  storeRef,
	})

	return router, broker, storeRef
}

func performRequest(t *testing.T, router http.Handler, method string, path string, body any) *httptest.ResponseRecorder {
	t.Helper()

	var requestBody *bytes.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal request body: %v", err)
		}
		requestBody = bytes.NewReader(payload)
	} else {
		requestBody = bytes.NewReader(nil)
	}

	request := httptest.NewRequest(method, path, requestBody)
	request.Header.Set("Content-Type", "application/json")

	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func assertStatus(t *testing.T, response *httptest.ResponseRecorder, expected int) {
	t.Helper()
	if response.Code != expected {
		t.Fatalf("expected status %d, got %d: %s", expected, response.Code, response.Body.String())
	}
}

func assertJSONField(t *testing.T, response *httptest.ResponseRecorder, field string, expected string) {
	t.Helper()

	var body map[string]any
	decodeResponse(t, response, &body)

	actual, ok := body[field].(string)
	if !ok {
		t.Fatalf("expected JSON field %q to be string, got %+v", field, body[field])
	}
	if actual != expected {
		t.Fatalf("expected JSON field %q to be %q, got %q", field, expected, actual)
	}
}

func decodeResponse(t *testing.T, response *httptest.ResponseRecorder, target any) {
	t.Helper()
	if err := json.Unmarshal(response.Body.Bytes(), target); err != nil {
		t.Fatalf("decode response body %q: %v", response.Body.String(), err)
	}
}

type fakePublisher struct {
	published []publishedEvent
	err       error
}

type publishedEvent struct {
	routingKey string
	value      any
}

func (p *fakePublisher) PublishJSON(_ context.Context, routingKey string, value any) error {
	if p.err != nil {
		return p.err
	}

	p.published = append(p.published, publishedEvent{
		routingKey: routingKey,
		value:      value,
	})
	return nil
}

type fakeSalesStore struct {
	salesEventExists bool
	ticket           TicketReadModel
	ticketErr        error
	listResponse     ListSalesResponse
	listErr          error
	sale             SaleDetailDTO
	getSaleErr       error
	paymentResult    ProcessPaymentResult
	paymentErr       error
	err              error
}

func (s *fakeSalesStore) SalesEventExists(context.Context, string) (bool, error) {
	if s.err != nil {
		return false, s.err
	}
	return s.salesEventExists, nil
}

func (s *fakeSalesStore) GetTicketForEvent(context.Context, string, string) (TicketReadModel, error) {
	if s.ticketErr != nil {
		return TicketReadModel{}, s.ticketErr
	}
	if s.ticket == (TicketReadModel{}) {
		return TicketReadModel{}, errTicketNotFound
	}
	return s.ticket, nil
}

func (s *fakeSalesStore) ListSales(context.Context, listSalesFilter) (ListSalesResponse, error) {
	if s.listErr != nil {
		return ListSalesResponse{}, s.listErr
	}
	if s.listResponse.Data == nil {
		s.listResponse.Data = []SaleListItemDTO{}
	}
	return s.listResponse, nil
}

func (s *fakeSalesStore) GetSale(context.Context, string, string) (SaleDetailDTO, error) {
	if s.getSaleErr != nil {
		return SaleDetailDTO{}, s.getSaleErr
	}
	if s.sale.ID == "" {
		return SaleDetailDTO{}, errors.New("test sale was not configured")
	}
	return s.sale, nil
}

func (s *fakeSalesStore) ProcessPayment(_ context.Context, req ProcessPaymentRequest) (ProcessPaymentResult, error) {
	if s.paymentErr != nil {
		return ProcessPaymentResult{}, s.paymentErr
	}
	if s.paymentResult.SaleID == "" {
		return ProcessPaymentResult{}, fmt.Errorf("test payment result was not configured for sale %s", req.SaleID)
	}
	return s.paymentResult, nil
}
