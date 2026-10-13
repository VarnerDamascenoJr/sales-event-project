package analytics

import "time"

type demandKey struct {
	SalesEventID string
	TicketID     string
	TicketType   string
	WindowSize   string
}

type demandSeries struct {
	key       demandKey
	duration  time.Duration
	byWindow  map[time.Time]int
	first     time.Time
	last      time.Time
	hasWindow bool
}

func buildDemandSeries(events []Event, specs []WindowSpec) map[demandKey]demandSeries {
	seriesByKey := make(map[demandKey]demandSeries)
	for _, event := range normalizeEvents(events) {
		if event.EventType != "sale.item.created" || event.TicketID == "" {
			continue
		}
		quantity := event.Quantity
		if quantity <= 0 {
			quantity = 1
		}
		for _, spec := range specs {
			start := event.OccurredAt.Truncate(spec.Duration)
			key := demandKey{
				SalesEventID: event.SalesEventID,
				TicketID:     event.TicketID,
				TicketType:   event.TicketType,
				WindowSize:   spec.Name,
			}
			series := seriesByKey[key]
			if series.byWindow == nil {
				series = demandSeries{
					key:      key,
					duration: spec.Duration,
					byWindow: make(map[time.Time]int),
					first:    start,
					last:     start,
				}
			}
			if !series.hasWindow || start.Before(series.first) {
				series.first = start
			}
			if !series.hasWindow || start.After(series.last) {
				series.last = start
			}
			series.hasWindow = true
			series.byWindow[start] += quantity
			seriesByKey[key] = series
		}
	}
	return seriesByKey
}

func denseDemandQuantities(series demandSeries) ([]int, []time.Time) {
	if !series.hasWindow {
		return nil, nil
	}
	quantities := make([]int, 0)
	starts := make([]time.Time, 0)
	for current := series.first; !current.After(series.last); current = current.Add(series.duration) {
		quantities = append(quantities, series.byWindow[current])
		starts = append(starts, current)
	}
	return quantities, starts
}

func denseDemandWindowCount(series demandSeries) int {
	if !series.hasWindow || series.duration <= 0 {
		return 0
	}
	return int(series.last.Sub(series.first)/series.duration) + 1
}
