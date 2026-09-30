package httpapi

import (
	"time"

	"github.com/varner/sales-event-project/internal/correlation"
)

type TicketReadModel struct {
	Name              string
	Price             int
	AvailableQuantity int
}

type CreateSaleRequest struct {
	SalesEventID  string           `json:"salesEventId"`
	CustomerID    string           `json:"customerId"`
	CustomerName  string           `json:"customerName"`
	CustomerEmail string           `json:"customerEmail"`
	Status        string           `json:"status"`
	Items         []CreateSaleItem `json:"items"`
}

type CreateSaleItem struct {
	TicketID  string `json:"ticketId"`
	Quantity  int    `json:"quantity"`
	UnitPrice int    `json:"unitPrice"`
}

type CreatePaymentRequest struct {
	Amount   int    `json:"amount"`
	Provider string `json:"provider"`
	Status   string `json:"status"`
}

type CreatePaymentIntentRequest struct {
	Amount   int    `json:"amount"`
	Provider string `json:"provider"`
}

type CreateCheckInRequest struct {
	TicketCode string `json:"ticketCode"`
}

type CreateEmailEventRequest struct {
	SaleID          string    `json:"saleId"`
	EventType       string    `json:"eventType"`
	OccurredAt      time.Time `json:"occurredAt"`
	ProviderEventID string    `json:"providerEventId"`
	Reason          string    `json:"reason"`
}

type CreatePaymentWebhookRequest struct {
	PaymentIntentID   string    `json:"paymentIntentId"`
	SaleID            string    `json:"saleId"`
	Provider          string    `json:"provider"`
	Status            string    `json:"status"`
	Amount            int       `json:"amount"`
	OccurredAt        time.Time `json:"occurredAt"`
	ProviderReference string    `json:"providerReference"`
}

type CreatePaymentIntentStoreRequest struct {
	SaleID   string
	Amount   int
	Provider string
}

type ProcessPaymentRequest struct {
	SaleID   string
	Amount   int
	Provider string
	Status   string
}

type ProcessPaymentWebhookRequest struct {
	PaymentIntentID   string
	SaleID            string
	Provider          string
	Status            string
	Amount            int
	OccurredAt        time.Time
	ProviderReference string
}

type CheckInTicketRequest struct {
	SalesEventID   string
	IssuedTicketID string
	TicketCode     string
}

type RecordEmailEventRequest struct {
	SaleID          string
	EventType       string
	OccurredAt      time.Time
	ProviderEventID string
	Reason          string
}

type ProcessPaymentResult struct {
	SaleID           string               `json:"saleId"`
	SalesEventID     string               `json:"salesEventId"`
	SaleStatus       string               `json:"saleStatus"`
	Payment          PaymentDTO           `json:"payment"`
	Metadata         correlation.Metadata `json:"-"`
	IdempotentReplay bool                 `json:"-"`
}

type CheckInTicketResult struct {
	CheckInID      string               `json:"checkInId"`
	IssuedTicketID string               `json:"issuedTicketId"`
	SalesEventID   string               `json:"salesEventId"`
	SaleID         string               `json:"saleId"`
	TicketID       string               `json:"ticketId"`
	TicketName     string               `json:"ticketName"`
	CustomerID     string               `json:"customerId"`
	CustomerName   string               `json:"customerName"`
	CheckedInAt    time.Time            `json:"checkedInAt"`
	Metadata       correlation.Metadata `json:"-"`
}

type RecordEmailEventResult struct {
	SaleID          string               `json:"saleId"`
	Status          string               `json:"status"`
	ProviderEventID string               `json:"providerEventId"`
	RecordedAt      time.Time            `json:"recordedAt"`
	Metadata        correlation.Metadata `json:"-"`
}

type ListSalesResponse struct {
	Page     int               `json:"page"`
	PageSize int               `json:"pageSize"`
	Total    int               `json:"total"`
	Data     []SaleListItemDTO `json:"data"`
}

type SaleListItemDTO struct {
	ID             string    `json:"id"`
	SalesEventID   string    `json:"salesEventId"`
	SalesEventName string    `json:"salesEventName"`
	CustomerID     string    `json:"customerId"`
	CustomerName   string    `json:"customerName"`
	CustomerEmail  string    `json:"customerEmail"`
	Status         string    `json:"status"`
	TotalAmount    int       `json:"totalAmount"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

type SaleDetailDTO struct {
	ID             string            `json:"id"`
	SalesEventID   string            `json:"salesEventId"`
	SalesEventName string            `json:"salesEventName"`
	CustomerID     string            `json:"customerId"`
	CustomerName   string            `json:"customerName"`
	CustomerEmail  string            `json:"customerEmail"`
	Status         string            `json:"status"`
	TotalAmount    int               `json:"totalAmount"`
	Payment        PaymentDTO        `json:"payment"`
	Items          []SaleItemReadDTO `json:"items"`
	CreatedAt      time.Time         `json:"createdAt"`
	UpdatedAt      time.Time         `json:"updatedAt"`
}

type PaymentDTO struct {
	Status      string    `json:"status"`
	Amount      int       `json:"amount"`
	Provider    string    `json:"provider"`
	ProcessedAt time.Time `json:"processedAt"`
}

type PaymentIntentDTO struct {
	ID                string               `json:"id"`
	SaleID            string               `json:"saleId"`
	Status            string               `json:"status"`
	Provider          string               `json:"provider"`
	Amount            int                  `json:"amount"`
	ProviderReference string               `json:"providerReference"`
	ClientSecret      string               `json:"clientSecret"`
	CreatedAt         time.Time            `json:"createdAt"`
	UpdatedAt         time.Time            `json:"updatedAt"`
	Metadata          correlation.Metadata `json:"-"`
}

type SaleItemReadDTO struct {
	TicketID   string    `json:"ticketId"`
	TicketName string    `json:"ticketName"`
	Quantity   int       `json:"quantity"`
	UnitPrice  int       `json:"unitPrice"`
	CreatedAt  time.Time `json:"createdAt"`
}

type listSalesFilter struct {
	SalesEventID string
	Status       string
	EventName    string
	Page         int
	PageSize     int
}
