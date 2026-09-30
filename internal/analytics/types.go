package analytics

import "time"

type WindowSpec struct {
	Name     string
	Duration time.Duration
}

type Document struct {
	SchemaVersion    string             `json:"schemaVersion"`
	GeneratedAt      time.Time          `json:"generatedAt"`
	Source           Source             `json:"source"`
	Summary          Summary            `json:"summary"`
	Events           []Event            `json:"events"`
	Windows          []WindowAggregate  `json:"windows"`
	Funnels          []FunnelSegment    `json:"funnels,omitempty"`
	SurvivalAnalyses []SurvivalAnalysis `json:"survivalAnalyses,omitempty"`
	DemandForecasts  []DemandForecast   `json:"demandForecasts,omitempty"`
	StockoutRisks    []StockoutRisk     `json:"stockoutRisks,omitempty"`
	SimulationPriors *SimulationPriors  `json:"simulationPriors,omitempty"`
}

type Source struct {
	Service      string   `json:"service"`
	SalesEventID string   `json:"salesEventId,omitempty"`
	Filter       string   `json:"filter,omitempty"`
	WindowSizes  []string `json:"windowSizes"`
}

type Summary struct {
	EventCount      int            `json:"eventCount"`
	WindowCount     int            `json:"windowCount"`
	EventTypeCounts map[string]int `json:"eventTypeCounts"`
}

type Event struct {
	EventID         string    `json:"eventId,omitempty"`
	EventType       string    `json:"eventType"`
	OccurredAt      time.Time `json:"occurredAt"`
	SalesEventID    string    `json:"salesEventId,omitempty"`
	SaleID          string    `json:"saleId,omitempty"`
	TicketID        string    `json:"ticketId,omitempty"`
	TicketType      string    `json:"ticketType,omitempty"`
	Status          string    `json:"status,omitempty"`
	Provider        string    `json:"provider,omitempty"`
	OutboxEventType string    `json:"outboxEventType,omitempty"`
	AmountCents     int       `json:"amountCents,omitempty"`
	Quantity        int       `json:"quantity,omitempty"`
	Attempts        int       `json:"attempts,omitempty"`
	RequestID       string    `json:"requestId,omitempty"`
	CorrelationID   string    `json:"correlationId,omitempty"`
	TransactionID   string    `json:"transactionId,omitempty"`
}

type WindowAggregate struct {
	WindowSize  string          `json:"windowSize"`
	WindowStart time.Time       `json:"windowStart"`
	WindowEnd   time.Time       `json:"windowEnd"`
	Counts      AggregateCounts `json:"counts"`
}

type AggregateCounts struct {
	TotalEvents          int            `json:"totalEvents"`
	EventTypeCounts      map[string]int `json:"eventTypeCounts"`
	SaleStatusCounts     map[string]int `json:"saleStatusCounts,omitempty"`
	PaymentStatusCounts  map[string]int `json:"paymentStatusCounts,omitempty"`
	OutboxStatusCounts   map[string]int `json:"outboxStatusCounts,omitempty"`
	EmailStatusCounts    map[string]int `json:"emailStatusCounts,omitempty"`
	CheckInCount         int            `json:"checkInCount,omitempty"`
	TicketQuantityByType map[string]int `json:"ticketQuantityByType,omitempty"`
	TotalAmountCents     int            `json:"totalAmountCents,omitempty"`
}
