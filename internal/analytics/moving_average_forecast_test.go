package analytics

import (
	"testing"
	"time"
)

func TestMovingAverageUsesAvailableTrailingWindow(t *testing.T) {
	if got := movingAverage([]int{2, 4, 6, 8}, 2); !almostEqual(got, 7) {
		t.Fatalf("expected trailing average 7, got %f", got)
	}
	if got := movingAverage([]int{2, 4}, 5); !almostEqual(got, 3) {
		t.Fatalf("expected average over available values, got %f", got)
	}
	if got := movingAverage(nil, 3); got != 0 {
		t.Fatalf("expected zero for empty input, got %f", got)
	}
}

func TestDemandTestWindowCountClampsToValidHoldout(t *testing.T) {
	options := DemandForecastOptions{
		TestFraction:       0.5,
		MinimumTestWindows: 3,
	}

	if got := demandTestWindowCount(10, options); got != 5 {
		t.Fatalf("expected fractional holdout to win, got %d", got)
	}
	if got := demandTestWindowCount(4, options); got != 3 {
		t.Fatalf("expected minimum holdout to win, got %d", got)
	}
	if got := demandTestWindowCount(2, options); got != 1 {
		t.Fatalf("expected holdout to leave one training window, got %d", got)
	}
}

func TestBuildDemandForecastRejectsInsufficientSeries(t *testing.T) {
	base := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	series := demandSeries{
		key: demandKey{
			SalesEventID: "event-1",
			TicketID:     "ticket-vip",
			TicketType:   "vip",
			WindowSize:   "5m",
		},
		duration:  5 * time.Minute,
		byWindow:  map[time.Time]int{base: 2},
		first:     base,
		last:      base,
		hasWindow: true,
	}

	if _, ok := buildDemandForecast(series, normalizeDemandForecastOptions(DemandForecastOptions{})); ok {
		t.Fatalf("expected single-point series to be rejected")
	}
}

func TestBuildDemandForecastRejectsSeriesAboveWindowLimit(t *testing.T) {
	base := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	series := demandSeries{
		key: demandKey{
			SalesEventID: "event-1",
			TicketID:     "ticket-vip",
			TicketType:   "vip",
			WindowSize:   "5m",
		},
		duration: 5 * time.Minute,
		byWindow: map[time.Time]int{
			base:                       1,
			base.Add(20 * time.Minute): 1,
		},
		first:     base,
		last:      base.Add(20 * time.Minute),
		hasWindow: true,
	}

	if _, ok := buildDemandForecast(series, DemandForecastOptions{MaxSeriesWindows: 3}); ok {
		t.Fatalf("expected capped series to be rejected")
	}
}
