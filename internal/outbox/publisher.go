package outbox

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/varner/sales-event-project/internal/events"
	"github.com/varner/sales-event-project/internal/metrics"
)

const (
	StatusPending    = "PENDING"
	StatusFailed     = "FAILED"
	StatusPublished  = "PUBLISHED"
	StatusDeadLetter = "DEAD_LETTER"

	maxAttempts = 5
)

type EventPublisher interface {
	PublishJSON(ctx context.Context, routingKey string, value any) error
}

type Publisher struct {
	db     *pgxpool.Pool
	broker EventPublisher
}

func NewPublisher(db *pgxpool.Pool, broker EventPublisher) *Publisher {
	return &Publisher{db: db, broker: broker}
}

func (p *Publisher) Run(ctx context.Context, interval time.Duration, batchSize int) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		if err := p.PublishPending(ctx, batchSize); err != nil {
			slog.Error("publish outbox batch failed", "error", err)
		}

		select {
		case <-ctx.Done():
			slog.Info("outbox publisher stopped")
			return
		case <-ticker.C:
		}
	}
}

func (p *Publisher) PublishPending(ctx context.Context, batchSize int) error {
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	rows, err := tx.Query(ctx, `
		SELECT event_id, event_type, payload, attempts
		FROM outbox_events
		WHERE status IN ('PENDING', 'FAILED')
		  AND published_at IS NULL
		  AND next_attempt_at <= NOW()
		  AND attempts < $2
		ORDER BY created_at ASC
		LIMIT $1
		FOR UPDATE SKIP LOCKED
	`, batchSize, maxAttempts)
	if err != nil {
		return err
	}

	pending := make([]pendingEvent, 0, batchSize)
	for rows.Next() {
		var event pendingEvent
		if err := rows.Scan(&event.ID, &event.Type, &event.Payload, &event.Attempts); err != nil {
			rows.Close()
			return err
		}
		pending = append(pending, event)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	for _, event := range pending {
		routingKey, ok := routingKeyForEventType(event.Type)
		if !ok {
			reason := fmt.Sprintf("outbox event type %q has no routing key", event.Type)
			slog.Warn("outbox event type has no routing key", "event_id", event.ID, "event_type", event.Type)
			if err := markFailed(ctx, tx, event, reason); err != nil {
				return err
			}
			continue
		}

		if err := p.broker.PublishJSON(ctx, routingKey, event.Payload); err != nil {
			slog.Error("outbox event publish failed", "event_id", event.ID, "event_type", event.Type, "attempts", event.Attempts+1, "error", err)
			if markErr := markFailed(ctx, tx, event, err.Error()); markErr != nil {
				return markErr
			}
			continue
		}
		if err := markPublished(ctx, tx, event.ID); err != nil {
			return err
		}
		metrics.OutboxEventsProcessedTotal.WithLabelValues(event.Type, StatusPublished).Inc()
		slog.Info("outbox event published", "event_id", event.ID, "event_type", event.Type, "routing_key", routingKey)
	}

	return tx.Commit(ctx)
}

type pendingEvent struct {
	ID       string
	Type     string
	Payload  json.RawMessage
	Attempts int
}

func routingKeyForEventType(eventType string) (string, bool) {
	switch eventType {
	case "SALE_COMPLETED":
		return events.SaleCompletedRoutingKey, true
	case "SALE_FAILED":
		return events.SaleFailedRoutingKey, true
	default:
		return "", false
	}
}

func markPublished(ctx context.Context, tx pgx.Tx, eventID string) error {
	_, err := tx.Exec(ctx, `
		UPDATE outbox_events
		SET status = $2,
		    published_at = NOW(),
		    last_error = NULL,
		    next_attempt_at = NOW()
		WHERE event_id = $1
	`, eventID, StatusPublished)
	return err
}

func markFailed(ctx context.Context, tx pgx.Tx, event pendingEvent, reason string) error {
	nextAttempts := event.Attempts + 1
	status := StatusFailed
	if nextAttempts >= maxAttempts {
		status = StatusDeadLetter
	}

	_, err := tx.Exec(ctx, `
		UPDATE outbox_events
		SET status = $2,
		    attempts = attempts + 1,
		    last_error = $3,
		    next_attempt_at = $4
		WHERE event_id = $1
	`, event.ID, status, trimError(reason), time.Now().UTC().Add(nextBackoff(nextAttempts)))
	if err == nil {
		metrics.OutboxEventsProcessedTotal.WithLabelValues(event.Type, status).Inc()
	}
	return err
}

func nextBackoff(attempts int) time.Duration {
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

func trimError(value string) string {
	const maxLength = 1000
	if len(value) <= maxLength {
		return value
	}
	return value[:maxLength]
}
