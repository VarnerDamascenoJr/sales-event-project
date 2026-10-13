package analytics

import (
	"testing"
	"time"
)

func TestBuildDemandSeriesAggregatesSaleItemDemandByWindow(t *testing.T) {
	base := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	events := []Event{
		demandEvent(base.Add(30*time.Second), "vip", 2),
		demandEvent(base.Add(90*time.Second), "vip", 3),
		demandEvent(base.Add(6*time.Minute), "vip", 4),
		{
			EventType:    "sale.created",
			OccurredAt:   base.Add(time.Minute),
			SalesEventID: "event-1",
			TicketID:     "ticket-vip",
			TicketType:   "vip",
			Quantity:     99,
		},
		{
			EventType:    "sale.item.created",
			OccurredAt:   base.Add(2 * time.Minute),
			SalesEventID: "event-1",
			Quantity:     99,
		},
	}

	seriesByKey := buildDemandSeries(events, []WindowSpec{
		{Name: "5m", Duration: 5 * time.Minute},
	})

	key := demandKey{
		SalesEventID: "event-1",
		TicketID:     "ticket-vip",
		TicketType:   "vip",
		WindowSize:   "5m",
	}
	series, ok := seriesByKey[key]
	if !ok {
		t.Fatalf("expected demand series for %+v", key)
	}

	quantities, starts := denseDemandQuantities(series)
	if len(quantities) != 2 {
		t.Fatalf("expected two dense windows, got %d", len(quantities))
	}
	if starts[0] != base || starts[1] != base.Add(5*time.Minute) {
		t.Fatalf("unexpected dense starts: %v", starts)
	}
	if quantities[0] != 5 || quantities[1] != 4 {
		t.Fatalf("unexpected quantities: %v", quantities)
	}
	if denseDemandWindowCount(series) != 2 {
		t.Fatalf("unexpected dense window count: %d", denseDemandWindowCount(series))
	}
}

func TestBuildDemandSeriesDefaultsNonPositiveQuantityToOne(t *testing.T) {
	base := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	events := []Event{
		demandEvent(base, "standard", 0),
		demandEvent(base.Add(time.Minute), "standard", -3),
	}

	seriesByKey := buildDemandSeries(events, []WindowSpec{
		{Name: "5m", Duration: 5 * time.Minute},
	})

	key := demandKey{
		SalesEventID: "event-1",
		TicketID:     "ticket-standard",
		TicketType:   "standard",
		WindowSize:   "5m",
	}
	series := seriesByKey[key]
	quantities, _ := denseDemandQuantities(series)
	if len(quantities) != 1 || quantities[0] != 2 {
		t.Fatalf("expected non-positive quantities to count as one each, got %v", quantities)
	}
}

func TestDenseDemandQuantitiesFillsMissingWindows(t *testing.T) {
	base := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	seriesByKey := buildDemandSeries([]Event{
		demandEvent(base, "standard", 3),
		demandEvent(base.Add(10*time.Minute), "standard", 9),
	}, []WindowSpec{{Name: "5m", Duration: 5 * time.Minute}})

	key := demandKey{
		SalesEventID: "event-1",
		TicketID:     "ticket-standard",
		TicketType:   "standard",
		WindowSize:   "5m",
	}
	quantities, starts := denseDemandQuantities(seriesByKey[key])

	if len(quantities) != 3 {
		t.Fatalf("expected three dense windows, got %d", len(quantities))
	}
	if starts[1] != base.Add(5*time.Minute) {
		t.Fatalf("expected middle dense start at %s, got %s", base.Add(5*time.Minute), starts[1])
	}
	if quantities[0] != 3 || quantities[1] != 0 || quantities[2] != 9 {
		t.Fatalf("unexpected dense quantities: %v", quantities)
	}
}

func TestDenseDemandQuantitiesReturnsEmptyForMissingSeries(t *testing.T) {
	quantities, starts := denseDemandQuantities(demandSeries{})
	if quantities != nil || starts != nil {
		t.Fatalf("expected nil dense values for empty series, got quantities=%v starts=%v", quantities, starts)
	}
	if denseDemandWindowCount(demandSeries{}) != 0 {
		t.Fatalf("expected zero dense window count for empty series")
	}
}
