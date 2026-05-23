package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/varner/sales-event-project/internal/events"
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

	sale, err := s.getSaleForPayment(ctx, tx, req.SaleID)
	if err != nil {
		return ProcessPaymentResult{}, err
	}
	if sale.Status == events.SaleCompletedStatus {
		return ProcessPaymentResult{}, errSaleAlreadyPaid
	}
	if sale.Status != events.SalePendingPaymentStatus {
		return ProcessPaymentResult{}, errSaleCannotBePaid
	}
	if req.Amount != sale.TotalAmount {
		return ProcessPaymentResult{}, errValidation("amount does not match sale total amount")
	}

	saleStatus := events.SaleCompletedStatus
	if paymentStatus == events.PaymentFailedStatus {
		saleStatus = events.SaleFailedStatus
		if err := s.releaseReservedTickets(ctx, tx, req.SaleID); err != nil {
			return ProcessPaymentResult{}, err
		}
	}

	var payment PaymentDTO
	if err := tx.QueryRow(ctx, `
		INSERT INTO payments (sale_id, status, amount, provider, processed_at)
		VALUES ($1, $2, $3, $4, NOW())
		ON CONFLICT (sale_id) DO UPDATE
		SET status = EXCLUDED.status,
		    amount = EXCLUDED.amount,
		    provider = EXCLUDED.provider,
		    processed_at = NOW()
		RETURNING status, amount, provider, processed_at
	`, req.SaleID, paymentStatus, req.Amount, req.Provider).Scan(
		&payment.Status,
		&payment.Amount,
		&payment.Provider,
		&payment.ProcessedAt,
	); err != nil {
		return ProcessPaymentResult{}, err
	}

	if _, err := tx.Exec(ctx, `
		UPDATE sales
		SET status = $2,
		    updated_at = NOW()
		WHERE id = $1
	`, req.SaleID, saleStatus); err != nil {
		return ProcessPaymentResult{}, err
	}

	eventID := uuid.NewString()
	payload, err := paymentOutboxPayload(eventID, sale.SalesEventID, req, paymentStatus, saleStatus)
	if err != nil {
		return ProcessPaymentResult{}, err
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO outbox_events (event_id, event_type, aggregate_id, payload)
		VALUES ($1, $2, $3, $4)
	`, eventID, saleStatusEventName(saleStatus), req.SaleID, payload); err != nil {
		return ProcessPaymentResult{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return ProcessPaymentResult{}, err
	}

	return ProcessPaymentResult{
		SaleID:       req.SaleID,
		SalesEventID: sale.SalesEventID,
		SaleStatus:   saleStatus,
		Payment:      payment,
	}, nil
}

func paymentOutboxPayload(eventID string, salesEventID string, req ProcessPaymentRequest, paymentStatus string, saleStatus string) ([]byte, error) {
	if saleStatus == events.SaleCompletedStatus {
		return json.Marshal(events.SaleCompleted{
			EventID:      eventID,
			EventType:    "SALE_COMPLETED",
			OccurredAt:   time.Now().UTC(),
			SaleID:       req.SaleID,
			SalesEventID: salesEventID,
		})
	}

	return json.Marshal(map[string]any{
		"eventId":       eventID,
		"eventType":     "SALE_FAILED",
		"occurredAt":    time.Now().UTC(),
		"saleId":        req.SaleID,
		"salesEventId":  salesEventID,
		"paymentStatus": paymentStatus,
		"amount":        req.Amount,
		"provider":      req.Provider,
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
}

func (s *PostgresSalesStore) getSaleForPayment(ctx context.Context, tx pgx.Tx, saleID string) (paymentSale, error) {
	var sale paymentSale
	if err := tx.QueryRow(ctx, `
		SELECT s.id, s.sales_event_id, se.name, se.starts_at, s.customer_id, c.name, c.email, s.status, s.total_amount
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
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return paymentSale{}, errSaleNotFound
		}
		return paymentSale{}, err
	}
	return sale, nil
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
