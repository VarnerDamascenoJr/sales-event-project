package analytics

import (
	"math"
	"sort"
	"time"
)

const (
	StockoutRiskMethodPoissonRate = "poisson_rate"

	StockoutRiskStatusEstimated           = "estimated"
	StockoutRiskStatusInsufficientHistory = "insufficient_history"
	StockoutRiskStatusNoObservedDemand    = "no_observed_demand"
	StockoutRiskStatusStockout            = "stockout"

	StockoutRiskBandLow      = "low"
	StockoutRiskBandMedium   = "medium"
	StockoutRiskBandHigh     = "high"
	StockoutRiskBandCritical = "critical"
)

type TicketInventory struct {
	SalesEventID      string `json:"salesEventId,omitempty"`
	TicketID          string `json:"ticketId"`
	TicketType        string `json:"ticketType,omitempty"`
	AvailableQuantity int    `json:"availableQuantity"`
}

type StockoutRiskOptions struct {
	WindowSize             string
	HorizonWindows         int
	MinimumObservedWindows int
	MaxSeriesWindows       int
}

func DefaultStockoutRiskOptions() StockoutRiskOptions {
	return StockoutRiskOptions{
		WindowSize:             "5m",
		HorizonWindows:         12,
		MinimumObservedWindows: 2,
		MaxSeriesWindows:       500,
	}
}

type StockoutRisk struct {
	SalesEventID              string                     `json:"salesEventId,omitempty"`
	TicketID                  string                     `json:"ticketId"`
	TicketType                string                     `json:"ticketType,omitempty"`
	AvailableQuantity         int                        `json:"availableQuantity"`
	WindowSize                string                     `json:"windowSize"`
	HorizonWindowCount        int                        `json:"horizonWindowCount"`
	HorizonStart              time.Time                  `json:"horizonStart"`
	HorizonEnd                time.Time                  `json:"horizonEnd"`
	Method                    string                     `json:"method"`
	Status                    string                     `json:"status"`
	RiskBand                  string                     `json:"riskBand"`
	ObservedWindowCount       int                        `json:"observedWindowCount"`
	ObservedDemandQuantity    int                        `json:"observedDemandQuantity"`
	MeanDemandPerWindow       float64                    `json:"meanDemandPerWindow"`
	VarianceDemandPerWindow   float64                    `json:"varianceDemandPerWindow"`
	ExpectedDemand            float64                    `json:"expectedDemand"`
	StockoutProbability       float64                    `json:"stockoutProbability"`
	ExpectedWindowsToStockout *float64                   `json:"expectedWindowsToStockout,omitempty"`
	ExpectedStockoutAt        *time.Time                 `json:"expectedStockoutAt,omitempty"`
	Distribution              []StockoutProbabilityPoint `json:"distribution,omitempty"`
}

type StockoutProbabilityPoint struct {
	WindowOffset             int       `json:"windowOffset"`
	WindowStart              time.Time `json:"windowStart"`
	WindowEnd                time.Time `json:"windowEnd"`
	CumulativeExpectedDemand float64   `json:"cumulativeExpectedDemand"`
	StockoutProbability      float64   `json:"stockoutProbability"`
}

func BuildStockoutRisks(events []Event, specs []WindowSpec, inventory []TicketInventory, generatedAt time.Time, options StockoutRiskOptions) []StockoutRisk {
	if len(inventory) == 0 {
		return nil
	}

	normalizedSpecs := normalizeWindowSpecs(specs)
	normalizedOptions := normalizeStockoutRiskOptions(options)
	spec, ok := stockoutWindowSpec(normalizedSpecs, normalizedOptions.WindowSize)
	if !ok {
		return nil
	}

	seriesByKey := buildDemandSeries(events, []WindowSpec{spec})
	normalizedInventory := normalizeTicketInventory(inventory)
	risks := make([]StockoutRisk, 0, len(normalizedInventory))
	for _, item := range normalizedInventory {
		key := demandKey{
			SalesEventID: item.SalesEventID,
			TicketID:     item.TicketID,
			TicketType:   item.TicketType,
			WindowSize:   spec.Name,
		}
		risks = append(risks, buildStockoutRisk(item, seriesByKey[key], generatedAt.UTC(), spec, normalizedOptions))
	}
	return risks
}

func normalizeStockoutRiskOptions(options StockoutRiskOptions) StockoutRiskOptions {
	defaults := DefaultStockoutRiskOptions()
	if options.WindowSize == "" {
		options.WindowSize = defaults.WindowSize
	}
	if options.HorizonWindows <= 0 {
		options.HorizonWindows = defaults.HorizonWindows
	}
	if options.MinimumObservedWindows <= 0 {
		options.MinimumObservedWindows = defaults.MinimumObservedWindows
	}
	if options.MaxSeriesWindows <= 0 {
		options.MaxSeriesWindows = defaults.MaxSeriesWindows
	}
	return options
}

func normalizeTicketInventory(inventory []TicketInventory) []TicketInventory {
	normalized := make([]TicketInventory, 0, len(inventory))
	for _, item := range inventory {
		if item.TicketID == "" {
			continue
		}
		normalized = append(normalized, item)
	}
	sort.SliceStable(normalized, func(i, j int) bool {
		left := normalized[i]
		right := normalized[j]
		if left.SalesEventID != right.SalesEventID {
			return left.SalesEventID < right.SalesEventID
		}
		if left.TicketType != right.TicketType {
			return left.TicketType < right.TicketType
		}
		return left.TicketID < right.TicketID
	})
	return normalized
}

func stockoutWindowSpec(specs []WindowSpec, windowSize string) (WindowSpec, bool) {
	for _, spec := range specs {
		if spec.Name == windowSize {
			return spec, true
		}
	}
	return WindowSpec{}, false
}

func buildStockoutRisk(item TicketInventory, series demandSeries, generatedAt time.Time, spec WindowSpec, options StockoutRiskOptions) StockoutRisk {
	horizonStart := generatedAt.UTC()
	horizonEnd := horizonStart.Add(spec.Duration * time.Duration(options.HorizonWindows))
	risk := StockoutRisk{
		SalesEventID:       item.SalesEventID,
		TicketID:           item.TicketID,
		TicketType:         item.TicketType,
		AvailableQuantity:  item.AvailableQuantity,
		WindowSize:         spec.Name,
		HorizonWindowCount: options.HorizonWindows,
		HorizonStart:       horizonStart,
		HorizonEnd:         horizonEnd,
		Method:             StockoutRiskMethodPoissonRate,
		Status:             StockoutRiskStatusEstimated,
		RiskBand:           StockoutRiskBandLow,
	}

	if item.AvailableQuantity <= 0 {
		zero := 0.0
		risk.Status = StockoutRiskStatusStockout
		risk.RiskBand = StockoutRiskBandCritical
		risk.StockoutProbability = 1
		risk.ExpectedWindowsToStockout = &zero
		risk.ExpectedStockoutAt = &horizonStart
		return risk
	}

	if !series.hasWindow {
		risk.Status = StockoutRiskStatusNoObservedDemand
		return risk
	}

	if denseDemandWindowCount(series) > options.MaxSeriesWindows {
		risk.Status = StockoutRiskStatusInsufficientHistory
		return risk
	}

	quantities, _ := denseDemandQuantities(series)
	if len(quantities) < options.MinimumObservedWindows {
		risk.Status = StockoutRiskStatusInsufficientHistory
		risk.ObservedWindowCount = len(quantities)
		risk.ObservedDemandQuantity = sumInts(quantities)
		return risk
	}

	risk.ObservedWindowCount = len(quantities)
	risk.ObservedDemandQuantity = sumInts(quantities)
	risk.MeanDemandPerWindow = meanInts(quantities)
	risk.VarianceDemandPerWindow = sampleVarianceInts(quantities, risk.MeanDemandPerWindow)
	expectedDemand := risk.MeanDemandPerWindow * float64(options.HorizonWindows)
	risk.ExpectedDemand = finiteOrZero(expectedDemand)
	risk.StockoutProbability = poissonStockoutProbability(expectedDemand, item.AvailableQuantity)
	risk.RiskBand = stockoutRiskBand(risk.StockoutProbability)
	risk.Distribution = buildStockoutDistribution(horizonStart, spec.Duration, options.HorizonWindows, risk.MeanDemandPerWindow, item.AvailableQuantity)

	if risk.MeanDemandPerWindow > 0 {
		expectedWindows := float64(item.AvailableQuantity) / risk.MeanDemandPerWindow
		expectedStockoutAt := horizonStart.Add(durationFromWindows(spec.Duration, expectedWindows))
		risk.ExpectedWindowsToStockout = &expectedWindows
		risk.ExpectedStockoutAt = &expectedStockoutAt
	}

	return risk
}

func buildStockoutDistribution(start time.Time, duration time.Duration, horizonWindows int, meanDemandPerWindow float64, availableQuantity int) []StockoutProbabilityPoint {
	if horizonWindows <= 0 || meanDemandPerWindow <= 0 {
		return nil
	}

	points := make([]StockoutProbabilityPoint, 0, horizonWindows)
	for offset := 1; offset <= horizonWindows; offset++ {
		windowStart := start.Add(duration * time.Duration(offset-1))
		expectedDemand := meanDemandPerWindow * float64(offset)
		points = append(points, StockoutProbabilityPoint{
			WindowOffset:             offset,
			WindowStart:              windowStart,
			WindowEnd:                windowStart.Add(duration),
			CumulativeExpectedDemand: finiteOrZero(expectedDemand),
			StockoutProbability:      poissonStockoutProbability(expectedDemand, availableQuantity),
		})
	}
	return points
}

func sumInts(values []int) int {
	var total int
	for _, value := range values {
		total += value
	}
	return total
}

func meanInts(values []int) float64 {
	if len(values) == 0 {
		return 0
	}
	return float64(sumInts(values)) / float64(len(values))
}

func sampleVarianceInts(values []int, mean float64) float64 {
	if len(values) < 2 {
		return 0
	}
	var sumSquaredDiffs float64
	for _, value := range values {
		diff := float64(value) - mean
		sumSquaredDiffs += diff * diff
	}
	return sumSquaredDiffs / float64(len(values)-1)
}

func poissonStockoutProbability(expectedDemand float64, availableQuantity int) float64 {
	if availableQuantity < 0 {
		return 1
	}
	if math.IsNaN(expectedDemand) || expectedDemand <= 0 {
		return 0
	}
	if math.IsInf(expectedDemand, 1) {
		return 1
	}
	if math.IsInf(expectedDemand, -1) {
		return 0
	}

	term := math.Exp(-expectedDemand)
	if math.IsNaN(term) {
		return 0
	}
	cdf := term
	for k := 1; k <= availableQuantity; k++ {
		term *= expectedDemand / float64(k)
		if math.IsNaN(term) || math.IsInf(term, 0) {
			return 1
		}
		cdf += term
		if math.IsNaN(cdf) || math.IsInf(cdf, 0) {
			return 1
		}
		if term == 0 {
			break
		}
	}
	return clampProbability(1 - cdf)
}

func stockoutRiskBand(probability float64) string {
	switch {
	case probability >= 0.8:
		return StockoutRiskBandCritical
	case probability >= 0.5:
		return StockoutRiskBandHigh
	case probability >= 0.2:
		return StockoutRiskBandMedium
	default:
		return StockoutRiskBandLow
	}
}

func durationFromWindows(duration time.Duration, windows float64) time.Duration {
	return time.Duration(float64(duration) * windows)
}
