package main

import (
	"encoding/json"
	"math/rand"
	"strings"
	"testing"
	"time"
)

func TestBuildSalePayloadUsesDemoSeedValues(t *testing.T) {
	cfg := testConfig(t, "--sales", "5", "--run-id", "testrun")

	payload := buildSalePayload(1, cfg)

	if payload.SalesEventID != defaultSalesEventID {
		t.Fatalf("SalesEventID = %q, want %q", payload.SalesEventID, defaultSalesEventID)
	}
	if payload.CustomerID != "demo-testrun-0001" {
		t.Fatalf("CustomerID = %q", payload.CustomerID)
	}
	if got := payload.Items[0].TicketID; got != defaultGeneralTicketID {
		t.Fatalf("ticket ID = %q, want %q", got, defaultGeneralTicketID)
	}
	if got := saleAmount(payload); got != 10000 {
		t.Fatalf("sale amount = %d, want 10000", got)
	}
}

func TestBuildSalePayloadMixesVIPTicketsAndQuantities(t *testing.T) {
	cfg := testConfig(t, "--sales", "8", "--run-id", "testrun")

	vipPayload := buildSalePayload(4, cfg)
	twoTicketPayload := buildSalePayload(7, cfg)

	if got := vipPayload.Items[0].TicketID; got != defaultVIPTicketID {
		t.Fatalf("VIP ticket ID = %q, want %q", got, defaultVIPTicketID)
	}
	if got := vipPayload.Items[0].UnitPrice; got != 25000 {
		t.Fatalf("VIP unit price = %d, want 25000", got)
	}
	if got := twoTicketPayload.Items[0].Quantity; got != 2 {
		t.Fatalf("quantity = %d, want 2", got)
	}
	if got := expectedTicketCount(twoTicketPayload); got != 2 {
		t.Fatalf("expected ticket count = %d, want 2", got)
	}
}

func TestBuildPaymentPayloadSetsStatus(t *testing.T) {
	approved := buildPaymentPayload(10000, true)
	failed := buildPaymentPayload(10000, false)

	if approved.Status != "APPROVED" {
		t.Fatalf("approved status = %q", approved.Status)
	}
	if failed.Status != "FAILED" {
		t.Fatalf("failed status = %q", failed.Status)
	}
	if approved.Provider != "demo_generator" || approved.Amount != 10000 {
		t.Fatalf("unexpected approved payload: %+v", approved)
	}
}

func TestBuildEmailPayloadUsesSupportedEventType(t *testing.T) {
	now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)

	payload := buildEmailEventPayload("sale-1", "OPENED", 3, now)

	if payload.SaleID != "sale-1" {
		t.Fatalf("SaleID = %q", payload.SaleID)
	}
	if payload.EventType != "OPENED" {
		t.Fatalf("EventType = %q", payload.EventType)
	}
	if payload.OccurredAt != "2026-10-09T12:00:00Z" {
		t.Fatalf("OccurredAt = %q", payload.OccurredAt)
	}
	if payload.ProviderEventID == "" {
		t.Fatal("ProviderEventID is empty")
	}
}

func TestParseConfigAcceptsDemoControls(t *testing.T) {
	cfg := testConfig(
		t,
		"--sales", "50",
		"--failed-payment-rate", "0.2",
		"--check-ins", "12",
		"--reset-demo-inventory",
		"--no-email-events",
		"--run-id", "manual",
	)

	if cfg.Sales != 50 {
		t.Fatalf("Sales = %d, want 50", cfg.Sales)
	}
	if cfg.FailedPaymentRate != 0.2 {
		t.Fatalf("FailedPaymentRate = %v, want 0.2", cfg.FailedPaymentRate)
	}
	if cfg.CheckIns != 12 {
		t.Fatalf("CheckIns = %d, want 12", cfg.CheckIns)
	}
	if !cfg.ResetDemoInventory {
		t.Fatal("ResetDemoInventory = false, want true")
	}
	if cfg.EmailEvents {
		t.Fatal("EmailEvents = true, want false")
	}
	if cfg.RunID != "manual" {
		t.Fatalf("RunID = %q, want manual", cfg.RunID)
	}
}

func TestRedactConfigHidesSecrets(t *testing.T) {
	cfg := testConfig(t, "--run-id", "manual")

	payload, err := json.Marshal(redactConfig(cfg))
	if err != nil {
		t.Fatalf("marshal redacted config: %v", err)
	}
	body := string(payload)
	if body == "" {
		t.Fatal("empty redacted payload")
	}
	for _, secret := range []string{defaultPaymentAPIKey, defaultCheckInAPIKey, defaultEmailWebhookSecret} {
		if strings.Contains(body, secret) {
			t.Fatalf("redacted config leaked secret %q: %s", secret, body)
		}
	}
}

func TestShouldApproveKeepsFirstSaleApprovedWhenRateIsPartial(t *testing.T) {
	rng := rand.New(rand.NewSource(1))

	if !shouldApprove(1, rng, 0.99) {
		t.Fatal("first sale should be approved for partial failure rates")
	}
	if shouldApprove(1, rng, 1) {
		t.Fatal("all-failed rate should fail every sale")
	}
}

func testConfig(t *testing.T, args ...string) config {
	t.Helper()
	cfg, err := parseConfig(args, time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	return cfg
}
