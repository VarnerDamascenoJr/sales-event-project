package analytics

import (
	"testing"
	"time"
)

func TestBuildFunnelRecordsAppliesSaleDefaultsAndPaymentToTicketRecords(t *testing.T) {
	base := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	records := buildFunnelRecords([]Event{
		{EventType: "sale.created", OccurredAt: base, SalesEventID: "event-1", SaleID: "sale-1", Status: "PENDING_PAYMENT"},
		{EventType: "payment.processed", OccurredAt: base.Add(time.Minute), SaleID: "sale-1", Status: "APPROVED", Provider: "card"},
		{EventType: "sale.item.created", OccurredAt: base.Add(2 * time.Minute), SalesEventID: "event-1", SaleID: "sale-1", TicketID: "ticket-general", TicketType: "General", Status: "PENDING_PAYMENT"},
		{EventType: "ticket.issued", OccurredAt: base.Add(3 * time.Minute), SalesEventID: "event-1", SaleID: "sale-1", TicketID: "ticket-general", TicketType: "General"},
		{EventType: "checkin.completed", OccurredAt: base.Add(4 * time.Minute), SalesEventID: "event-1", SaleID: "sale-1", TicketID: "ticket-general", TicketType: "General"},
	})

	if len(records) != 1 {
		t.Fatalf("expected one funnel record, got %d", len(records))
	}
	record := records[0]
	if record.SalesEventID != "event-1" || record.SaleID != "sale-1" || record.TicketID != "ticket-general" || record.TicketType != "General" {
		t.Fatalf("unexpected dimensions: %+v", record)
	}
	if !record.Accepted || !record.Pending || !record.Paid || !record.TicketIssued || !record.CheckedIn {
		t.Fatalf("expected all funnel stages to be reached, got %+v", record)
	}
	if record.Provider != "card" {
		t.Fatalf("expected payment provider to be propagated, got %q", record.Provider)
	}
	if !record.AcceptedAt.Equal(base) {
		t.Fatalf("expected sale accepted time, got %s", record.AcceptedAt)
	}
}

func TestBuildFunnelRecordsUsesUnknownProviderAndSkipsUnacceptedTickets(t *testing.T) {
	base := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	records := buildFunnelRecords([]Event{
		{EventType: "sale.item.created", OccurredAt: base, SalesEventID: "event-1", SaleID: "sale-1", TicketID: "ticket-general", TicketType: "General", Status: "FAILED"},
		{EventType: "ticket.issued", OccurredAt: base.Add(time.Minute), SalesEventID: "event-1", SaleID: "sale-2", TicketID: "ticket-vip", TicketType: "VIP"},
	})

	if len(records) != 1 {
		t.Fatalf("expected only accepted sale item to produce a record, got %d", len(records))
	}
	if records[0].Provider != "unknown" {
		t.Fatalf("expected unknown provider, got %q", records[0].Provider)
	}
}
