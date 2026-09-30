package main

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

type exportDocument struct {
	SchemaVersion string        `json:"schemaVersion"`
	GeneratedAt   time.Time     `json:"generatedAt"`
	Source        exportSource  `json:"source"`
	Summary       exportSummary `json:"summary"`
	Sales         []exportSale  `json:"sales"`
}

type exportSource struct {
	Service       string     `json:"service"`
	SalesEventID  string     `json:"salesEventId,omitempty"`
	EventName     string     `json:"eventName,omitempty"`
	EventStartsAt *time.Time `json:"eventStartsAt,omitempty"`
	Filter        string     `json:"filter,omitempty"`
}

type exportSummary struct {
	SaleCount          int            `json:"saleCount"`
	CompletedSaleCount int            `json:"completedSaleCount"`
	TotalItems         int            `json:"totalItems"`
	TotalAmount        int            `json:"totalAmount"`
	StatusCounts       map[string]int `json:"statusCounts"`
}

type exportSale struct {
	SaleID               string         `json:"saleId"`
	SalesEventID         string         `json:"salesEventId"`
	SalesEventName       string         `json:"salesEventName"`
	CustomerID           string         `json:"customerId"`
	CustomerName         string         `json:"customerName"`
	CustomerEmail        string         `json:"customerEmail"`
	Status               string         `json:"status"`
	TotalAmount          int            `json:"totalAmount"`
	RequestID            string         `json:"requestId,omitempty"`
	CorrelationID        string         `json:"correlationId,omitempty"`
	TransactionID        string         `json:"transactionId,omitempty"`
	CreatedAt            time.Time      `json:"createdAt"`
	UpdatedAt            time.Time      `json:"updatedAt"`
	Payment              *paymentExport `json:"payment,omitempty"`
	Email                *emailExport   `json:"email,omitempty"`
	IssuedTicketCount    int            `json:"issuedTicketCount"`
	CheckedInTicketCount int            `json:"checkedInTicketCount"`
	Items                []itemExport   `json:"items"`
}

type paymentExport struct {
	Status      string     `json:"status"`
	Amount      int        `json:"amount"`
	Provider    string     `json:"provider"`
	ProcessedAt *time.Time `json:"processedAt,omitempty"`
}

type emailExport struct {
	Status      string     `json:"status"`
	Attempts    int        `json:"attempts"`
	SentAt      *time.Time `json:"sentAt,omitempty"`
	DeliveredAt *time.Time `json:"deliveredAt,omitempty"`
	OpenedAt    *time.Time `json:"openedAt,omitempty"`
	ClickedAt   *time.Time `json:"clickedAt,omitempty"`
	BouncedAt   *time.Time `json:"bouncedAt,omitempty"`
}

type itemExport struct {
	TicketID   string    `json:"ticketId"`
	TicketName string    `json:"ticketName"`
	Quantity   int       `json:"quantity"`
	UnitPrice  int       `json:"unitPrice"`
	LineTotal  int       `json:"lineTotal"`
	CreatedAt  time.Time `json:"createdAt"`
}

type queryer interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}
