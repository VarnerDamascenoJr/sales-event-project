package worker

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rabbitmq/amqp091-go"
	"github.com/varner/sales-event-project/internal/correlation"
	"github.com/varner/sales-event-project/internal/events"
	"github.com/varner/sales-event-project/internal/metrics"
	"github.com/varner/sales-event-project/internal/observability"
)

type SalesProcessor struct {
	db *pgxpool.Pool
}

func NewSalesProcessor(db *pgxpool.Pool) *SalesProcessor {
	return &SalesProcessor{db: db}
}

func (p *SalesProcessor) Handle(ctx context.Context, delivery amqp091.Delivery) error {
	start := time.Now()

	var event events.SaleCreated
	if err := json.Unmarshal(delivery.Body, &event); err != nil {
		metrics.WorkerSalesProcessedTotal.WithLabelValues(events.SaleFailedStatus).Inc()
		return err
	}
	ctx = contextWithEventMetadata(ctx, event.Metadata)

	status := events.SalePendingPaymentStatus
	if len(event.Items) == 0 {
		status = events.SaleFailedStatus
	}

	if err := p.persistSale(ctx, event, status); err != nil {
		metrics.WorkerSalesProcessedTotal.WithLabelValues(events.SaleFailedStatus).Inc()
		return err
	}

	metrics.WorkerSalesProcessedTotal.WithLabelValues(status).Inc()
	metrics.WorkerProcessingDuration.Observe(time.Since(start).Seconds())
	slog.InfoContext(ctx, "sale reservation processed",
		"sale_id", event.SaleID,
		"sales_event_id", event.SalesEventID,
		"customer_id", event.CustomerID,
		"status", status,
		"items", len(event.Items),
		"duration_ms", time.Since(start).Milliseconds(),
	)
	return nil
}

func (p *SalesProcessor) persistSale(ctx context.Context, event events.SaleCreated, status string) error {
	alreadyProcessed, err := p.saleAlreadyProcessed(ctx, event.SaleID)
	if err != nil {
		return err
	}
	if alreadyProcessed {
		return nil
	}

	tx, err := p.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	totalAmount := 0
	for _, item := range event.Items {
		totalAmount += item.Quantity * item.UnitPrice
	}
	metadata := correlation.WithTransactionID(correlation.Metadata{
		RequestID:     event.Metadata.RequestID,
		CorrelationID: event.Metadata.CorrelationID,
		TransactionID: event.Metadata.TransactionID,
	}, event.SaleID)

	_, err = tx.Exec(ctx, `
		INSERT INTO customers (id, email, name, created_at, updated_at)
		VALUES ($1, $2, $3, NOW(), NOW())
		ON CONFLICT (id) DO UPDATE
		SET email = EXCLUDED.email,
		    name = EXCLUDED.name,
		    updated_at = NOW()
	`, event.CustomerID, event.CustomerEmail, event.CustomerName)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO sales (id, sales_event_id, customer_id, status, total_amount, created_at, updated_at, request_id, correlation_id, transaction_id)
		VALUES ($1, $2, $3, $4, $5, $6, NOW(), $7, $8, $9)
		ON CONFLICT (id) DO UPDATE
		SET status = EXCLUDED.status,
		    total_amount = EXCLUDED.total_amount,
		    request_id = COALESCE(NULLIF(sales.request_id, ''), EXCLUDED.request_id),
		    correlation_id = COALESCE(NULLIF(sales.correlation_id, ''), EXCLUDED.correlation_id),
		    transaction_id = COALESCE(NULLIF(sales.transaction_id, ''), EXCLUDED.transaction_id),
		    updated_at = NOW()
	`, event.SaleID, event.SalesEventID, event.CustomerID, status, totalAmount, event.OccurredAt, metadata.RequestID, metadata.CorrelationID, metadata.TransactionID)
	if err != nil {
		return err
	}

	for _, item := range event.Items {
		result, err := tx.Exec(ctx, `
			UPDATE tickets
			SET available_quantity = available_quantity - $1
			WHERE id = $2
			  AND available_quantity >= $1
		`, item.Quantity, item.TicketID)
		if err != nil {
			return err
		}
		if result.RowsAffected() == 0 {
			return errInsufficientTickets
		}

		_, err = tx.Exec(ctx, `
			INSERT INTO sale_items (sale_id, ticket_id, quantity, unit_price)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT DO NOTHING
		`, event.SaleID, item.TicketID, item.Quantity, item.UnitPrice)
		if err != nil {
			return err
		}
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO payments (sale_id, status, amount, provider, processed_at)
		VALUES ($1, $2, $3, $4, NOW())
		ON CONFLICT (sale_id) DO UPDATE
		SET status = EXCLUDED.status,
		    amount = EXCLUDED.amount,
		    processed_at = NOW()
	`, event.SaleID, paymentStatusForSale(status), totalAmount, "pending")
	if err != nil {
		return err
	}

	if status == events.SaleFailedStatus {
		payload, err := json.Marshal(event)
		if err != nil {
			return err
		}

		_, err = tx.Exec(ctx, `
			INSERT INTO outbox_events (event_id, event_type, aggregate_id, payload, trace_context, request_id, correlation_id, transaction_id)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			ON CONFLICT (event_id) DO NOTHING
		`, event.EventID, statusEventName(status), event.SaleID, payload, observability.TraceContext(ctx), metadata.RequestID, metadata.CorrelationID, metadata.TransactionID)
		if err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

func (p *SalesProcessor) saleAlreadyProcessed(ctx context.Context, saleID string) (bool, error) {
	var exists bool
	err := p.db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM sales
			WHERE id = $1
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
	if status == events.SalePendingPaymentStatus {
		return events.PaymentPendingStatus
	}
	return events.PaymentFailedStatus
}
