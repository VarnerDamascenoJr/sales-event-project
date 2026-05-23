package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rabbitmq/amqp091-go"
	"github.com/skip2/go-qrcode"
	"github.com/varner/sales-event-project/internal/events"
	"github.com/varner/sales-event-project/internal/metrics"
	"github.com/varner/sales-event-project/internal/notification"
)

type TicketDeliveryProcessor struct {
	db     *pgxpool.Pool
	sender notification.Sender
}

func NewTicketDeliveryProcessor(db *pgxpool.Pool, sender notification.Sender) *TicketDeliveryProcessor {
	if sender == nil {
		sender = notification.LogSender{}
	}
	return &TicketDeliveryProcessor{db: db, sender: sender}
}

func (p *TicketDeliveryProcessor) Handle(ctx context.Context, delivery amqp091.Delivery) error {
	var event events.SaleCompleted
	if err := json.Unmarshal(delivery.Body, &event); err != nil {
		metrics.TicketDeliveryTotal.WithLabelValues("invalid_message").Inc()
		return err
	}

	start := time.Now()
	ticketEmail, err := p.prepareTicketEmail(ctx, event.SaleID)
	if err != nil {
		metrics.TicketDeliveryTotal.WithLabelValues("failed").Inc()
		return err
	}
	if ticketEmail == nil {
		metrics.TicketDeliveryTotal.WithLabelValues("skipped").Inc()
		slog.Info("ticket delivery skipped", "sale_id", event.SaleID)
		return nil
	}

	if err := p.sender.SendTickets(ctx, *ticketEmail); err != nil {
		metrics.TicketDeliveryTotal.WithLabelValues("failed").Inc()
		slog.Error("send ticket email failed", "sale_id", event.SaleID, "recipient", ticketEmail.To, "error", err)
		if markErr := p.markTicketEmailFailed(ctx, event.SaleID, err); markErr != nil {
			slog.Error("mark ticket email failed", "sale_id", event.SaleID, "error", markErr)
		}
		return err
	}

	if err := p.markTicketEmailSent(ctx, event.SaleID); err != nil {
		metrics.TicketDeliveryTotal.WithLabelValues("failed").Inc()
		return err
	}

	metrics.TicketDeliveryTotal.WithLabelValues("sent").Inc()
	metrics.IssuedTicketsTotal.Add(float64(len(ticketEmail.Tickets)))
	slog.Info("ticket email delivered",
		"sale_id", event.SaleID,
		"sales_event_id", event.SalesEventID,
		"recipient", ticketEmail.To,
		"tickets", len(ticketEmail.Tickets),
		"duration_ms", time.Since(start).Milliseconds(),
	)
	return nil
}

func (p *TicketDeliveryProcessor) prepareTicketEmail(ctx context.Context, saleID string) (*notification.TicketEmail, error) {
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	sale, err := p.getCompletedSale(ctx, tx, saleID)
	if err != nil {
		return nil, err
	}
	if sale.Status != events.SaleCompletedStatus {
		return nil, tx.Commit(ctx)
	}
	alreadySent, err := p.ticketEmailAlreadySent(ctx, tx, sale.ID)
	if err != nil {
		return nil, err
	}
	if alreadySent {
		return nil, tx.Commit(ctx)
	}

	issuedTickets, err := p.issueTickets(ctx, tx, sale)
	if err != nil {
		return nil, err
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO email_notifications (sale_id, recipient_email, status, created_at, updated_at)
		VALUES ($1, $2, 'PENDING', NOW(), NOW())
		ON CONFLICT (sale_id) DO UPDATE
		SET recipient_email = EXCLUDED.recipient_email,
		    status = CASE
		        WHEN email_notifications.status = 'SENT' THEN email_notifications.status
		        ELSE 'PENDING'
		    END,
		    error_message = NULL,
		    updated_at = NOW()
	`, sale.ID, sale.CustomerEmail); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return &notification.TicketEmail{
		To:             sale.CustomerEmail,
		CustomerName:   sale.CustomerName,
		SalesEventName: sale.SalesEventName,
		StartsAt:       sale.StartsAt,
		Tickets:        issuedTickets,
	}, nil
}

func (p *TicketDeliveryProcessor) ticketEmailAlreadySent(ctx context.Context, tx pgx.Tx, saleID string) (bool, error) {
	var status string
	if err := tx.QueryRow(ctx, `
		SELECT status
		FROM email_notifications
		WHERE sale_id = $1
	`, saleID).Scan(&status); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return status == "SENT", nil
}

type completedSale struct {
	ID             string
	SalesEventName string
	StartsAt       time.Time
	CustomerID     string
	CustomerName   string
	CustomerEmail  string
	Status         string
}

func (p *TicketDeliveryProcessor) getCompletedSale(ctx context.Context, tx pgx.Tx, saleID string) (completedSale, error) {
	var sale completedSale
	if err := tx.QueryRow(ctx, `
		SELECT s.id, se.name, se.starts_at, s.customer_id, c.name, c.email, s.status
		FROM sales s
		JOIN sales_events se ON se.id = s.sales_event_id
		JOIN customers c ON c.id = s.customer_id
		WHERE s.id = $1
		FOR UPDATE
	`, saleID).Scan(
		&sale.ID,
		&sale.SalesEventName,
		&sale.StartsAt,
		&sale.CustomerID,
		&sale.CustomerName,
		&sale.CustomerEmail,
		&sale.Status,
	); err != nil {
		return completedSale{}, err
	}
	return sale, nil
}

func (p *TicketDeliveryProcessor) issueTickets(ctx context.Context, tx pgx.Tx, sale completedSale) ([]notification.IssuedTicket, error) {
	rows, err := tx.Query(ctx, `
		SELECT si.ticket_id, t.name, si.quantity
		FROM sale_items si
		JOIN tickets t ON t.id = si.ticket_id
		WHERE si.sale_id = $1
		ORDER BY si.created_at ASC
	`, sale.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]completedSaleItem, 0)
	for rows.Next() {
		var item completedSaleItem
		if err := rows.Scan(&item.TicketID, &item.TicketName, &item.Quantity); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	issuedTickets := make([]notification.IssuedTicket, 0)
	for _, item := range items {
		for sequence := 1; sequence <= item.Quantity; sequence++ {
			issuedTicketID := deterministicIssuedTicketID(sale.ID, item.TicketID, sequence)
			qrPayload := fmt.Sprintf("issued_ticket:%s", issuedTicketID)
			qrCodePNG, err := qrcode.Encode(qrPayload, qrcode.Medium, 256)
			if err != nil {
				return nil, err
			}

			if _, err := tx.Exec(ctx, `
				INSERT INTO issued_tickets (id, sale_id, ticket_id, customer_id, sequence, qr_code_payload)
				VALUES ($1, $2, $3, $4, $5, $6)
				ON CONFLICT (sale_id, ticket_id, sequence) DO NOTHING
			`, issuedTicketID, sale.ID, item.TicketID, sale.CustomerID, sequence, qrPayload); err != nil {
				return nil, err
			}

			issuedTickets = append(issuedTickets, notification.IssuedTicket{
				ID:         issuedTicketID,
				TicketName: item.TicketName,
				QRCodePNG:  qrCodePNG,
				QRPayload:  qrPayload,
			})
		}
	}

	return issuedTickets, nil
}

type completedSaleItem struct {
	TicketID   string
	TicketName string
	Quantity   int
}

func (p *TicketDeliveryProcessor) markTicketEmailSent(ctx context.Context, saleID string) error {
	_, err := p.db.Exec(ctx, `
		UPDATE email_notifications
		SET status = 'SENT',
		    error_message = NULL,
		    sent_at = NOW(),
		    updated_at = NOW()
		WHERE sale_id = $1
	`, saleID)
	if err != nil {
		return err
	}

	_, err = p.db.Exec(ctx, `
		UPDATE issued_tickets
		SET emailed_at = NOW()
		WHERE sale_id = $1
	`, saleID)
	return err
}

func (p *TicketDeliveryProcessor) markTicketEmailFailed(ctx context.Context, saleID string, sendErr error) error {
	_, err := p.db.Exec(ctx, `
		UPDATE email_notifications
		SET status = 'FAILED',
		    error_message = $2,
		    updated_at = NOW()
		WHERE sale_id = $1
	`, saleID, sendErr.Error())
	return err
}

func deterministicIssuedTicketID(saleID string, ticketID string, sequence int) string {
	name := fmt.Sprintf("issued-ticket:%s:%s:%d", saleID, ticketID, sequence)
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte(name)).String()
}
