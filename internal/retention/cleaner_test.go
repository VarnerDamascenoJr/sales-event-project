package retention

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestCleanerDeletesExpiredTemporaryRows(t *testing.T) {
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	db := &fakeDB{
		tags: []pgconn.CommandTag{
			pgconn.NewCommandTag("DELETE 3"),
			pgconn.NewCommandTag("DELETE 5"),
		},
	}
	cleaner := NewCleaner(db)

	result, err := cleaner.Cleanup(context.Background(), now, Policy{
		PublishedOutboxMaxAge: 30 * 24 * time.Hour,
		SentEmailMaxAge:       90 * 24 * time.Hour,
	})
	if err != nil {
		t.Fatalf("cleanup failed: %v", err)
	}

	if result.OutboxEventsDeleted != 3 || result.EmailNotificationsDeleted != 5 {
		t.Fatalf("unexpected cleanup result: %+v", result)
	}
	if len(db.calls) != 2 {
		t.Fatalf("expected two delete calls, got %d", len(db.calls))
	}
	assertCall(t, db.calls[0], "outbox_events", now.Add(-30*24*time.Hour))
	assertCall(t, db.calls[1], "email_notifications", now.Add(-90*24*time.Hour))
}

func TestCleanerSkipsDisabledRetentionRules(t *testing.T) {
	db := &fakeDB{}
	cleaner := NewCleaner(db)

	result, err := cleaner.Cleanup(context.Background(), time.Now().UTC(), Policy{})
	if err != nil {
		t.Fatalf("cleanup failed: %v", err)
	}

	if result != (CleanupResult{}) {
		t.Fatalf("expected empty cleanup result, got %+v", result)
	}
	if len(db.calls) != 0 {
		t.Fatalf("expected no delete calls, got %d", len(db.calls))
	}
}

func assertCall(t *testing.T, call execCall, table string, cutoff time.Time) {
	t.Helper()

	if !strings.Contains(call.sql, table) {
		t.Fatalf("expected SQL to target %s, got %s", table, call.sql)
	}
	if len(call.arguments) != 1 {
		t.Fatalf("expected one argument, got %d", len(call.arguments))
	}
	got, ok := call.arguments[0].(time.Time)
	if !ok {
		t.Fatalf("expected cutoff time argument, got %T", call.arguments[0])
	}
	if !got.Equal(cutoff) {
		t.Fatalf("expected cutoff %s, got %s", cutoff, got)
	}
}

type fakeDB struct {
	calls []execCall
	tags  []pgconn.CommandTag
}

type execCall struct {
	sql       string
	arguments []any
}

func (db *fakeDB) Exec(_ context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
	db.calls = append(db.calls, execCall{sql: sql, arguments: arguments})
	if len(db.tags) == 0 {
		return pgconn.NewCommandTag("DELETE 0"), nil
	}

	tag := db.tags[0]
	db.tags = db.tags[1:]
	return tag, nil
}
