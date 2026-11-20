package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"math"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/varner/sales-event-project/internal/analytics"
	"github.com/varner/sales-event-project/internal/events"
)

func TestAnalyticsExportReturnsDocument(t *testing.T) {
	generatedAt := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	expectedDocument := analytics.BuildDocument(generatedAt, analytics.Source{
		SalesEventID: testSalesEventID,
		Filter:       "sales_event_id",
	}, []analytics.Event{
		{
			EventType:    "sale.created",
			OccurredAt:   generatedAt.Add(-time.Minute),
			SalesEventID: testSalesEventID,
			SaleID:       testSaleID,
			Status:       events.SaleCompletedStatus,
			AmountCents:  10000,
		},
	}, analytics.DefaultWindowSpecs())
	exporter := &fakeAnalyticsExporter{document: expectedDocument}
	router, _, _ := newTestRouterWithAnalytics(fakeSalesStore{}, exporter)

	response := performRequestWithAPIKey(t, router, http.MethodGet, "/analytics/export?salesEventId="+testSalesEventID+"&start=2026-09-25T10:00:00Z&end=2026-09-25T11:00:00Z&limit=50", nil, "dev-support-key")

	assertStatus(t, response, http.StatusOK)
	if exporter.lastFilter.SalesEventID != testSalesEventID || exporter.lastFilter.Start != "2026-09-25T10:00:00Z" || exporter.lastFilter.End != "2026-09-25T11:00:00Z" || exporter.lastFilter.Limit != 50 {
		t.Fatalf("unexpected analytics filter: %+v", exporter.lastFilter)
	}

	var body analytics.Document
	decodeResponse(t, response, &body)
	if body.SchemaVersion != analytics.SchemaVersion || body.Source.SalesEventID != testSalesEventID || body.Summary.EventCount != 1 {
		t.Fatalf("unexpected analytics document: %+v", body)
	}
	if response.Body.Len() == 0 {
		t.Fatal("expected non-empty analytics response body")
	}
}

func TestAnalyticsExportRequiresSupportOrAdmin(t *testing.T) {
	router, _, _ := newTestRouterWithAnalytics(fakeSalesStore{}, &fakeAnalyticsExporter{})

	response := performRequest(t, router, http.MethodGet, "/analytics/export", nil)
	assertStatus(t, response, http.StatusUnauthorized)
	assertJSONField(t, response, "error", "api key is required")

	response = performRequestWithAPIKey(t, router, http.MethodGet, "/analytics/export", nil, "dev-payment-provider-key")
	assertStatus(t, response, http.StatusForbidden)
	assertJSONField(t, response, "error", "api key role is not allowed")
}

func TestAnalyticsExportRejectsInvalidLimit(t *testing.T) {
	router, _, _ := newTestRouterWithAnalytics(fakeSalesStore{}, &fakeAnalyticsExporter{})

	response := performRequestWithAPIKey(t, router, http.MethodGet, "/analytics/export?limit=0", nil, "dev-support-key")

	assertStatus(t, response, http.StatusBadRequest)
	assertJSONField(t, response, "error", "limit must be greater than or equal to 1")
}

func TestAnalyticsExportRejectsInvalidTimestamp(t *testing.T) {
	router, _, _ := newTestRouterWithAnalytics(fakeSalesStore{}, &fakeAnalyticsExporter{})

	response := performRequestWithAPIKey(t, router, http.MethodGet, "/analytics/export?start=not-a-time", nil, "dev-support-key")

	assertStatus(t, response, http.StatusBadRequest)
	assertJSONField(t, response, "error", "start must be RFC3339: parsing time \"not-a-time\" as \"2006-01-02T15:04:05Z07:00\": cannot parse \"not-a-time\" as \"2006\"")
}

func TestAnalyticsExportReturnsInternalError(t *testing.T) {
	router, _, _ := newTestRouterWithAnalytics(fakeSalesStore{}, &fakeAnalyticsExporter{err: errors.New("database offline")})

	response := performRequestWithAPIKey(t, router, http.MethodGet, "/analytics/export", nil, "dev-admin-key")

	assertStatus(t, response, http.StatusInternalServerError)
	assertJSONField(t, response, "error", "export analytics failed")
}

func TestAnalyticsExportReturnsInternalErrorWhenDocumentCannotMarshal(t *testing.T) {
	var logs bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previousLogger) })

	document := analytics.Document{
		SchemaVersion: analytics.SchemaVersion,
		GeneratedAt:   time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC),
		Source:        analytics.Source{SalesEventID: testSalesEventID},
		Summary:       analytics.Summary{EventTypeCounts: map[string]int{}},
		Events:        []analytics.Event{},
		Windows:       []analytics.WindowAggregate{},
		StockoutRisks: []analytics.StockoutRisk{{TicketID: testTicketID, StockoutProbability: math.NaN()}},
	}
	router, _, _ := newTestRouterWithAnalytics(fakeSalesStore{}, &fakeAnalyticsExporter{document: document})

	response := performRequestWithAPIKey(t, router, http.MethodGet, "/analytics/export?salesEventId="+testSalesEventID+"&limit=10000", nil, "dev-support-key")

	assertStatus(t, response, http.StatusInternalServerError)
	assertJSONField(t, response, "error", "serialize analytics export failed")
	if response.Body.Len() == 0 {
		t.Fatal("expected error response body")
	}
	if contentType := response.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "application/json") {
		t.Fatalf("expected JSON error response, got content type %q", contentType)
	}
	if strings.Contains(response.Body.String(), "NaN") || strings.Contains(response.Body.String(), "dev-support-key") {
		t.Fatalf("error response exposed internal details: %s", response.Body.String())
	}

	if strings.Contains(logs.String(), "dev-support-key") {
		t.Fatal("serialization log exposed the API key")
	}
	decoder := json.NewDecoder(&logs)
	var entry map[string]any
	if err := decoder.Decode(&entry); err != nil {
		t.Fatalf("decode serialization log: %v", err)
	}
	cause, _ := entry["error"].(string)
	if entry["msg"] != "marshal analytics export failed" || entry["route"] != "/analytics/export" ||
		entry["sales_event_id"] != testSalesEventID || entry["limit"] != float64(10000) ||
		!strings.Contains(cause, "NaN") {
		t.Fatalf("unexpected serialization log: %+v", entry)
	}
}
