package analytics

import "time"

const (
	FunnelStageAccepted     = "accepted"
	FunnelStagePending      = "pending_payment"
	FunnelStagePaid         = "paid"
	FunnelStageTicketIssued = "ticket_issued"
	FunnelStageCheckIn      = "check_in"
)

type FunnelOptions struct {
	ConfidenceLevel float64
	WilsonZ         float64
	PriorAlpha      float64
	PriorBeta       float64
}

func DefaultFunnelOptions() FunnelOptions {
	return FunnelOptions{
		ConfidenceLevel: 0.95,
		WilsonZ:         1.959963984540054,
		PriorAlpha:      1,
		PriorBeta:       1,
	}
}

type FunnelSegment struct {
	SalesEventID string       `json:"salesEventId,omitempty"`
	TicketID     string       `json:"ticketId,omitempty"`
	TicketType   string       `json:"ticketType,omitempty"`
	Provider     string       `json:"provider"`
	WindowSize   string       `json:"windowSize"`
	WindowStart  time.Time    `json:"windowStart"`
	WindowEnd    time.Time    `json:"windowEnd"`
	Steps        []FunnelStep `json:"steps"`
}

type FunnelStep struct {
	From                  string             `json:"from"`
	To                    string             `json:"to"`
	Trials                int                `json:"trials"`
	Successes             int                `json:"successes"`
	ConversionProbability float64            `json:"conversionProbability"`
	ConfidenceInterval    ConfidenceInterval `json:"confidenceInterval"`
	Bayesian              BetaBinomial       `json:"bayesian"`
}

type ConfidenceInterval struct {
	Method string  `json:"method"`
	Level  float64 `json:"level"`
	Lower  float64 `json:"lower"`
	Upper  float64 `json:"upper"`
}

type BetaBinomial struct {
	PriorAlpha     float64 `json:"priorAlpha"`
	PriorBeta      float64 `json:"priorBeta"`
	PosteriorAlpha float64 `json:"posteriorAlpha"`
	PosteriorBeta  float64 `json:"posteriorBeta"`
	PosteriorMean  float64 `json:"posteriorMean"`
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
