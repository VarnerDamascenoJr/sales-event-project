package outbox

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/varner/sales-event-project/internal/events"
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
		SELECT event_id, event_type, payload
		FROM outbox_events
		WHERE published_at IS NULL
		ORDER BY created_at ASC
		LIMIT $1
		FOR UPDATE SKIP LOCKED
	`, batchSize)
	if err != nil {
		return err
	}

	pending := make([]pendingEvent, 0, batchSize)
	for rows.Next() {
		var event pendingEvent
		if err := rows.Scan(&event.ID, &event.Type, &event.Payload); err != nil {
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
			slog.Warn("outbox event type has no routing key", "event_id", event.ID, "event_type", event.Type)
			if err := markPublished(ctx, tx, event.ID); err != nil {
				return err
			}
			continue
		}

		if err := p.broker.PublishJSON(ctx, routingKey, event.Payload); err != nil {
			return err
		}
		if err := markPublished(ctx, tx, event.ID); err != nil {
			return err
		}
		slog.Info("outbox event published", "event_id", event.ID, "event_type", event.Type, "routing_key", routingKey)
	}

	return tx.Commit(ctx)
}

type pendingEvent struct {
	ID      string
	Type    string
	Payload json.RawMessage
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
		SET published_at = NOW()
		WHERE event_id = $1
	`, eventID)
	return err
}
