package events

import "time"

const (
	SaleCreatedRoutingKey    = "sale.created"
	SaleCompletedRoutingKey  = "sale.completed"
	SaleFailedRoutingKey     = "sale.failed"
	SalePendingPaymentStatus = "PENDING_PAYMENT"
	SaleCompletedStatus      = "COMPLETED"
	SaleFailedStatus         = "FAILED"
	SaleProcessingStatus     = "PROCESSING"
	PaymentPendingStatus     = "PENDING"
	PaymentApprovedStatus    = "APPROVED"
	PaymentFailedStatus      = "FAILED"
	MaxTextLength            = 50
	MaxCustomerNameLength    = 100
	MaxEmailLength           = 255
	MaxMoneyAmountInCents    = 1_000_000_000
	MaxTicketQuantity        = 1_000_000
)

var AvailableSaleStatuses = []string{
	SalePendingPaymentStatus,
	SaleCompletedStatus,
	SaleFailedStatus,
}

type CorrelationMetadata struct {
	RequestID     string `json:"requestId,omitempty"`
	CorrelationID string `json:"correlationId,omitempty"`
	TransactionID string `json:"transactionId,omitempty"`
	CausationID   string `json:"causationId,omitempty"`
}

type SaleCreated struct {
	EventID       string     `json:"eventId"`
	EventType     string     `json:"eventType"`
	OccurredAt    time.Time  `json:"occurredAt"`
	SaleID        string     `json:"saleId"`
	SalesEventID  string     `json:"salesEventId"`
	CustomerID    string     `json:"customerId"`
	CustomerName  string     `json:"customerName"`
	CustomerEmail string     `json:"customerEmail"`
	Items         []SaleItem `json:"items"`
	Metadata      CorrelationMetadata `json:"metadata,omitempty"`
}

type SaleCompleted struct {
	EventID      string    `json:"eventId"`
	EventType    string    `json:"eventType"`
	OccurredAt   time.Time `json:"occurredAt"`
	SaleID       string    `json:"saleId"`
	SalesEventID string    `json:"salesEventId"`
	Metadata     CorrelationMetadata `json:"metadata,omitempty"`
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
