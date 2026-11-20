package analytics

import (
	"math"
	"strconv"
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

func TestPoissonStockoutProbabilityLargeDemand(t *testing.T) {
	tests := []struct {
		name      string
		demand    float64
		stock     int64
		want      float64
		tolerance float64
	}{
		{name: "below underflow threshold", demand: 700, stock: 700, want: 0.48994943414294543, tolerance: 1e-10},
		{name: "above underflow threshold", demand: 701, stock: 701, want: 0.4899566028208438, tolerance: 1e-10},
		{name: "stock well above demand", demand: 1000, stock: 2000, want: 0, tolerance: 1e-100},
		{name: "stock below demand", demand: 1000, stock: 950, want: 0.9421637070446768, tolerance: 1e-10},
		{name: "stock at demand", demand: 1000, stock: 1000, want: 0.491590632831494, tolerance: 1e-10},
		{name: "stock above demand", demand: 1000, stock: 1050, want: 0.05602883836367338, tolerance: 1e-10},
		{name: "extreme demand", demand: 1_000_000, stock: 1000, want: 1, tolerance: 1e-10},
		{name: "very large demand approximation", demand: 100_000_000_000, stock: 100_000_000_000, want: 0.5, tolerance: 1e-5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if strconv.IntSize == 32 && tt.stock > math.MaxInt32 {
				t.Skip("stock value exceeds the platform int size")
			}
			got := poissonStockoutProbability(tt.demand, int(tt.stock))
			if math.IsNaN(got) || math.IsInf(got, 0) || math.Abs(got-tt.want) > tt.tolerance {
				t.Fatalf("poissonStockoutProbability(%v, %d) = %.15g; want %.15g ± %g", tt.demand, tt.stock, got, tt.want, tt.tolerance)
			}
		})
	}
}
