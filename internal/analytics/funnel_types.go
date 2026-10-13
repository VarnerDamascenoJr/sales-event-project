package analytics

import "time"

func DefaultFunnelOptions() FunnelOptions {
	return FunnelOptions{
		ConfidenceLevel: 0.95,
		WilsonZ:         1.959963984540054,
		PriorAlpha:      1,
		PriorBeta:       1,
	}
}

type funnelRecord struct {
	SalesEventID string
	SaleID       string
	TicketID     string
	TicketType   string
	Provider     string
	AcceptedAt   time.Time
	Accepted     bool
	Pending      bool
	Paid         bool
	TicketIssued bool
	CheckedIn    bool
}

type funnelKey struct {
	SalesEventID string
	SaleID       string
	TicketID     string
}

type segmentKey struct {
	SalesEventID string
	TicketID     string
	TicketType   string
	Provider     string
	WindowSize   string
	WindowStart  time.Time
}

type funnelCounts struct {
	accepted     int
	pending      int
	paid         int
	ticketIssued int
	checkedIn    int
}
