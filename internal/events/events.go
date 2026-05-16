package events

import "time"

const (
	SaleCreatedRoutingKey = "sale.created"
	SaleCompletedStatus   = "COMPLETED"
	SaleFailedStatus      = "FAILED"
	SaleProcessingStatus  = "PROCESSING"
	PaymentApprovedStatus = "APPROVED"
	PaymentFailedStatus   = "FAILED"
	MaxTextLength         = 50
	MaxMoneyAmountInCents = 1_000_000_000
	MaxTicketQuantity     = 1_000_000
)

type SaleCreated struct {
	EventID      string     `json:"eventId"`
	EventType    string     `json:"eventType"`
	OccurredAt   time.Time  `json:"occurredAt"`
	SaleID       string     `json:"saleId"`
	SalesEventID string     `json:"salesEventId"`
	CustomerID   string     `json:"customerId"`
	Items        []SaleItem `json:"items"`
}

type SaleItem struct {
	TicketID  string `json:"ticketId"`
	Quantity  int    `json:"quantity"`
	UnitPrice int    `json:"unitPrice"`
}

type EventEnvelope[T any] struct {
	EventID    string    `json:"eventId"`
	EventType  string    `json:"eventType"`
	OccurredAt time.Time `json:"occurredAt"`
	Payload    T         `json:"payload"`
}
