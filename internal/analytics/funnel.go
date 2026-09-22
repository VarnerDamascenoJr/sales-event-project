package analytics

import (
	"math"
	"sort"
	"strings"
	"time"
)

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

func BuildFunnelSegments(events []Event, specs []WindowSpec, options FunnelOptions) []FunnelSegment {
	normalizedSpecs := normalizeWindowSpecs(specs)
	normalizedOptions := normalizeFunnelOptions(options)
	records := buildFunnelRecords(events)
	if len(records) == 0 {
		return nil
	}

	segments := make(map[segmentKey]*funnelCounts)
	keys := make([]segmentKey, 0)
	for _, record := range records {
		if !record.Accepted {
			continue
		}
		for _, spec := range normalizedSpecs {
			start := record.AcceptedAt.Truncate(spec.Duration)
			key := segmentKey{
				SalesEventID: record.SalesEventID,
				TicketID:     record.TicketID,
				TicketType:   record.TicketType,
				Provider:     providerOrUnknown(record.Provider),
				WindowSize:   spec.Name,
				WindowStart:  start,
			}
			counts := segments[key]
			if counts == nil {
				counts = &funnelCounts{}
				segments[key] = counts
				keys = append(keys, key)
			}
			addRecordToFunnelCounts(counts, record)
		}
	}

	sort.Slice(keys, func(i, j int) bool {
		left := keys[i]
		right := keys[j]
		if !left.WindowStart.Equal(right.WindowStart) {
			return left.WindowStart.Before(right.WindowStart)
		}
		if left.WindowSize != right.WindowSize {
			return left.WindowSize < right.WindowSize
		}
		if left.SalesEventID != right.SalesEventID {
			return left.SalesEventID < right.SalesEventID
		}
		if left.TicketType != right.TicketType {
			return left.TicketType < right.TicketType
		}
		return left.Provider < right.Provider
	})

	segmentsOut := make([]FunnelSegment, 0, len(keys))
	for _, key := range keys {
		spec := windowSpecByName(normalizedSpecs, key.WindowSize)
		counts := segments[key]
		segmentsOut = append(segmentsOut, FunnelSegment{
			SalesEventID: key.SalesEventID,
			TicketID:     key.TicketID,
			TicketType:   key.TicketType,
			Provider:     key.Provider,
			WindowSize:   key.WindowSize,
			WindowStart:  key.WindowStart,
			WindowEnd:    key.WindowStart.Add(spec.Duration),
			Steps:        buildFunnelSteps(*counts, normalizedOptions),
		})
	}
	return segmentsOut
}

func normalizeFunnelOptions(options FunnelOptions) FunnelOptions {
	defaults := DefaultFunnelOptions()
	if options.ConfidenceLevel == 0 {
		options.ConfidenceLevel = defaults.ConfidenceLevel
	}
	if options.WilsonZ == 0 {
		options.WilsonZ = defaults.WilsonZ
	}
	if options.PriorAlpha <= 0 {
		options.PriorAlpha = defaults.PriorAlpha
	}
	if options.PriorBeta <= 0 {
		options.PriorBeta = defaults.PriorBeta
	}
	return options
}

func buildFunnelRecords(events []Event) []funnelRecord {
	recordsByKey := make(map[funnelKey]*funnelRecord)
	saleDefaults := make(map[string]*funnelRecord)

	for _, event := range normalizeEvents(events) {
		if event.SaleID == "" {
			continue
		}
		if event.EventType == "sale.created" {
			record := saleDefaults[event.SaleID]
			if record == nil {
				record = &funnelRecord{SaleID: event.SaleID}
				saleDefaults[event.SaleID] = record
			}
			record.SalesEventID = event.SalesEventID
			record.AcceptedAt = event.OccurredAt
			record.Accepted = true
			record.Pending = saleReachedPending(event.Status)
			continue
		}
		if event.EventType == "payment.processed" {
			applyToSaleRecords(recordsByKey, event.SaleID, func(record *funnelRecord) {
				record.Provider = event.Provider
				if strings.EqualFold(event.Status, "APPROVED") {
					record.Paid = true
				}
			})
			if saleDefaults[event.SaleID] == nil {
				saleDefaults[event.SaleID] = &funnelRecord{SaleID: event.SaleID}
			}
			saleDefaults[event.SaleID].Provider = event.Provider
			if strings.EqualFold(event.Status, "APPROVED") {
				saleDefaults[event.SaleID].Paid = true
			}
			continue
		}

		if event.TicketID == "" {
			continue
		}
		key := funnelKey{SalesEventID: event.SalesEventID, SaleID: event.SaleID, TicketID: event.TicketID}
		record := recordsByKey[key]
		if record == nil {
			defaultRecord := saleDefaults[event.SaleID]
			record = &funnelRecord{
				SalesEventID: event.SalesEventID,
				SaleID:       event.SaleID,
				TicketID:     event.TicketID,
				TicketType:   event.TicketType,
			}
			if defaultRecord != nil {
				record.SalesEventID = firstNonEmpty(record.SalesEventID, defaultRecord.SalesEventID)
				record.AcceptedAt = defaultRecord.AcceptedAt
				record.Accepted = defaultRecord.Accepted
				record.Pending = defaultRecord.Pending
				record.Paid = defaultRecord.Paid
				record.Provider = defaultRecord.Provider
			}
			recordsByKey[key] = record
		}
		record.TicketType = firstNonEmpty(record.TicketType, event.TicketType)
		record.SalesEventID = firstNonEmpty(record.SalesEventID, event.SalesEventID)

		switch event.EventType {
		case "sale.item.created":
			record.Accepted = true
			record.Pending = record.Pending || saleReachedPending(event.Status)
			if record.AcceptedAt.IsZero() || event.OccurredAt.Before(record.AcceptedAt) {
				record.AcceptedAt = event.OccurredAt
			}
		case "ticket.issued":
			record.TicketIssued = true
		case "checkin.completed":
			record.CheckedIn = true
		}
	}

	records := make([]funnelRecord, 0, len(recordsByKey))
	for _, record := range recordsByKey {
		if record.AcceptedAt.IsZero() {
			continue
		}
		record.Provider = providerOrUnknown(record.Provider)
		records = append(records, *record)
	}
	sort.Slice(records, func(i, j int) bool {
		left := records[i]
		right := records[j]
		if !left.AcceptedAt.Equal(right.AcceptedAt) {
			return left.AcceptedAt.Before(right.AcceptedAt)
		}
		if left.SaleID != right.SaleID {
			return left.SaleID < right.SaleID
		}
		return left.TicketID < right.TicketID
	})
	return records
}

func applyToSaleRecords(records map[funnelKey]*funnelRecord, saleID string, apply func(*funnelRecord)) {
	for key, record := range records {
		if key.SaleID == saleID {
			apply(record)
		}
	}
}

func addRecordToFunnelCounts(counts *funnelCounts, record funnelRecord) {
	counts.accepted++
	if record.Pending {
		counts.pending++
	}
	if record.Paid {
		counts.paid++
	}
	if record.TicketIssued {
		counts.ticketIssued++
	}
	if record.CheckedIn {
		counts.checkedIn++
	}
}

func buildFunnelSteps(counts funnelCounts, options FunnelOptions) []FunnelStep {
	return []FunnelStep{
		newFunnelStep(FunnelStageAccepted, FunnelStagePending, counts.accepted, counts.pending, options),
		newFunnelStep(FunnelStagePending, FunnelStagePaid, counts.pending, counts.paid, options),
		newFunnelStep(FunnelStagePaid, FunnelStageTicketIssued, counts.paid, counts.ticketIssued, options),
		newFunnelStep(FunnelStageTicketIssued, FunnelStageCheckIn, counts.ticketIssued, counts.checkedIn, options),
	}
}

func newFunnelStep(from string, to string, trials int, successes int, options FunnelOptions) FunnelStep {
	return FunnelStep{
		From:                  from,
		To:                    to,
		Trials:                trials,
		Successes:             successes,
		ConversionProbability: proportion(successes, trials),
		ConfidenceInterval:    wilsonInterval(successes, trials, options),
		Bayesian:              betaPosterior(successes, trials, options),
	}
}

func wilsonInterval(successes int, trials int, options FunnelOptions) ConfidenceInterval {
	interval := ConfidenceInterval{
		Method: "wilson",
		Level:  options.ConfidenceLevel,
	}
	if trials == 0 {
		return interval
	}
	n := float64(trials)
	p := float64(successes) / n
	z := options.WilsonZ
	z2 := z * z
	denominator := 1 + z2/n
	center := (p + z2/(2*n)) / denominator
	margin := z * math.Sqrt((p*(1-p)+z2/(4*n))/n) / denominator
	interval.Lower = clampProbability(center - margin)
	interval.Upper = clampProbability(center + margin)
	return interval
}

func betaPosterior(successes int, trials int, options FunnelOptions) BetaBinomial {
	failures := trials - successes
	if failures < 0 {
		failures = 0
	}
	posteriorAlpha := options.PriorAlpha + float64(successes)
	posteriorBeta := options.PriorBeta + float64(failures)
	return BetaBinomial{
		PriorAlpha:     options.PriorAlpha,
		PriorBeta:      options.PriorBeta,
		PosteriorAlpha: posteriorAlpha,
		PosteriorBeta:  posteriorBeta,
		PosteriorMean:  posteriorAlpha / (posteriorAlpha + posteriorBeta),
	}
}

func proportion(successes int, trials int) float64 {
	if trials == 0 {
		return 0
	}
	return float64(successes) / float64(trials)
}

func clampProbability(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func saleReachedPending(status string) bool {
	switch strings.ToUpper(status) {
	case "PENDING_PAYMENT", "COMPLETED":
		return true
	default:
		return false
	}
}

func providerOrUnknown(provider string) string {
	if provider == "" {
		return "unknown"
	}
	return provider
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func windowSpecByName(specs []WindowSpec, name string) WindowSpec {
	for _, spec := range specs {
		if spec.Name == name {
			return spec
		}
	}
	return WindowSpec{Name: name, Duration: time.Minute}
}
