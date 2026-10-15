package analyticsdb

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/varner/sales-event-project/internal/analytics"
)

func TestExporterBuildsDocumentWithEventsAndInventory(t *testing.T) {
	occurredAt := time.Date(2026, 10, 14, 10, 0, 0, 0, time.UTC)
	eventRows := &fakeRows{values: [][]any{eventValues(occurredAt)}}
	inventoryRows := &fakeRows{values: [][]any{{"sales-event-123", "ticket-123", "VIP", 10}}}
	db := &fakeQueryer{results: []queryResult{{rows: eventRows}, {rows: inventoryRows}}}
	filter := Filter{SalesEventID: "sales-event-123", Start: "2026-10-14T00:00:00Z", Limit: 20}
	before := time.Now()
	got, err := NewExporter(db).ExportAnalytics(context.Background(), filter)
	after := time.Now()
	if err != nil {
		t.Fatalf("export analytics: %v", err)
	}
	if got.GeneratedAt.Before(before) || got.GeneratedAt.After(after) || got.GeneratedAt.Location() != time.UTC {
		t.Fatalf("expected a current UTC generation time, got %v", got.GeneratedAt)
	}
	events := []analytics.Event{{
		EventID: "event-123", EventType: "payment.processed", OccurredAt: occurredAt,
		SalesEventID: "sales-event-123", SaleID: "sale-123", TicketID: "ticket-123", TicketType: "VIP",
		Status: "APPROVED", Provider: "provider-123", OutboxEventType: "SALE_COMPLETED",
		AmountCents: 10000, Quantity: 2, Attempts: 3, RequestID: "req-123", CorrelationID: "corr-123", TransactionID: "tx-123",
	}}
	inventory := []analytics.TicketInventory{{SalesEventID: "sales-event-123", TicketID: "ticket-123", TicketType: "VIP", AvailableQuantity: 10}}
	want := analytics.BuildDocumentWithInventory(got.GeneratedAt,
		analytics.Source{SalesEventID: "sales-event-123", Filter: "sales_event_id,time_range"},
		events, analytics.DefaultWindowSpecs(), inventory)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("exported document differs from the expected event, inventory and source composition: got %+v, want %+v", got, want)
	}
	if len(db.calls) != 2 || !eventRows.closed || !inventoryRows.closed {
		t.Fatalf("expected two queries with closed rows, got %d queries", len(db.calls))
	}
}

func TestExporterStopsOnReadFailure(t *testing.T) {
	failure := errors.New("database offline")
	for _, stage := range []string{"events", "inventory"} {
		t.Run(stage, func(t *testing.T) {
			results := []queryResult{{err: failure}}
			wantCalls := 1
			if stage == "inventory" {
				results = []queryResult{{rows: &fakeRows{}}, {err: failure}}
				wantCalls = 2
			}
			db := &fakeQueryer{results: results}
			got, err := NewExporter(db).ExportAnalytics(context.Background(), Filter{})
			if !errors.Is(err, failure) || !reflect.DeepEqual(got, analytics.Document{}) {
				t.Fatalf("expected an empty document and read failure, got %+v, %v", got, err)
			}
			if len(db.calls) != wantCalls {
				t.Fatalf("expected export to stop after %d queries, got %d", wantCalls, len(db.calls))
			}
		})
	}
}
