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
	"github.com/varner/sales-event-project/internal/correlation"
	"github.com/varner/sales-event-project/internal/events"
	"github.com/varner/sales-event-project/internal/metrics"
	"github.com/varner/sales-event-project/internal/notification"
	"github.com/varner/sales-event-project/internal/observability"
	"go.opentelemetry.io/otel/attribute"
)

type TicketDeliveryProcessor struct {
	db     *pgxpool.Pool
	sender notification.Sender
}

const (
	emailStatusPending    = "PENDING"
	emailStatusSent       = "SENT"
	emailStatusFailed     = "FAILED"
	emailStatusDeadLetter = "DEAD_LETTER"
	maxEmailRetryAttempts = 5
)

func NewTicketDeliveryProcessor(db *pgxpool.Pool, sender notification.Sender) *TicketDeliveryProcessor {
	if sender == nil {
		sender = notification.LogSender{}
	}
	return &TicketDeliveryProcessor{db: db, sender: sender}
}

func (p *TicketDeliveryProcessor) RunEmailRetries(ctx context.Context, interval time.Duration, batchSize int) {
	if interval <= 0 {
		slog.Warn("email retry worker disabled because interval is not positive", "interval", interval)
		return
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		if err := p.RetryFailedEmails(ctx, batchSize); err != nil {
			slog.Error("email retry batch failed", "error", err)
		}

		select {
		case <-ctx.Done():
			slog.Info("email retry worker stopped")
			return
		case <-ticker.C:
		}
	}
}

func (p *TicketDeliveryProcessor) Handle(ctx context.Context, delivery amqp091.Delivery) error {
	var event events.SaleCompleted
	if err := json.Unmarshal(delivery.Body, &event); err != nil {
		metrics.TicketDeliveryTotal.WithLabelValues("invalid_message").Inc()
		return err
	}
	ctx = contextWithEventMetadata(ctx, event.Metadata)
	ctx, span := observability.StartBusinessSpan(ctx, "ticket.delivery",
		attribute.String("sale.id", event.SaleID),
		attribute.String("sales_event.id", event.SalesEventID),
	)
	var spanErr error
	defer func() {
		observability.EndSpan(span, spanErr)
	}()

	start := time.Now()
	ticketEmail, err := p.prepareTicketEmail(ctx, event.SaleID)
	if err != nil {
		spanErr = err
		metrics.TicketDeliveryTotal.WithLabelValues("failed").Inc()
		return err
	}
	if ticketEmail == nil {
		span.SetAttributes(attribute.String("ticket.delivery.status", "skipped"))
		metrics.TicketDeliveryTotal.WithLabelValues("skipped").Inc()
		slog.InfoContext(ctx, "ticket delivery skipped", "sale_id", event.SaleID)
		return nil
	}
	span.SetAttributes(attribute.Int("ticket.issued.count", len(ticketEmail.Tickets)))

	emailCtx, emailSpan := observability.StartBusinessSpan(ctx, "email.send",
		attribute.String("sale.id", event.SaleID),
		attribute.Int("ticket.issued.count", len(ticketEmail.Tickets)),
	)
	err = p.sender.SendTickets(emailCtx, *ticketEmail)
	observability.EndSpan(emailSpan, err)
	if err != nil {
		spanErr = err
		metrics.TicketDeliveryTotal.WithLabelValues("failed").Inc()
		slog.ErrorContext(ctx, "send ticket email failed", "sale_id", event.SaleID, "recipient", ticketEmail.To, "error", err)
		if markErr := p.markTicketEmailFailed(ctx, event.SaleID, err); markErr != nil {
			slog.ErrorContext(ctx, "mark ticket email failed", "sale_id", event.SaleID, "error", markErr)
		}
		return err
	}

	if err := p.markTicketEmailSent(ctx, event.SaleID); err != nil {
		spanErr = err
		metrics.TicketDeliveryTotal.WithLabelValues("failed").Inc()
		return err
	}
	span.SetAttributes(attribute.String("ticket.delivery.status", "sent"))

	metrics.TicketDeliveryTotal.WithLabelValues("sent").Inc()
	metrics.IssuedTicketsTotal.Add(float64(len(ticketEmail.Tickets)))
	slog.InfoContext(ctx, "ticket email delivered",
		"sale_id", event.SaleID,
		"sales_event_id", event.SalesEventID,
		"recipient", ticketEmail.To,
		"tickets", len(ticketEmail.Tickets),
		"duration_ms", time.Since(start).Milliseconds(),
	)
	return nil
}

func (p *TicketDeliveryProcessor) prepareTicketEmail(ctx context.Context, saleID string) (*notification.TicketEmail, error) {
	ctx, span := observability.StartBusinessSpan(ctx, "ticket.email.prepare", attribute.String("sale.id", saleID))
	var spanErr error
	defer func() {
		observability.EndSpan(span, spanErr)
	}()

	tx, err := p.db.Begin(ctx)
	if err != nil {
		spanErr = err
		return nil, err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	sale, err := p.getCompletedSale(ctx, tx, saleID)
	if err != nil {
		spanErr = err
		return nil, err
	}
	if sale.Status != events.SaleCompletedStatus {
		span.SetAttributes(attribute.String("ticket.delivery.status", "skipped"), attribute.String("sale.status", sale.Status))
		if err := tx.Commit(ctx); err != nil {
			spanErr = err
			return nil, err
		}
		return nil, nil
	}
	alreadySent, err := p.ticketEmailAlreadySent(ctx, tx, sale.ID)
	if err != nil {
		spanErr = err
		return nil, err
	}
	if alreadySent {
		span.SetAttributes(attribute.String("ticket.delivery.status", "already_sent"))
		if err := tx.Commit(ctx); err != nil {
			spanErr = err
			return nil, err
		}
		return nil, nil
	}

	issuedTickets, err := p.issueTickets(ctx, tx, sale)
	if err != nil {
		spanErr = err
		return nil, err
	}
	span.SetAttributes(attribute.Int("ticket.issued.count", len(issuedTickets)))

	if _, err := tx.Exec(ctx, `
		INSERT INTO email_notifications (sale_id, recipient_email, status, created_at, updated_at)
		VALUES ($1, $2, 'PENDING', NOW(), NOW())
		ON CONFLICT (sale_id) DO UPDATE
		SET recipient_email = EXCLUDED.recipient_email,
		    status = CASE
		        WHEN email_notifications.status = 'SENT' THEN email_notifications.status
		        ELSE 'PENDING'
		    END,
		    next_retry_at = NOW(),
		    error_message = NULL,
		    updated_at = NOW()
	`, sale.ID, sale.CustomerEmail); err != nil {
		spanErr = err
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		spanErr = err
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
	return status == emailStatusSent, nil
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
	ctx, span := observability.StartBusinessSpan(ctx, "ticket.issue", attribute.String("sale.id", sale.ID))
	var spanErr error
	defer func() {
		observability.EndSpan(span, spanErr)
	}()

	rows, err := tx.Query(ctx, `
		SELECT si.ticket_id, t.name, si.quantity
		FROM sale_items si
		JOIN tickets t ON t.id = si.ticket_id
		WHERE si.sale_id = $1
		ORDER BY si.created_at ASC
	`, sale.ID)
	if err != nil {
		spanErr = err
		return nil, err
	}
	defer rows.Close()

	items := make([]completedSaleItem, 0)
	for rows.Next() {
		var item completedSaleItem
		if err := rows.Scan(&item.TicketID, &item.TicketName, &item.Quantity); err != nil {
			spanErr = err
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		spanErr = err
		return nil, err
	}

	issuedTickets := make([]notification.IssuedTicket, 0)
	for _, item := range items {
		for sequence := 1; sequence <= item.Quantity; sequence++ {
			issuedTicketID := deterministicIssuedTicketID(sale.ID, item.TicketID, sequence)
			qrPayload := fmt.Sprintf("issued_ticket:%s", issuedTicketID)
			qrCodePNG, err := qrcode.Encode(qrPayload, qrcode.Medium, 256)
			if err != nil {
				spanErr = err
				return nil, err
			}

			if _, err := tx.Exec(ctx, `
				INSERT INTO issued_tickets (id, sale_id, ticket_id, customer_id, sequence, qr_code_payload)
				VALUES ($1, $2, $3, $4, $5, $6)
				ON CONFLICT (sale_id, ticket_id, sequence) DO NOTHING
			`, issuedTicketID, sale.ID, item.TicketID, sale.CustomerID, sequence, qrPayload); err != nil {
				spanErr = err
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

	span.SetAttributes(attribute.Int("ticket.issued.count", len(issuedTickets)))
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
		SET status = $2,
		    error_message = NULL,
		    sent_at = NOW(),
		    next_retry_at = NOW(),
		    updated_at = NOW()
		WHERE sale_id = $1
	`, saleID, emailStatusSent)
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
	nextAttempts, status, nextRetryAt := emailRetryState(1)

	_, err := p.db.Exec(ctx, `
		UPDATE email_notifications
		SET status = $2,
		    attempts = attempts + 1,
		    error_message = $3,
		    next_retry_at = $4,
		    updated_at = NOW()
		WHERE sale_id = $1
	`, saleID, status, trimEmailError(sendErr.Error()), nextRetryAt)
	if err == nil && status == emailStatusDeadLetter {
		slog.Warn("email notification moved to dead letter", "sale_id", saleID, "attempts", nextAttempts)
	}
	return err
}

func (p *TicketDeliveryProcessor) RetryFailedEmails(ctx context.Context, batchSize int) error {
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	rows, err := tx.Query(ctx, `
		SELECT en.sale_id, en.attempts, s.request_id, s.correlation_id, s.transaction_id
		FROM email_notifications en
		JOIN sales s ON s.id = en.sale_id
		WHERE en.status = $1
		  AND en.next_retry_at <= NOW()
		  AND en.attempts < $2
		ORDER BY en.updated_at ASC
		LIMIT $3
		FOR UPDATE OF en SKIP LOCKED
	`, emailStatusFailed, maxEmailRetryAttempts, batchSize)
	if err != nil {
		return err
	}

	pending := make([]pendingEmailRetry, 0, batchSize)
	for rows.Next() {
		var retry pendingEmailRetry
		if err := rows.Scan(
			&retry.SaleID,
			&retry.Attempts,
			&retry.Metadata.RequestID,
			&retry.Metadata.CorrelationID,
			&retry.Metadata.TransactionID,
		); err != nil {
			rows.Close()
			return err
		}
		pending = append(pending, retry)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	for _, retry := range pending {
		retryCtx := ctx
		if retry.Metadata != (correlation.Metadata{}) {
			retryCtx = correlation.ContextWithMetadata(retryCtx, retry.Metadata)
		}
		retryCtx, retrySpan := observability.StartBusinessSpan(retryCtx, "email.retry",
			attribute.String("sale.id", retry.SaleID),
			attribute.Int("email.retry.attempts", retry.Attempts+1),
		)
		var retryErr error

		email, err := p.loadRetryTicketEmail(retryCtx, tx, retry.SaleID)
		if err != nil {
			retryErr = err
			if markErr := p.markRetryFailed(retryCtx, tx, retry, err.Error()); markErr != nil {
				observability.EndSpan(retrySpan, markErr)
				return markErr
			}
			metrics.TicketDeliveryTotal.WithLabelValues("retry_failed").Inc()
			observability.EndSpan(retrySpan, retryErr)
			continue
		}

		emailCtx, emailSpan := observability.StartBusinessSpan(retryCtx, "email.send.retry",
			attribute.String("sale.id", retry.SaleID),
			attribute.Int("ticket.issued.count", len(email.Tickets)),
		)
		err = p.sender.SendTickets(emailCtx, *email)
		observability.EndSpan(emailSpan, err)
		if err != nil {
			retryErr = err
			slog.ErrorContext(retryCtx, "retry ticket email failed", "sale_id", retry.SaleID, "recipient", email.To, "attempts", retry.Attempts+1, "error", err)
			if markErr := p.markRetryFailed(retryCtx, tx, retry, err.Error()); markErr != nil {
				observability.EndSpan(retrySpan, markErr)
				return markErr
			}
			metrics.TicketDeliveryTotal.WithLabelValues("retry_failed").Inc()
			observability.EndSpan(retrySpan, retryErr)
			continue
		}

		if err := markTicketEmailSentTx(retryCtx, tx, retry.SaleID); err != nil {
			observability.EndSpan(retrySpan, err)
			return err
		}
		metrics.TicketDeliveryTotal.WithLabelValues("retry_sent").Inc()
		slog.InfoContext(retryCtx, "retry ticket email delivered", "sale_id", retry.SaleID, "recipient", email.To)
		observability.EndSpan(retrySpan, nil)
	}

	return tx.Commit(ctx)
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

func (p *TicketDeliveryProcessor) loadRetryTicketEmail(ctx context.Context, tx pgx.Tx, saleID string) (*notification.TicketEmail, error) {
	var email retryTicketEmail
	if err := tx.QueryRow(ctx, `
		SELECT c.email, c.name, se.name, se.starts_at
		FROM sales s
		JOIN sales_events se ON se.id = s.sales_event_id
		JOIN customers c ON c.id = s.customer_id
		WHERE s.id = $1
		  AND s.status = $2
	`, saleID, events.SaleCompletedStatus).Scan(
		&email.To,
		&email.CustomerName,
		&email.SalesEventName,
		&email.StartsAt,
	); err != nil {
		return nil, err
	}

	rows, err := tx.Query(ctx, `
		SELECT it.id, t.name, it.qr_code_payload
		FROM issued_tickets it
		JOIN tickets t ON t.id = it.ticket_id
		WHERE it.sale_id = $1
		ORDER BY it.sequence ASC
	`, saleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	email.Tickets = make([]notification.IssuedTicket, 0)
	for rows.Next() {
		var ticket notification.IssuedTicket
		if err := rows.Scan(&ticket.ID, &ticket.TicketName, &ticket.QRPayload); err != nil {
			return nil, err
		}
		qrCodePNG, err := qrcode.Encode(ticket.QRPayload, qrcode.Medium, 256)
		if err != nil {
			return nil, err
		}
		ticket.QRCodePNG = qrCodePNG
		email.Tickets = append(email.Tickets, ticket)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(email.Tickets) == 0 {
		return nil, errors.New("sale has no issued tickets available for email retry")
	}

	return &notification.TicketEmail{
		To:             email.To,
		CustomerName:   email.CustomerName,
		SalesEventName: email.SalesEventName,
		StartsAt:       email.StartsAt,
		Tickets:        email.Tickets,
	}, nil
}

func (p *TicketDeliveryProcessor) markRetryFailed(ctx context.Context, tx pgx.Tx, retry pendingEmailRetry, reason string) error {
	nextAttempts, status, nextRetryAt := emailRetryState(retry.Attempts + 1)

	_, err := tx.Exec(ctx, `
		UPDATE email_notifications
		SET status = $2,
		    attempts = attempts + 1,
		    error_message = $3,
		    next_retry_at = $4,
		    updated_at = NOW()
		WHERE sale_id = $1
	`, retry.SaleID, status, trimEmailError(reason), nextRetryAt)
	if err == nil && status == emailStatusDeadLetter {
		slog.Warn("email notification moved to dead letter", "sale_id", retry.SaleID, "attempts", nextAttempts)
	}
	return err
}

func markTicketEmailSentTx(ctx context.Context, tx pgx.Tx, saleID string) error {
	if _, err := tx.Exec(ctx, `
		UPDATE email_notifications
		SET status = $2,
		    error_message = NULL,
		    sent_at = NOW(),
		    next_retry_at = NOW(),
		    updated_at = NOW()
		WHERE sale_id = $1
	`, saleID, emailStatusSent); err != nil {
		return err
	}

	_, err := tx.Exec(ctx, `
		UPDATE issued_tickets
		SET emailed_at = NOW()
		WHERE sale_id = $1
	`, saleID)
	return err
}

func emailRetryState(attempts int) (int, string, time.Time) {
	status := emailStatusFailed
	if attempts >= maxEmailRetryAttempts {
		status = emailStatusDeadLetter
	}
	return attempts, status, time.Now().UTC().Add(emailRetryBackoff(attempts))
}

func emailRetryBackoff(attempts int) time.Duration {
	if attempts <= 0 {
		return time.Minute
	}

	backoff := time.Minute
	for i := 1; i < attempts; i++ {
		backoff *= 2
		if backoff >= time.Hour {
			return time.Hour
		}
	}
	return backoff
}

func trimEmailError(value string) string {
	const maxLength = 2000
	if len(value) <= maxLength {
		return value
	}
	return value[:maxLength]
}

func deterministicIssuedTicketID(saleID string, ticketID string, sequence int) string {
	name := fmt.Sprintf("issued-ticket:%s:%s:%d", saleID, ticketID, sequence)
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte(name)).String()
}
