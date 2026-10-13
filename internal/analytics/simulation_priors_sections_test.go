package analytics

import (
	"math"
	"testing"
)

func TestAggregateConversionStagePriorsCombinesMatchingSteps(t *testing.T) {
	priors := aggregateConversionStagePriors([]FunnelSegment{
		{Steps: []FunnelStep{
			{From: FunnelStageAccepted, To: FunnelStagePending, Trials: 2, Successes: 1},
			{From: FunnelStagePending, To: FunnelStagePaid, Trials: 1, Successes: 1},
		}},
		{Steps: []FunnelStep{
			{From: FunnelStageAccepted, To: FunnelStagePending, Trials: 3, Successes: 2},
		}},
	})

	if len(priors) != 2 {
		t.Fatalf("expected two conversion priors, got %d", len(priors))
	}
	first := priors[0]
	if first.From != FunnelStageAccepted || first.To != FunnelStagePending || first.Trials != 5 || first.Successes != 3 {
		t.Fatalf("unexpected aggregated prior: %+v", first)
	}
	if !almostEqual(first.ConversionProbability, 0.6) {
		t.Fatalf("unexpected conversion probability: %f", first.ConversionProbability)
	}
}

func TestSectionPriorsSanitizeAndCopyAnalyticsInputs(t *testing.T) {
	demand := demandPriors([]DemandForecast{{
		SalesEventID:        "event-1",
		TicketID:            "ticket-vip",
		WindowSize:          "5m",
		Method:              DemandForecastMethodMovingAverage,
		TrainWindowCount:    2,
		TestWindowCount:     1,
		ForecastQuantity:    math.Inf(1),
		Metrics:             DemandForecastMetrics{MAE: math.NaN(), RMSE: math.Inf(1)},
		MovingAverageWindow: 2,
	}})
	if len(demand) != 1 || demand[0].ObservedWindows != 3 || demand[0].ForecastQuantity != 0 || demand[0].Metrics.MAE != 0 || demand[0].Metrics.RMSE != 0 {
		t.Fatalf("unexpected demand priors: %+v", demand)
	}

	percentiles := map[string]float64{"p50": 60, "p90": math.NaN()}
	timing := operationalTimingPriors([]SurvivalAnalysis{{EventName: "time_to_payment", Percentiles: percentiles}})
	percentiles["p50"] = 999
	if len(timing) != 1 || timing[0].Percentiles["p50"] != 60 || timing[0].Percentiles["p90"] != 0 {
		t.Fatalf("unexpected timing priors: %+v", timing)
	}

	risks := stockoutRiskPriors([]StockoutRisk{{
		TicketID:            "ticket-vip",
		ExpectedDemand:      math.Inf(1),
		StockoutProbability: math.Inf(1),
	}})
	if len(risks) != 1 || risks[0].ExpectedDemand != 0 || risks[0].StockoutProbability != 1 {
		t.Fatalf("unexpected stockout priors: %+v", risks)
	}
}
