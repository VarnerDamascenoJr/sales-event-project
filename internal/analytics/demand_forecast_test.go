package analytics

import (
	"testing"
	"time"
)

func TestBuildDemandForecastsBuildsForecastDocument(t *testing.T) {
	base := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	events := []Event{
		demandEvent(base, "vip", 2),
		demandEvent(base.Add(5*time.Minute), "vip", 4),
		demandEvent(base.Add(10*time.Minute), "vip", 6),
		demandEvent(base.Add(15*time.Minute), "vip", 8),
		demandEvent(base.Add(20*time.Minute), "vip", 10),
	}

	forecasts := BuildDemandForecasts(events, []WindowSpec{
		{Name: "5m", Duration: 5 * time.Minute},
	}, DemandForecastOptions{
		MovingAverageWindow: 2,
		TestFraction:        0.4,
		MinimumTestWindows:  1,
		HorizonWindows:      1,
	})

	if len(forecasts) != 1 {
		t.Fatalf("expected one forecast, got %d", len(forecasts))
	}
	forecast := forecasts[0]
	if forecast.Method != DemandForecastMethodMovingAverage {
		t.Fatalf("unexpected method: %s", forecast.Method)
	}
	if forecast.TrainWindowCount != 3 || forecast.TestWindowCount != 2 {
		t.Fatalf("unexpected split: train=%d test=%d", forecast.TrainWindowCount, forecast.TestWindowCount)
	}
	if !almostEqual(forecast.Metrics.MAE, 3) || !almostEqual(forecast.Metrics.RMSE, 3) {
		t.Fatalf("unexpected metrics: %+v", forecast.Metrics)
	}
	if !almostEqual(forecast.ForecastQuantity, 9) {
		t.Fatalf("unexpected forecast quantity: %f", forecast.ForecastQuantity)
	}
	if forecast.ForecastWindowStart != base.Add(25*time.Minute) {
		t.Fatalf("unexpected forecast window start: %s", forecast.ForecastWindowStart)
	}
	if len(forecast.Series) != 6 {
		t.Fatalf("expected train/test/forecast points, got %d", len(forecast.Series))
	}
	if forecast.Series[3].Split != "test" || forecast.Series[3].PredictedQuantity == nil || !almostEqual(*forecast.Series[3].PredictedQuantity, 5) {
		t.Fatalf("unexpected first test point: %+v", forecast.Series[3])
	}
	if forecast.Series[5].Split != "forecast" || forecast.Series[5].ObservedQuantity != nil {
		t.Fatalf("unexpected forecast point: %+v", forecast.Series[5])
	}
}

func TestBuildDemandForecastsReturnsSortedForecasts(t *testing.T) {
	base := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	events := []Event{
		demandEvent(base, "vip", 1),
		demandEvent(base.Add(5*time.Minute), "vip", 2),
		demandEvent(base, "standard", 3),
		demandEvent(base.Add(5*time.Minute), "standard", 4),
	}

	forecasts := BuildDemandForecasts(events, []WindowSpec{
		{Name: "5m", Duration: 5 * time.Minute},
		{Name: "1m", Duration: time.Minute},
	}, DemandForecastOptions{
		MovingAverageWindow: 1,
		TestFraction:        0.5,
		MinimumTestWindows:  1,
		HorizonWindows:      1,
	})

	if len(forecasts) != 4 {
		t.Fatalf("expected forecasts for both ticket types and window sizes, got %d", len(forecasts))
	}

	expected := []struct {
		windowSize string
		ticketType string
	}{
		{windowSize: "1m", ticketType: "standard"},
		{windowSize: "1m", ticketType: "vip"},
		{windowSize: "5m", ticketType: "standard"},
		{windowSize: "5m", ticketType: "vip"},
	}
	for i, want := range expected {
		got := forecasts[i]
		if got.WindowSize != want.windowSize || got.TicketType != want.ticketType {
			t.Fatalf("forecast %d: expected %s/%s, got %s/%s", i, want.windowSize, want.ticketType, got.WindowSize, got.TicketType)
		}
	}
}

func TestBuildDemandForecastsReturnsEmptyWithoutTicketDemand(t *testing.T) {
	forecasts := BuildDemandForecasts([]Event{
		{
			EventType:  "sale.created",
			OccurredAt: time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC),
			Quantity:   1,
		},
	}, []WindowSpec{{Name: "5m", Duration: 5 * time.Minute}}, DemandForecastOptions{})

	if len(forecasts) != 0 {
		t.Fatalf("expected no forecasts, got %+v", forecasts)
	}
}
