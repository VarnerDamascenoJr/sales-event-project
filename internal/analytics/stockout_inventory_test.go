package analytics

import "testing"

func TestNormalizeTicketInventoryFiltersEmptyTicketsAndSorts(t *testing.T) {
	inventory := []TicketInventory{
		{SalesEventID: "event-2", TicketID: "ticket-standard", TicketType: "standard", AvailableQuantity: 10},
		{SalesEventID: "event-1", TicketID: "ticket-vip", TicketType: "vip", AvailableQuantity: 5},
		{SalesEventID: "event-1", TicketID: "", TicketType: "ignored", AvailableQuantity: 100},
		{SalesEventID: "event-1", TicketID: "ticket-standard", TicketType: "standard", AvailableQuantity: 20},
	}

	normalized := normalizeTicketInventory(inventory)

	if len(normalized) != 3 {
		t.Fatalf("expected three inventory items, got %d", len(normalized))
	}
	gotOrder := []string{
		normalized[0].SalesEventID + "/" + normalized[0].TicketType + "/" + normalized[0].TicketID,
		normalized[1].SalesEventID + "/" + normalized[1].TicketType + "/" + normalized[1].TicketID,
		normalized[2].SalesEventID + "/" + normalized[2].TicketType + "/" + normalized[2].TicketID,
	}
	wantOrder := []string{
		"event-1/standard/ticket-standard",
		"event-1/vip/ticket-vip",
		"event-2/standard/ticket-standard",
	}
	for i := range wantOrder {
		if gotOrder[i] != wantOrder[i] {
			t.Fatalf("unexpected normalized order: got %v want %v", gotOrder, wantOrder)
		}
	}
}
