package analyticsdb

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/varner/sales-event-project/internal/analytics"
)

// ReadEvents reads the filtered operational history in chronological order.
func ReadEvents(ctx context.Context, db Queryer, filter Filter) ([]analytics.Event, error) {
	if filter.Limit <= 0 {
		filter.Limit = DefaultLimit
	}
	rows, err := db.Query(ctx, eventsQuery, filter.SalesEventID, filter.Start, filter.End, filter.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	events := make([]analytics.Event, 0)
	for rows.Next() {
		event, err := scanEvent(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return events, nil
}

func scanEvent(row pgx.Row) (analytics.Event, error) {
	var (
		event           analytics.Event
		occurredAt      time.Time
		eventID         pgtype.Text
		ticketID        pgtype.Text
		ticketType      pgtype.Text
		status          pgtype.Text
		provider        pgtype.Text
		outboxEventType pgtype.Text
		requestID       pgtype.Text
		correlationID   pgtype.Text
		transactionID   pgtype.Text
	)
	if err := row.Scan(
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
		return analytics.Event{}, err
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
	return event, nil
}

func textValue(value pgtype.Text) string {
	if !value.Valid {
		return ""
	}
	return value.String
}
