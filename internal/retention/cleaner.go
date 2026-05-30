package retention

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/varner/sales-event-project/internal/metrics"
)

type DB interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}

type Policy struct {
	PublishedOutboxMaxAge time.Duration
	SentEmailMaxAge       time.Duration
}

type CleanupResult struct {
	OutboxEventsDeleted       int64
	EmailNotificationsDeleted int64
}

type Cleaner struct {
	db DB
}

func NewCleaner(db DB) *Cleaner {
	return &Cleaner{db: db}
}

func (c *Cleaner) Run(ctx context.Context, interval time.Duration, policy Policy) {
	if interval <= 0 {
		slog.Warn("retention cleaner disabled because interval is not positive", "interval", interval)
		return
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		result, err := c.Cleanup(ctx, time.Now().UTC(), policy)
		if err != nil {
			slog.Error("retention cleanup failed", "error", err)
		} else {
			slog.Info("retention cleanup completed",
				"outbox_events_deleted", result.OutboxEventsDeleted,
				"email_notifications_deleted", result.EmailNotificationsDeleted,
			)
		}

		select {
		case <-ctx.Done():
			slog.Info("retention cleaner stopped")
			return
		case <-ticker.C:
		}
	}
}

func (c *Cleaner) Cleanup(ctx context.Context, now time.Time, policy Policy) (CleanupResult, error) {
	var result CleanupResult

	if policy.PublishedOutboxMaxAge > 0 {
		tag, err := c.db.Exec(ctx, `
			DELETE FROM outbox_events
			WHERE published_at IS NOT NULL
			  AND published_at < $1
		`, now.Add(-policy.PublishedOutboxMaxAge))
		if err != nil {
			return CleanupResult{}, err
		}
		result.OutboxEventsDeleted = tag.RowsAffected()
		metrics.RetentionDeletedRowsTotal.WithLabelValues("outbox_events").Add(float64(result.OutboxEventsDeleted))
	}

	if policy.SentEmailMaxAge > 0 {
		tag, err := c.db.Exec(ctx, `
			DELETE FROM email_notifications
			WHERE status = 'SENT'
			  AND COALESCE(sent_at, updated_at) < $1
		`, now.Add(-policy.SentEmailMaxAge))
		if err != nil {
			return CleanupResult{}, err
		}
		result.EmailNotificationsDeleted = tag.RowsAffected()
		metrics.RetentionDeletedRowsTotal.WithLabelValues("email_notifications").Add(float64(result.EmailNotificationsDeleted))
	}

	return result, nil
}
