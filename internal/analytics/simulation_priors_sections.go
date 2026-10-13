package analytics

import "sort"

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
			ForecastQuantity: finiteOrZero(forecast.ForecastQuantity),
			Metrics: DemandForecastMetrics{
				MAE:  finiteOrZero(forecast.Metrics.MAE),
				RMSE: finiteOrZero(forecast.Metrics.RMSE),
			},
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
			ExpectedDemand:      finiteOrZero(risk.ExpectedDemand),
			StockoutProbability: clampProbability(risk.StockoutProbability),
			RiskBand:            risk.RiskBand,
			Status:              risk.Status,
		})
	}
	return priors
}

func copyFloatMap(values map[string]float64) map[string]float64 {
	if len(values) == 0 {
		return nil
	}
	copied := make(map[string]float64, len(values))
	for key, value := range values {
		copied[key] = finiteOrZero(value)
	}
	return copied
}
