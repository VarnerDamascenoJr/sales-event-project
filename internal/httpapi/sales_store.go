package httpapi

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
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
