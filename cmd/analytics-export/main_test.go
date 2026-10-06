package main

import (
	"testing"
	"time"

	"github.com/varner/sales-event-project/internal/analytics"
)

func TestMarshalJSONHandlesExtremeAnalyticsDocument(t *testing.T) {
	base := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	document := analytics.BuildDocumentWithInventory(
		base.Add(time.Hour),
		analytics.Source{SalesEventID: "event-1", Filter: "sales_event_id"},
		[]analytics.Event{
			{EventType: "sale.created", OccurredAt: base, SalesEventID: "event-1", SaleID: "sale-1", Status: "COMPLETED"},
			{EventType: "sale.item.created", OccurredAt: base, SalesEventID: "event-1", SaleID: "sale-1", TicketID: "ticket-vip", TicketType: "vip", Quantity: 1_000_000},
			{EventType: "sale.created", OccurredAt: base.Add(5 * time.Minute), SalesEventID: "event-1", SaleID: "sale-2", Status: "COMPLETED"},
			{EventType: "sale.item.created", OccurredAt: base.Add(5 * time.Minute), SalesEventID: "event-1", SaleID: "sale-2", TicketID: "ticket-vip", TicketType: "vip", Quantity: 1_000_000},
		},
		[]analytics.WindowSpec{{Name: "5m", Duration: 5 * time.Minute}},
		[]analytics.TicketInventory{{SalesEventID: "event-1", TicketID: "ticket-vip", TicketType: "vip", AvailableQuantity: 1000}},
	)

	if _, err := marshalJSON(document, false); err != nil {
		t.Fatalf("marshal compact analytics export: %v", err)
	}
	if _, err := marshalJSON(document, true); err != nil {
		t.Fatalf("marshal pretty analytics export: %v", err)
	}
}
