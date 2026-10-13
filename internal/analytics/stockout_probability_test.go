package analytics

import (
	"math"
	"testing"
)

func TestStockoutDemandStatistics(t *testing.T) {
	values := []int{1, 2, 3, 6}
	mean := meanInts(values)

	if sum := sumInts(values); sum != 12 {
		t.Fatalf("unexpected sum: got %d want 12", sum)
	}
	if !almostEqual(mean, 3) {
		t.Fatalf("unexpected mean: got %f want 3", mean)
	}
	if !almostEqual(sampleVarianceInts(values, mean), 4.666666666666667) {
		t.Fatalf("unexpected sample variance")
	}
}

func TestPoissonStockoutProbabilityHandlesInvalidInventory(t *testing.T) {
	got := poissonStockoutProbability(2, -1)

	if got != 1 {
		t.Fatalf("expected negative inventory to be treated as certain stockout, got %f", got)
	}
}

func TestPoissonStockoutProbabilityStaysJSONSafe(t *testing.T) {
	tests := []struct {
		name              string
		expectedDemand    float64
		availableQuantity int
	}{
		{name: "large demand", expectedDemand: 1_000_000, availableQuantity: 1000},
		{name: "high inventory", expectedDemand: 2, availableQuantity: 100_000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := poissonStockoutProbability(tt.expectedDemand, tt.availableQuantity)
			if math.IsNaN(got) || math.IsInf(got, 0) {
				t.Fatalf("expected JSON-safe probability, got %f", got)
			}
			if got < 0 || got > 1 {
				t.Fatalf("expected probability in range [0,1], got %f", got)
			}
		})
	}
}
