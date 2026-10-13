package analytics

import (
	"math"
	"testing"
	"time"
)

func TestBuildSalePriorRecordsAggregatesCompletionAndDemand(t *testing.T) {
	base := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	records := buildSalePriorRecords([]Event{
		{EventType: "sale.created", OccurredAt: base.Add(time.Minute), SaleID: "sale-1", Status: "PENDING_PAYMENT"},
		{EventType: "sale.created", OccurredAt: base, SaleID: "sale-1", Status: "PENDING_PAYMENT"},
		{EventType: "sale.item.created", OccurredAt: base, SaleID: "sale-1", Quantity: 0},
		{EventType: "sale.item.created", OccurredAt: base, SaleID: "sale-1", Quantity: 3},
		{EventType: "payment.processed", OccurredAt: base.Add(2 * time.Minute), SaleID: "sale-1", Status: "approved"},
		{EventType: "sale.item.created", OccurredAt: base, Quantity: 99},
	})

	record := records["sale-1"]
	if record == nil {
		t.Fatal("expected sale record")
	}
	if !record.CreatedAt.Equal(base) {
		t.Fatalf("expected earliest created time, got %s", record.CreatedAt)
	}
	if !record.Completed {
		t.Fatalf("expected approved payment to mark sale completed")
	}
	if record.Demand != 4 {
		t.Fatalf("expected demand to include default quantity, got %d", record.Demand)
	}
}

func TestSimulationPriorSampleSizeAndEstimates(t *testing.T) {
	base := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	records := map[string]*salePriorRecord{
		"sale-1": {CreatedAt: base, Completed: true, Demand: 2},
		"sale-2": {CreatedAt: base.Add(5 * time.Minute), Completed: false, Demand: 4},
		"sale-3": {CreatedAt: base.Add(10 * time.Minute), Completed: true, Demand: 4},
	}

	period := simulationPriorPeriod(records)
	if period.StartedAt != "2026-09-01T10:00:00Z" || period.EndedAt != "2026-09-01T10:10:00Z" {
		t.Fatalf("unexpected period: %+v", period)
	}

	sampleSize, completedDemand := simulationPriorSampleSize(records, []string{"sale-1", "sale-2", "sale-3"})
	if sampleSize.SaleCount != 3 || sampleSize.CompletedSaleCount != 2 || sampleSize.CancelledSaleCount != 1 {
		t.Fatalf("unexpected sample size: %+v", sampleSize)
	}
	if sampleSize.TotalItemQuantity != 10 || sampleSize.CompletedItemQuantity != 6 || sampleSize.CompletedDemandCount != 2 {
		t.Fatalf("unexpected demand sample size: %+v", sampleSize)
	}

	estimates := simulationPriorEstimates(sampleSize, completedDemand)
	if !almostEqual(estimates.ConversionProbability, 0.6667) || !almostEqual(estimates.CancellationProbability, 0.3333) {
		t.Fatalf("unexpected conversion estimates: %+v", estimates)
	}
	if estimates.ModalDemand != 2 || !almostEqual(estimates.DemandMean, 3) || !almostEqual(estimates.DemandStdDev, 1) {
		t.Fatalf("unexpected demand estimates: %+v", estimates)
	}
	uncertainty := simulationPriorUncertainty(estimates)
	if uncertainty.CancellationProbability != estimates.CancellationProbability {
		t.Fatalf("unexpected uncertainty: %+v", uncertainty)
	}
}

func TestSimulationPriorNumericHelpers(t *testing.T) {
	if got := round4(1.23456); got != 1.2346 {
		t.Fatalf("expected rounded value, got %f", got)
	}
	if got := round4(math.Inf(1)); got != 0 {
		t.Fatalf("expected non-finite value to round to zero, got %f", got)
	}
	if got := intMode([]int{4, 2, 4, 2}); got != 2 {
		t.Fatalf("expected smallest tied mode, got %d", got)
	}
}
