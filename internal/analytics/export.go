package analytics

import (
	"sort"
	"strings"
	"time"
)

const SchemaVersion = "sales-analytics-export.v1"

type WindowSpec struct {
	Name     string
	Duration time.Duration
}

func DefaultWindowSpecs() []WindowSpec {
	return []WindowSpec{
		{Name: "1m", Duration: time.Minute},
		{Name: "5m", Duration: 5 * time.Minute},
		{Name: "1h", Duration: time.Hour},
		{Name: "1d", Duration: 24 * time.Hour},
	}
}

type Document struct {
	SchemaVersion string            `json:"schemaVersion"`
	GeneratedAt   time.Time         `json:"generatedAt"`
	Source        Source            `json:"source"`
	Summary       Summary           `json:"summary"`
	Events        []Event           `json:"events"`
	Windows       []WindowAggregate `json:"windows"`
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

func BuildDocument(generatedAt time.Time, source Source, events []Event, specs []WindowSpec) Document {
	normalizedEvents := normalizeEvents(events)
	normalizedSpecs := normalizeWindowSpecs(specs)
	source.Service = "sales-event-project"
	source.WindowSizes = windowSizeNames(normalizedSpecs)

	summary := Summary{
		EventCount:      len(normalizedEvents),
		EventTypeCounts: make(map[string]int),
	}
	for _, event := range normalizedEvents {
		summary.EventTypeCounts[event.EventType]++
	}

	windows := buildWindows(normalizedEvents, normalizedSpecs)
	summary.WindowCount = len(windows)

	return Document{
		SchemaVersion: SchemaVersion,
		GeneratedAt:   generatedAt.UTC(),
		Source:        source,
		Summary:       summary,
		Events:        normalizedEvents,
		Windows:       windows,
	}
}

func normalizeEvents(events []Event) []Event {
	normalized := make([]Event, len(events))
	copy(normalized, events)
	for i := range normalized {
		normalized[i].OccurredAt = normalized[i].OccurredAt.UTC()
	}
	sort.SliceStable(normalized, func(i, j int) bool {
		left := normalized[i]
		right := normalized[j]
		if !left.OccurredAt.Equal(right.OccurredAt) {
			return left.OccurredAt.Before(right.OccurredAt)
		}
		if left.EventType != right.EventType {
			return left.EventType < right.EventType
		}
		if left.SaleID != right.SaleID {
			return left.SaleID < right.SaleID
		}
		return left.EventID < right.EventID
	})
	return normalized
}

func normalizeWindowSpecs(specs []WindowSpec) []WindowSpec {
	if len(specs) == 0 {
		return DefaultWindowSpecs()
	}

	normalized := make([]WindowSpec, 0, len(specs))
	for _, spec := range specs {
		if spec.Name == "" || spec.Duration <= 0 {
			continue
		}
		normalized = append(normalized, spec)
	}
	if len(normalized) == 0 {
		return DefaultWindowSpecs()
	}
	return normalized
}

func windowSizeNames(specs []WindowSpec) []string {
	names := make([]string, 0, len(specs))
	for _, spec := range specs {
		names = append(names, spec.Name)
	}
	return names
}

func buildWindows(events []Event, specs []WindowSpec) []WindowAggregate {
	byKey := make(map[string]*WindowAggregate)
	keys := make([]string, 0)

	for _, spec := range specs {
		for _, event := range events {
			start := event.OccurredAt.Truncate(spec.Duration)
			key := spec.Name + "|" + start.Format(time.RFC3339Nano)
			window := byKey[key]
			if window == nil {
				aggregate := WindowAggregate{
					WindowSize:  spec.Name,
					WindowStart: start,
					WindowEnd:   start.Add(spec.Duration),
					Counts:      newAggregateCounts(),
				}
				window = &aggregate
				byKey[key] = window
				keys = append(keys, key)
			}
			addEventToAggregate(&window.Counts, event)
		}
	}

	sort.Slice(keys, func(i, j int) bool {
		left := byKey[keys[i]]
		right := byKey[keys[j]]
		if !left.WindowStart.Equal(right.WindowStart) {
			return left.WindowStart.Before(right.WindowStart)
		}
		return left.WindowSize < right.WindowSize
	})

	windows := make([]WindowAggregate, 0, len(keys))
	for _, key := range keys {
		window := *byKey[key]
		trimEmptyCountMaps(&window.Counts)
		windows = append(windows, window)
	}
	return windows
}

func newAggregateCounts() AggregateCounts {
	return AggregateCounts{
		EventTypeCounts:      make(map[string]int),
		SaleStatusCounts:     make(map[string]int),
		PaymentStatusCounts:  make(map[string]int),
		OutboxStatusCounts:   make(map[string]int),
		EmailStatusCounts:    make(map[string]int),
		TicketQuantityByType: make(map[string]int),
	}
}

func addEventToAggregate(counts *AggregateCounts, event Event) {
	counts.TotalEvents++
	counts.EventTypeCounts[event.EventType]++
	counts.TotalAmountCents += event.AmountCents

	if event.TicketType != "" && event.Quantity > 0 {
		counts.TicketQuantityByType[event.TicketType] += event.Quantity
	}

	switch {
	case strings.HasPrefix(event.EventType, "sale.") && event.Status != "":
		counts.SaleStatusCounts[event.Status]++
	case strings.HasPrefix(event.EventType, "payment.") && event.Status != "":
		counts.PaymentStatusCounts[event.Status]++
	case strings.HasPrefix(event.EventType, "outbox.") && event.Status != "":
		counts.OutboxStatusCounts[event.Status]++
	case strings.HasPrefix(event.EventType, "email.") && event.Status != "":
		counts.EmailStatusCounts[event.Status]++
	case strings.HasPrefix(event.EventType, "checkin."):
		counts.CheckInCount++
	}
}

func trimEmptyCountMaps(counts *AggregateCounts) {
	if len(counts.SaleStatusCounts) == 0 {
		counts.SaleStatusCounts = nil
	}
	if len(counts.PaymentStatusCounts) == 0 {
		counts.PaymentStatusCounts = nil
	}
	if len(counts.OutboxStatusCounts) == 0 {
		counts.OutboxStatusCounts = nil
	}
	if len(counts.EmailStatusCounts) == 0 {
		counts.EmailStatusCounts = nil
	}
	if len(counts.TicketQuantityByType) == 0 {
		counts.TicketQuantityByType = nil
	}
}
