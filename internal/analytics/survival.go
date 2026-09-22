package analytics

import (
	"sort"
	"time"
)

type SurvivalIntervalSpec struct {
	StartSeconds float64
	EndSeconds   float64
}

func DefaultSurvivalIntervals() []SurvivalIntervalSpec {
	return []SurvivalIntervalSpec{
		{StartSeconds: 0, EndSeconds: 60},
		{StartSeconds: 60, EndSeconds: 300},
		{StartSeconds: 300, EndSeconds: 900},
		{StartSeconds: 900, EndSeconds: 3600},
		{StartSeconds: 3600, EndSeconds: 0},
	}
}

type SurvivalAnalysis struct {
	EventName        string                 `json:"eventName"`
	FromStage        string                 `json:"fromStage"`
	ToStage          string                 `json:"toStage"`
	UnitOfAnalysis   string                 `json:"unitOfAnalysis"`
	ObservationCount int                    `json:"observationCount"`
	EventCount       int                    `json:"eventCount"`
	CensoredCount    int                    `json:"censoredCount"`
	Percentiles      map[string]float64     `json:"percentilesSeconds,omitempty"`
	HazardTable      []SurvivalHazardBucket `json:"hazardTable"`
}

type SurvivalHazardBucket struct {
	IntervalStartSeconds float64 `json:"intervalStartSeconds"`
	IntervalEndSeconds   float64 `json:"intervalEndSeconds,omitempty"`
	AtRisk               int     `json:"atRisk"`
	Events               int     `json:"events"`
	Censored             int     `json:"censored"`
	Hazard               float64 `json:"hazard"`
	SurvivalProbability  float64 `json:"survivalProbability"`
}

type survivalObservation struct {
	DurationSeconds float64
	Observed        bool
}

type saleSurvivalRecord struct {
	SalesEventID string
	SaleID       string
	AcceptedAt   time.Time
	PaymentAt    time.Time
	EmailSentAt  time.Time
}

type ticketSurvivalRecord struct {
	SalesEventID string
	SaleID       string
	TicketID     string
	AcceptedAt   time.Time
	CheckInAt    time.Time
}

func BuildSurvivalAnalyses(events []Event, censorAt time.Time, intervals []SurvivalIntervalSpec) []SurvivalAnalysis {
	normalizedEvents := normalizeEvents(events)
	normalizedIntervals := normalizeSurvivalIntervals(intervals)
	sales, tickets := buildSurvivalRecords(normalizedEvents)
	analyses := make([]SurvivalAnalysis, 0, 3)

	if paymentObservations := paymentSurvivalObservations(sales, censorAt); len(paymentObservations) > 0 {
		analyses = append(analyses, newSurvivalAnalysis(
			"time_to_payment",
			FunnelStageAccepted,
			FunnelStagePaid,
			"sale",
			paymentObservations,
			normalizedIntervals,
		))
	}
	if emailObservations := emailSurvivalObservations(sales, censorAt); len(emailObservations) > 0 {
		analyses = append(analyses, newSurvivalAnalysis(
			"time_to_email_sent",
			FunnelStageAccepted,
			"email_sent",
			"sale",
			emailObservations,
			normalizedIntervals,
		))
	}
	if checkInObservations := checkInSurvivalObservations(tickets, censorAt); len(checkInObservations) > 0 {
		analyses = append(analyses, newSurvivalAnalysis(
			"time_to_check_in",
			FunnelStageAccepted,
			FunnelStageCheckIn,
			"sale_ticket",
			checkInObservations,
			normalizedIntervals,
		))
	}

	return analyses
}

func buildSurvivalRecords(events []Event) (map[string]*saleSurvivalRecord, map[string]*ticketSurvivalRecord) {
	sales := make(map[string]*saleSurvivalRecord)
	tickets := make(map[string]*ticketSurvivalRecord)
	for _, event := range events {
		if event.SaleID == "" {
			continue
		}
		sale := sales[event.SaleID]
		if sale == nil {
			sale = &saleSurvivalRecord{SaleID: event.SaleID}
			sales[event.SaleID] = sale
		}
		sale.SalesEventID = firstNonEmpty(sale.SalesEventID, event.SalesEventID)

		switch event.EventType {
		case "sale.created":
			if sale.AcceptedAt.IsZero() || event.OccurredAt.Before(sale.AcceptedAt) {
				sale.AcceptedAt = event.OccurredAt
			}
		case "sale.item.created":
			if sale.AcceptedAt.IsZero() || event.OccurredAt.Before(sale.AcceptedAt) {
				sale.AcceptedAt = event.OccurredAt
			}
			if event.TicketID != "" {
				ticket := ticketRecord(tickets, event)
				if ticket.AcceptedAt.IsZero() || event.OccurredAt.Before(ticket.AcceptedAt) {
					ticket.AcceptedAt = event.OccurredAt
				}
			}
		case "payment.processed":
			if event.Status == "APPROVED" && (sale.PaymentAt.IsZero() || event.OccurredAt.Before(sale.PaymentAt)) {
				sale.PaymentAt = event.OccurredAt
			}
		case "email.sent":
			if sale.EmailSentAt.IsZero() || event.OccurredAt.Before(sale.EmailSentAt) {
				sale.EmailSentAt = event.OccurredAt
			}
		case "checkin.completed":
			if event.TicketID != "" {
				ticket := ticketRecord(tickets, event)
				if ticket.CheckInAt.IsZero() || event.OccurredAt.Before(ticket.CheckInAt) {
					ticket.CheckInAt = event.OccurredAt
				}
			}
		}
	}
	return sales, tickets
}

func ticketRecord(records map[string]*ticketSurvivalRecord, event Event) *ticketSurvivalRecord {
	key := event.SaleID + "|" + event.TicketID
	record := records[key]
	if record == nil {
		record = &ticketSurvivalRecord{
			SalesEventID: event.SalesEventID,
			SaleID:       event.SaleID,
			TicketID:     event.TicketID,
		}
		records[key] = record
	}
	record.SalesEventID = firstNonEmpty(record.SalesEventID, event.SalesEventID)
	return record
}

func paymentSurvivalObservations(records map[string]*saleSurvivalRecord, censorAt time.Time) []survivalObservation {
	observations := make([]survivalObservation, 0, len(records))
	for _, record := range records {
		if record.AcceptedAt.IsZero() {
			continue
		}
		observations = append(observations, survivalObservationFromTimes(record.AcceptedAt, record.PaymentAt, censorAt))
	}
	return observations
}

func emailSurvivalObservations(records map[string]*saleSurvivalRecord, censorAt time.Time) []survivalObservation {
	observations := make([]survivalObservation, 0, len(records))
	for _, record := range records {
		if record.AcceptedAt.IsZero() {
			continue
		}
		observations = append(observations, survivalObservationFromTimes(record.AcceptedAt, record.EmailSentAt, censorAt))
	}
	return observations
}

func checkInSurvivalObservations(records map[string]*ticketSurvivalRecord, censorAt time.Time) []survivalObservation {
	observations := make([]survivalObservation, 0, len(records))
	for _, record := range records {
		if record.AcceptedAt.IsZero() {
			continue
		}
		observations = append(observations, survivalObservationFromTimes(record.AcceptedAt, record.CheckInAt, censorAt))
	}
	return observations
}

func survivalObservationFromTimes(start time.Time, eventAt time.Time, censorAt time.Time) survivalObservation {
	if !eventAt.IsZero() {
		return survivalObservation{DurationSeconds: nonNegativeSeconds(eventAt.Sub(start)), Observed: true}
	}
	return survivalObservation{DurationSeconds: nonNegativeSeconds(censorAt.Sub(start)), Observed: false}
}

func newSurvivalAnalysis(eventName string, fromStage string, toStage string, unitOfAnalysis string, observations []survivalObservation, intervals []SurvivalIntervalSpec) SurvivalAnalysis {
	eventCount := 0
	for _, observation := range observations {
		if observation.Observed {
			eventCount++
		}
	}
	return SurvivalAnalysis{
		EventName:        eventName,
		FromStage:        fromStage,
		ToStage:          toStage,
		UnitOfAnalysis:   unitOfAnalysis,
		ObservationCount: len(observations),
		EventCount:       eventCount,
		CensoredCount:    len(observations) - eventCount,
		Percentiles:      observedPercentiles(observations),
		HazardTable:      buildHazardTable(observations, intervals),
	}
}

func buildHazardTable(observations []survivalObservation, intervals []SurvivalIntervalSpec) []SurvivalHazardBucket {
	table := make([]SurvivalHazardBucket, 0, len(intervals))
	survivalProbability := 1.0
	for _, interval := range intervals {
		bucket := SurvivalHazardBucket{
			IntervalStartSeconds: interval.StartSeconds,
			IntervalEndSeconds:   interval.EndSeconds,
		}
		for _, observation := range observations {
			if observation.DurationSeconds >= interval.StartSeconds {
				bucket.AtRisk++
			}
			if durationInInterval(observation.DurationSeconds, interval) {
				if observation.Observed {
					bucket.Events++
				} else {
					bucket.Censored++
				}
			}
		}
		if bucket.AtRisk > 0 {
			bucket.Hazard = float64(bucket.Events) / float64(bucket.AtRisk)
		}
		survivalProbability *= 1 - bucket.Hazard
		bucket.SurvivalProbability = survivalProbability
		table = append(table, bucket)
	}
	return table
}

func observedPercentiles(observations []survivalObservation) map[string]float64 {
	durations := make([]float64, 0, len(observations))
	for _, observation := range observations {
		if observation.Observed {
			durations = append(durations, observation.DurationSeconds)
		}
	}
	if len(durations) == 0 {
		return nil
	}
	sort.Float64s(durations)
	return map[string]float64{
		"p50": nearestRankPercentile(durations, 0.50),
		"p90": nearestRankPercentile(durations, 0.90),
		"p95": nearestRankPercentile(durations, 0.95),
	}
}

func nearestRankPercentile(sortedValues []float64, percentile float64) float64 {
	if len(sortedValues) == 0 {
		return 0
	}
	rank := int(percentile*float64(len(sortedValues)) + 0.999999999)
	if rank < 1 {
		rank = 1
	}
	if rank > len(sortedValues) {
		rank = len(sortedValues)
	}
	return sortedValues[rank-1]
}

func normalizeSurvivalIntervals(intervals []SurvivalIntervalSpec) []SurvivalIntervalSpec {
	if len(intervals) == 0 {
		return DefaultSurvivalIntervals()
	}
	normalized := make([]SurvivalIntervalSpec, 0, len(intervals))
	for _, interval := range intervals {
		if interval.StartSeconds < 0 || (interval.EndSeconds > 0 && interval.EndSeconds <= interval.StartSeconds) {
			continue
		}
		normalized = append(normalized, interval)
	}
	if len(normalized) == 0 {
		return DefaultSurvivalIntervals()
	}
	return normalized
}

func durationInInterval(durationSeconds float64, interval SurvivalIntervalSpec) bool {
	if durationSeconds < interval.StartSeconds {
		return false
	}
	if interval.EndSeconds == 0 {
		return true
	}
	return durationSeconds < interval.EndSeconds
}

func nonNegativeSeconds(duration time.Duration) float64 {
	if duration < 0 {
		return 0
	}
	return duration.Seconds()
}
