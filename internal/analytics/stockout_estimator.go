package analytics

import "time"

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
