package analytics

import (
	"testing"
	"time"
)

func TestBuildStockoutRisksEstimatesPoissonRisk(t *testing.T) {
	base := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	generatedAt := time.Date(2026, 9, 1, 11, 0, 0, 0, time.UTC)
	events := []Event{
		demandEvent(base, "vip", 2),
		demandEvent(base.Add(5*time.Minute), "vip", 2),
		demandEvent(base.Add(10*time.Minute), "vip", 2),
		demandEvent(base.Add(15*time.Minute), "vip", 2),
	}

	risks := BuildStockoutRisks(events, []WindowSpec{{Name: "5m", Duration: 5 * time.Minute}}, []TicketInventory{
		{
			SalesEventID:      "event-1",
			TicketID:          "ticket-vip",
			TicketType:        "vip",
			AvailableQuantity: 5,
		},
	}, generatedAt, StockoutRiskOptions{
		WindowSize:             "5m",
		HorizonWindows:         3,
		MinimumObservedWindows: 2,
	})

	if len(risks) != 1 {
		t.Fatalf("expected one risk, got %d", len(risks))
	}
	risk := risks[0]
	if risk.Status != StockoutRiskStatusEstimated {
		t.Fatalf("unexpected status: %s", risk.Status)
	}
	if risk.RiskBand != StockoutRiskBandHigh {
		t.Fatalf("unexpected risk band: %s", risk.RiskBand)
	}
	if risk.ObservedWindowCount != 4 || risk.ObservedDemandQuantity != 8 {
		t.Fatalf("unexpected observed demand: %+v", risk)
	}
	if !almostEqual(risk.MeanDemandPerWindow, 2) || !almostEqual(risk.ExpectedDemand, 6) {
		t.Fatalf("unexpected demand statistics: %+v", risk)
	}
	if risk.StockoutProbability < 0.55 || risk.StockoutProbability > 0.56 {
		t.Fatalf("unexpected stockout probability: %f", risk.StockoutProbability)
	}
	if risk.ExpectedWindowsToStockout == nil || !almostEqual(*risk.ExpectedWindowsToStockout, 2.5) {
		t.Fatalf("unexpected expected windows to stockout: %v", risk.ExpectedWindowsToStockout)
	}
	if risk.ExpectedStockoutAt == nil || !risk.ExpectedStockoutAt.Equal(generatedAt.Add(12*time.Minute+30*time.Second)) {
		t.Fatalf("unexpected expected stockout at: %v", risk.ExpectedStockoutAt)
	}
	if len(risk.Distribution) != 3 {
		t.Fatalf("expected three distribution points, got %d", len(risk.Distribution))
	}
	if risk.Distribution[0].StockoutProbability >= risk.Distribution[2].StockoutProbability {
		t.Fatalf("expected cumulative stockout probability to increase: %+v", risk.Distribution)
	}
}

func TestBuildStockoutRisksHandlesNoObservedDemand(t *testing.T) {
	generatedAt := time.Date(2026, 9, 1, 11, 0, 0, 0, time.UTC)

	risks := BuildStockoutRisks(nil, []WindowSpec{{Name: "5m", Duration: 5 * time.Minute}}, []TicketInventory{
		{SalesEventID: "event-1", TicketID: "ticket-standard", TicketType: "standard", AvailableQuantity: 10},
	}, generatedAt, StockoutRiskOptions{WindowSize: "5m", HorizonWindows: 3})

	if len(risks) != 1 {
		t.Fatalf("expected one risk, got %d", len(risks))
	}
	if risks[0].Status != StockoutRiskStatusNoObservedDemand {
		t.Fatalf("unexpected status: %s", risks[0].Status)
	}
	if risks[0].StockoutProbability != 0 || len(risks[0].Distribution) != 0 {
		t.Fatalf("unexpected risk without demand: %+v", risks[0])
	}
}

func TestBuildStockoutRisksMarksZeroInventoryAsStockout(t *testing.T) {
	generatedAt := time.Date(2026, 9, 1, 11, 0, 0, 0, time.UTC)

	risks := BuildStockoutRisks(nil, []WindowSpec{{Name: "5m", Duration: 5 * time.Minute}}, []TicketInventory{
		{SalesEventID: "event-1", TicketID: "ticket-standard", TicketType: "standard", AvailableQuantity: 0},
	}, generatedAt, StockoutRiskOptions{WindowSize: "5m", HorizonWindows: 3})

	if len(risks) != 1 {
		t.Fatalf("expected one risk, got %d", len(risks))
	}
	risk := risks[0]
	if risk.Status != StockoutRiskStatusStockout || risk.RiskBand != StockoutRiskBandCritical || risk.StockoutProbability != 1 {
		t.Fatalf("unexpected stockout risk: %+v", risk)
	}
	if risk.ExpectedWindowsToStockout == nil || *risk.ExpectedWindowsToStockout != 0 {
		t.Fatalf("unexpected expected windows: %v", risk.ExpectedWindowsToStockout)
	}
}
