package analytics

import (
	"testing"
	"time"
)

func TestBuildStockoutDistributionUsesCumulativeWindows(t *testing.T) {
	start := time.Date(2026, 9, 1, 11, 0, 0, 0, time.UTC)

	points := buildStockoutDistribution(start, 5*time.Minute, 3, 2, 5)

	if len(points) != 3 {
		t.Fatalf("expected three points, got %d", len(points))
	}
	for i, point := range points {
		wantOffset := i + 1
		if point.WindowOffset != wantOffset {
			t.Fatalf("unexpected offset at index %d: got %d want %d", i, point.WindowOffset, wantOffset)
		}
		wantStart := start.Add(time.Duration(i) * 5 * time.Minute)
		if !point.WindowStart.Equal(wantStart) || !point.WindowEnd.Equal(wantStart.Add(5*time.Minute)) {
			t.Fatalf("unexpected window at index %d: %+v", i, point)
		}
		if !almostEqual(point.CumulativeExpectedDemand, float64(wantOffset*2)) {
			t.Fatalf("unexpected cumulative demand at index %d: %+v", i, point)
		}
	}
	if points[0].StockoutProbability >= points[2].StockoutProbability {
		t.Fatalf("expected stockout probability to increase across the horizon: %+v", points)
	}
}

func TestBuildStockoutDistributionSkipsInvalidHorizon(t *testing.T) {
	start := time.Date(2026, 9, 1, 11, 0, 0, 0, time.UTC)

	if points := buildStockoutDistribution(start, 5*time.Minute, 0, 2, 5); len(points) != 0 {
		t.Fatalf("expected empty distribution for zero horizon, got %+v", points)
	}
	if points := buildStockoutDistribution(start, 5*time.Minute, 3, 0, 5); len(points) != 0 {
		t.Fatalf("expected empty distribution for zero demand, got %+v", points)
	}
}

func TestStockoutRiskBandThresholds(t *testing.T) {
	tests := []struct {
		name        string
		probability float64
		want        string
	}{
		{name: "low", probability: 0.19, want: StockoutRiskBandLow},
		{name: "medium", probability: 0.2, want: StockoutRiskBandMedium},
		{name: "high", probability: 0.5, want: StockoutRiskBandHigh},
		{name: "critical", probability: 0.8, want: StockoutRiskBandCritical},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := stockoutRiskBand(tt.probability); got != tt.want {
				t.Fatalf("unexpected risk band: got %s want %s", got, tt.want)
			}
		})
	}
}
