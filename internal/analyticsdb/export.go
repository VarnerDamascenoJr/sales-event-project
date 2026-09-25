package analyticsdb

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/varner/sales-event-project/internal/analytics"
)

const (
	DefaultLimit = 2000
	MaxLimit     = 10000
)

type Queryer interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

type Filter struct {
	SalesEventID string
	Start        string
	End          string
	Limit        int
}

type Exporter struct {
	DB Queryer
}

func NewExporter(db Queryer) Exporter {
	return Exporter{DB: db}
}

func (e Exporter) ExportAnalytics(ctx context.Context, filter Filter) (analytics.Document, error) {
	events, err := ReadEvents(ctx, e.DB, filter)
	if err != nil {
		return analytics.Document{}, err
	}

	return analytics.BuildDocument(time.Now(), SourceForFilter(filter), events, analytics.DefaultWindowSpecs()), nil
}

func ReadEvents(ctx context.Context, db Queryer, filter Filter) ([]analytics.Event, error) {
	if filter.Limit <= 0 {
		filter.Limit = DefaultLimit
	}

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
`, filter.SalesEventID, filter.Start, filter.End, filter.Limit)
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

func ValidateTimestampBound(value string, name string) error {
	if value == "" {
		return nil
	}
	if _, err := time.Parse(time.RFC3339, value); err != nil {
		return fmt.Errorf("%s must be RFC3339: %w", name, err)
	}
	return nil
}

func SourceForFilter(filter Filter) analytics.Source {
	source := analytics.Source{}
	filters := make([]string, 0, 2)
	if filter.SalesEventID != "" {
		source.SalesEventID = filter.SalesEventID
		filters = append(filters, "sales_event_id")
	}
	if filter.Start != "" || filter.End != "" {
		filters = append(filters, "time_range")
	}
	source.Filter = strings.Join(filters, ",")
	return source
}

func textValue(value pgtype.Text) string {
	if !value.Valid {
		return ""
	}
	return value.String
}
