package main

type summary struct {
	SalesRequested        int `json:"salesRequested"`
	SalesCreated          int `json:"salesCreated"`
	PaymentsApproved      int `json:"paymentsApproved"`
	PaymentsFailed        int `json:"paymentsFailed"`
	EmailEvents           int `json:"emailEvents"`
	CheckIns              int `json:"checkIns"`
	ExpectedIssuedTickets int `json:"expectedIssuedTickets"`
	RateLimitRetries      int `json:"rateLimitRetries"`
	Errors                int `json:"errors"`
}

type salePayload struct {
	SalesEventID  string     `json:"salesEventId"`
	CustomerID    string     `json:"customerId"`
	CustomerName  string     `json:"customerName"`
	CustomerEmail string     `json:"customerEmail"`
	Items         []saleItem `json:"items"`
}

type saleItem struct {
	TicketID  string `json:"ticketId"`
	Quantity  int    `json:"quantity"`
	UnitPrice int    `json:"unitPrice"`
}

type paymentPayload struct {
	Amount   int    `json:"amount"`
	Provider string `json:"provider"`
	Status   string `json:"status"`
}

type emailEventPayload struct {
	SaleID          string `json:"saleId"`
	EventType       string `json:"eventType"`
	OccurredAt      string `json:"occurredAt"`
	ProviderEventID string `json:"providerEventId"`
	Reason          string `json:"reason"`
}
