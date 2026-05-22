package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
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

type SalesProcessor struct {
	db     *pgxpool.Pool
	sender notification.Sender
}

func NewSalesProcessor(db *pgxpool.Pool, sender notification.Sender) *SalesProcessor {
	if sender == nil {
		sender = notification.LogSender{}
	}
	return &SalesProcessor{db: db, sender: sender}
}

func (p *SalesProcessor) Handle(ctx context.Context, delivery amqp091.Delivery) error {
	start := time.Now()

	var event events.SaleCreated
	if err := json.Unmarshal(delivery.Body, &event); err != nil {
		metrics.WorkerSalesProcessedTotal.WithLabelValues(events.SaleFailedStatus).Inc()
		return err
	}

	status := events.SaleCompletedStatus
	if len(event.Items) == 0 {
		status = events.SaleFailedStatus
	}

	ticketEmail, err := p.persistSale(ctx, event, status)
	if err != nil {
		metrics.WorkerSalesProcessedTotal.WithLabelValues(events.SaleFailedStatus).Inc()
		return err
	}
	if ticketEmail != nil {
		if err := p.sender.SendTickets(ctx, *ticketEmail); err != nil {
			log.Printf("send ticket email failed sale_id=%s recipient=%s: %v", event.SaleID, ticketEmail.To, err)
			if markErr := p.markTicketEmailFailed(ctx, event.SaleID, err); markErr != nil {
				log.Printf("mark ticket email failed sale_id=%s: %v", event.SaleID, markErr)
			}
		} else if err := p.markTicketEmailSent(ctx, event.SaleID); err != nil {
			log.Printf("mark ticket email sent sale_id=%s: %v", event.SaleID, err)
		}
	}

	metrics.WorkerSalesProcessedTotal.WithLabelValues(status).Inc()
	metrics.WorkerProcessingDuration.Observe(time.Since(start).Seconds())
	log.Printf("processed sale_id=%s status=%s", event.SaleID, status)
	return nil
}

func (p *SalesProcessor) persistSale(ctx context.Context, event events.SaleCreated, status string) (*notification.TicketEmail, error) {
	alreadyProcessed, err := p.saleAlreadyProcessed(ctx, event.SaleID)
	if err != nil {
		return nil, err
	}
	if alreadyProcessed {
		return nil, nil
	}

	tx, err := p.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	totalAmount := 0
	for _, item := range event.Items {
		totalAmount += item.Quantity * item.UnitPrice
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO customers (id, email, name, created_at, updated_at)
		VALUES ($1, $2, $3, NOW(), NOW())
		ON CONFLICT (id) DO UPDATE
		SET email = EXCLUDED.email,
		    name = EXCLUDED.name,
		    updated_at = NOW()
	`, event.CustomerID, event.CustomerEmail, event.CustomerName)
	if err != nil {
		return nil, err
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO sales (id, sales_event_id, customer_id, status, total_amount, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW())
		ON CONFLICT (id) DO UPDATE
		SET status = EXCLUDED.status,
		    total_amount = EXCLUDED.total_amount,
		    updated_at = NOW()
	`, event.SaleID, event.SalesEventID, event.CustomerID, status, totalAmount, event.OccurredAt)
	if err != nil {
		return nil, err
	}

	var issuedTickets []notification.IssuedTicket
	for _, item := range event.Items {
		result, err := tx.Exec(ctx, `
			UPDATE tickets
			SET available_quantity = available_quantity - $1
			WHERE id = $2
			  AND available_quantity >= $1
		`, item.Quantity, item.TicketID)
		if err != nil {
			return nil, err
		}
		if result.RowsAffected() == 0 {
			return nil, errInsufficientTickets
		}

		_, err = tx.Exec(ctx, `
			INSERT INTO sale_items (sale_id, ticket_id, quantity, unit_price)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT DO NOTHING
		`, event.SaleID, item.TicketID, item.Quantity, item.UnitPrice)
		if err != nil {
			return nil, err
		}

		if status == events.SaleCompletedStatus {
			tickets, err := p.issueTickets(ctx, tx, event, item)
			if err != nil {
				return nil, err
			}
			issuedTickets = append(issuedTickets, tickets...)
		}
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO payments (sale_id, status, amount, provider, processed_at)
		VALUES ($1, $2, $3, $4, NOW())
		ON CONFLICT (sale_id) DO UPDATE
		SET status = EXCLUDED.status,
		    amount = EXCLUDED.amount,
		    processed_at = NOW()
	`, event.SaleID, paymentStatusForSale(status), totalAmount, "simulation")
	if err != nil {
		return nil, err
	}

	payload, err := json.Marshal(event)
	if err != nil {
		return nil, err
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO outbox_events (event_id, event_type, aggregate_id, payload)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (event_id) DO NOTHING
	`, event.EventID, statusEventName(status), event.SaleID, payload)
	if err != nil {
		return nil, err
	}

	var ticketEmail *notification.TicketEmail
	if status == events.SaleCompletedStatus {
		var eventName string
		var startsAt time.Time
		if err := tx.QueryRow(ctx, `
			SELECT name, starts_at
			FROM sales_events
			WHERE id = $1
		`, event.SalesEventID).Scan(&eventName, &startsAt); err != nil {
			return nil, err
		}

		_, err = tx.Exec(ctx, `
			INSERT INTO email_notifications (sale_id, recipient_email, status, created_at, updated_at)
			VALUES ($1, $2, 'PENDING', NOW(), NOW())
			ON CONFLICT (sale_id) DO NOTHING
		`, event.SaleID, event.CustomerEmail)
		if err != nil {
			return nil, err
		}

		ticketEmail = &notification.TicketEmail{
			To:             event.CustomerEmail,
			CustomerName:   event.CustomerName,
			SalesEventName: eventName,
			StartsAt:       startsAt,
			Tickets:        issuedTickets,
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return ticketEmail, nil
}

func (p *SalesProcessor) issueTickets(ctx context.Context, tx pgx.Tx, event events.SaleCreated, item events.SaleItem) ([]notification.IssuedTicket, error) {
	var ticketName string
	if err := tx.QueryRow(ctx, `
		SELECT name
		FROM tickets
		WHERE id = $1
	`, item.TicketID).Scan(&ticketName); err != nil {
		return nil, err
	}

	issuedTickets := make([]notification.IssuedTicket, 0, item.Quantity)
	for range item.Quantity {
		issuedTicketID := uuid.NewString()
		qrPayload := fmt.Sprintf("issued_ticket:%s", issuedTicketID)
		qrCodePNG, err := qrcode.Encode(qrPayload, qrcode.Medium, 256)
		if err != nil {
			return nil, err
		}

		_, err = tx.Exec(ctx, `
			INSERT INTO issued_tickets (id, sale_id, ticket_id, customer_id, qr_code_payload)
			VALUES ($1, $2, $3, $4, $5)
		`, issuedTicketID, event.SaleID, item.TicketID, event.CustomerID, qrPayload)
		if err != nil {
			return nil, err
		}

		issuedTickets = append(issuedTickets, notification.IssuedTicket{
			ID:         issuedTicketID,
			TicketName: ticketName,
			QRCodePNG:  qrCodePNG,
			QRPayload:  qrPayload,
		})
	}

	return issuedTickets, nil
}

func (p *SalesProcessor) markTicketEmailSent(ctx context.Context, saleID string) error {
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

func (p *SalesProcessor) markTicketEmailFailed(ctx context.Context, saleID string, sendErr error) error {
	_, err := p.db.Exec(ctx, `
		UPDATE email_notifications
		SET status = 'FAILED',
		    error_message = $2,
		    updated_at = NOW()
		WHERE sale_id = $1
	`, saleID, sendErr.Error())
	return err
}

func (p *SalesProcessor) saleAlreadyProcessed(ctx context.Context, saleID string) (bool, error) {
	var exists bool
	err := p.db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM sales
			WHERE id = $1
			  AND status IN ('COMPLETED', 'FAILED')
		)
	`, saleID).Scan(&exists)
	return exists, err
}

var errInsufficientTickets = errProcessing("insufficient tickets while reserving sale")

type errProcessing string

func (e errProcessing) Error() string {
	return string(e)
}

func statusEventName(status string) string {
	if status == events.SaleCompletedStatus {
		return "SALE_COMPLETED"
	}
	return "SALE_FAILED"
}

func paymentStatusForSale(status string) string {
	if status == events.SaleCompletedStatus {
		return events.PaymentApprovedStatus
	}
	return events.PaymentFailedStatus
}
