package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/mail"

	"github.com/google/uuid"
	"github.com/varner/sales-event-project/internal/events"
)

func listSales(ctx context.Context, store SalesStore, filter listSalesFilter) (ListSalesResponse, error) {
	if err := validateSalesEventID(filter.SalesEventID); err != nil {
		return ListSalesResponse{}, err
	}
	if err := validateSaleStatusFilter(filter.Status); err != nil {
		return ListSalesResponse{}, err
	}
	if len(filter.EventName) > events.MaxTextLength {
		return ListSalesResponse{}, errValidation("eventName is too long; maximum length is 50 characters")
	}

	eventExists, err := store.SalesEventExists(ctx, filter.SalesEventID)
	if err != nil {
		return ListSalesResponse{}, err
	}
	if !eventExists {
		return ListSalesResponse{
			Page:     filter.Page,
			PageSize: filter.PageSize,
			Total:    0,
			Data:     []SaleListItemDTO{},
		}, nil
	}

	return store.ListSales(ctx, filter)
}

func getSale(ctx context.Context, store SalesStore, salesEventID string, saleID string) (SaleDetailDTO, error) {
	if err := validateSalesEventID(salesEventID); err != nil {
		return SaleDetailDTO{}, err
	}
	if _, err := uuid.Parse(saleID); err != nil {
		return SaleDetailDTO{}, errValidation("saleId must be a valid UUID")
	}

	eventExists, err := store.SalesEventExists(ctx, salesEventID)
	if err != nil {
		return SaleDetailDTO{}, err
	}
	if !eventExists {
		return SaleDetailDTO{}, errSalesEventNotFound
	}

	return store.GetSale(ctx, salesEventID, saleID)
}

func validateSaleRequest(ctx context.Context, store SalesStore, req CreateSaleRequest) error {
	if req.SalesEventID == "" {
		return errValidation("salesEventId is required")
	}
	if len(req.SalesEventID) > events.MaxTextLength {
		return errValidation("salesEventId is too long; maximum length is 50 characters")
	}
	if req.CustomerID == "" {
		return errValidation("customerId is required")
	}
	if len(req.CustomerID) > events.MaxTextLength {
		return errValidation("customerId is too long; maximum length is 50 characters")
	}
	if req.CustomerName == "" {
		return errValidation("customerName is required")
	}
	if len(req.CustomerName) > events.MaxCustomerNameLength {
		return errValidation("customerName is too long; maximum length is 100 characters")
	}
	if req.CustomerEmail == "" {
		return errValidation("customerEmail is required")
	}
	if len(req.CustomerEmail) > events.MaxEmailLength {
		return errValidation("customerEmail is too long; maximum length is 255 characters")
	}
	if _, err := mail.ParseAddress(req.CustomerEmail); err != nil {
		return errValidation("customerEmail must be a valid email address")
	}
	if req.Status != "" && req.Status != events.SaleProcessingStatus {
		return errValidation("status must be PROCESSING when creating a sale")
	}
	if len(req.Items) == 0 {
		return errValidation("items must contain at least one ticket")
	}

	eventExists, err := store.SalesEventExists(ctx, req.SalesEventID)
	if err != nil {
		return err
	}
	if !eventExists {
		return errValidation("sales event does not exist")
	}

	totalAmount := 0
	for index, item := range req.Items {
		if item.TicketID == "" {
			return errValidation(fmt.Sprintf("items[%d].ticketId is required", index))
		}
		if len(item.TicketID) > events.MaxTextLength {
			return errValidation(fmt.Sprintf("items[%d].ticketId is too long; maximum length is 50 characters", index))
		}
		if item.Quantity < 0 {
			return errValidation(fmt.Sprintf("items[%d].quantity cannot be negative", index))
		}
		if item.Quantity == 0 {
			return errValidation(fmt.Sprintf("items[%d].quantity must be greater than zero", index))
		}
		if item.Quantity > events.MaxTicketQuantity {
			return errValidation(fmt.Sprintf("items[%d].quantity is too high; maximum is %d", index, events.MaxTicketQuantity))
		}
		if item.UnitPrice < 0 {
			return errValidation(fmt.Sprintf("items[%d].unitPrice cannot be negative", index))
		}
		if item.UnitPrice == 0 {
			return errValidation(fmt.Sprintf("items[%d].unitPrice must be greater than zero", index))
		}
		if item.UnitPrice > events.MaxMoneyAmountInCents {
			return errValidation(fmt.Sprintf("items[%d].unitPrice is too high; maximum is %d", index, events.MaxMoneyAmountInCents))
		}
		if totalAmount > events.MaxMoneyAmountInCents-(item.Quantity*item.UnitPrice) {
			return errValidation(fmt.Sprintf("sale total amount is too high; maximum is %d", events.MaxMoneyAmountInCents))
		}
		totalAmount += item.Quantity * item.UnitPrice

		ticket, err := store.GetTicketForEvent(ctx, item.TicketID, req.SalesEventID)
		if err != nil {
			if errors.Is(err, errTicketNotFound) {
				return errValidation(fmt.Sprintf("items[%d].ticketId does not exist for this sales event", index))
			}
			return err
		}
		if item.Quantity > ticket.AvailableQuantity {
			return errValidation(fmt.Sprintf("items[%d].quantity exceeds available tickets; available quantity is %d", index, ticket.AvailableQuantity))
		}
		if item.UnitPrice != ticket.Price {
			return errValidation(fmt.Sprintf("items[%d].unitPrice does not match ticket price", index))
		}
	}

	return nil
}
