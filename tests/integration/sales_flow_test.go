//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	defaultAPIBaseURL    = "http://localhost:8080"
	defaultDatabaseURL   = "postgres://sales:sales@localhost:5432/sales_event?sslmode=disable"
	defaultRabbitBaseURL = "http://localhost:15672"
	salesEventID         = "11111111-1111-1111-1111-111111111111"
	generalTicketID      = "22222222-2222-2222-2222-222222222222"
	generalTicketPrice   = 10000
	checkInAPIKey        = "dev-check-in-key"
	paymentAPIKey        = "dev-payment-provider-key"
)

func TestApprovedPaymentIssuesTicketsAndIsIdempotent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	db := connectDB(t, ctx)
	defer db.Close()

	beforeInventory := availableQuantity(t, ctx, db, generalTicketID)
	saleID := createSale(t, salesFlowRequest{
		CustomerID:    "customer-it-" + uuid.NewString(),
		CustomerName:  "Integration Buyer",
		CustomerEmail: fmt.Sprintf("buyer-%s@example.com", uuid.NewString()),
		Quantity:      2,
	})

	waitForSaleStatus(t, ctx, db, saleID, "PENDING_PAYMENT")
	if got := availableQuantity(t, ctx, db, generalTicketID); got != beforeInventory-2 {
		t.Fatalf("expected inventory to decrease by 2 after reservation, before=%d got=%d", beforeInventory, got)
	}

	payment := paySale(t, saleID, 2*generalTicketPrice, "APPROVED")
	if payment.SaleStatus != "COMPLETED" || payment.Payment.Status != "APPROVED" {
		t.Fatalf("unexpected payment response: %+v", payment)
	}

	waitForIssuedTickets(t, ctx, db, saleID, 2)
	waitForEmailStatus(t, ctx, db, saleID, "SENT")

	ticketCode := issuedTicketCode(t, ctx, db, saleID)
	checkIn := checkInTicket(t, ticketCode, http.StatusCreated)
	if checkIn.IssuedTicketID == "" || checkIn.SaleID != saleID {
		t.Fatalf("unexpected check-in response: %+v", checkIn)
	}
	checkInTicket(t, ticketCode, http.StatusConflict)

	publishSaleCompleted(t, saleID)
	time.Sleep(2 * time.Second)

	if got := issuedTicketCount(t, ctx, db, saleID); got != 2 {
		t.Fatalf("duplicate SALE_COMPLETED should not create duplicate tickets; got %d", got)
	}
	if got := availableQuantity(t, ctx, db, generalTicketID); got != beforeInventory-2 {
		t.Fatalf("approved sale should keep inventory reserved, before=%d got=%d", beforeInventory, got)
	}
}

func TestFailedPaymentRestoresInventory(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	db := connectDB(t, ctx)
	defer db.Close()

	beforeInventory := availableQuantity(t, ctx, db, generalTicketID)
	saleID := createSale(t, salesFlowRequest{
		CustomerID:    "customer-it-" + uuid.NewString(),
		CustomerName:  "Failed Buyer",
		CustomerEmail: fmt.Sprintf("failed-%s@example.com", uuid.NewString()),
		Quantity:      1,
	})

	waitForSaleStatus(t, ctx, db, saleID, "PENDING_PAYMENT")
	if got := availableQuantity(t, ctx, db, generalTicketID); got != beforeInventory-1 {
		t.Fatalf("expected inventory to decrease by 1 after reservation, before=%d got=%d", beforeInventory, got)
	}

	payment := paySale(t, saleID, generalTicketPrice, "FAILED")
	if payment.SaleStatus != "FAILED" || payment.Payment.Status != "FAILED" {
		t.Fatalf("unexpected failed payment response: %+v", payment)
	}

	waitForSaleStatus(t, ctx, db, saleID, "FAILED")
	if got := availableQuantity(t, ctx, db, generalTicketID); got != beforeInventory {
		t.Fatalf("failed payment should restore inventory, before=%d got=%d", beforeInventory, got)
	}
	if got := issuedTicketCount(t, ctx, db, saleID); got != 0 {
		t.Fatalf("failed payment should not issue tickets; got %d", got)
	}
}

type salesFlowRequest struct {
	CustomerID    string
	CustomerName  string
	CustomerEmail string
	Quantity      int
}

type createSaleResponse struct {
	SaleID string `json:"saleId"`
	Status string `json:"status"`
}

type paymentResponse struct {
	SaleID     string `json:"saleId"`
	SaleStatus string `json:"saleStatus"`
	Payment    struct {
		Status   string `json:"status"`
		Amount   int    `json:"amount"`
		Provider string `json:"provider"`
	} `json:"payment"`
}

type checkInResponse struct {
	CheckInID      string `json:"checkInId"`
	IssuedTicketID string `json:"issuedTicketId"`
	SaleID         string `json:"saleId"`
}

func createSale(t *testing.T, req salesFlowRequest) string {
	t.Helper()

	body := map[string]any{
		"salesEventId":  salesEventID,
		"customerId":    req.CustomerID,
		"customerName":  req.CustomerName,
		"customerEmail": req.CustomerEmail,
		"items": []map[string]any{
			{
				"ticketId":  generalTicketID,
				"quantity":  req.Quantity,
				"unitPrice": generalTicketPrice,
			},
		},
	}

	var response createSaleResponse
	doJSON(t, http.MethodPost, apiBaseURL()+"/sales", body, http.StatusAccepted, &response)
	if response.SaleID == "" || response.Status != "PROCESSING" {
		t.Fatalf("unexpected create sale response: %+v", response)
	}
	return response.SaleID
}

func paySale(t *testing.T, saleID string, amount int, status string) paymentResponse {
	t.Helper()

	body := map[string]any{
		"amount":   amount,
		"provider": "integration_test",
		"status":   status,
	}

	var response paymentResponse
	doJSONWithHeaders(t, http.MethodPost, apiBaseURL()+"/sales/"+saleID+"/payments", body, http.StatusAccepted, &response, map[string]string{
		"X-API-Key": paymentAPIKey,
	})
	return response
}

func checkInTicket(t *testing.T, ticketCode string, expectedStatus int) checkInResponse {
	t.Helper()

	body := map[string]any{
		"ticketCode": ticketCode,
	}

	var response checkInResponse
	var target any
	if expectedStatus == http.StatusCreated {
		target = &response
	}
	doJSONWithHeaders(t, http.MethodPost, apiBaseURL()+"/sales-events/"+salesEventID+"/check-ins", body, expectedStatus, target, map[string]string{
		"X-API-Key": checkInAPIKey,
	})
	return response
}

func publishSaleCompleted(t *testing.T, saleID string) {
	t.Helper()

	payload, err := json.Marshal(map[string]any{
		"eventId":      uuid.NewString(),
		"eventType":    "SALE_COMPLETED",
		"occurredAt":   time.Now().UTC(),
		"saleId":       saleID,
		"salesEventId": salesEventID,
	})
	if err != nil {
		t.Fatalf("marshal sale completed payload: %v", err)
	}

	body := map[string]any{
		"properties":       map[string]any{},
		"routing_key":      "sale.completed",
		"payload":          string(payload),
		"payload_encoding": "string",
	}

	doJSON(t, http.MethodPost, rabbitBaseURL()+"/api/exchanges/%2F/sales.exchange/publish", body, http.StatusOK, nil)
}

func doJSON(t *testing.T, method string, url string, body any, expectedStatus int, target any) {
	t.Helper()
	doJSONWithHeaders(t, method, url, body, expectedStatus, target, nil)
}

func doJSONWithHeaders(t *testing.T, method string, url string, body any, expectedStatus int, target any, headers map[string]string) {
	t.Helper()

	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request body: %v", err)
	}

	req, err := http.NewRequest(method, url, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	if strings.HasPrefix(url, rabbitBaseURL()) {
		req.SetBasicAuth("guest", "guest")
	}

	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s failed: %v", method, url, err)
	}
	defer response.Body.Close()

	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	if response.StatusCode != expectedStatus {
		t.Fatalf("expected status %d for %s %s, got %d: %s", expectedStatus, method, url, response.StatusCode, string(responseBody))
	}
	if target != nil {
		if err := json.Unmarshal(responseBody, target); err != nil {
			t.Fatalf("decode response body %q: %v", string(responseBody), err)
		}
	}
}

func connectDB(t *testing.T, ctx context.Context) *pgxpool.Pool {
	t.Helper()

	db, err := pgxpool.New(ctx, databaseURL())
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	if err := db.Ping(ctx); err != nil {
		db.Close()
		t.Fatalf("ping postgres: %v", err)
	}
	return db
}

func availableQuantity(t *testing.T, ctx context.Context, db *pgxpool.Pool, ticketID string) int {
	t.Helper()

	var quantity int
	if err := db.QueryRow(ctx, `SELECT available_quantity FROM tickets WHERE id = $1`, ticketID).Scan(&quantity); err != nil {
		t.Fatalf("query available quantity: %v", err)
	}
	return quantity
}

func issuedTicketCount(t *testing.T, ctx context.Context, db *pgxpool.Pool, saleID string) int {
	t.Helper()

	var count int
	if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM issued_tickets WHERE sale_id = $1`, saleID).Scan(&count); err != nil {
		t.Fatalf("query issued tickets: %v", err)
	}
	return count
}

func issuedTicketCode(t *testing.T, ctx context.Context, db *pgxpool.Pool, saleID string) string {
	t.Helper()

	var payload string
	if err := db.QueryRow(ctx, `
		SELECT qr_code_payload
		FROM issued_tickets
		WHERE sale_id = $1
		ORDER BY created_at ASC
		LIMIT 1
	`, saleID).Scan(&payload); err != nil {
		t.Fatalf("query issued ticket payload: %v", err)
	}
	return payload
}

func waitForSaleStatus(t *testing.T, ctx context.Context, db *pgxpool.Pool, saleID string, expected string) {
	t.Helper()

	waitFor(t, ctx, func() (bool, string) {
		var status string
		if err := db.QueryRow(ctx, `SELECT status FROM sales WHERE id = $1`, saleID).Scan(&status); err != nil {
			return false, err.Error()
		}
		return status == expected, status
	})
}

func waitForIssuedTickets(t *testing.T, ctx context.Context, db *pgxpool.Pool, saleID string, expected int) {
	t.Helper()

	waitFor(t, ctx, func() (bool, string) {
		count := issuedTicketCount(t, ctx, db, saleID)
		return count == expected, fmt.Sprintf("%d", count)
	})
}

func waitForEmailStatus(t *testing.T, ctx context.Context, db *pgxpool.Pool, saleID string, expected string) {
	t.Helper()

	waitFor(t, ctx, func() (bool, string) {
		var status string
		if err := db.QueryRow(ctx, `SELECT status FROM email_notifications WHERE sale_id = $1`, saleID).Scan(&status); err != nil {
			return false, err.Error()
		}
		return status == expected, status
	})
}

func waitFor(t *testing.T, ctx context.Context, check func() (bool, string)) {
	t.Helper()

	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()

	var last string
	for {
		ok, detail := check()
		if ok {
			return
		}
		last = detail

		select {
		case <-ctx.Done():
			t.Fatalf("condition not met before timeout; last observed value: %s", last)
		case <-ticker.C:
		}
	}
}

func apiBaseURL() string {
	return envOrDefault("INTEGRATION_API_BASE_URL", defaultAPIBaseURL)
}

func databaseURL() string {
	return envOrDefault("INTEGRATION_DATABASE_URL", defaultDatabaseURL)
}

func rabbitBaseURL() string {
	return envOrDefault("INTEGRATION_RABBITMQ_BASE_URL", defaultRabbitBaseURL)
}

func envOrDefault(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}
