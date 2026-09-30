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

	"github.com/gin-gonic/gin"
	"github.com/varner/sales-event-project/internal/analytics"
	"github.com/varner/sales-event-project/internal/analyticsdb"
	"github.com/varner/sales-event-project/internal/correlation"
)

const (
	testSalesEventID = "11111111-1111-1111-1111-111111111111"
	testSaleID       = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	testTicketID     = "22222222-2222-2222-2222-222222222222"
)

func newTestRouter(store fakeSalesStore) (*gin.Engine, *fakePublisher, *fakeSalesStore) {
	return newTestRouterWithAnalytics(store, nil)
}

func newTestRouterWithAnalytics(store fakeSalesStore, analyticsExporter AnalyticsExporter) (*gin.Engine, *fakePublisher, *fakeSalesStore) {
	gin.SetMode(gin.TestMode)

	broker := &fakePublisher{}
	storeRef := &store
	router := NewRouter(RouterDeps{
		Broker:               broker,
		Store:                storeRef,
		AuthStore:            newFakeAuthStore(),
		AnalyticsExporter:    analyticsExporter,
		WebhookSecret:        "test-webhook-secret",
		PaymentWebhookSecret: "test-payment-secret",
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

func performRequestWithAPIKey(t *testing.T, router http.Handler, method string, path string, body any, apiKey string) *httptest.ResponseRecorder {
	t.Helper()
	return performRequestWithHeaders(t, router, method, path, body, map[string]string{
		apiKeyHeader: apiKey,
	})
}

func performRequestWithHeaders(t *testing.T, router http.Handler, method string, path string, body any, headers map[string]string) *httptest.ResponseRecorder {
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
	for key, value := range headers {
		request.Header.Set(key, value)
	}

	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func performSignedPaymentWebhook(t *testing.T, router http.Handler, body any) *httptest.ResponseRecorder {
	t.Helper()

	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request body: %v", err)
	}

	request := httptest.NewRequest(http.MethodPost, "/webhooks/payments", bytes.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(paymentSignatureHeader, computeBodyHMAC(payload, "test-payment-secret"))

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

func assertCorrelationHeaders(t *testing.T, response *httptest.ResponseRecorder, metadata correlation.Metadata) {
	t.Helper()
	if response.Header().Get(correlation.RequestIDHeader) != metadata.RequestID {
		t.Fatalf("expected request id header %q, got %q", metadata.RequestID, response.Header().Get(correlation.RequestIDHeader))
	}
	if response.Header().Get(correlation.CorrelationIDHeader) != metadata.CorrelationID {
		t.Fatalf("expected correlation id header %q, got %q", metadata.CorrelationID, response.Header().Get(correlation.CorrelationIDHeader))
	}
	if response.Header().Get(correlation.TransactionIDHeader) != metadata.TransactionID {
		t.Fatalf("expected transaction id header %q, got %q", metadata.TransactionID, response.Header().Get(correlation.TransactionIDHeader))
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

type fakeAnalyticsExporter struct {
	document   analytics.Document
	err        error
	lastFilter analyticsdb.Filter
}

func (e *fakeAnalyticsExporter) ExportAnalytics(_ context.Context, filter analyticsdb.Filter) (analytics.Document, error) {
	e.lastFilter = filter
	if e.err != nil {
		return analytics.Document{}, e.err
	}
	return e.document, nil
}

type fakeAuthStore struct {
	keys map[string]APIKeyPrincipal
}

func newFakeAuthStore() *fakeAuthStore {
	return &fakeAuthStore{
		keys: map[string]APIKeyPrincipal{
			"admin-key":            {ID: "auth-admin", Name: "Admin", Role: RoleAdmin},
			"support-key":          {ID: "auth-support", Name: "Support", Role: RoleSupport},
			"check-in-key":         {ID: "auth-check-in", Name: "Check-in", Role: RoleCheckIn},
			"payment-provider-key": {ID: "auth-payment", Name: "Payment Provider", Role: RolePaymentProvider},
		},
	}
}

func (s *fakeAuthStore) AuthenticateAPIKey(_ context.Context, key string) (APIKeyPrincipal, error) {
	principal, ok := s.keys[key]
	if !ok {
		return APIKeyPrincipal{}, errInvalidAPIKey
	}
	return principal, nil
}

type publishedEvent struct {
	routingKey string
	value      any
	metadata   correlation.Metadata
}

func (p *fakePublisher) PublishJSON(ctx context.Context, routingKey string, value any) error {
	if p.err != nil {
		return p.err
	}

	metadata, _ := correlation.FromContext(ctx)
	p.published = append(p.published, publishedEvent{
		routingKey: routingKey,
		value:      value,
		metadata:   metadata,
	})
	return nil
}

type fakeSalesStore struct {
	salesEventExists      bool
	ticket                TicketReadModel
	ticketErr             error
	listResponse          ListSalesResponse
	listErr               error
	sale                  SaleDetailDTO
	getSaleErr            error
	paymentResult         ProcessPaymentResult
	paymentErr            error
	paymentIntentResult   PaymentIntentDTO
	paymentIntentErr      error
	paymentWebhookResult  ProcessPaymentResult
	paymentWebhookErr     error
	checkInResult         CheckInTicketResult
	checkInErr            error
	lastCheckInRequest    CheckInTicketRequest
	emailEventResult      RecordEmailEventResult
	emailEventErr         error
	lastEmailEventRequest RecordEmailEventRequest
	err                   error
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

func (s *fakeSalesStore) CreatePaymentIntent(_ context.Context, req CreatePaymentIntentStoreRequest) (PaymentIntentDTO, error) {
	if s.paymentIntentErr != nil {
		return PaymentIntentDTO{}, s.paymentIntentErr
	}
	if s.paymentIntentResult.ID == "" {
		return PaymentIntentDTO{}, fmt.Errorf("test payment intent result was not configured for sale %s", req.SaleID)
	}
	return s.paymentIntentResult, nil
}

func (s *fakeSalesStore) ProcessPaymentWebhook(_ context.Context, req ProcessPaymentWebhookRequest) (ProcessPaymentResult, error) {
	if s.paymentWebhookErr != nil {
		return ProcessPaymentResult{}, s.paymentWebhookErr
	}
	if s.paymentWebhookResult.SaleID == "" {
		return ProcessPaymentResult{}, fmt.Errorf("test payment webhook result was not configured for sale %s", req.SaleID)
	}
	return s.paymentWebhookResult, nil
}

func (s *fakeSalesStore) CheckInTicket(_ context.Context, req CheckInTicketRequest) (CheckInTicketResult, error) {
	s.lastCheckInRequest = req
	if s.checkInErr != nil {
		return CheckInTicketResult{}, s.checkInErr
	}
	if s.checkInResult.CheckInID == "" {
		return CheckInTicketResult{}, fmt.Errorf("test check-in result was not configured for ticket %s", req.IssuedTicketID)
	}
	return s.checkInResult, nil
}

func (s *fakeSalesStore) RecordEmailEvent(_ context.Context, req RecordEmailEventRequest) (RecordEmailEventResult, error) {
	s.lastEmailEventRequest = req
	if s.emailEventErr != nil {
		return RecordEmailEventResult{}, s.emailEventErr
	}
	if s.emailEventResult.SaleID == "" {
		return RecordEmailEventResult{}, fmt.Errorf("test email event result was not configured for sale %s", req.SaleID)
	}
	return s.emailEventResult, nil
}
