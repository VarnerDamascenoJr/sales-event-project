package httpapi

import (
	"time"

	"github.com/varner/sales-event-project/internal/correlation"
)

type paymentSale struct {
	ID             string
	SalesEventID   string
	SalesEventName string
	StartsAt       time.Time
	CustomerID     string
	CustomerName   string
	CustomerEmail  string
	Status         string
	TotalAmount    int
	CreatedAt      time.Time
	UpdatedAt      time.Time
	Metadata       correlation.Metadata
}

type paymentIntentRecord struct {
	ID       string
	SaleID   string
	Provider string
	Amount   int
	Status   string
}

type issuedTicketForCheckIn struct {
	IssuedTicketID string
	SalesEventID   string
	SaleID         string
	SaleStatus     string
	PaymentStatus  string
	TicketID       string
	TicketName     string
	CustomerID     string
	CustomerName   string
	Metadata       correlation.Metadata
}
