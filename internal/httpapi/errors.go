package httpapi

import "errors"

type errValidation string

func (e errValidation) Error() string {
	return string(e)
}

var (
	errSalesEventNotFound      = errors.New("sales event does not exist")
	errSaleNotFound            = errors.New("sale does not exist for this sales event")
	errPaymentIntentNotFound   = errors.New("payment intent does not exist")
	errTicketNotFound          = errors.New("ticket does not exist for this sales event")
	errIssuedTicketNotFound    = errors.New("issued ticket does not exist")
	errTicketWrongEvent        = errors.New("issued ticket does not belong to this sales event")
	errTicketAlreadyCheckedIn  = errors.New("ticket is already checked in")
	errTicketCannotBeCheckedIn = errors.New("ticket cannot be checked in because sale is not completed")
	errSaleAlreadyPaid         = errors.New("sale is already paid")
	errSaleCannotBePaid        = errors.New("sale cannot be paid in its current status")
)
