package httpapi

import (
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestMetricsRouteRequiresAdminWhenProtected(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := NewRouter(RouterDeps{
		Broker:               &fakePublisher{},
		Store:                &fakeSalesStore{},
		AuthStore:            newFakeAuthStore(),
		WebhookSecret:        "test-webhook-secret",
		PaymentWebhookSecret: "test-payment-secret",
		MetricsProtected:     true,
	})

	response := performRequest(t, router, http.MethodGet, "/metrics", nil)
	assertStatus(t, response, http.StatusUnauthorized)

	response = performRequestWithAPIKey(t, router, http.MethodGet, "/metrics", nil, "dev-support-key")
	assertStatus(t, response, http.StatusForbidden)

	response = performRequestWithAPIKey(t, router, http.MethodGet, "/metrics", nil, "dev-admin-key")
	assertStatus(t, response, http.StatusOK)
}

func TestPublicRateLimiterReturnsTooManyRequests(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := NewRouter(RouterDeps{
		Broker: &fakePublisher{},
		Store: &fakeSalesStore{
			salesEventExists: true,
			ticket: TicketReadModel{
				Name:              "General Admission",
				Price:             10000,
				AvailableQuantity: 10,
			},
		},
		AuthStore:         newFakeAuthStore(),
		WebhookSecret:     "test-webhook-secret",
		MetricsProtected:  false,
		PublicRateLimiter: NewRateLimiter(1, 1, time.Minute),
	})

	first := performRequest(t, router, http.MethodPost, "/sales", map[string]any{
		"salesEventId":  testSalesEventID,
		"customerId":    "customer-001",
		"customerName":  "Ada Lovelace",
		"customerEmail": "ada@example.com",
		"items":         []map[string]any{{"ticketId": testTicketID, "quantity": 1, "unitPrice": 10000}},
	})
	assertStatus(t, first, http.StatusAccepted)

	second := performRequest(t, router, http.MethodPost, "/sales", map[string]any{
		"salesEventId":  testSalesEventID,
		"customerId":    "customer-002",
		"customerName":  "Grace Hopper",
		"customerEmail": "grace@example.com",
		"items":         []map[string]any{{"ticketId": testTicketID, "quantity": 1, "unitPrice": 10000}},
	})
	assertStatus(t, second, http.StatusTooManyRequests)
}

func TestProtectedRoutesRequireAPIKey(t *testing.T) {
	router, _, _ := newTestRouter(fakeSalesStore{})

	response := performRequest(t, router, http.MethodGet, "/sales-events/"+testSalesEventID+"/sales", nil)

	assertStatus(t, response, http.StatusUnauthorized)
	assertJSONField(t, response, "error", "api key is required")
}

func TestProtectedRoutesRejectWrongRole(t *testing.T) {
	router, _, _ := newTestRouter(fakeSalesStore{})

	response := performRequestWithAPIKey(t, router, http.MethodPost, "/sales-events/"+testSalesEventID+"/check-ins", map[string]any{
		"ticketCode": "issued_ticket:bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb",
	}, "dev-support-key")

	assertStatus(t, response, http.StatusForbidden)
	assertJSONField(t, response, "error", "api key role is not allowed")
}
