package analytics

import (
	"math"
	"sort"
	"strings"
	"time"
)

const SimulationPriorsSchemaVersion = "optiflow-sales-priors.v1"

type SimulationPriors struct {
	SchemaVersion     string                     `json:"schemaVersion"`
	GeneratedAt       time.Time                  `json:"generatedAt"`
	Source            SimulationPriorSource      `json:"source"`
	Period            SimulationPriorPeriod      `json:"period"`
	SampleSize        SimulationPriorSampleSize  `json:"sampleSize"`
	Estimates         SimulationPriorEstimates   `json:"estimates"`
	Uncertainty       SimulationPriorUncertainty `json:"uncertainty"`
	ConversionStages  []ConversionStagePrior     `json:"conversionStages,omitempty"`
	Demand            []DemandPrior              `json:"demand,omitempty"`
	OperationalTiming []OperationalTimingPrior   `json:"operationalTiming,omitempty"`
	StockoutRisks     []StockoutRiskPrior        `json:"stockoutRisks,omitempty"`
}

type SimulationPriorSource struct {
	Service       string `json:"service"`
	SchemaVersion string `json:"schemaVersion"`
	SalesEventID  string `json:"salesEventId,omitempty"`
	Filter        string `json:"filter,omitempty"`
}

type SimulationPriorPeriod struct {
	StartedAt string `json:"startedAt"`
	EndedAt   string `json:"endedAt"`
}

type SimulationPriorSampleSize struct {
	SaleCount             int `json:"saleCount"`
	CompletedSaleCount    int `json:"completedSaleCount"`
	CancelledSaleCount    int `json:"cancelledSaleCount"`
	CompletedDemandCount  int `json:"completedDemandCount"`
	TotalItemQuantity     int `json:"totalItemQuantity"`
	CompletedItemQuantity int `json:"completedItemQuantity"`
}

type SimulationPriorEstimates struct {
	ConversionProbability        float64 `json:"conversionProbability"`
	CancellationProbability      float64 `json:"cancellationProbability"`
	DemandMean                   float64 `json:"demandMean"`
	DemandStdDev                 float64 `json:"demandStdDev"`
	DemandCoefficientOfVariation float64 `json:"demandCoefficientOfVariation"`
	ModalDemand                  int     `json:"modalDemand"`
	DemandDeviationProbability   float64 `json:"demandDeviationProbability"`
}

type SimulationPriorUncertainty struct {
	CancellationProbability    float64 `json:"cancellationProbability"`
	DemandVariationProbability float64 `json:"demandVariationProbability"`
	DemandVariationRate        float64 `json:"demandVariationRate"`
}

type ConversionStagePrior struct {
	From                  string             `json:"from"`
	To                    string             `json:"to"`
	Trials                int                `json:"trials"`
	Successes             int                `json:"successes"`
	ConversionProbability float64            `json:"conversionProbability"`
	ConfidenceInterval    ConfidenceInterval `json:"confidenceInterval"`
	Bayesian              BetaBinomial       `json:"bayesian"`
}

type DemandPrior struct {
	SalesEventID     string                `json:"salesEventId,omitempty"`
	TicketID         string                `json:"ticketId,omitempty"`
	TicketType       string                `json:"ticketType,omitempty"`
	WindowSize       string                `json:"windowSize"`
	Method           string                `json:"method"`
	ObservedWindows  int                   `json:"observedWindows"`
	ForecastQuantity float64               `json:"forecastQuantity"`
	Metrics          DemandForecastMetrics `json:"metrics"`
}

type OperationalTimingPrior struct {
	EventName        string             `json:"eventName"`
	FromStage        string             `json:"fromStage"`
	ToStage          string             `json:"toStage"`
	UnitOfAnalysis   string             `json:"unitOfAnalysis"`
	ObservationCount int                `json:"observationCount"`
	EventCount       int                `json:"eventCount"`
	CensoredCount    int                `json:"censoredCount"`
	Percentiles      map[string]float64 `json:"percentilesSeconds,omitempty"`
}

type StockoutRiskPrior struct {
	SalesEventID        string  `json:"salesEventId,omitempty"`
	TicketID            string  `json:"ticketId"`
	TicketType          string  `json:"ticketType,omitempty"`
	AvailableQuantity   int     `json:"availableQuantity"`
	WindowSize          string  `json:"windowSize"`
	HorizonWindowCount  int     `json:"horizonWindowCount"`
	ExpectedDemand      float64 `json:"expectedDemand"`
	StockoutProbability float64 `json:"stockoutProbability"`
	RiskBand            string  `json:"riskBand"`
	Status              string  `json:"status"`
}

type salePriorRecord struct {
	CreatedAt time.Time
	Completed bool
	Demand    int
}

func BuildSimulationPriors(generatedAt time.Time, source Source, events []Event, funnels []FunnelSegment, survivalAnalyses []SurvivalAnalysis, demandForecasts []DemandForecast, stockoutRisks []StockoutRisk) *SimulationPriors {
	records := buildSalePriorRecords(events)
	if len(records) == 0 {
		return nil
	}

	saleIDs := make([]string, 0, len(records))
	for saleID := range records {
		saleIDs = append(saleIDs, saleID)
	}
	sort.Strings(saleIDs)

	period := simulationPriorPeriod(records)
	sampleSize, completedDemand := simulationPriorSampleSize(records, saleIDs)
	estimates := simulationPriorEstimates(sampleSize, completedDemand)
	return &SimulationPriors{
		SchemaVersion: SimulationPriorsSchemaVersion,
		GeneratedAt:   generatedAt.UTC(),
		Source: SimulationPriorSource{
			Service:       firstNonEmpty(source.Service, "sales-event-project"),
			SchemaVersion: SchemaVersion,
			SalesEventID:  source.SalesEventID,
			Filter:        source.Filter,
		},
		Period:            period,
		SampleSize:        sampleSize,
		Estimates:         estimates,
		Uncertainty:       simulationPriorUncertainty(estimates),
		ConversionStages:  aggregateConversionStagePriors(funnels),
		Demand:            demandPriors(demandForecasts),
		OperationalTiming: operationalTimingPriors(survivalAnalyses),
		StockoutRisks:     stockoutRiskPriors(stockoutRisks),
	}
}

func buildSalePriorRecords(events []Event) map[string]*salePriorRecord {
	records := make(map[string]*salePriorRecord)
	for _, event := range events {
		if event.SaleID == "" {
			continue
		}
		record := records[event.SaleID]
		if record == nil {
			record = &salePriorRecord{}
			records[event.SaleID] = record
		}
		switch event.EventType {
		case "sale.created":
			if record.CreatedAt.IsZero() || event.OccurredAt.Before(record.CreatedAt) {
				record.CreatedAt = event.OccurredAt
			}
			if strings.EqualFold(event.Status, "COMPLETED") {
				record.Completed = true
			}
		case "payment.processed":
			if strings.EqualFold(event.Status, "APPROVED") {
				record.Completed = true
			}
		case "sale.item.created":
			record.Demand += eventQuantity(event)
		}
	}
	return records
}

func simulationPriorPeriod(records map[string]*salePriorRecord) SimulationPriorPeriod {
	var start time.Time
	var end time.Time
	for _, record := range records {
		if record.CreatedAt.IsZero() {
			continue
		}
		if start.IsZero() || record.CreatedAt.Before(start) {
			start = record.CreatedAt
		}
		if end.IsZero() || record.CreatedAt.After(end) {
			end = record.CreatedAt
		}
	}
	return SimulationPriorPeriod{
		StartedAt: formatPriorTime(start),
		EndedAt:   formatPriorTime(end),
	}
}

func simulationPriorSampleSize(records map[string]*salePriorRecord, saleIDs []string) (SimulationPriorSampleSize, []int) {
	sampleSize := SimulationPriorSampleSize{SaleCount: len(records)}
	completedDemand := make([]int, 0)
	for _, saleID := range saleIDs {
		record := records[saleID]
		sampleSize.TotalItemQuantity += record.Demand
		if !record.Completed {
			continue
		}
		sampleSize.CompletedSaleCount++
		sampleSize.CompletedItemQuantity += record.Demand
		if record.Demand > 0 {
			sampleSize.CompletedDemandCount++
			completedDemand = append(completedDemand, record.Demand)
		}
	}
	sampleSize.CancelledSaleCount = sampleSize.SaleCount - sampleSize.CompletedSaleCount
	return sampleSize, completedDemand
}

func simulationPriorEstimates(sampleSize SimulationPriorSampleSize, completedDemand []int) SimulationPriorEstimates {
	demandMean := meanFloat64(completedDemand)
	demandStdDev := populationStdDev(completedDemand, demandMean)
	demandCoefficientOfVariation := 0.0
	if demandMean > 0 {
		demandCoefficientOfVariation = demandStdDev / demandMean
	}
	modalDemand := intMode(completedDemand)
	deviationProbability := 0.0
	if len(completedDemand) > 0 {
		deviations := 0
		for _, demand := range completedDemand {
			if demand != modalDemand {
				deviations++
			}
		}
		deviationProbability = float64(deviations) / float64(len(completedDemand))
	}

	conversionProbability := 0.0
	cancellationProbability := 0.0
	if sampleSize.SaleCount > 0 {
		conversionProbability = float64(sampleSize.CompletedSaleCount) / float64(sampleSize.SaleCount)
		cancellationProbability = float64(sampleSize.CancelledSaleCount) / float64(sampleSize.SaleCount)
	}

	return SimulationPriorEstimates{
		ConversionProbability:        round4(conversionProbability),
		CancellationProbability:      round4(cancellationProbability),
		DemandMean:                   round4(demandMean),
		DemandStdDev:                 round4(demandStdDev),
		DemandCoefficientOfVariation: round4(demandCoefficientOfVariation),
		ModalDemand:                  modalDemand,
		DemandDeviationProbability:   round4(deviationProbability),
	}
}

func simulationPriorUncertainty(estimates SimulationPriorEstimates) SimulationPriorUncertainty {
	return SimulationPriorUncertainty{
		CancellationProbability:    estimates.CancellationProbability,
		DemandVariationProbability: estimates.DemandDeviationProbability,
		DemandVariationRate:        estimates.DemandCoefficientOfVariation,
	}
}

func aggregateConversionStagePriors(funnels []FunnelSegment) []ConversionStagePrior {
	if len(funnels) == 0 {
		return nil
	}
	type key struct {
		from string
		to   string
	}
	type counts struct {
		trials    int
		successes int
	}
	byKey := make(map[key]*counts)
	keys := make([]key, 0)
	for _, segment := range funnels {
		for _, step := range segment.Steps {
			k := key{from: step.From, to: step.To}
			current := byKey[k]
			if current == nil {
				current = &counts{}
				byKey[k] = current
				keys = append(keys, k)
			}
			current.trials += step.Trials
			current.successes += step.Successes
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].from != keys[j].from {
			return keys[i].from < keys[j].from
		}
		return keys[i].to < keys[j].to
	})

	options := DefaultFunnelOptions()
	priors := make([]ConversionStagePrior, 0, len(keys))
	for _, k := range keys {
		count := byKey[k]
		priors = append(priors, ConversionStagePrior{
			From:                  k.from,
			To:                    k.to,
			Trials:                count.trials,
			Successes:             count.successes,
			ConversionProbability: round4(proportion(count.successes, count.trials)),
			ConfidenceInterval:    wilsonInterval(count.successes, count.trials, options),
			Bayesian:              betaPosterior(count.successes, count.trials, options),
		})
	}
	return priors
}

func demandPriors(forecasts []DemandForecast) []DemandPrior {
	if len(forecasts) == 0 {
		return nil
	}
	priors := make([]DemandPrior, 0, len(forecasts))
	for _, forecast := range forecasts {
		observedWindows := forecast.TrainWindowCount + forecast.TestWindowCount
		priors = append(priors, DemandPrior{
			SalesEventID:     forecast.SalesEventID,
			TicketID:         forecast.TicketID,
			TicketType:       forecast.TicketType,
			WindowSize:       forecast.WindowSize,
			Method:           forecast.Method,
			ObservedWindows:  observedWindows,
			ForecastQuantity: forecast.ForecastQuantity,
			Metrics:          forecast.Metrics,
		})
	}
	return priors
}

func operationalTimingPriors(analyses []SurvivalAnalysis) []OperationalTimingPrior {
	if len(analyses) == 0 {
		return nil
	}
	priors := make([]OperationalTimingPrior, 0, len(analyses))
	for _, analysis := range analyses {
		priors = append(priors, OperationalTimingPrior{
			EventName:        analysis.EventName,
			FromStage:        analysis.FromStage,
			ToStage:          analysis.ToStage,
			UnitOfAnalysis:   analysis.UnitOfAnalysis,
			ObservationCount: analysis.ObservationCount,
			EventCount:       analysis.EventCount,
			CensoredCount:    analysis.CensoredCount,
			Percentiles:      copyFloatMap(analysis.Percentiles),
		})
	}
	return priors
}

func stockoutRiskPriors(risks []StockoutRisk) []StockoutRiskPrior {
	if len(risks) == 0 {
		return nil
	}
	priors := make([]StockoutRiskPrior, 0, len(risks))
	for _, risk := range risks {
		priors = append(priors, StockoutRiskPrior{
			SalesEventID:        risk.SalesEventID,
			TicketID:            risk.TicketID,
			TicketType:          risk.TicketType,
			AvailableQuantity:   risk.AvailableQuantity,
			WindowSize:          risk.WindowSize,
			HorizonWindowCount:  risk.HorizonWindowCount,
			ExpectedDemand:      risk.ExpectedDemand,
			StockoutProbability: risk.StockoutProbability,
			RiskBand:            risk.RiskBand,
			Status:              risk.Status,
		})
	}
	return priors
}

func eventQuantity(event Event) int {
	if event.Quantity <= 0 {
		return 1
	}
	return event.Quantity
}

func formatPriorTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}

func meanFloat64(values []int) float64 {
	if len(values) == 0 {
		return 0
	}
	total := 0
	for _, value := range values {
		total += value
	}
	return float64(total) / float64(len(values))
}

func populationStdDev(values []int, mean float64) float64 {
	if len(values) == 0 {
		return 0
	}
	var sumSquaredDiffs float64
	for _, value := range values {
		diff := float64(value) - mean
		sumSquaredDiffs += diff * diff
	}
	return math.Sqrt(sumSquaredDiffs / float64(len(values)))
}

func intMode(values []int) int {
	if len(values) == 0 {
		return 0
	}
	counts := make(map[int]int)
	for _, value := range values {
		counts[value]++
	}
	mode := values[0]
	for value, count := range counts {
		if count > counts[mode] || (count == counts[mode] && value < mode) {
			mode = value
		}
	}
	return mode
}

func copyFloatMap(values map[string]float64) map[string]float64 {
	if len(values) == 0 {
		return nil
	}
	copied := make(map[string]float64, len(values))
	for key, value := range values {
		copied[key] = value
	}
	return copied
}

func round4(value float64) float64 {
	return math.Round(value*10000) / 10000
}
