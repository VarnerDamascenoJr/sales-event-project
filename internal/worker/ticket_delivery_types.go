package worker

import (
	"time"

	"github.com/varner/sales-event-project/internal/correlation"
	"github.com/varner/sales-event-project/internal/notification"
)

type completedSale struct {
	ID             string
	SalesEventName string
	StartsAt       time.Time
	CustomerID     string
	CustomerName   string
	CustomerEmail  string
	Status         string
}

type completedSaleItem struct {
	TicketID   string
	TicketName string
	Quantity   int
}

type retryTicketEmail struct {
	To             string
	CustomerName   string
	SalesEventName string
	StartsAt       time.Time
	Tickets        []notification.IssuedTicket
}

type pendingEmailRetry struct {
	SaleID   string
	Attempts int
	Metadata correlation.Metadata
}
