package analyticsdb

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/varner/sales-event-project/internal/analytics"
)

func TestReadTicketInventory(t *testing.T) {
	for _, tt := range []struct {
		name   string
		values [][]any
		want   []analytics.TicketInventory
	}{
		{name: "empty inventory", want: []analytics.TicketInventory{}},
		{
			name: "available and sold out tickets",
			values: [][]any{
				{"event-123", "ticket-123", "General", 10},
				{"event-123", "ticket-456", "VIP", 0},
			},
			want: []analytics.TicketInventory{
				{SalesEventID: "event-123", TicketID: "ticket-123", TicketType: "General", AvailableQuantity: 10},
				{SalesEventID: "event-123", TicketID: "ticket-456", TicketType: "VIP", AvailableQuantity: 0},
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rows := &fakeRows{values: tt.values}
			db := &fakeQueryer{results: []queryResult{{rows: rows}}}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			got, err := ReadTicketInventory(ctx, db, Filter{SalesEventID: "event-123", Start: "start", End: "end", Limit: 1})
			if err != nil {
				t.Fatalf("read ticket inventory: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) || !rows.closed {
				t.Fatalf("expected inventory %+v and closed rows, got %+v, closed=%t", tt.want, got, rows.closed)
			}
			if len(db.calls) != 1 {
				t.Fatalf("expected one inventory query, got %d", len(db.calls))
			}
			call := db.calls[0]
			if call.ctx != ctx || !strings.Contains(call.sql, "FROM tickets t") || !reflect.DeepEqual(call.args, []any{"event-123"}) {
				t.Fatalf("inventory must preserve context and filter only by sales event, got %+v", call)
			}
		})
	}
}
