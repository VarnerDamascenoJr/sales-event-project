package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/varner/sales-event-project/internal/analytics"
	"github.com/varner/sales-event-project/internal/config"
	"github.com/varner/sales-event-project/internal/database"
)

func main() {
	var salesEventID string
	var outputPath string
	var start string
	var end string
	var pretty bool
	var limit int

	flag.StringVar(&salesEventID, "sales-event-id", "", "optional sales event UUID filter")
	flag.StringVar(&outputPath, "output", "", "optional output path; stdout is used when empty")
	flag.StringVar(&start, "start", "", "optional inclusive RFC3339 lower timestamp bound")
	flag.StringVar(&end, "end", "", "optional exclusive RFC3339 upper timestamp bound")
	flag.BoolVar(&pretty, "pretty", true, "render indented JSON")
	flag.IntVar(&limit, "limit", 2000, "maximum number of analytic events to export")
	flag.Parse()

	if limit <= 0 {
		slog.Error("limit must be greater than zero")
		os.Exit(2)
	}
	if err := validateTimestampBound(start, "start"); err != nil {
		slog.Error("invalid timestamp bound", "error", err)
		os.Exit(2)
	}
	if err := validateTimestampBound(end, "end"); err != nil {
		slog.Error("invalid timestamp bound", "error", err)
		os.Exit(2)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	cfg := config.Load()
	db, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("connect postgres failed", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	events, err := readAnalyticsEvents(ctx, db, salesEventID, start, end, limit)
	if err != nil {
		slog.Error("export analytics failed", "error", err)
		os.Exit(1)
	}

	source := analytics.Source{}
	if salesEventID != "" {
		source.SalesEventID = salesEventID
		source.Filter = "sales_event_id"
	}
	document := analytics.BuildDocument(time.Now(), source, events, analytics.DefaultWindowSpecs())

	payload, err := marshalJSON(document, pretty)
	if err != nil {
		slog.Error("marshal analytics export failed", "error", err)
		os.Exit(1)
	}

	if outputPath == "" {
		fmt.Println(string(payload))
		return
	}

	if err := os.WriteFile(outputPath, append(payload, '\n'), 0o600); err != nil {
		slog.Error("write analytics export failed", "path", outputPath, "error", err)
		os.Exit(1)
	}
}

type queryer interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func readAnalyticsEvents(ctx context.Context, db queryer, salesEventID string, start string, end string, limit int) ([]analytics.Event, error) {
	rows, err := db.Query(ctx, `
WITH analytics_events AS (
	SELECT
		''::text AS event_id,
		'sale.created'::text AS event_type,
		s.created_at AS occurred_at,
		COALESCE(s.sales_event_id::text, '') AS sales_event_id,
		s.id::text AS sale_id,
		''::text AS ticket_id,
		''::text AS ticket_type,
		s.status AS status,
		''::text AS provider,
		''::text AS outbox_event_type,
		s.total_amount::int AS amount_cents,
		0::int AS quantity,
		0::int AS attempts,
		s.request_id AS request_id,
		s.correlation_id AS correlation_id,
		s.transaction_id AS transaction_id
	FROM sales s
	UNION ALL
	SELECT
		''::text,
		'sale.item.created'::text,
		si.created_at,
		COALESCE(s.sales_event_id::text, ''),
		s.id::text,
		si.ticket_id::text,
		t.name,
		s.status,
		''::text,
		''::text,
		(si.quantity * si.unit_price)::int,
		si.quantity::int,
		0::int,
		s.request_id,
		s.correlation_id,
		s.transaction_id
	FROM sale_items si
	JOIN sales s ON s.id = si.sale_id
	JOIN tickets t ON t.id = si.ticket_id
	UNION ALL
	SELECT
		''::text,
		'payment.processed'::text,
		p.processed_at,
		COALESCE(s.sales_event_id::text, ''),
		s.id::text,
		''::text,
		''::text,
		p.status,
		p.provider,
		''::text,
		p.amount::int,
		0::int,
		0::int,
		s.request_id,
		s.correlation_id,
		s.transaction_id
	FROM payments p
	JOIN sales s ON s.id = p.sale_id
	UNION ALL
	SELECT
		oe.event_id::text,
		'outbox.created'::text,
		oe.created_at,
		COALESCE(s.sales_event_id::text, ''),
		COALESCE(s.id::text, oe.aggregate_id::text),
		''::text,
		''::text,
		oe.status,
		''::text,
		oe.event_type,
		0::int,
		0::int,
		oe.attempts::int,
		oe.request_id,
		oe.correlation_id,
		oe.transaction_id
	FROM outbox_events oe
	LEFT JOIN sales s ON s.id = oe.aggregate_id
	UNION ALL
	SELECT
		oe.event_id::text,
		'outbox.published'::text,
		oe.published_at,
		COALESCE(s.sales_event_id::text, ''),
		COALESCE(s.id::text, oe.aggregate_id::text),
		''::text,
		''::text,
		oe.status,
		''::text,
		oe.event_type,
		0::int,
		0::int,
		oe.attempts::int,
		oe.request_id,
		oe.correlation_id,
		oe.transaction_id
	FROM outbox_events oe
	LEFT JOIN sales s ON s.id = oe.aggregate_id
	WHERE oe.published_at IS NOT NULL
	UNION ALL
	SELECT
		en.id::text,
		'email.status'::text,
		en.created_at,
		COALESCE(s.sales_event_id::text, ''),
		s.id::text,
		''::text,
		''::text,
		en.status,
		''::text,
		''::text,
		0::int,
		0::int,
		en.attempts::int,
		s.request_id,
		s.correlation_id,
		s.transaction_id
	FROM email_notifications en
	JOIN sales s ON s.id = en.sale_id
	UNION ALL
	SELECT en.id::text, 'email.sent'::text, en.sent_at, COALESCE(s.sales_event_id::text, ''), s.id::text, ''::text, ''::text, 'SENT'::text, ''::text, ''::text, 0::int, 0::int, en.attempts::int, s.request_id, s.correlation_id, s.transaction_id
	FROM email_notifications en JOIN sales s ON s.id = en.sale_id WHERE en.sent_at IS NOT NULL
	UNION ALL
	SELECT en.id::text, 'email.delivered'::text, en.delivered_at, COALESCE(s.sales_event_id::text, ''), s.id::text, ''::text, ''::text, 'DELIVERED'::text, ''::text, ''::text, 0::int, 0::int, en.attempts::int, s.request_id, s.correlation_id, s.transaction_id
	FROM email_notifications en JOIN sales s ON s.id = en.sale_id WHERE en.delivered_at IS NOT NULL
	UNION ALL
	SELECT en.id::text, 'email.opened'::text, en.opened_at, COALESCE(s.sales_event_id::text, ''), s.id::text, ''::text, ''::text, 'OPENED'::text, ''::text, ''::text, 0::int, 0::int, en.attempts::int, s.request_id, s.correlation_id, s.transaction_id
	FROM email_notifications en JOIN sales s ON s.id = en.sale_id WHERE en.opened_at IS NOT NULL
	UNION ALL
	SELECT en.id::text, 'email.clicked'::text, en.clicked_at, COALESCE(s.sales_event_id::text, ''), s.id::text, ''::text, ''::text, 'CLICKED'::text, ''::text, ''::text, 0::int, 0::int, en.attempts::int, s.request_id, s.correlation_id, s.transaction_id
	FROM email_notifications en JOIN sales s ON s.id = en.sale_id WHERE en.clicked_at IS NOT NULL
	UNION ALL
	SELECT en.id::text, 'email.bounced'::text, en.bounced_at, COALESCE(s.sales_event_id::text, ''), s.id::text, ''::text, ''::text, 'BOUNCED'::text, ''::text, ''::text, 0::int, 0::int, en.attempts::int, s.request_id, s.correlation_id, s.transaction_id
	FROM email_notifications en JOIN sales s ON s.id = en.sale_id WHERE en.bounced_at IS NOT NULL
	UNION ALL
	SELECT
		it.id::text,
		'ticket.issued'::text,
		it.created_at,
		COALESCE(s.sales_event_id::text, ''),
		it.sale_id::text,
		it.ticket_id::text,
		t.name,
		'ISSUED'::text,
		''::text,
		''::text,
		0::int,
		1::int,
		0::int,
		s.request_id,
		s.correlation_id,
		s.transaction_id
	FROM issued_tickets it
	JOIN tickets t ON t.id = it.ticket_id
	JOIN sales s ON s.id = it.sale_id
	UNION ALL
	SELECT
		tci.id::text,
		'checkin.completed'::text,
		tci.checked_in_at,
		tci.sales_event_id::text,
		it.sale_id::text,
		it.ticket_id::text,
		t.name,
		'COMPLETED'::text,
		''::text,
		''::text,
		0::int,
		1::int,
		0::int,
		s.request_id,
		s.correlation_id,
		s.transaction_id
	FROM ticket_check_ins tci
	JOIN issued_tickets it ON it.id = tci.issued_ticket_id
	JOIN tickets t ON t.id = it.ticket_id
	JOIN sales s ON s.id = it.sale_id
)
SELECT
	event_id,
	event_type,
	occurred_at,
	sales_event_id,
	sale_id,
	ticket_id,
	ticket_type,
	status,
	provider,
	outbox_event_type,
	amount_cents,
	quantity,
	attempts,
	request_id,
	correlation_id,
	transaction_id
FROM analytics_events
WHERE ($1 = '' OR sales_event_id = $1)
  AND ($2 = '' OR occurred_at >= $2::timestamptz)
  AND ($3 = '' OR occurred_at < $3::timestamptz)
ORDER BY occurred_at ASC, event_type ASC, sale_id ASC, event_id ASC
LIMIT $4
`, salesEventID, start, end, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	events := make([]analytics.Event, 0)
	for rows.Next() {
		var event analytics.Event
		var occurredAt time.Time
		var eventID pgtype.Text
		var ticketID pgtype.Text
		var ticketType pgtype.Text
		var status pgtype.Text
		var provider pgtype.Text
		var outboxEventType pgtype.Text
		var requestID pgtype.Text
		var correlationID pgtype.Text
		var transactionID pgtype.Text

		if err := rows.Scan(
			&eventID,
			&event.EventType,
			&occurredAt,
			&event.SalesEventID,
			&event.SaleID,
			&ticketID,
			&ticketType,
			&status,
			&provider,
			&outboxEventType,
			&event.AmountCents,
			&event.Quantity,
			&event.Attempts,
			&requestID,
			&correlationID,
			&transactionID,
		); err != nil {
			return nil, err
		}
		event.OccurredAt = occurredAt.UTC()
		event.EventID = textValue(eventID)
		event.TicketID = textValue(ticketID)
		event.TicketType = textValue(ticketType)
		event.Status = textValue(status)
		event.Provider = textValue(provider)
		event.OutboxEventType = textValue(outboxEventType)
		event.RequestID = textValue(requestID)
		event.CorrelationID = textValue(correlationID)
		event.TransactionID = textValue(transactionID)
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return events, nil
}

func validateTimestampBound(value string, name string) error {
	if value == "" {
		return nil
	}
	if _, err := time.Parse(time.RFC3339, value); err != nil {
		return fmt.Errorf("%s must be RFC3339: %w", name, err)
	}
	return nil
}

func marshalJSON(value any, pretty bool) ([]byte, error) {
	if pretty {
		return json.MarshalIndent(value, "", "  ")
	}
	return json.Marshal(value)
}

func textValue(value pgtype.Text) string {
	if !value.Valid {
		return ""
	}
	return value.String
}
