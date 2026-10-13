package analytics

import (
	"math"
	"time"
)

func demandEvent(at time.Time, ticketType string, quantity int) Event {
	return Event{
		EventType:    "sale.item.created",
		OccurredAt:   at,
		SalesEventID: "event-1",
		SaleID:       "sale-1",
		TicketID:     "ticket-" + ticketType,
		TicketType:   ticketType,
		Quantity:     quantity,
	}
}

func almostEqual(left float64, right float64) bool {
	return math.Abs(left-right) < 0.000001
}
