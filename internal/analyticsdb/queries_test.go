package analyticsdb

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

type queryCall struct {
	ctx  context.Context
	sql  string
	args []any
}

type queryResult struct {
	rows pgx.Rows
	err  error
}

type fakeQueryer struct {
	results []queryResult
	calls   []queryCall
}

func (db *fakeQueryer) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	db.calls = append(db.calls, queryCall{ctx: ctx, sql: sql, args: append([]any(nil), args...)})
	if len(db.results) == 0 {
		return nil, errors.New("unexpected query")
	}
	result := db.results[0]
	db.results = db.results[1:]
	return result.rows, result.err
}

type fakeRows struct {
	pgx.Rows
	values   [][]any
	position int
	closed   bool
	scanErr  error
	rowsErr  error
}

func (rows *fakeRows) Next() bool {
	if rows.position >= len(rows.values) {
		return false
	}
	rows.position++
	return true
}

func (rows *fakeRows) Scan(dest ...any) error {
	if rows.scanErr != nil {
		return rows.scanErr
	}
	values := rows.values[rows.position-1]
	if len(dest) != len(values) {
		return fmt.Errorf("expected %d scan destinations, got %d", len(values), len(dest))
	}
	for i, value := range values {
		if scanner, ok := dest[i].(interface{ Scan(any) error }); ok {
			if err := scanner.Scan(value); err != nil {
				return err
			}
			continue
		}
		target := reflect.ValueOf(dest[i]).Elem()
		source := reflect.ValueOf(value)
		if !source.IsValid() || !source.Type().AssignableTo(target.Type()) {
			return fmt.Errorf("cannot scan column %d (%T) into %T", i, value, dest[i])
		}
		target.Set(source)
	}
	return nil
}

func (rows *fakeRows) Close()     { rows.closed = true }
func (rows *fakeRows) Err() error { return rows.rowsErr }

func eventValues(occurredAt time.Time) []any {
	return []any{
		"event-123", "payment.processed", occurredAt, "sales-event-123", "sale-123",
		"ticket-123", "VIP", "APPROVED", "provider-123", "SALE_COMPLETED",
		10000, 2, 3, "req-123", "corr-123", "tx-123",
	}
}

func TestReadersPropagateErrorsAndCloseRows(t *testing.T) {
	failure := errors.New("database failure")
	for _, reader := range []struct {
		name   string
		values []any
		read   func(Queryer) error
	}{
		{
			name: "events", values: eventValues(time.Now()),
			read: func(db Queryer) error {
				_, err := ReadEvents(context.Background(), db, Filter{})
				return err
			},
		},
		{
			name: "inventory", values: []any{"sales-event-123", "ticket-123", "VIP", 10},
			read: func(db Queryer) error {
				_, err := ReadTicketInventory(context.Background(), db, Filter{})
				return err
			},
		},
	} {
		for _, stage := range []string{"query", "scan", "iteration"} {
			t.Run(reader.name+"/"+stage, func(t *testing.T) {
				rows := &fakeRows{values: [][]any{reader.values}}
				result := queryResult{rows: rows}
				switch stage {
				case "query":
					result = queryResult{err: failure}
				case "scan":
					rows.scanErr = failure
				case "iteration":
					rows.rowsErr = failure
				}
				db := &fakeQueryer{results: []queryResult{result}}
				if err := reader.read(db); !errors.Is(err, failure) {
					t.Fatalf("expected database failure, got %v", err)
				}
				if len(db.calls) != 1 {
					t.Fatalf("expected one query, got %d", len(db.calls))
				}
				if stage != "query" && !rows.closed {
					t.Fatal("query rows were not closed after failure")
				}
			})
		}
	}
}
