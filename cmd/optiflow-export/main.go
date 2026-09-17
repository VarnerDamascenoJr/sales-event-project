package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/varner/sales-event-project/internal/config"
	"github.com/varner/sales-event-project/internal/database"
)

const schemaVersion = "sales-event-optiflow-export.v1"

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

func main() {
	var salesEventID string
	var outputPath string
	var pretty bool
	var limit int

	flag.StringVar(&salesEventID, "sales-event-id", "", "optional sales event UUID filter")
	flag.StringVar(&outputPath, "output", "", "optional output path; stdout is used when empty")
	flag.BoolVar(&pretty, "pretty", true, "render indented JSON")
	flag.IntVar(&limit, "limit", 500, "maximum number of sales to export")
	flag.Parse()

	if limit <= 0 {
		slog.Error("limit must be greater than zero")
		os.Exit(2)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	cfg := config.Load()
	db, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("connect postgres failed", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	document, err := readExport(ctx, db, salesEventID, limit)
	if err != nil {
		slog.Error("export sales failed", "error", err)
		os.Exit(1)
	}

	payload, err := marshalJSON(document, pretty)
	if err != nil {
		slog.Error("marshal export failed", "error", err)
		os.Exit(1)
	}

	if outputPath == "" {
		fmt.Println(string(payload))
		return
	}

	if err := os.WriteFile(outputPath, append(payload, '\n'), 0o600); err != nil {
		slog.Error("write export failed", "path", outputPath, "error", err)
		os.Exit(1)
	}
}

type queryer interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func readExport(ctx context.Context, db queryer, salesEventID string, limit int) (exportDocument, error) {
	queryRows, err := db.Query(ctx, `
WITH sale_item_rows AS (
	SELECT
		si.sale_id,
		jsonb_agg(
			jsonb_build_object(
				'ticketId', si.ticket_id::text,
				'ticketName', t.name,
				'quantity', si.quantity,
				'unitPrice', si.unit_price,
				'lineTotal', si.quantity * si.unit_price,
				'createdAt', si.created_at
			)
			ORDER BY si.created_at ASC, si.ticket_id::text ASC
		) AS items
	FROM sale_items si
	JOIN tickets t ON t.id = si.ticket_id
	GROUP BY si.sale_id
),
issued_ticket_counts AS (
	SELECT sale_id, COUNT(*)::int AS issued_ticket_count
	FROM issued_tickets
	GROUP BY sale_id
),
check_in_counts AS (
	SELECT it.sale_id, COUNT(tci.id)::int AS checked_in_ticket_count
	FROM issued_tickets it
	LEFT JOIN ticket_check_ins tci ON tci.issued_ticket_id = it.id
	GROUP BY it.sale_id
)
SELECT
	se.id::text,
	se.name,
	se.starts_at,
	s.id::text,
	s.customer_id,
	c.name,
	c.email,
	s.status,
	s.total_amount,
	s.request_id,
	s.correlation_id,
	s.transaction_id,
	s.created_at,
	s.updated_at,
	p.status,
	p.amount,
	p.provider,
	p.processed_at,
	en.status,
	en.attempts,
	en.sent_at,
	en.delivered_at,
	en.opened_at,
	en.clicked_at,
	en.bounced_at,
	COALESCE(itc.issued_ticket_count, 0),
	COALESCE(cic.checked_in_ticket_count, 0),
	COALESCE(sir.items, '[]'::jsonb)
FROM sales s
JOIN sales_events se ON se.id = s.sales_event_id
JOIN customers c ON c.id = s.customer_id
LEFT JOIN payments p ON p.sale_id = s.id
LEFT JOIN email_notifications en ON en.sale_id = s.id
LEFT JOIN sale_item_rows sir ON sir.sale_id = s.id
LEFT JOIN issued_ticket_counts itc ON itc.sale_id = s.id
LEFT JOIN check_in_counts cic ON cic.sale_id = s.id
WHERE ($1 = '' OR s.sales_event_id::text = $1)
ORDER BY s.created_at ASC, s.id::text ASC
LIMIT $2
`, salesEventID, limit)
	if err != nil {
		return exportDocument{}, err
	}
	defer queryRows.Close()

	document := exportDocument{
		SchemaVersion: schemaVersion,
		GeneratedAt:   time.Now().UTC(),
		Source: exportSource{
			Service:      "sales-event-project",
			SalesEventID: salesEventID,
		},
		Summary: exportSummary{
			StatusCounts: make(map[string]int),
		},
		Sales: make([]exportSale, 0),
	}

	for queryRows.Next() {
		var sale exportSale
		var eventStartsAt time.Time
		var paymentStatus pgtype.Text
		var paymentAmount pgtype.Int4
		var paymentProvider pgtype.Text
		var paymentProcessedAt pgtype.Timestamptz
		var emailStatus pgtype.Text
		var emailAttempts pgtype.Int4
		var emailSentAt pgtype.Timestamptz
		var emailDeliveredAt pgtype.Timestamptz
		var emailOpenedAt pgtype.Timestamptz
		var emailClickedAt pgtype.Timestamptz
		var emailBouncedAt pgtype.Timestamptz
		var itemsPayload []byte

		if err := queryRows.Scan(
			&sale.SalesEventID,
			&sale.SalesEventName,
			&eventStartsAt,
			&sale.SaleID,
			&sale.CustomerID,
			&sale.CustomerName,
			&sale.CustomerEmail,
			&sale.Status,
			&sale.TotalAmount,
			&sale.RequestID,
			&sale.CorrelationID,
			&sale.TransactionID,
			&sale.CreatedAt,
			&sale.UpdatedAt,
			&paymentStatus,
			&paymentAmount,
			&paymentProvider,
			&paymentProcessedAt,
			&emailStatus,
			&emailAttempts,
			&emailSentAt,
			&emailDeliveredAt,
			&emailOpenedAt,
			&emailClickedAt,
			&emailBouncedAt,
			&sale.IssuedTicketCount,
			&sale.CheckedInTicketCount,
			&itemsPayload,
		); err != nil {
			return exportDocument{}, err
		}

		if err := json.Unmarshal(itemsPayload, &sale.Items); err != nil {
			return exportDocument{}, fmt.Errorf("decode sale items for sale %s: %w", sale.SaleID, err)
		}

		if paymentStatus.Valid {
			sale.Payment = &paymentExport{
				Status:      paymentStatus.String,
				Amount:      int(paymentAmount.Int32),
				Provider:    paymentProvider.String,
				ProcessedAt: timePtr(paymentProcessedAt),
			}
		}

		if emailStatus.Valid {
			sale.Email = &emailExport{
				Status:      emailStatus.String,
				Attempts:    int(emailAttempts.Int32),
				SentAt:      timePtr(emailSentAt),
				DeliveredAt: timePtr(emailDeliveredAt),
				OpenedAt:    timePtr(emailOpenedAt),
				ClickedAt:   timePtr(emailClickedAt),
				BouncedAt:   timePtr(emailBouncedAt),
			}
		}

		if document.Source.EventName == "" {
			document.Source.EventName = sale.SalesEventName
			document.Source.EventStartsAt = &eventStartsAt
		} else if document.Source.SalesEventID == "" && document.Source.EventName != sale.SalesEventName {
			document.Source.EventName = "multiple sales events"
			document.Source.EventStartsAt = nil
		}

		document.Summary.SaleCount++
		document.Summary.TotalAmount += sale.TotalAmount
		document.Summary.StatusCounts[sale.Status]++
		if sale.Status == "COMPLETED" {
			document.Summary.CompletedSaleCount++
		}
		for _, item := range sale.Items {
			document.Summary.TotalItems += item.Quantity
		}

		document.Sales = append(document.Sales, sale)
	}

	if err := queryRows.Err(); err != nil {
		return exportDocument{}, err
	}

	if salesEventID != "" {
		document.Source.Filter = "sales_event_id"
	}

	return document, nil
}

func marshalJSON(value any, pretty bool) ([]byte, error) {
	if pretty {
		return json.MarshalIndent(value, "", "  ")
	}
	return json.Marshal(value)
}

func timePtr(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	t := value.Time.UTC()
	return &t
}
