package httpapi

import (
	"fmt"

	"github.com/google/uuid"
	"github.com/varner/sales-event-project/internal/events"
)

func validateSalesEventID(salesEventID string) error {
	if salesEventID == "" {
		return errValidation("salesEventId is required")
	}
	if _, err := uuid.Parse(salesEventID); err != nil {
		return errValidation("salesEventId must be a valid UUID")
	}
	return nil
}

func validateSaleStatusFilter(status string) error {
	if status == "" {
		return nil
	}
	for _, availableStatus := range events.AvailableSaleStatuses {
		if status == availableStatus {
			return nil
		}
	}
	return errValidation("status is invalid")
}

func validatePaymentRequest(req ProcessPaymentRequest) error {
	if _, err := uuid.Parse(req.SaleID); err != nil {
		return errValidation("saleId must be a valid UUID")
	}
	if req.Amount <= 0 {
		return errValidation("amount must be greater than zero")
	}
	if req.Amount > events.MaxMoneyAmountInCents {
		return errValidation(fmt.Sprintf("amount is too high; maximum is %d", events.MaxMoneyAmountInCents))
	}
	if req.Provider == "" {
		return errValidation("provider is required")
	}
	if len(req.Provider) > events.MaxTextLength {
		return errValidation("provider is too long; maximum length is 50 characters")
	}
	if req.Status == "" {
		return nil
	}
	if req.Status != events.PaymentApprovedStatus && req.Status != events.PaymentFailedStatus {
		return errValidation("status must be APPROVED or FAILED")
	}
	return nil
}

func validateCreatePaymentIntentRequest(req CreatePaymentIntentStoreRequest) error {
	if _, err := uuid.Parse(req.SaleID); err != nil {
		return errValidation("saleId must be a valid UUID")
	}
	if req.Amount <= 0 {
		return errValidation("amount must be greater than zero")
	}
	if req.Amount > events.MaxMoneyAmountInCents {
		return errValidation(fmt.Sprintf("amount is too high; maximum is %d", events.MaxMoneyAmountInCents))
	}
	if req.Provider == "" {
		return errValidation("provider is required")
	}
	if len(req.Provider) > events.MaxTextLength {
		return errValidation("provider is too long; maximum length is 50 characters")
	}
	return nil
}

func validatePaymentWebhookRequest(req ProcessPaymentWebhookRequest) error {
	if _, err := uuid.Parse(req.PaymentIntentID); err != nil {
		return errValidation("paymentIntentId must be a valid UUID")
	}
	if err := validatePaymentRequest(ProcessPaymentRequest{
		SaleID:   req.SaleID,
		Amount:   req.Amount,
		Provider: req.Provider,
		Status:   req.Status,
	}); err != nil {
		return err
	}
	if req.OccurredAt.IsZero() {
		return errValidation("occurredAt is required")
	}
	if len(req.ProviderReference) > 100 {
		return errValidation("providerReference is too long; maximum length is 100 characters")
	}
	return nil
}

func validateCheckInRequest(req *CheckInTicketRequest) error {
	if err := validateSalesEventID(req.SalesEventID); err != nil {
		return err
	}
	if req.TicketCode == "" {
		return errValidation("ticketCode is required")
	}
	if len(req.TicketCode) > 255 {
		return errValidation("ticketCode is too long; maximum length is 255 characters")
	}

	const payloadPrefix = "issued_ticket:"
	issuedTicketID := req.TicketCode
	if len(req.TicketCode) > len(payloadPrefix) && req.TicketCode[:len(payloadPrefix)] == payloadPrefix {
		issuedTicketID = req.TicketCode[len(payloadPrefix):]
	}
	if _, err := uuid.Parse(issuedTicketID); err != nil {
		return errValidation("ticketCode must contain a valid issued ticket id")
	}
	req.IssuedTicketID = issuedTicketID
	return nil
}

func validateEmailEventRequest(req RecordEmailEventRequest) error {
	if _, err := uuid.Parse(req.SaleID); err != nil {
		return errValidation("saleId must be a valid UUID")
	}
	switch req.EventType {
	case "DELIVERED", "OPENED", "CLICKED", "BOUNCED":
	default:
		return errValidation("eventType must be DELIVERED, OPENED, CLICKED, or BOUNCED")
	}
	if req.OccurredAt.IsZero() {
		return errValidation("occurredAt is required")
	}
	if len(req.ProviderEventID) > 100 {
		return errValidation("providerEventId is too long; maximum length is 100 characters")
	}
	if len(req.Reason) > 1000 {
		return errValidation("reason is too long; maximum length is 1000 characters")
	}
	return nil
}
