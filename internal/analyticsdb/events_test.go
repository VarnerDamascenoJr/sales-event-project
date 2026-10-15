package analyticsdb

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/varner/sales-event-project/internal/analytics"
)

func TestReadEventsMapsColumnsAndNormalizesUTC(t *testing.T) {
	occurredAt := time.Date(2026, 10, 14, 10, 30, 0, 0, time.FixedZone("local", -3*60*60))
	nullable := []any{nil, "sale.created", occurredAt, "sales-event-123", "sale-456", nil, nil, nil, nil, nil, 0, 0, 0, nil, nil, nil}
	rows := &fakeRows{values: [][]any{eventValues(occurredAt), nullable}}
	db := &fakeQueryer{results: []queryResult{{rows: rows}}}
	got, err := ReadEvents(context.Background(), db, Filter{})
	if err != nil {
		t.Fatalf("read events: %v", err)
	}
	want := []analytics.Event{
		{
			EventID: "event-123", EventType: "payment.processed", OccurredAt: occurredAt.UTC(),
			SalesEventID: "sales-event-123", SaleID: "sale-123", TicketID: "ticket-123", TicketType: "VIP",
			Status: "APPROVED", Provider: "provider-123", OutboxEventType: "SALE_COMPLETED",
			AmountCents: 10000, Quantity: 2, Attempts: 3, RequestID: "req-123", CorrelationID: "corr-123", TransactionID: "tx-123",
		},
		{EventType: "sale.created", OccurredAt: occurredAt.UTC(), SalesEventID: "sales-event-123", SaleID: "sale-456"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected events %+v, got %+v", want, got)
	}
	if !rows.closed {
		t.Fatal("event rows were not closed")
	}
}

func TestReadEventsPassesFiltersAndPreservesLimitPolicy(t *testing.T) {
	for _, tt := range []struct {
		name  string
		limit int
		want  int
	}{
		{name: "zero uses default", want: DefaultLimit},
		{name: "negative uses default", limit: -1, want: DefaultLimit},
		{name: "positive limit", limit: 25, want: 25},
		{name: "reader does not cap caller limit", limit: MaxLimit + 1, want: MaxLimit + 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rows := &fakeRows{}
			db := &fakeQueryer{results: []queryResult{{rows: rows}}}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			filter := Filter{SalesEventID: "event-123", Start: "2026-10-14T10:00:00Z", End: "2026-10-15T10:00:00Z", Limit: tt.limit}
			got, err := ReadEvents(ctx, db, filter)
			if err != nil {
				t.Fatalf("read empty event result: %v", err)
			}
			if got == nil || len(got) != 0 || !rows.closed {
				t.Fatalf("expected a non-nil empty result and closed rows, got %v, closed=%t", got, rows.closed)
			}
			if len(db.calls) != 1 {
				t.Fatalf("expected one event query, got %d", len(db.calls))
			}
			call := db.calls[0]
			if call.ctx != ctx || !strings.Contains(call.sql, "WITH analytics_events AS") {
				t.Fatal("event query or request context changed")
			}
			if want := []any{filter.SalesEventID, filter.Start, filter.End, tt.want}; !reflect.DeepEqual(call.args, want) {
				t.Fatalf("expected query arguments %v, got %v", want, call.args)
			}
			if filter.Limit != tt.limit {
				t.Fatal("reader changed the caller's filter")
			}
		})
	}
}

func TestTextValue(t *testing.T) {
	for _, tt := range []struct {
		value pgtype.Text
		want  string
	}{
		{value: pgtype.Text{String: "value", Valid: true}, want: "value"},
		{value: pgtype.Text{Valid: true}},
		{value: pgtype.Text{String: "stale", Valid: false}},
	} {
		if got := textValue(tt.value); got != tt.want {
			t.Fatalf("expected nullable text %q, got %q", tt.want, got)
		}
	}
}
