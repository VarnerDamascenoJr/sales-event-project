package analytics

import (
	"encoding/json"
	"math"
	"testing"
	"time"
)

func TestBuildSimulationPriorsSummarizesSalesAndUncertainty(t *testing.T) {
	base := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	events := []Event{
		{EventType: "sale.created", OccurredAt: base, SalesEventID: "event-1", SaleID: "sale-1", Status: "COMPLETED"},
		{EventType: "sale.item.created", OccurredAt: base, SalesEventID: "event-1", SaleID: "sale-1", TicketID: "ticket-vip", TicketType: "vip", Quantity: 2},
		{EventType: "payment.processed", OccurredAt: base.Add(time.Minute), SalesEventID: "event-1", SaleID: "sale-1", Status: "APPROVED"},
		{EventType: "sale.created", OccurredAt: base.Add(5 * time.Minute), SalesEventID: "event-1", SaleID: "sale-2", Status: "FAILED"},
		{EventType: "sale.item.created", OccurredAt: base.Add(5 * time.Minute), SalesEventID: "event-1", SaleID: "sale-2", TicketID: "ticket-vip", TicketType: "vip", Quantity: 4},
		{EventType: "sale.created", OccurredAt: base.Add(10 * time.Minute), SalesEventID: "event-1", SaleID: "sale-3", Status: "COMPLETED"},
		{EventType: "sale.item.created", OccurredAt: base.Add(10 * time.Minute), SalesEventID: "event-1", SaleID: "sale-3", TicketID: "ticket-vip", TicketType: "vip", Quantity: 4},
		{EventType: "payment.processed", OccurredAt: base.Add(11 * time.Minute), SalesEventID: "event-1", SaleID: "sale-3", Status: "APPROVED"},
	}
	generatedAt := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)

	priors := BuildSimulationPriors(generatedAt, Source{Service: "sales-event-project", SalesEventID: "event-1", Filter: "sales_event_id"}, events, nil, nil, nil, nil)

	if priors == nil {
		t.Fatal("expected simulation priors")
	}
	if priors.SchemaVersion != SimulationPriorsSchemaVersion {
		t.Fatalf("unexpected schema version: %s", priors.SchemaVersion)
	}
	if priors.Source.SchemaVersion != SchemaVersion || priors.Source.SalesEventID != "event-1" {
		t.Fatalf("unexpected source: %+v", priors.Source)
	}
	if priors.Period.StartedAt != "2026-09-01T10:00:00Z" || priors.Period.EndedAt != "2026-09-01T10:10:00Z" {
		t.Fatalf("unexpected period: %+v", priors.Period)
	}
	if priors.SampleSize.SaleCount != 3 || priors.SampleSize.CompletedSaleCount != 2 || priors.SampleSize.CancelledSaleCount != 1 {
		t.Fatalf("unexpected sample size: %+v", priors.SampleSize)
	}
	if priors.SampleSize.TotalItemQuantity != 10 || priors.SampleSize.CompletedItemQuantity != 6 || priors.SampleSize.CompletedDemandCount != 2 {
		t.Fatalf("unexpected demand sample size: %+v", priors.SampleSize)
	}
	if !almostEqual(priors.Estimates.ConversionProbability, 0.6667) || !almostEqual(priors.Estimates.CancellationProbability, 0.3333) {
		t.Fatalf("unexpected conversion estimates: %+v", priors.Estimates)
	}
	if priors.Estimates.ModalDemand != 2 || !almostEqual(priors.Estimates.DemandMean, 3) || !almostEqual(priors.Estimates.DemandStdDev, 1) {
		t.Fatalf("unexpected demand estimates: %+v", priors.Estimates)
	}
	if !almostEqual(priors.Uncertainty.CancellationProbability, priors.Estimates.CancellationProbability) {
		t.Fatalf("unexpected uncertainty: %+v", priors.Uncertainty)
	}
}

func TestBuildSimulationPriorsUsesAnalyticsSections(t *testing.T) {
	generatedAt := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	events := []Event{
		{EventType: "sale.created", OccurredAt: generatedAt.Add(-time.Hour), SalesEventID: "event-1", SaleID: "sale-1", Status: "COMPLETED"},
	}

	priors := BuildSimulationPriors(
		generatedAt,
		Source{SalesEventID: "event-1"},
		events,
		[]FunnelSegment{{Steps: []FunnelStep{{From: "accepted", To: "paid", Trials: 2, Successes: 1}}}},
		[]SurvivalAnalysis{{EventName: "time_to_payment", FromStage: "accepted", ToStage: "paid", UnitOfAnalysis: "sale", ObservationCount: 2, EventCount: 1, CensoredCount: 1, Percentiles: map[string]float64{"p50": 60}}},
		[]DemandForecast{{SalesEventID: "event-1", TicketID: "ticket-vip", TicketType: "vip", WindowSize: "5m", Method: DemandForecastMethodMovingAverage, TrainWindowCount: 2, TestWindowCount: 1, ForecastQuantity: 3, Metrics: DemandForecastMetrics{MAE: 1, RMSE: 1}}},
		[]StockoutRisk{{SalesEventID: "event-1", TicketID: "ticket-vip", TicketType: "vip", AvailableQuantity: 5, WindowSize: "5m", HorizonWindowCount: 12, ExpectedDemand: 4, StockoutProbability: 0.25, RiskBand: StockoutRiskBandMedium, Status: StockoutRiskStatusEstimated}},
	)

	if priors == nil {
		t.Fatal("expected simulation priors")
	}
	if len(priors.ConversionStages) != 1 || priors.ConversionStages[0].Trials != 2 || priors.ConversionStages[0].Successes != 1 {
		t.Fatalf("unexpected conversion stages: %+v", priors.ConversionStages)
	}
	if len(priors.Demand) != 1 || priors.Demand[0].ObservedWindows != 3 || priors.Demand[0].ForecastQuantity != 3 {
		t.Fatalf("unexpected demand priors: %+v", priors.Demand)
	}
	if len(priors.OperationalTiming) != 1 || priors.OperationalTiming[0].Percentiles["p50"] != 60 {
		t.Fatalf("unexpected timing priors: %+v", priors.OperationalTiming)
	}
	if len(priors.StockoutRisks) != 1 || priors.StockoutRisks[0].StockoutProbability != 0.25 {
		t.Fatalf("unexpected stockout priors: %+v", priors.StockoutRisks)
	}
}

func TestBuildSimulationPriorsSanitizesNonFiniteAnalyticsValues(t *testing.T) {
	generatedAt := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	events := []Event{
		{EventType: "sale.created", OccurredAt: generatedAt.Add(-time.Hour), SalesEventID: "event-1", SaleID: "sale-1", Status: "COMPLETED"},
	}

	priors := BuildSimulationPriors(
		generatedAt,
		Source{SalesEventID: "event-1"},
		events,
		nil,
		[]SurvivalAnalysis{{EventName: "time_to_payment", Percentiles: map[string]float64{"p50": math.NaN()}}},
		[]DemandForecast{{SalesEventID: "event-1", TicketID: "ticket-vip", WindowSize: "5m", ForecastQuantity: math.Inf(1), Metrics: DemandForecastMetrics{MAE: math.NaN(), RMSE: math.Inf(1)}}},
		[]StockoutRisk{{SalesEventID: "event-1", TicketID: "ticket-vip", WindowSize: "5m", ExpectedDemand: math.Inf(1), StockoutProbability: math.NaN()}},
	)

	if priors == nil {
		t.Fatal("expected simulation priors")
	}
	if _, err := json.Marshal(priors); err != nil {
		t.Fatalf("marshal sanitized priors: %v", err)
	}
	if priors.Demand[0].ForecastQuantity != 0 || priors.Demand[0].Metrics.MAE != 0 || priors.Demand[0].Metrics.RMSE != 0 {
		t.Fatalf("unexpected demand sanitization: %+v", priors.Demand[0])
	}
	if priors.OperationalTiming[0].Percentiles["p50"] != 0 {
		t.Fatalf("unexpected percentile sanitization: %+v", priors.OperationalTiming[0].Percentiles)
	}
	if priors.StockoutRisks[0].ExpectedDemand != 0 || priors.StockoutRisks[0].StockoutProbability != 0 {
		t.Fatalf("unexpected stockout sanitization: %+v", priors.StockoutRisks[0])
	}
}
