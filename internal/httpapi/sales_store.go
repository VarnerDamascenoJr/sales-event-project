package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/skip2/go-qrcode"
	"github.com/varner/sales-event-project/internal/events"
	"github.com/varner/sales-event-project/internal/notification"
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
	defer tx.Rollback(ctx)

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

	payload, err := json.Marshal(map[string]any{
		"saleId":        req.SaleID,
		"paymentStatus": paymentStatus,
		"amount":        req.Amount,
		"provider":      req.Provider,
	})
	if err != nil {
		return ProcessPaymentResult{}, err
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO outbox_events (event_id, event_type, aggregate_id, payload)
		VALUES ($1, $2, $3, $4)
	`, uuid.NewString(), saleStatusEventName(saleStatus), req.SaleID, payload); err != nil {
		return ProcessPaymentResult{}, err
	}

	var ticketEmail *notification.TicketEmail
	if paymentStatus == events.PaymentApprovedStatus {
		issuedTickets, err := s.issueTickets(ctx, tx, sale)
		if err != nil {
			return ProcessPaymentResult{}, err
		}

		if _, err := tx.Exec(ctx, `
			INSERT INTO email_notifications (sale_id, recipient_email, status, created_at, updated_at)
			VALUES ($1, $2, 'PENDING', NOW(), NOW())
			ON CONFLICT (sale_id) DO UPDATE
			SET recipient_email = EXCLUDED.recipient_email,
			    status = 'PENDING',
			    error_message = NULL,
			    updated_at = NOW()
		`, req.SaleID, sale.CustomerEmail); err != nil {
			return ProcessPaymentResult{}, err
		}

		ticketEmail = &notification.TicketEmail{
			To:             sale.CustomerEmail,
			CustomerName:   sale.CustomerName,
			SalesEventName: sale.SalesEventName,
			StartsAt:       sale.StartsAt,
			Tickets:        issuedTickets,
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return ProcessPaymentResult{}, err
	}

	return ProcessPaymentResult{
		SaleID:      req.SaleID,
		SaleStatus:  saleStatus,
		Payment:     payment,
		TicketEmail: ticketEmail,
	}, nil
}

func (s *PostgresSalesStore) MarkTicketEmailSent(ctx context.Context, saleID string) error {
	_, err := s.db.Exec(ctx, `
		UPDATE email_notifications
		SET status = 'SENT',
		    error_message = NULL,
		    sent_at = NOW(),
		    updated_at = NOW()
		WHERE sale_id = $1
	`, saleID)
	if err != nil {
		return err
	}

	_, err = s.db.Exec(ctx, `
		UPDATE issued_tickets
		SET emailed_at = NOW()
		WHERE sale_id = $1
	`, saleID)
	return err
}

func (s *PostgresSalesStore) MarkTicketEmailFailed(ctx context.Context, saleID string, sendErr error) error {
	_, err := s.db.Exec(ctx, `
		UPDATE email_notifications
		SET status = 'FAILED',
		    error_message = $2,
		    updated_at = NOW()
		WHERE sale_id = $1
	`, saleID, sendErr.Error())
	return err
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

func (s *PostgresSalesStore) issueTickets(ctx context.Context, tx pgx.Tx, sale paymentSale) ([]notification.IssuedTicket, error) {
	rows, err := tx.Query(ctx, `
		SELECT si.ticket_id, t.name, si.quantity
		FROM sale_items si
		JOIN tickets t ON t.id = si.ticket_id
		WHERE si.sale_id = $1
		ORDER BY si.created_at ASC
	`, sale.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	issuedTickets := make([]notification.IssuedTicket, 0)
	for rows.Next() {
		var ticketID string
		var ticketName string
		var quantity int
		if err := rows.Scan(&ticketID, &ticketName, &quantity); err != nil {
			return nil, err
		}

		for range quantity {
			issuedTicketID := uuid.NewString()
			qrPayload := fmt.Sprintf("issued_ticket:%s", issuedTicketID)
			qrCodePNG, err := qrcode.Encode(qrPayload, qrcode.Medium, 256)
			if err != nil {
				return nil, err
			}

			if _, err := tx.Exec(ctx, `
				INSERT INTO issued_tickets (id, sale_id, ticket_id, customer_id, qr_code_payload)
				VALUES ($1, $2, $3, $4, $5)
			`, issuedTicketID, sale.ID, ticketID, sale.CustomerID, qrPayload); err != nil {
				return nil, err
			}

			issuedTickets = append(issuedTickets, notification.IssuedTicket{
				ID:         issuedTicketID,
				TicketName: ticketName,
				QRCodePNG:  qrCodePNG,
				QRPayload:  qrPayload,
			})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return issuedTickets, nil
}

func saleStatusEventName(status string) string {
	if status == events.SaleCompletedStatus {
		return "SALE_COMPLETED"
	}
	return "SALE_FAILED"
}
