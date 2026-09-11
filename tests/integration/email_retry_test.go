//go:build integration

package integration_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/varner/sales-event-project/internal/notification"
	"github.com/varner/sales-event-project/internal/worker"
)

func TestEmailRetryMovesPersistentFailureToDeadLetter(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	db := connectDB(t, ctx)
	defer db.Close()

	saleID := uuid.NewString()
	customerID := "rc-" + uuid.NewString()
	issuedTicketID := uuid.NewString()
	seedFailedEmailRetry(t, ctx, db, saleID, customerID, issuedTicketID)

	processor := worker.NewTicketDeliveryProcessor(db, failingTicketSender{
		err: errors.New("smtp unavailable for retry smoke"),
	})
	if err := processor.RetryFailedEmails(ctx, 10); err != nil {
		t.Fatalf("retry failed email: %v", err)
	}

	status, attempts, message := emailRetryStateForSale(t, ctx, db, saleID)
	if status != "DEAD_LETTER" {
		t.Fatalf("expected DEAD_LETTER status, got %q", status)
	}
	if attempts != 5 {
		t.Fatalf("expected 5 attempts after final retry, got %d", attempts)
	}
	if !strings.Contains(message, "smtp unavailable") {
		t.Fatalf("expected stored retry error message, got %q", message)
	}
}

type failingTicketSender struct {
	err error
}

func (s failingTicketSender) SendTickets(context.Context, notification.TicketEmail) error {
	return s.err
}

func seedFailedEmailRetry(t *testing.T, ctx context.Context, db *pgxpool.Pool, saleID string, customerID string, issuedTicketID string) {
	t.Helper()

	_, err := db.Exec(ctx, `
		INSERT INTO customers (id, email, name)
		VALUES ($1, $2, 'Retry Customer')
	`, customerID, customerID+"@example.com")
	if err != nil {
		t.Fatalf("insert retry customer: %v", err)
	}

	_, err = db.Exec(ctx, `
		INSERT INTO sales (id, sales_event_id, customer_id, status, total_amount)
		VALUES ($1, $2, $3, 'COMPLETED', $4)
	`, saleID, salesEventID, customerID, generalTicketPrice)
	if err != nil {
		t.Fatalf("insert retry sale: %v", err)
	}

	_, err = db.Exec(ctx, `
		INSERT INTO issued_tickets (id, sale_id, ticket_id, customer_id, sequence, qr_code_payload)
		VALUES ($1, $2, $3, $4, 1, $5)
	`, issuedTicketID, saleID, generalTicketID, customerID, "issued_ticket:"+issuedTicketID)
	if err != nil {
		t.Fatalf("insert retry issued ticket: %v", err)
	}

	_, err = db.Exec(ctx, `
		INSERT INTO email_notifications (sale_id, recipient_email, status, attempts, next_retry_at)
		VALUES ($1, $2, 'FAILED', 4, NOW() - INTERVAL '1 minute')
	`, saleID, customerID+"@example.com")
	if err != nil {
		t.Fatalf("insert failed email notification: %v", err)
	}
}

func emailRetryStateForSale(t *testing.T, ctx context.Context, db *pgxpool.Pool, saleID string) (string, int, string) {
	t.Helper()

	var status string
	var attempts int
	var message string
	if err := db.QueryRow(ctx, `
		SELECT status, attempts, COALESCE(error_message, '')
		FROM email_notifications
		WHERE sale_id = $1
	`, saleID).Scan(&status, &attempts, &message); err != nil {
		t.Fatalf("query email retry state: %v", err)
	}
	return status, attempts, message
}
