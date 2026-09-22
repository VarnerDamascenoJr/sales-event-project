package analytics

import (
	"encoding/json"
	"os"
	"testing"
	"time"
)

func TestBuildDocumentAggregatesEventsIntoMultipleWindowSizes(t *testing.T) {
	base := time.Date(2026, 9, 1, 10, 0, 30, 0, time.FixedZone("BRT", -3*60*60))
	generatedAt := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)

	document := BuildDocument(generatedAt, Source{SalesEventID: "event-1", Filter: "sales_event_id"}, []Event{
		{
			EventType:    "sale.created",
			OccurredAt:   base,
			SalesEventID: "event-1",
			SaleID:       "sale-1",
			Status:       "PENDING_PAYMENT",
			AmountCents:  10000,
		},
		{
			EventType:    "payment.processed",
			OccurredAt:   base.Add(45 * time.Second),
			SalesEventID: "event-1",
			SaleID:       "sale-1",
			Status:       "APPROVED",
			Provider:     "credit_card",
			AmountCents:  10000,
		},
		{
			EventType:    "email.sent",
			OccurredAt:   base.Add(6 * time.Minute),
			SalesEventID: "event-1",
			SaleID:       "sale-1",
			Status:       "SENT",
			Attempts:     1,
		},
		{
			EventType:    "checkin.completed",
			OccurredAt:   base.Add(6*time.Minute + 20*time.Second),
			SalesEventID: "event-1",
			SaleID:       "sale-1",
			Status:       "COMPLETED",
		},
	}, []WindowSpec{
		{Name: "1m", Duration: time.Minute},
		{Name: "5m", Duration: 5 * time.Minute},
	})

	if document.SchemaVersion != SchemaVersion {
		t.Fatalf("unexpected schema version: %s", document.SchemaVersion)
	}
	if got := document.Source.WindowSizes; len(got) != 2 || got[0] != "1m" || got[1] != "5m" {
		t.Fatalf("unexpected window sizes: %+v", got)
	}
	if document.Summary.EventCount != 4 {
		t.Fatalf("unexpected event count: %d", document.Summary.EventCount)
	}
	if document.Events[0].OccurredAt.Location() != time.UTC {
		t.Fatalf("expected normalized UTC timestamp, got %s", document.Events[0].OccurredAt.Location())
	}

	firstFiveMinuteWindow := findWindow(t, document.Windows, "5m", time.Date(2026, 9, 1, 13, 0, 0, 0, time.UTC))
	if firstFiveMinuteWindow.Counts.TotalEvents != 2 {
		t.Fatalf("unexpected first 5m total events: %d", firstFiveMinuteWindow.Counts.TotalEvents)
	}
	if got := firstFiveMinuteWindow.Counts.SaleStatusCounts["PENDING_PAYMENT"]; got != 1 {
		t.Fatalf("unexpected pending sales count: %d", got)
	}
	if got := firstFiveMinuteWindow.Counts.PaymentStatusCounts["APPROVED"]; got != 1 {
		t.Fatalf("unexpected approved payments count: %d", got)
	}
	if firstFiveMinuteWindow.Counts.TotalAmountCents != 20000 {
		t.Fatalf("unexpected total amount cents: %d", firstFiveMinuteWindow.Counts.TotalAmountCents)
	}

	secondFiveMinuteWindow := findWindow(t, document.Windows, "5m", time.Date(2026, 9, 1, 13, 5, 0, 0, time.UTC))
	if got := secondFiveMinuteWindow.Counts.EmailStatusCounts["SENT"]; got != 1 {
		t.Fatalf("unexpected sent email count: %d", got)
	}
	if secondFiveMinuteWindow.Counts.CheckInCount != 1 {
		t.Fatalf("unexpected check-in count: %d", secondFiveMinuteWindow.Counts.CheckInCount)
	}
}

func TestAnalyticsFixtureMatchesSchema(t *testing.T) {
	payload, err := os.ReadFile("../../tests/fixtures/sales-analytics-export.v1.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	var document Document
	if err := json.Unmarshal(payload, &document); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}

	if document.SchemaVersion != SchemaVersion {
		t.Fatalf("unexpected fixture schema version: %s", document.SchemaVersion)
	}
	if len(document.Events) < 2 {
		t.Fatalf("expected at least two fixture events, got %d", len(document.Events))
	}

	fiveMinuteWindows := 0
	for _, window := range document.Windows {
		if window.WindowSize == "5m" {
			fiveMinuteWindows++
		}
	}
	if fiveMinuteWindows < 2 {
		t.Fatalf("expected at least two 5m windows, got %d", fiveMinuteWindows)
	}
}

func findWindow(t *testing.T, windows []WindowAggregate, size string, start time.Time) WindowAggregate {
	t.Helper()
	for _, window := range windows {
		if window.WindowSize == size && window.WindowStart.Equal(start) {
			return window
		}
	}
	t.Fatalf("window %s %s not found", size, start.Format(time.RFC3339))
	return WindowAggregate{}
}
