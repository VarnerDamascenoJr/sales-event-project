package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/varner/sales-event-project/internal/correlation"
	"github.com/varner/sales-event-project/internal/events"
	"github.com/varner/sales-event-project/internal/observability"
	"go.opentelemetry.io/otel/attribute"
)

type PostgresSalesStore struct {
	db *pgxpool.Pool
}

func NewPostgresSalesStore(db *pgxpool.Pool) *PostgresSalesStore {
	return &PostgresSalesStore{db: db}
}

func (s *PostgresSalesStore) SalesEventExists(ctx context.Context, salesEventID string) (bool, error) {
	var exists bool
	err := s.db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM sales_events
			WHERE id = $1
		)
	`, salesEventID).Scan(&exists)
	return exists, err
}

func (s *PostgresSalesStore) GetTicketForEvent(ctx context.Context, ticketID string, salesEventID string) (TicketReadModel, error) {
	var ticket TicketReadModel
	if err := s.db.QueryRow(ctx, `
		SELECT name, price, available_quantity
		FROM tickets
		WHERE id = $1 AND sales_event_id = $2
	`, ticketID, salesEventID).Scan(&ticket.Name, &ticket.Price, &ticket.AvailableQuantity); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return TicketReadModel{}, errTicketNotFound
		}
		return TicketReadModel{}, err
	}

	return ticket, nil
}

func (s *PostgresSalesStore) ListSales(ctx context.Context, filter listSalesFilter) (ListSalesResponse, error) {
	offset := (filter.Page - 1) * filter.PageSize

	var total int
	if err := s.db.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM sales s
		JOIN sales_events se ON se.id = s.sales_event_id
		WHERE s.sales_event_id = $1
		  AND ($2 = '' OR s.status = $2)
		  AND ($3 = '' OR se.name ILIKE '%' || $3 || '%')
	`, filter.SalesEventID, filter.Status, filter.EventName).Scan(&total); err != nil {
		return ListSalesResponse{}, err
	}

	rows, err := s.db.Query(ctx, `
		SELECT s.id, s.sales_event_id, se.name, s.customer_id, c.name, c.email, s.status, s.total_amount, s.created_at, s.updated_at
		FROM sales s
		JOIN sales_events se ON se.id = s.sales_event_id
		JOIN customers c ON c.id = s.customer_id
		WHERE s.sales_event_id = $1
		  AND ($2 = '' OR s.status = $2)
		  AND ($3 = '' OR se.name ILIKE '%' || $3 || '%')
		ORDER BY s.created_at DESC
		LIMIT $4 OFFSET $5
	`, filter.SalesEventID, filter.Status, filter.EventName, filter.PageSize, offset)
	if err != nil {
		return ListSalesResponse{}, err
	}
	defer rows.Close()

	data := make([]SaleListItemDTO, 0)
	for rows.Next() {
		var sale SaleListItemDTO
		if err := rows.Scan(
			&sale.ID,
			&sale.SalesEventID,
			&sale.SalesEventName,
			&sale.CustomerID,
			&sale.CustomerName,
			&sale.CustomerEmail,
			&sale.Status,
			&sale.TotalAmount,
			&sale.CreatedAt,
			&sale.UpdatedAt,
		); err != nil {
			return ListSalesResponse{}, err
		}
		data = append(data, sale)
	}
	if err := rows.Err(); err != nil {
		return ListSalesResponse{}, err
	}

	return ListSalesResponse{
		Page:     filter.Page,
		PageSize: filter.PageSize,
		Total:    total,
		Data:     data,
	}, nil
}

func (s *PostgresSalesStore) GetSale(ctx context.Context, salesEventID string, saleID string) (SaleDetailDTO, error) {
	var sale SaleDetailDTO
	if err := s.db.QueryRow(ctx, `
		SELECT s.id, s.sales_event_id, se.name, s.customer_id, c.name, c.email, s.status, s.total_amount,
		       p.status, p.amount, p.provider, p.processed_at,
		       s.created_at, s.updated_at
		FROM sales s
		JOIN sales_events se ON se.id = s.sales_event_id
		JOIN customers c ON c.id = s.customer_id
		JOIN payments p ON p.sale_id = s.id
		WHERE s.sales_event_id = $1
		  AND s.id = $2
	`, salesEventID, saleID).Scan(
		&sale.ID,
		&sale.SalesEventID,
		&sale.SalesEventName,
		&sale.CustomerID,
		&sale.CustomerName,
		&sale.CustomerEmail,
		&sale.Status,
		&sale.TotalAmount,
		&sale.Payment.Status,
		&sale.Payment.Amount,
		&sale.Payment.Provider,
		&sale.Payment.ProcessedAt,
		&sale.CreatedAt,
		&sale.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return SaleDetailDTO{}, errSaleNotFound
		}
		return SaleDetailDTO{}, err
	}

	rows, err := s.db.Query(ctx, `
		SELECT si.ticket_id, t.name, si.quantity, si.unit_price, si.created_at
		FROM sale_items si
		JOIN tickets t ON t.id = si.ticket_id
		WHERE si.sale_id = $1
		ORDER BY si.created_at ASC
	`, saleID)
	if err != nil {
		return SaleDetailDTO{}, err
	}
	defer rows.Close()

	sale.Items = make([]SaleItemReadDTO, 0)
	for rows.Next() {
		var item SaleItemReadDTO
		if err := rows.Scan(&item.TicketID, &item.TicketName, &item.Quantity, &item.UnitPrice, &item.CreatedAt); err != nil {
			return SaleDetailDTO{}, err
		}
		sale.Items = append(sale.Items, item)
	}
	if err := rows.Err(); err != nil {
		return SaleDetailDTO{}, err
	}

	return sale, nil
}

func (s *PostgresSalesStore) ProcessPayment(ctx context.Context, req ProcessPaymentRequest) (ProcessPaymentResult, error) {
	paymentStatus := req.Status
	if paymentStatus == "" {
		paymentStatus = events.PaymentApprovedStatus
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return ProcessPaymentResult{}, err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	result, err := s.processPaymentTx(ctx, tx, req, paymentStatus, time.Time{})
	if err != nil {
		return ProcessPaymentResult{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return ProcessPaymentResult{}, err
	}

	return result, nil
}

func (s *PostgresSalesStore) CreatePaymentIntent(ctx context.Context, req CreatePaymentIntentStoreRequest) (PaymentIntentDTO, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return PaymentIntentDTO{}, err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	sale, err := s.getSaleForPayment(ctx, tx, req.SaleID)
	if err != nil {
		return PaymentIntentDTO{}, err
	}
	if sale.Status == events.SaleCompletedStatus {
		return PaymentIntentDTO{}, errSaleAlreadyPaid
	}
	if sale.Status != events.SalePendingPaymentStatus {
		return PaymentIntentDTO{}, errSaleCannotBePaid
	}
	if req.Amount != sale.TotalAmount {
		return PaymentIntentDTO{}, errValidation("amount does not match sale total amount")
	}

	intentID := uuid.NewString()
	providerReference := "pay_" + uuid.NewString()
	clientSecret := "pi_" + uuid.NewString()
	intentStatus := events.PaymentPendingStatus
	var createdAt time.Time
	var updatedAt time.Time

	if err := tx.QueryRow(ctx, `
		INSERT INTO payment_intents (id, sale_id, provider, amount, status, provider_reference, client_secret, created_at, updated_at)
		VALUES ($1, $2, $3, $4, 'PENDING', $5, $6, NOW(), NOW())
		ON CONFLICT (sale_id) DO UPDATE
		SET provider = EXCLUDED.provider,
		    amount = EXCLUDED.amount,
		    provider_reference = EXCLUDED.provider_reference,
		    client_secret = EXCLUDED.client_secret,
		    status = 'PENDING',
		    updated_at = NOW()
		RETURNING id, sale_id, status, provider, amount, provider_reference, client_secret, created_at, updated_at
	`, intentID, req.SaleID, req.Provider, req.Amount, providerReference, clientSecret).Scan(
		&intentID,
		&req.SaleID,
		&intentStatus,
		&req.Provider,
		&req.Amount,
		&providerReference,
		&clientSecret,
		&createdAt,
		&updatedAt,
	); err != nil {
		return PaymentIntentDTO{}, err
	}

	if _, err := tx.Exec(ctx, `
		UPDATE payments
		SET provider = $2,
		    amount = $3,
		    processed_at = NOW()
		WHERE sale_id = $1
	`, req.SaleID, req.Provider, req.Amount); err != nil {
		return PaymentIntentDTO{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return PaymentIntentDTO{}, err
	}

	return PaymentIntentDTO{
		ID:                intentID,
		SaleID:            req.SaleID,
		Status:            intentStatus,
		Provider:          req.Provider,
		Amount:            req.Amount,
		ProviderReference: providerReference,
		ClientSecret:      clientSecret,
		CreatedAt:         createdAt,
		UpdatedAt:         updatedAt,
		Metadata:          storedSaleMetadata(ctx, sale.Metadata, req.SaleID),
	}, nil
}

func (s *PostgresSalesStore) ProcessPaymentWebhook(ctx context.Context, req ProcessPaymentWebhookRequest) (ProcessPaymentResult, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return ProcessPaymentResult{}, err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	intent, err := s.getPaymentIntent(ctx, tx, req.PaymentIntentID)
	if err != nil {
		return ProcessPaymentResult{}, err
	}
	if intent.SaleID != req.SaleID {
		return ProcessPaymentResult{}, errValidation("paymentIntentId does not belong to this sale")
	}
	if req.Amount != intent.Amount {
		return ProcessPaymentResult{}, errValidation("amount does not match payment intent amount")
	}
	if req.Provider != intent.Provider {
		return ProcessPaymentResult{}, errValidation("provider does not match payment intent provider")
	}

	sale, err := s.getSaleForPayment(ctx, tx, req.SaleID)
	if err != nil {
		return ProcessPaymentResult{}, err
	}

	if sale.Status == events.SaleCompletedStatus && req.Status == events.PaymentApprovedStatus {
		return ProcessPaymentResult{
			SaleID:           sale.ID,
			SalesEventID:     sale.SalesEventID,
			SaleStatus:       sale.Status,
			IdempotentReplay: true,
			Payment: PaymentDTO{
				Status:      req.Status,
				Amount:      req.Amount,
				Provider:    req.Provider,
				ProcessedAt: req.OccurredAt.UTC(),
			},
			Metadata: storedSaleMetadata(ctx, sale.Metadata, sale.ID),
		}, nil
	}
	if sale.Status == events.SaleFailedStatus && req.Status == events.PaymentFailedStatus {
		return ProcessPaymentResult{
			SaleID:           sale.ID,
			SalesEventID:     sale.SalesEventID,
			SaleStatus:       sale.Status,
			IdempotentReplay: true,
			Payment: PaymentDTO{
				Status:      req.Status,
				Amount:      req.Amount,
				Provider:    req.Provider,
				ProcessedAt: req.OccurredAt.UTC(),
			},
			Metadata: storedSaleMetadata(ctx, sale.Metadata, sale.ID),
		}, nil
	}

	result, err := s.processPaymentTx(ctx, tx, ProcessPaymentRequest{
		SaleID:   req.SaleID,
		Amount:   req.Amount,
		Provider: req.Provider,
		Status:   req.Status,
	}, req.Status, req.OccurredAt)
	if err != nil {
		return ProcessPaymentResult{}, err
	}

	if _, err := tx.Exec(ctx, `
		UPDATE payment_intents
		SET status = $2,
		    provider_reference = CASE WHEN $3 = '' THEN provider_reference ELSE $3 END,
		    updated_at = NOW()
		WHERE id = $1
	`, req.PaymentIntentID, webhookIntentStatus(req.Status), req.ProviderReference); err != nil {
		return ProcessPaymentResult{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return ProcessPaymentResult{}, err
	}
	return result, nil
}

func (s *PostgresSalesStore) processPaymentTx(ctx context.Context, tx pgx.Tx, req ProcessPaymentRequest, paymentStatus string, occurredAt time.Time) (ProcessPaymentResult, error) {
	ctx, span := observability.StartBusinessSpan(ctx, "payment.persist",
		attribute.String("sale.id", req.SaleID),
		attribute.String("payment.provider", req.Provider),
		attribute.String("payment.status", paymentStatus),
		attribute.Int("payment.amount", req.Amount),
	)
	var spanErr error
	defer func() {
		observability.EndSpan(span, spanErr)
	}()

	sale, err := s.getSaleForPayment(ctx, tx, req.SaleID)
	if err != nil {
		spanErr = err
		return ProcessPaymentResult{}, err
	}
	if sale.Status == events.SaleCompletedStatus {
		spanErr = errSaleAlreadyPaid
		return ProcessPaymentResult{}, errSaleAlreadyPaid
	}
	if sale.Status != events.SalePendingPaymentStatus {
		spanErr = errSaleCannotBePaid
		return ProcessPaymentResult{}, errSaleCannotBePaid
	}
	if req.Amount != sale.TotalAmount {
		spanErr = errValidation("amount does not match sale total amount")
		return ProcessPaymentResult{}, spanErr
	}

	metadata := storedSaleMetadata(ctx, sale.Metadata, req.SaleID)
	ctx = correlation.ContextWithMetadata(ctx, metadata)
	span.SetAttributes(observability.CorrelationAttributes(ctx)...)
	saleStatus := events.SaleCompletedStatus
	if paymentStatus == events.PaymentFailedStatus {
		saleStatus = events.SaleFailedStatus
		if err := s.releaseReservedTickets(ctx, tx, req.SaleID); err != nil {
			spanErr = err
			return ProcessPaymentResult{}, err
		}
	}
	span.SetAttributes(
		attribute.String("sales_event.id", sale.SalesEventID),
		attribute.String("sale.status", saleStatus),
	)

	processedAt := time.Now().UTC()
	if !occurredAt.IsZero() {
		processedAt = occurredAt.UTC()
	}

	var payment PaymentDTO
	if err := tx.QueryRow(ctx, `
		INSERT INTO payments (sale_id, status, amount, provider, processed_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (sale_id) DO UPDATE
		SET status = EXCLUDED.status,
		    amount = EXCLUDED.amount,
		    provider = EXCLUDED.provider,
		    processed_at = EXCLUDED.processed_at
		RETURNING status, amount, provider, processed_at
	`, req.SaleID, paymentStatus, req.Amount, req.Provider, processedAt).Scan(
		&payment.Status,
		&payment.Amount,
		&payment.Provider,
		&payment.ProcessedAt,
	); err != nil {
		spanErr = err
		return ProcessPaymentResult{}, err
	}

	if _, err := tx.Exec(ctx, `
		UPDATE sales
		SET status = $2,
		    updated_at = NOW()
		WHERE id = $1
	`, req.SaleID, saleStatus); err != nil {
		spanErr = err
		return ProcessPaymentResult{}, err
	}

	eventID := uuid.NewString()
	payload, err := paymentOutboxPayload(eventID, sale.SalesEventID, req, paymentStatus, saleStatus, processedAt, metadata)
	if err != nil {
		spanErr = err
		return ProcessPaymentResult{}, err
	}

	outboxCtx, outboxSpan := observability.StartBusinessSpan(ctx, "outbox.enqueue",
		attribute.String("outbox.event.id", eventID),
		attribute.String("outbox.event.type", saleStatusEventName(saleStatus)),
		attribute.String("sale.id", req.SaleID),
	)
	if _, err := tx.Exec(ctx, `
		INSERT INTO outbox_events (event_id, event_type, aggregate_id, payload, trace_context, request_id, correlation_id, transaction_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, eventID, saleStatusEventName(saleStatus), req.SaleID, payload, observability.TraceContext(outboxCtx), metadata.RequestID, metadata.CorrelationID, metadata.TransactionID); err != nil {
		observability.EndSpan(outboxSpan, err)
		spanErr = err
		return ProcessPaymentResult{}, err
	}
	observability.EndSpan(outboxSpan, nil)

	return ProcessPaymentResult{
		SaleID:       req.SaleID,
		SalesEventID: sale.SalesEventID,
		SaleStatus:   saleStatus,
		Payment:      payment,
		Metadata:     metadata,
	}, nil
}

func paymentOutboxPayload(eventID string, salesEventID string, req ProcessPaymentRequest, paymentStatus string, saleStatus string, occurredAt time.Time, metadata correlation.Metadata) ([]byte, error) {
	if saleStatus == events.SaleCompletedStatus {
		return json.Marshal(events.SaleCompleted{
			EventID:      eventID,
			EventType:    "SALE_COMPLETED",
			OccurredAt:   occurredAt,
			SaleID:       req.SaleID,
			SalesEventID: salesEventID,
			Metadata:     eventCorrelationMetadata(metadata),
		})
	}

	return json.Marshal(map[string]any{
		"eventId":       eventID,
		"eventType":     "SALE_FAILED",
		"occurredAt":    occurredAt,
		"saleId":        req.SaleID,
		"salesEventId":  salesEventID,
		"paymentStatus": paymentStatus,
		"amount":        req.Amount,
		"provider":      req.Provider,
		"metadata":      eventCorrelationMetadata(metadata),
	})
}

type paymentSale struct {
	ID             string
	SalesEventID   string
	SalesEventName string
	StartsAt       time.Time
	CustomerID     string
	CustomerName   string
	CustomerEmail  string
	Status         string
	TotalAmount    int
	CreatedAt      time.Time
	UpdatedAt      time.Time
	Metadata       correlation.Metadata
}

func (s *PostgresSalesStore) getSaleForPayment(ctx context.Context, tx pgx.Tx, saleID string) (paymentSale, error) {
	var sale paymentSale
	if err := tx.QueryRow(ctx, `
		SELECT s.id, s.sales_event_id, se.name, se.starts_at, s.customer_id, c.name, c.email, s.status, s.total_amount,
		       s.created_at, s.updated_at, s.request_id, s.correlation_id, s.transaction_id
		FROM sales s
		JOIN sales_events se ON se.id = s.sales_event_id
		JOIN customers c ON c.id = s.customer_id
		WHERE s.id = $1
		FOR UPDATE
	`, saleID).Scan(
		&sale.ID,
		&sale.SalesEventID,
		&sale.SalesEventName,
		&sale.StartsAt,
		&sale.CustomerID,
		&sale.CustomerName,
		&sale.CustomerEmail,
		&sale.Status,
		&sale.TotalAmount,
		&sale.CreatedAt,
		&sale.UpdatedAt,
		&sale.Metadata.RequestID,
		&sale.Metadata.CorrelationID,
		&sale.Metadata.TransactionID,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return paymentSale{}, errSaleNotFound
		}
		return paymentSale{}, err
	}
	return sale, nil
}

type paymentIntentRecord struct {
	ID       string
	SaleID   string
	Provider string
	Amount   int
	Status   string
}

func (s *PostgresSalesStore) getPaymentIntent(ctx context.Context, tx pgx.Tx, paymentIntentID string) (paymentIntentRecord, error) {
	var intent paymentIntentRecord
	if err := tx.QueryRow(ctx, `
		SELECT id, sale_id, provider, amount, status
		FROM payment_intents
		WHERE id = $1
		FOR UPDATE
	`, paymentIntentID).Scan(
		&intent.ID,
		&intent.SaleID,
		&intent.Provider,
		&intent.Amount,
		&intent.Status,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return paymentIntentRecord{}, errPaymentIntentNotFound
		}
		return paymentIntentRecord{}, err
	}
	return intent, nil
}

func webhookIntentStatus(paymentStatus string) string {
	if paymentStatus == events.PaymentApprovedStatus {
		return "SUCCEEDED"
	}
	return "FAILED"
}

func (s *PostgresSalesStore) releaseReservedTickets(ctx context.Context, tx pgx.Tx, saleID string) error {
	_, err := tx.Exec(ctx, `
		UPDATE tickets t
		SET available_quantity = t.available_quantity + si.quantity
		FROM sale_items si
		WHERE si.ticket_id = t.id
		  AND si.sale_id = $1
	`, saleID)
	return err
}

func saleStatusEventName(status string) string {
	if status == events.SaleCompletedStatus {
		return "SALE_COMPLETED"
	}
	return "SALE_FAILED"
}

func (s *PostgresSalesStore) CheckInTicket(ctx context.Context, req CheckInTicketRequest) (CheckInTicketResult, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return CheckInTicketResult{}, err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	ticket, err := s.getIssuedTicketForCheckIn(ctx, tx, req.IssuedTicketID)
	if err != nil {
		return CheckInTicketResult{}, err
	}
	if ticket.SalesEventID != req.SalesEventID {
		return CheckInTicketResult{}, errTicketWrongEvent
	}
	if ticket.SaleStatus != events.SaleCompletedStatus || ticket.PaymentStatus != events.PaymentApprovedStatus {
		return CheckInTicketResult{}, errTicketCannotBeCheckedIn
	}

	result := CheckInTicketResult{
		IssuedTicketID: ticket.IssuedTicketID,
		SalesEventID:   ticket.SalesEventID,
		SaleID:         ticket.SaleID,
		TicketID:       ticket.TicketID,
		TicketName:     ticket.TicketName,
		CustomerID:     ticket.CustomerID,
		CustomerName:   ticket.CustomerName,
		Metadata:       storedSaleMetadata(ctx, ticket.Metadata, ticket.SaleID),
	}

	if err := tx.QueryRow(ctx, `
		INSERT INTO ticket_check_ins (issued_ticket_id, sales_event_id)
		VALUES ($1, $2)
		RETURNING id, checked_in_at
	`, ticket.IssuedTicketID, ticket.SalesEventID).Scan(&result.CheckInID, &result.CheckedInAt); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return CheckInTicketResult{}, errTicketAlreadyCheckedIn
		}
		return CheckInTicketResult{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return CheckInTicketResult{}, err
	}
	return result, nil
}

func (s *PostgresSalesStore) RecordEmailEvent(ctx context.Context, req RecordEmailEventRequest) (RecordEmailEventResult, error) {
	var metadata correlation.Metadata
	err := s.db.QueryRow(ctx, `
		UPDATE email_notifications en
		SET status = CASE
		        WHEN $2 = 'DELIVERED' THEN 'DELIVERED'
		        WHEN $2 = 'OPENED' THEN 'OPENED'
		        WHEN $2 = 'CLICKED' THEN 'CLICKED'
		        WHEN $2 = 'BOUNCED' THEN 'BOUNCED'
		        ELSE en.status
		    END,
		    delivered_at = CASE
		        WHEN $2 = 'DELIVERED' THEN COALESCE(en.delivered_at, $3)
		        ELSE en.delivered_at
		    END,
		    opened_at = CASE
		        WHEN $2 = 'OPENED' THEN COALESCE(en.opened_at, $3)
		        ELSE en.opened_at
		    END,
		    clicked_at = CASE
		        WHEN $2 = 'CLICKED' THEN COALESCE(en.clicked_at, $3)
		        ELSE en.clicked_at
		    END,
		    bounced_at = CASE
		        WHEN $2 = 'BOUNCED' THEN COALESCE(en.bounced_at, $3)
		        ELSE en.bounced_at
		    END,
		    provider_event_id = CASE
		        WHEN $4 = '' THEN en.provider_event_id
		        ELSE COALESCE(en.provider_event_id, $4)
		    END,
		    error_message = CASE
		        WHEN $2 = 'BOUNCED' AND $5 <> '' THEN $5
		        WHEN $2 IN ('DELIVERED', 'OPENED', 'CLICKED') THEN NULL
		        ELSE en.error_message
		    END,
		    updated_at = NOW()
		FROM sales s
		WHERE en.sale_id = $1
		  AND s.id = en.sale_id
		RETURNING s.request_id, s.correlation_id, s.transaction_id
	`, req.SaleID, req.EventType, req.OccurredAt.UTC(), req.ProviderEventID, req.Reason).Scan(
		&metadata.RequestID,
		&metadata.CorrelationID,
		&metadata.TransactionID,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return RecordEmailEventResult{}, errSaleNotFound
		}
		return RecordEmailEventResult{}, err
	}

	status := req.EventType
	if status == "BOUNCED" {
		status = "BOUNCED"
	}

	return RecordEmailEventResult{
		SaleID:          req.SaleID,
		Status:          status,
		ProviderEventID: req.ProviderEventID,
		RecordedAt:      req.OccurredAt.UTC(),
		Metadata:        storedSaleMetadata(ctx, metadata, req.SaleID),
	}, nil
}

type issuedTicketForCheckIn struct {
	IssuedTicketID string
	SalesEventID   string
	SaleID         string
	SaleStatus     string
	PaymentStatus  string
	TicketID       string
	TicketName     string
	CustomerID     string
	CustomerName   string
	Metadata       correlation.Metadata
}

func (s *PostgresSalesStore) getIssuedTicketForCheckIn(ctx context.Context, tx pgx.Tx, issuedTicketID string) (issuedTicketForCheckIn, error) {
	var ticket issuedTicketForCheckIn
	if err := tx.QueryRow(ctx, `
		SELECT it.id, s.sales_event_id, s.id, s.status, p.status, t.id, t.name, c.id, c.name,
		       s.request_id, s.correlation_id, s.transaction_id
		FROM issued_tickets it
		JOIN sales s ON s.id = it.sale_id
		JOIN payments p ON p.sale_id = s.id
		JOIN tickets t ON t.id = it.ticket_id
		JOIN customers c ON c.id = it.customer_id
		WHERE it.id = $1
		FOR UPDATE OF it
	`, issuedTicketID).Scan(
		&ticket.IssuedTicketID,
		&ticket.SalesEventID,
		&ticket.SaleID,
		&ticket.SaleStatus,
		&ticket.PaymentStatus,
		&ticket.TicketID,
		&ticket.TicketName,
		&ticket.CustomerID,
		&ticket.CustomerName,
		&ticket.Metadata.RequestID,
		&ticket.Metadata.CorrelationID,
		&ticket.Metadata.TransactionID,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return issuedTicketForCheckIn{}, errIssuedTicketNotFound
		}
		return issuedTicketForCheckIn{}, err
	}
	return ticket, nil
}

func storedSaleMetadata(ctx context.Context, metadata correlation.Metadata, saleID string) correlation.Metadata {
	metadata = correlation.WithTransactionID(metadata, saleID)
	current, _ := correlation.FromContext(ctx)
	return correlation.Merge(metadata, current)
}
