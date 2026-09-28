package analytics

import (
	"math"
	"testing"
	"time"
)

func TestBuildDemandForecastsUsesMovingAverageAndTemporalHoldout(t *testing.T) {
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

func TestBuildDemandForecastsDensifiesMissingWindows(t *testing.T) {
	base := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	events := []Event{
		demandEvent(base, "standard", 3),
		demandEvent(base.Add(10*time.Minute), "standard", 9),
	}

	forecasts := BuildDemandForecasts(events, []WindowSpec{
		{Name: "5m", Duration: 5 * time.Minute},
	}, DemandForecastOptions{
		MovingAverageWindow: 2,
		TestFraction:        0.34,
		MinimumTestWindows:  1,
		HorizonWindows:      1,
	})

	if len(forecasts) != 1 {
		t.Fatalf("expected one forecast, got %d", len(forecasts))
	}
	forecast := forecasts[0]
	if len(forecast.Series) != 4 {
		t.Fatalf("expected dense train/test plus forecast, got %d", len(forecast.Series))
	}
	middle := forecast.Series[1]
	if middle.ObservedQuantity == nil || *middle.ObservedQuantity != 0 {
		t.Fatalf("expected zero-filled missing window, got %+v", middle)
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

func TestBuildDemandForecastsSkipsSeriesAboveWindowLimit(t *testing.T) {
	base := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	forecasts := BuildDemandForecasts([]Event{
		demandEvent(base, "vip", 1),
		demandEvent(base.Add(20*time.Minute), "vip", 1),
	}, []WindowSpec{{Name: "5m", Duration: 5 * time.Minute}}, DemandForecastOptions{
		MaxSeriesWindows: 3,
	})

	if len(forecasts) != 0 {
		t.Fatalf("expected capped series to be skipped, got %+v", forecasts)
	}
}

func demandEvent(at time.Time, ticketType string, quantity int) Event {
	return Event{
		EventType:    "sale.item.created",
		OccurredAt:   at,
		SalesEventID: "event-1",
		SaleID:       "sale-1",
		TicketID:     "ticket-" + ticketType,
		TicketType:   ticketType,
		Quantity:     quantity,
	}
}

func almostEqual(left float64, right float64) bool {
	return math.Abs(left-right) < 0.000001
}
